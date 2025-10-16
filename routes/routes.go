package routes

import (
	"Gin_postgres_redis_rent_tool/app"
	"Gin_postgres_redis_rent_tool/controllers"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine, a *app.App) {
	// 控制器与依赖
	s := controllers.GetSrv(a)
	// appSess := s.GetAppSess()
	uc := controllers.GetUserController(s)
	itemCtl := controllers.NewItemController(s)
	inviteCtl := controllers.GetInviteController(s)
	lc := controllers.NewLockController(s)
	// 复用的中间件
	authMW := app.AuthRequired(s.AppSess, s.Repo, a.Config)
	adminMW := app.AdminOnly(a.Config, s.Repo)
	seenMW := app.TouchLastSeen(s.Repo, a.RDB, 5*time.Minute)
	secureCookie := strings.HasPrefix(a.Config.WebOrigin, "https://")

	// ------------------------------
	// WebAuthn（公开+受保护）
	// ------------------------------
	wa := r.Group("/webauthn")
	{
		// 公开：注册/登录流程
		wa.POST("/register/begin", s.BeginRegistration)
		wa.POST("/register/finish", s.FinishRegistration)

		wa.POST("/login/begin", s.BeginLogin)
		wa.POST("/login/finish", s.FinishLogin)
	}

	waAuth := wa.Group("", authMW, seenMW)
	{
		// whoami：返回完整用户信息
		waAuth.GET("/whoami", func(c *app.Ctx) {
			v, _ := c.Get("userID")
			uid, _ := v.(string)

			isAdmin := false
			if x, ok := c.Get("isAdmin"); ok {
				if b, ok2 := x.(bool); ok2 {
					isAdmin = b
				}
			}
			c.JSON(http.StatusOK, app.H{"userID": uid, "isAdmin": isAdmin})
		})

		// 登出
		waAuth.POST("/logout", func(c *app.Ctx) {
			if ck, err := c.Request.Cookie(app.AppSessionCookie); err == nil && ck.Value != "" {
				_ = s.AppSess.Delete(c.Request.Context(), ck.Value)
			}
			http.SetCookie(c.Writer, &http.Cookie{
				Name:     app.AppSessionCookie,
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				Secure:   secureCookie,
			})
			c.JSON(http.StatusOK, app.H{"ok": true})
		})
	}

	// 已登录用户添加新凭据（绑定手机等）
	creds := r.Group("/api/credentials", authMW, seenMW)
	{
		creds.POST("/add/begin", s.BeginAddCredential)
		creds.POST("/add/finish", s.FinishAddCredential)
	}

	// ------------------------------
	// 邀请（仅管理员）
	// ------------------------------
	admin := r.Group("/admin", authMW, adminMW)
	{
		admin.POST("/invites", inviteCtl.CreateInvite)

	}

	// 只需要登录即可查看单个用户
	userPublic := r.Group("/api/users", authMW)
	{
		userPublic.GET("/:id", uc.GetUser)
	}
	// ------------------------------
	// 用户管理（仅管理员）
	// ------------------------------
	users := r.Group("/api/users", authMW, adminMW)
	{
		users.GET("", uc.ListUsers) // ?q=&page=&size=
		users.GET("/loan", uc.ListUsersWithOpenLoans)
		users.DELETE("/:id", uc.DeleteUser)
		users.PATCH("/:id", uc.UpdateUserAdmin)
	}

	// ------------------------------
	// 借还（Item 唯一件）
	// ------------------------------
	// 管理：创建物品
	itemsAdmin := r.Group("/api/admin", authMW, adminMW)
	// itemsAdmin := r.Group("/api/admin")

	{
		itemsAdmin.POST("", itemCtl.CreateItem)
		itemsAdmin.POST("/borrow", itemCtl.AdminBorrow)  // 管理员代借
		itemsAdmin.POST("/return", itemCtl.AdminReturn)  // 管理员代还
		itemsAdmin.GET("/items", itemCtl.ListItemsAdmin) // ?q=&status=&page=&size=)

	}

	// 用户：浏览/借/还/记录
	items := r.Group("/api/items", authMW, seenMW)
	{
		items.GET("", itemCtl.ListItems)
		items.POST("/:id/borrow", itemCtl.Borrow)
		items.POST("/loans/:loanId/return", itemCtl.Return)
		// items.GET("/loans", itemCtl.ListLoans) // ?status=open|returned&userId=&itemId=
		items.GET("/loans/open", itemCtl.ListMyOpenLoans)

	}
	//  unlock
	unlock := r.Group("/api/unlock", authMW)
	{
		unlock.POST("", lc.Unlock) // 解锁（解冻）被锁定的物品
	}
}

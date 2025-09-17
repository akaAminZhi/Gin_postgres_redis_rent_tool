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
	appSess := s.GetAppSess()
	uc := controllers.GetUserController(s.Repo, appSess, a.Config)
	itemCtl := controllers.NewItemController(s.Repo)
	inviteCtl := controllers.GetInviteController(s.Repo)

	// 复用的中间件
	authMW := app.AuthRequired(s, s.Repo)
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

			// u, err := s.Repo.FindUserByID(c.Request.Context(), uid)
			// if err != nil {
			// 	c.JSON(http.StatusUnauthorized, app.H{"error": "unauthorized"})
			// 	return
			// }
			// credCount, _ := s.Repo.CountCredentials(c.Request.Context(), uid)

			// isAdmin := false
			// for _, mail := range a.Config.AdminEmails {
			// 	if strings.EqualFold(mail, u.Username) {
			// 		isAdmin = true
			// 		break
			// 	}
			// }

			c.JSON(http.StatusOK, app.H{
				"userID": uid,
			})
		})

		// 登出
		waAuth.POST("/logout", func(c *app.Ctx) {
			if ck, err := c.Request.Cookie(app.AppSessionCookie); err == nil && ck.Value != "" {
				_ = s.AppSessions().Delete(c.Request.Context(), ck.Value)
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

	// ------------------------------
	// 用户管理（仅管理员）
	// ------------------------------
	users := r.Group("/api/users", authMW, adminMW)
	{
		users.GET("", uc.ListUsers)   // ?q=&page=&size=
		users.GET("/:id", uc.GetUser) // 精确查单个
		users.DELETE("/:id", uc.DeleteUser)
	}

	// ------------------------------
	// 借还（Item 唯一件）
	// ------------------------------
	// 管理：创建物品
	itemsAdmin := r.Group("/api/items", authMW, adminMW)
	{
		itemsAdmin.POST("", itemCtl.CreateItem)
	}

	// 用户：浏览/借/还/记录
	items := r.Group("/api/items", authMW, seenMW)
	{
		items.GET("", itemCtl.ListItems)
		items.POST("/:id/borrow", itemCtl.Borrow)
		items.POST("/loans/:loanId/return", itemCtl.Return)
		items.GET("/loans", itemCtl.ListLoans) // ?status=open|returned&userId=&itemId=
	}
}

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
	s := controllers.GetSrv(a)
	// main appSess
	appSess := s.GetAppSess()

	// WebAuthn 相关路由
	userRoutes := r.Group("/webauthn")
	{
		userRoutes.POST("/register/begin", s.BeginRegistration)
		userRoutes.POST("/register/finish", s.FinishRegistration)

		userRoutes.POST("/login/begin", s.BeginLogin)
		userRoutes.POST("/login/finish", s.FinishLogin)

	}

	r.POST("/api/credentials/add/begin", app.AuthRequired(s, s.Repo), s.BeginAddCredential)
	r.POST("/api/credentials/add/finish", app.AuthRequired(s, s.Repo), s.FinishAddCredential)

	// 受保护接口
	r.GET("/webauthn/whoami",
		app.AuthRequired(s, s.Repo),
		app.TouchLastSeen(s.Repo, a.RDB, 5*time.Minute),
		func(c *app.Ctx) {
			uid, _ := c.Get("userID")
			c.JSON(200, app.H{"userID": uid})
		})
	// 管理员邀请route
	ic := controllers.GetInviteController(s.Repo)
	r.POST("/admin/invites", ic.CreateInvite)
	ic_for_item := controllers.NewItemController(s.Repo)

	// 管理（需要管理员）
	admin := r.Group("/api/items",
		app.AuthRequired(s, s.Repo),
		app.AdminOnly(a.Config, s.Repo),
	)
	admin.POST("", ic_for_item.CreateItem)

	// 用户借还（需要登录）
	auth := r.Group("/api/items",
		app.AuthRequired(s, s.Repo),
		app.TouchLastSeen(s.Repo, a.RDB, 5*time.Minute),
	)
	auth.GET("", ic_for_item.ListItems)
	auth.POST("/:id/borrow", ic_for_item.Borrow)
	auth.POST("/loans/:loanId/return", ic_for_item.Return)
	auth.GET("/loans", ic_for_item.ListLoans)

	// 用户管理 route
	uc := controllers.GetUserController(s.Repo, appSess, a.Config)
	g := r.Group("/api/users", app.AuthRequired(s, s.Repo), app.AdminOnly(a.Config, s.Repo))
	// g := r.Group("/api/users")

	g.GET("", uc.ListUsers)
	g.DELETE("/:id", uc.DeleteUser)
	r.GET("api/user/:id", app.AuthRequired(s, s.Repo), uc.GetUser)

	// 登出：删 Redis，会话 Cookie 置空
	r.POST("/webauthn/logout", app.AuthRequired(s, s.Repo), func(c *app.Ctx) {
		ck, _ := c.Request.Cookie(app.AppSessionCookie)
		_ = s.AppSessions().Delete(c.Request.Context(), ck.Value)

		http.SetCookie(c.Writer, &http.Cookie{
			Name:     app.AppSessionCookie,
			Value:    "",
			Path:     "/",
			MaxAge:   -1, // 删除
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   strings.HasPrefix(a.Config.WebOrigin, "https://"),
		})
		c.JSON(200, app.H{"ok": true})
	})
}

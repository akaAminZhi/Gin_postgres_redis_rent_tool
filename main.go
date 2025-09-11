package main

import (
	"Gin_postgres_redis_rent_tool/app"
	"Gin_postgres_redis_rent_tool/config"
	"Gin_postgres_redis_rent_tool/routes"
	"log"
	"os"
)

func main() {
	config.LoadEnv()
	// db.ConnectDB()
	// r := gin.Default()
	// routes.RegisterRoutes(r)
	// r.Run()
	application := app.MustNew()
	defer application.Close()

	r := application.Router

	// Health
	r.GET("/healthz", func(c *app.Ctx) { c.JSON(200, app.H{"ok": true}) })

	// // 登出：删 Redis，会话 Cookie 置空
	// r.POST("/webauthn/logout", app.AuthRequired(application), func(c *app.Ctx) {
	// 	ck, _ := c.Request.Cookie(app.AppSessionCookie)
	// 	_ = application.AppSessions().Delete(c.Request.Context(), ck.Value)

	// 	http.SetCookie(c.Writer, &http.Cookie{
	// 		Name:     app.AppSessionCookie,
	// 		Value:    "",
	// 		Path:     "/",
	// 		MaxAge:   -1, // 删除
	// 		HttpOnly: true,
	// 		SameSite: http.SameSiteLaxMode,
	// 		Secure:   strings.HasPrefix(application.Config.WebOrigin, "https://"),
	// 	})
	// 	c.JSON(200, app.H{"ok": true})
	// })
	// WebAuthn routes
	routes.RegisterRoutes(r, application)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3001"
	}
	log.Printf("listening on :%s", port)
	_ = r.Run(":" + port)
}

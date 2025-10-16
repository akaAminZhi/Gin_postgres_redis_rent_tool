package app

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func useCORS(r *gin.Engine, origin string) {
	cfg := cors.Config{
		AllowOrigins:     []string{origin, "https://87701380e67f.ngrok-free.app", "https://textile-alternate-relationships-pie.trycloudflare.com"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	r.Use(cors.New(cfg))
}

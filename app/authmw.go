package app

import (
	"Gin_postgres_redis_rent_tool/db"
	"Gin_postgres_redis_rent_tool/session"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const AppSessionCookie = "app_session"

func AuthRequired(appSess *session.AppSessionStore, repo *db.Repo, cfg Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		ck, err := c.Request.Cookie(AppSessionCookie)
		if err != nil || ck.Value == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, H{"error": "unauthorized"})
			return
		}
		as, err := appSess.Get(c.Request.Context(), ck.Value)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, H{"error": "invalid session"})
			return
		}

		// 这里确认用户仍存在，并把 isAdmin 放进 Context（只查一次）
		u, err := repo.FindUserByID(c.Request.Context(), as.UserID)
		if err != nil {
			_ = appSess.Delete(c.Request.Context(), ck.Value)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		// 把 userID 放进上下文，后续 handler 可用
		c.Set("userID", as.UserID)
		c.Set("username", u.Username)
		email := strings.ToLower(u.Username)
		for _, admin := range cfg.AdminEmails {
			if email == admin {
				c.Set("isAdmin", true)
			}
		}
		c.Set("isAdmin", u.IsAdmin)

		c.Next()
	}
}

func AdminOnly(cfg Config, repo *db.Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 已有 AuthRequired 设置的 userID
		v, ok := c.Get("userID")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, H{"error": "unauthorized"})
			return
		}
		uid, _ := v.(string)
		u, err := repo.FindUserByID(c.Request.Context(), uid)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, H{"error": "unauthorized"})
			return
		}
		email := strings.ToLower(u.Username)
		for _, admin := range cfg.AdminEmails {
			if email == admin {
				c.Next()
				return
			}
		}

		if !u.IsAdmin {
			c.AbortWithStatusJSON(403, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
		// c.AbortWithStatusJSON(http.StatusForbidden, H{"error": "forbidden"})
	}
}

package app

import (
	"Gin_postgres_redis_rent_tool/db"
	"Gin_postgres_redis_rent_tool/session"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const AppSessionCookie = "app_session"

type AuthDeps interface {
	AppSessions() *session.AppSessionStore
}

func AuthRequired(deps AuthDeps, repo *db.Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		ck, err := c.Request.Cookie(AppSessionCookie)
		if err != nil || ck.Value == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, H{"error": "unauthorized"})
			return
		}
		as, err := deps.AppSessions().Get(c.Request.Context(), ck.Value)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, H{"error": "invalid session"})
			return
		}

		// ✅ 关键：确认用户仍存在（也可以查 is_disabled 字段）
		if _, err := repo.FindUserByID(c.Request.Context(), as.UserID); err != nil {
			// 用户已删/禁用 → 清理这个会话，避免反复命中
			_ = deps.AppSessions().Delete(c.Request.Context(), ck.Value)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		// 把 userID 放进上下文，后续 handler 可用
		c.Set("userID", as.UserID)
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
		c.AbortWithStatusJSON(http.StatusForbidden, H{"error": "forbidden"})
	}
}

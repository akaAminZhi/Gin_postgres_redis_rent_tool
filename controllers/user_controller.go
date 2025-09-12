package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"Gin_postgres_redis_rent_tool/app"
	"Gin_postgres_redis_rent_tool/db"
	"Gin_postgres_redis_rent_tool/session"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UserController struct {
	repo    *db.Repo
	appSess *session.AppSessionStore
	cfg     app.Config
}

func GetUserController(repo *db.Repo, appSess *session.AppSessionStore, cfg app.Config) *UserController {
	return &UserController{repo: repo, appSess: appSess, cfg: cfg}
}

// GET /api/users?q=alice&page=1&size=20
func (uc *UserController) ListUsers(c *gin.Context) {
	q := c.Query("q")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))

	res, err := uc.repo.ListUsers(c.Request.Context(), q, page, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, app.H{"error": err.Error()})
		return
	}

	// 可选：返回每个用户的 credential 数（简单起见，这里不额外查询；如需，可批量统计）
	c.JSON(http.StatusOK, app.H{
		"total": res.Total,
		"users": res.Users,
	})
}

// GET /api/users?id=
func (uc *UserController) GetUser(c *gin.Context) {
	id := c.Param("id") // ✅ 从路径取
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id is required"})
		return
	}
	if _, err := uuid.Parse(id); err != nil { // ✅ 校验 UUID 格式
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid uuid"})
		return
	}
	user, err := uc.repo.FindUserByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, app.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, app.H{
		"user": user,
	})
}

// DELETE /api/users/:id
func (uc *UserController) DeleteUser(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, app.H{"error": "missing id"})
		return
	}

	// 不允许删除自己，避免锁死
	if v, ok := c.Get("userID"); ok {
		if uid, _ := v.(string); uid == id {
			c.JSON(http.StatusBadRequest, app.H{"error": "cannot delete yourself"})
			return
		}
	}

	// 查一下被删用户是否是管理员，保护起来（可选）
	target, err := uc.repo.FindUserByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, app.H{"error": "user not found"})
		return
	}
	email := strings.ToLower(target.Username)
	for _, admin := range uc.cfg.AdminEmails {
		if email == admin {
			c.JSON(http.StatusForbidden, app.H{"error": "cannot delete an admin"})
			return
		}
	}

	// 真正删除（会连带删 credentials）
	if err := uc.repo.DeleteUserByID(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, app.H{"error": err.Error()})
		return
	}
	// ✅ 关键：撤销该用户的所有登录会话
	_ = uc.appSess.RevokeAllForUser(c.Request.Context(), id)
	// c.Status(http.StatusNoContent)
	c.JSON(http.StatusOK, app.H{"ok": true})
}

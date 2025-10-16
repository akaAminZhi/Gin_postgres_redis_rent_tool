package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"Gin_postgres_redis_rent_tool/app"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UserController struct{ *Srv }

func GetUserController(s *Srv) *UserController { return &UserController{Srv: s} }

func (uc *UserController) ListUsers(c *gin.Context) {
	q := c.Query("q")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))

	res, err := uc.Repo.ListUsers(c, q, page, size)
	if err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	c.JSON(200, app.H{"total": res.Total, "users": res.Users})
}

func (uc *UserController) ListUsersWithOpenLoans(c *gin.Context) {
	q := c.Query("q")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))

	res, err := uc.Repo.ListUsersWithOpenLoans(c.Request.Context(), q, page, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, app.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, app.H{"users": res.Users, "total": res.Total})
}
func (uc *UserController) GetUser(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(400, app.H{"error": "invalid uuid"})
		return
	}
	u, err := uc.Repo.FindUserByID(c, id)
	if err != nil {
		c.JSON(404, app.H{"error": "user not found"})
		return
	}
	c.JSON(200, app.H{"user": u})
}

func (uc *UserController) DeleteUser(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(400, app.H{"error": "missing id"})
		return
	}
	if v, ok := c.Get("userID"); ok {
		if uid, _ := v.(string); uid == id {
			c.JSON(400, app.H{"error": "cannot delete yourself"})
			return
		}
	}
	target, err := uc.Repo.FindUserByID(c, id)
	if err != nil {
		c.JSON(404, app.H{"error": "user not found"})
		return
	}
	email := strings.ToLower(target.Username)
	for _, admin := range uc.Cfg.AdminEmails {
		if email == admin {
			// c.JSON(403, app.H{"error": "cannot delete an admin"})
			// return
		}
	}
	if err := uc.Repo.DeleteUserByID(c, id); err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	_ = uc.AppSess.RevokeAllForUser(c, id)
	c.JSON(200, app.H{"ok": true})
}

func (uc *UserController) UpdateUserAdmin(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(400, app.H{"error": "missing id"})
		return
	}
	if err := uc.Repo.SetUserAdmin(c, id, true); err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"userId":  id,
		"isAdmin": true,
	})
}

// controllers/items_controller.go
package controllers

import (
	"net/http"
	"strconv"
	"time"

	"Gin_postgres_redis_rent_tool/app"
	"Gin_postgres_redis_rent_tool/db"
	"Gin_postgres_redis_rent_tool/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ItemController struct{ *Srv }

func NewItemController(s *Srv) *ItemController { return &ItemController{Srv: s} }

// 管理员创建一件唯一物品
func (ic *ItemController) CreateItem(c *gin.Context) {
	var in struct {
		Name   string `json:"name" binding:"required"`
		Serial string `json:"serial" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, app.H{"error": err.Error()})
		return
	}
	it := &models.Item{ID: uuid.NewString(), Name: in.Name, Serial: in.Serial}
	if err := ic.Repo.CreateItem(c.Request.Context(), it); err != nil {
		c.JSON(http.StatusInternalServerError, app.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, it)
}

// 列表（含是否可借）
func (ic *ItemController) ListItems(c *gin.Context) {
	items, err := ic.Repo.ListItems(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, app.H{"error": err.Error()})
		return
	}
	// 直接返回 items，里头就有 InUse
	c.JSON(http.StatusOK, app.H{"items": items})
}

// 借出
func (ic *ItemController) Borrow(c *gin.Context) {
	itemID := c.Param("id")
	if itemID == "" {
		c.JSON(400, app.H{"error": "missing item id"})
		return
	}
	v, ok := c.Get("userID")
	if !ok {
		c.JSON(401, app.H{"error": "unauthorized"})
		return
	}
	userID, _ := v.(string)

	var in struct {
		DueAt *time.Time `json:"dueAt"`
		Note  string     `json:"note"`
	}
	_ = c.ShouldBindJSON(&in)

	loan, err := ic.Repo.BorrowItem(c.Request.Context(), userID, itemID, in.DueAt, in.Note)
	if err != nil {
		if err == db.ErrAlreadyBorrowed {
			c.JSON(409, app.H{"error": "already borrowed"})
			return
		}
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, loan)
}

// 归还
func (ic *ItemController) Return(c *gin.Context) {
	loanID := c.Param("loanId")
	if loanID == "" {
		c.JSON(400, app.H{"error": "missing loan id"})
		return
	}
	v, ok := c.Get("userID")
	if !ok {
		c.JSON(401, app.H{"error": "unauthorized"})
		return
	}
	userID, _ := v.(string)

	loan, err := ic.Repo.ReturnLoan(c.Request.Context(), loanID, userID)
	if err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, loan)
}

// 借还记录
func (ic *ItemController) ListLoans(c *gin.Context) {
	status := c.Query("status")
	userID := c.Query("userId")
	itemID := c.Query("itemId")
	ls, err := ic.Repo.ListLoans(c.Request.Context(), userID, itemID, status)
	if err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, app.H{"items": ls})
}

// 普通用户：查看自己手上正在借着的工具
func (ic *ItemController) ListMyOpenLoans(c *gin.Context) {
	// 假设 userID 是通过登录态或中间件注入的
	userID, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))

	q := db.MyOpenLoansQuery{
		Page: page,
		Size: size,
	}

	// 调用 Repo 层函数
	rows, err := ic.Repo.ListMyOpenLoans(c.Request.Context(), userID.(string), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": rows,
	})
}

func (ic *ItemController) ListItemsAdmin(c *gin.Context) {
	q := db.AdminItemsQuery{
		Q:      c.Query("q"),
		Status: c.Query("status"), // "", "open", "available", "overdue", "inactive"
	}
	if v := c.DefaultQuery("page", "1"); v != "" {
		q.Page, _ = strconv.Atoi(v)
	}
	if v := c.DefaultQuery("size", "20"); v != "" {
		q.Size, _ = strconv.Atoi(v)
	}

	res, err := ic.Repo.ListItemsWithCurrentLoan(c.Request.Context(), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, app.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, app.H{"ok": true, "items": res})
}

type AdminBorrowReq struct {
	ToolID   string     `json:"toolId" binding:"required"`
	UserName string     `json:"userName" binding:"required"`
	DueAt    *time.Time `json:"dueAt,omitempty"`
	Note     string     `json:"note,omitempty"`
}

func (ic *ItemController) AdminBorrow(c *gin.Context) {
	var req AdminBorrowReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	// 先用 username 查 userId
	user, err := ic.Repo.FindUserIDByUsername(c.Request.Context(), req.UserName)
	if err != nil {
		// 404 语义更合理，也可用 400
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	row, err := ic.Repo.CreateAdminLoan(c.Request.Context(), db.CreateAdminLoanInput{
		ItemID: req.ToolID,
		UserID: user.ID,
		DueAt:  req.DueAt,
		Note:   req.Note,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, row)
}

type AdminReturnReq struct {
	ToolID   string `json:"toolId" binding:"required"`
	Username string `json:"username" binding:"required"`
	Note     string `json:"note,omitempty"`
}

func (ic *ItemController) AdminReturn(c *gin.Context) {
	var req AdminReturnReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	// 1) 先用 username 查 userId（归还人）
	user, err := ic.Repo.FindUserIDByUsername(c.Request.Context(), req.Username)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 2) 执行归还
	row, err := ic.Repo.ReturnAdminLoan(c.Request.Context(), db.ReturnAdminLoanInput{
		ItemID:           req.ToolID,
		ReturnedByUserID: user.ID,
		Note:             req.Note,
	})
	if err != nil {
		// 典型错误：no open loan for this item
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, row)
}

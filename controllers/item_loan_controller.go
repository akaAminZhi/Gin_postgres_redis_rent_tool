// controllers/items_controller.go
package controllers

import (
	"net/http"
	"time"

	"Gin_postgres_redis_rent_tool/app"
	"Gin_postgres_redis_rent_tool/db"
	"Gin_postgres_redis_rent_tool/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ItemController struct{ Repo *db.Repo }

func NewItemController(repo *db.Repo) *ItemController { return &ItemController{Repo: repo} }

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

	type Row struct {
		models.Item
		Available bool `json:"available"`
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		ok, _ := ic.Repo.IsItemAvailable(c.Request.Context(), it.ID)
		rows = append(rows, Row{Item: it, Available: ok})
	}
	c.JSON(http.StatusOK, app.H{"items": rows})
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

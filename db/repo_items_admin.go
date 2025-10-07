// db/repo_items_admin.go
package db

import (
	"Gin_postgres_redis_rent_tool/models"
	"context"
	"strings"
	"time"
)

type AdminItemRow struct {
	// Item fields
	ID        string    `json:"id"`
	Serial    string    `json:"serial"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	InUse     bool      `json:"inUse"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// Current open loan (nullable)
	LoanID              *string    `json:"loanId,omitempty"`
	BorrowerID          *string    `json:"borrowerId,omitempty"`
	BorrowerUsername    *string    `json:"borrowerUsername,omitempty"`
	BorrowerDisplayName *string    `json:"borrowerDisplayName,omitempty"`
	BorrowedAt          *time.Time `json:"borrowedAt,omitempty"`
	DueAt               *time.Time `json:"dueAt,omitempty"`
	Overdue             bool       `json:"overdue"` // 由 SQL 计算
}

type AdminItemsQuery struct {
	Q      string // 模糊搜索：serial/name
	Status string // "", "open", "available", "overdue", "inactive"
	Page   int
	Size   int
}

type PagedAdminItems struct {
	Total int64          `json:"total"`
	Items []AdminItemRow `json:"items"`
}

func (r *Repo) ListItemsWithCurrentLoan(ctx context.Context, q AdminItemsQuery) (*PagedAdminItems, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.Size <= 0 || q.Size > 200 {
		q.Size = 20
	}
	offset := (q.Page - 1) * q.Size

	db := r.DB.WithContext(ctx)

	// 子查询：每件物品“当前未归还”的最新一条 Loan
	sub := db.
		Table(models.LoanTable + " l").
		Select(`
			DISTINCT ON (l.item_id)
			l.id, l.item_id, l.user_id, l.borrowed_at, l.due_at
		`).
		Where("l.returned_at IS NULL").
		Order("l.item_id, l.borrowed_at DESC")

	// 主查询
	qry := db.
		Table(models.ItemTable+" i").
		Select(`
			i.id, i.serial, i.name, i.status, i.in_use, i.created_at, i.updated_at,
			ol.id        AS loan_id,
			ol.user_id   AS borrower_id,
			ol.borrowed_at,
			ol.due_at,
			u.username   AS borrower_username,
			u.display_name AS borrower_display_name,
			CASE WHEN ol.due_at IS NOT NULL AND ol.due_at < NOW() THEN TRUE ELSE FALSE END AS overdue
		`).
		Joins("LEFT JOIN (?) AS ol ON ol.item_id = i.id", sub).
		Joins("LEFT JOIN lsb_users u ON u.id = ol.user_id")

	// 过滤
	if s := strings.TrimSpace(q.Q); s != "" {
		pat := "%" + strings.ToLower(s) + "%"
		qry = qry.Where("LOWER(i.serial) LIKE ? OR LOWER(i.name) LIKE ?", pat, pat)
	}
	switch q.Status {
	case "open":
		qry = qry.Where("i.in_use = TRUE")
	case "available":
		qry = qry.Where("i.in_use = FALSE")
	case "overdue":
		qry = qry.Where("ol.due_at IS NOT NULL AND ol.due_at < NOW()")
	case "inactive":
		qry = qry.Where("i.status <> 'active'")
	default:
		// all
	}

	// 统计总数（对 items 计数即可）
	var total int64
	if err := db.Table(models.ItemTable + " i").Select("i.id").
		Where(qry.Statement.Clauses["WHERE"].Expression). // 复用 where 条件
		Count(&total).Error; err != nil {
		return nil, err
	}

	// 排序+分页
	qry = qry.Order("i.created_at DESC").Offset(offset).Limit(q.Size)

	var rows []AdminItemRow
	if err := qry.Scan(&rows).Error; err != nil {
		return nil, err
	}

	return &PagedAdminItems{Total: total, Items: rows}, nil
}

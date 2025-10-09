// db/repo_items_admin.go
package db

import (
	"Gin_postgres_redis_rent_tool/models"
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

type CreateAdminLoanInput struct {
	ItemID string
	UserID string
	DueAt  *time.Time // optional
	Note   string     // optional
}

func (r *Repo) CreateAdminLoan(ctx context.Context, in CreateAdminLoanInput) (*AdminItemRow, error) {
	tx := r.DB.WithContext(ctx).Begin()
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
		}
	}()

	// 1) 锁定 item 行避免并发
	var it models.Item
	if err := tx.
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", in.ItemID).
		First(&it).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("item not found")
		}
		return nil, err
	}

	// 2) 业务校验（按你的规则可自行调整）
	if it.Status != "active" {
		tx.Rollback()
		return nil, errors.New("item is not active")
	}
	if it.InUse {
		tx.Rollback()
		return nil, errors.New("item is already in use")
	}

	// 3) 创建借用记录（依赖唯一部分索引防止并发重复打开）
	loan := models.Loan{
		// 如果你用 DB default uuid_generate_v4() 生成 ID，则不必在这里赋值
		ID:         uuid.NewString(),
		ItemID:     in.ItemID,
		UserID:     in.UserID,
		BorrowedAt: time.Now(),
		DueAt:      in.DueAt,
		Note:       in.Note,
	}
	if err := tx.Create(&loan).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// 4) 标记物品为 in_use = true
	if err := tx.Model(&models.Item{}).
		Where("id = ?", in.ItemID).
		Updates(map[string]any{
			"in_use":     true,
			"updated_at": time.Now(),
		}).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// 5) 读回统一视图（与你 ListItemsWithCurrentLoan 的 SELECT 保持一致）
	var row AdminItemRow
	if err := tx.
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
		Joins("LEFT JOIN "+models.LoanTable+" ol ON ol.item_id = i.id AND ol.returned_at IS NULL").
		Joins("LEFT JOIN lsb_users u ON u.id = ol.user_id").
		Where("i.id = ?", in.ItemID).
		Scan(&row).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &row, nil
}

type ReturnAdminLoanInput struct {
	ItemID           string
	ReturnedByUserID string
	Note             string // 可选：归还备注，若提供会合并写入 Loan.Note
}

// ReturnAdminLoan sets returned_at/returned_by on the open loan of the item,
// flips item.in_use = false, and returns a unified AdminItemRow.
func (r *Repo) ReturnAdminLoan(ctx context.Context, in ReturnAdminLoanInput) (*AdminItemRow, error) {
	tx := r.DB.WithContext(ctx).Begin()
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
		}
	}()

	// 1) 锁定 item 行，避免并发
	var it models.Item
	if err := tx.
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", in.ItemID).
		First(&it).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("item not found")
		}
		return nil, err
	}

	// 2) 查找该 item 的 open loan（唯一部分索引保证只有一条）
	var loan models.Loan
	if err := tx.
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("item_id = ? AND returned_at IS NULL", in.ItemID).
		First(&loan).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("no open loan for this item")
		}
		return nil, err
	}

	now := time.Now()

	// 3) 更新 loan：returned_at / returned_by (+ 合并备注)
	update := map[string]any{
		"returned_at": now,
		"returned_by": in.ReturnedByUserID,
		"updated_at":  now,
	}
	if strings.TrimSpace(in.Note) != "" {
		// 合并到 Note（简单拼接；如需更复杂策略可自行调整）
		newNote := strings.TrimSpace(strings.TrimSpace(loan.Note+" ") + in.Note)
		update["note"] = newNote
	}

	if err := tx.Model(&models.Loan{}).
		Where("id = ?", loan.ID).
		Updates(update).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// 4) 标记 item 为未借出
	if err := tx.Model(&models.Item{}).
		Where("id = ?", in.ItemID).
		Updates(map[string]any{
			"in_use":     false,
			"updated_at": now,
		}).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// 5) 读回统一行（此时无 open loan，应返回空的借用字段）
	var row AdminItemRow
	if err := tx.
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
		Joins("LEFT JOIN "+models.LoanTable+" ol ON ol.item_id = i.id AND ol.returned_at IS NULL").
		Joins("LEFT JOIN lsb_users u ON u.id = ol.user_id").
		Where("i.id = ?", in.ItemID).
		Scan(&row).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &row, nil
}

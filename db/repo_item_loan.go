package db

import (
	"context"
	"errors"
	"time"

	"Gin_postgres_redis_rent_tool/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Items
func (r *Repo) CreateItem(ctx context.Context, it *models.Item) error {
	return r.DB.WithContext(ctx).Create(it).Error
}
func (r *Repo) FindItemByID(ctx context.Context, id string) (*models.Item, error) {
	var it models.Item
	if err := r.DB.WithContext(ctx).First(&it, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &it, nil
}
func (r *Repo) ListItems(ctx context.Context) ([]models.Item, error) {
	var items []models.Item
	err := r.DB.WithContext(ctx).Order("created_at DESC").Find(&items).Error
	return items, err
}

// Loans
var ErrAlreadyBorrowed = errors.New("item already borrowed")

func (r *Repo) BorrowItem(ctx context.Context, userID, itemID string, dueAt *time.Time, note string) (*models.Loan, error) {
	var loan *models.Loan
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 锁住该物品，防止并发超借
		var it models.Item
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&it, "id = ? AND status = 'active'", itemID).Error; err != nil {
			return err
		}
		// 是否已被借出：未归还记录是否存在
		var n int64
		if err := tx.Model(&models.Loan{}).
			Where("item_id = ? AND returned_at IS NULL", itemID).
			Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrAlreadyBorrowed
		}
		now := time.Now().UTC()
		l := &models.Loan{
			ID:         uuid.NewString(),
			ItemID:     it.ID,
			UserID:     userID,
			BorrowedAt: now,
			DueAt:      dueAt,
			Note:       note,
		}
		if err := tx.Create(l).Error; err != nil {
			return err
		}
		loan = l
		return nil
	})
	return loan, err
}

func (r *Repo) ReturnLoan(ctx context.Context, loanID string, returnedBy string) (*models.Loan, error) {
	var l models.Loan
	if err := r.DB.WithContext(ctx).First(&l, "id = ?", loanID).Error; err != nil {
		return nil, err
	}
	if l.ReturnedAt != nil {
		return &l, nil // 幂等
	}
	now := time.Now().UTC()
	l.ReturnedAt = &now
	l.ReturnedBy = &returnedBy
	if err := r.DB.WithContext(ctx).Save(&l).Error; err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *Repo) ListLoans(ctx context.Context, userID, itemID, status string) ([]models.Loan, error) {
	q := r.DB.WithContext(ctx).Model(&models.Loan{}).Order("borrowed_at DESC")
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if itemID != "" {
		q = q.Where("item_id = ?", itemID)
	}
	if status == "open" {
		q = q.Where("returned_at IS NULL")
	} else if status == "returned" {
		q = q.Where("returned_at IS NOT NULL")
	}
	var ls []models.Loan
	if err := q.Find(&ls).Error; err != nil {
		return nil, err
	}
	return ls, nil
}

// 汇总：该物品是否可用
func (r *Repo) IsItemAvailable(ctx context.Context, itemID string) (bool, error) {
	var n int64
	err := r.DB.WithContext(ctx).
		Model(&models.Loan{}).
		Where("item_id = ? AND returned_at IS NULL", itemID).
		Count(&n).Error
	return n == 0, err
}

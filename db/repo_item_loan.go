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

// 借出：原子操作 = 锁住 item → 占用 in_use → 新建 loan
func (r *Repo) BorrowItem(ctx context.Context, userID, itemID string, dueAt *time.Time, note string) (*models.Loan, error) {
	var loan *models.Loan
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 锁住该物品
		var it models.Item
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&it, "id = ? AND status = 'active'", itemID).Error; err != nil {
			return err
		}
		// 2) 防并发：若已 in_use 或存在未归还 Loan，则拒绝
		if it.InUse {
			return ErrAlreadyBorrowed
		}
		var n int64
		if err := tx.Model(&models.Loan{}).
			Where("item_id = ? AND returned_at IS NULL", itemID).
			Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrAlreadyBorrowed
		}
		// 3) 先占位（UPDATE ... WHERE id=? AND in_use=false 也可）
		if err := tx.Model(&models.Item{}).
			Where("id = ? AND in_use = FALSE", it.ID).
			Update("in_use", true).Error; err != nil {
			return err
		}
		// 4) 新建 Loan
		now := time.Now().UTC()
		if dueAt == nil {
			d := now.Add(48 * time.Hour)
			dueAt = &d
		}

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

// 归还：原子操作 = 完成 loan → 释放 in_use
func (r *Repo) ReturnLoan(ctx context.Context, loanID string, returnedBy string) (*models.Loan, error) {
	var l models.Loan
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&l, "id = ?", loanID).Error; err != nil {
			return err
		}
		// 幂等：已归还直接返回
		if l.ReturnedAt != nil {
			return nil
		}
		now := time.Now().UTC()
		l.ReturnedAt = &now
		l.ReturnedBy = &returnedBy
		if err := tx.Save(&l).Error; err != nil {
			return err
		}
		// 释放占用
		if err := tx.Model(&models.Item{}).
			Where("id = ?", l.ItemID).
			Update("in_use", false).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
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
	if status == "avalible" {
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
	var inUse bool
	if err := r.DB.WithContext(ctx).
		Model(&models.Item{}).
		Select("in_use").
		Where("id = ?", itemID).
		Scan(&inUse).Error; err != nil {
		return false, err
	}
	return !inUse, nil
}

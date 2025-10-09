// models/item_loan.go
package models

import "time"

const LoanTable = "lsb_loans"
const ItemTable = "lsb_items"

type Item struct {
	ID        string    `gorm:"type:uuid;primaryKey" json:"id"`
	Serial    string    `gorm:"size:120;uniqueIndex;not null" json:"serial"`     // 唯一编号
	Name      string    `gorm:"size:200;not null" json:"name"`                   // 可选：显示名称
	Status    string    `gorm:"size:20;not null;default:'active'" json:"status"` // 生命周期：active/maintenance/retired...
	InUse     bool      `gorm:"not null;default:false" json:"inUse"`             // ✅ 冗余列：当前是否被借走
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Loan struct {
	ID         string     `gorm:"type:uuid;primaryKey" json:"id"`
	ItemID     string     `gorm:"type:uuid;index;not null" json:"itemId"`
	UserID     string     `gorm:"type:uuid;index;not null" json:"userId"`
	BorrowedAt time.Time  `gorm:"index;not null" json:"borrowedAt"`
	DueAt      *time.Time `json:"dueAt,omitempty"`

	ReturnedAt *time.Time `gorm:"index" json:"returnedAt,omitempty"`
	ReturnedBy *string    `gorm:"type:uuid" json:"returnedBy,omitempty"`

	Note      string    `gorm:"size:255" json:"note,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (Item) TableName() string { return ItemTable }
func (Loan) TableName() string { return LoanTable }

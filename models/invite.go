package models

import "time"

type Invite struct {
	ID        uint      `gorm:"primaryKey"`
	Email     string    `gorm:"index;size:255;not null"`
	Token     string    `gorm:"uniqueIndex;size:64;not null"`
	ExpiresAt time.Time `gorm:"index;not null"`
	UsedAt    *time.Time
	CreatedBy string `gorm:"size:255"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

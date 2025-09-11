package models

import (
	"time"
)

// User 使用 UUID 字节作为 WebAuthn userHandle（存字符串即可，用时转 []byte）
type User struct {
	ID          string `gorm:"primaryKey;type:uuid" json:"id"`
	Username    string `gorm:"uniqueIndex;size:255;not null" json:"username"`
	DisplayName string `gorm:"size:255;not null" json:"displayName"`

	LastLoginAt *time.Time `gorm:"index" json:"lastLoginAt,omitempty"`
	LastSeenAt  *time.Time `gorm:"index" json:"lastSeenAt,omitempty"`
	LoginCount  int64      `gorm:"not null;default:0" json:"loginCount"`
	LastLoginIP string     `gorm:"size:45" json:"-"`  // 可选，前端一般不直接展示
	LastLoginUA string     `gorm:"size:255" json:"-"` // 可选

	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Credentials []Credential
}

func (User) TableName() string {
	return "lsb_users"
}

// Credential 为每个注册的 Passkey 存档
// 注意：CredentialID / PublicKey 为二进制，GORM 在 Postgres 下可用 bytea
// AAGUID 也是 16 字节
// 你可根据需要额外存 transports 等

type Credential struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	UserID          string    `gorm:"type:uuid;index" json:"userId"`
	CredentialID    []byte    `gorm:"uniqueIndex" json:"credentialId"`
	PublicKey       []byte    `json:"publicKey"`
	AttestationType string    `gorm:"size:64" json:"attestationType"`
	AAGUID          []byte    `gorm:"type:bytea" json:"aaguid"`
	SignCount       uint32    `json:"signCount"`
	CloneWarning    bool      `json:"cloneWarning"`
	BackupEligible  bool      `json:"backupEligible"`
	BackupState     bool      `json:"backupState"`
	TransportsJSON  string    `gorm:"type:text" json:"transportsJson"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`

	LastUsedAt *time.Time `gorm:"index" json:"lastUsedAt,omitempty"`
}

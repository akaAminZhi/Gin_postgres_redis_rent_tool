package models

import "time"

// UnlockLog 记录解锁操作的审计信息
// 若你的目标主键不是 UUID，可把 TargetID 改为 string
// 并去掉 gorm 的 uuid 类型注解。
type UnlockLog struct {
	ID string `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	// TargetType    string    `json:"targetType"`
	// TargetID      *string   `gorm:"type:uuid" json:"targetId,omitempty"`
	ActorID       string    `gorm:"type:uuid" json:"actorId"`
	ActorUsername string    `json:"actorUsername"`
	Reason        *string   `json:"reason,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

func (UnlockLog) TableName() string { return "lsb_unlock_log" }

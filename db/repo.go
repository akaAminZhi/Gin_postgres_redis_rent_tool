package db

import (
	"Gin_postgres_redis_rent_tool/models"
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repo struct{ DB *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{DB: db} }

// Users

func (r *Repo) TouchUserLogin(ctx context.Context, userID, ip, ua string) error {
	// 用数据库时间更准，且避免并发覆盖：NOW() + 计数自增
	return r.DB.WithContext(ctx).Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"last_login_at": gorm.Expr("NOW()"),
			"last_seen_at":  gorm.Expr("NOW()"),
			"login_count":   gorm.Expr("COALESCE(login_count, 0) + 1"),
			"last_login_ip": ip,
			"last_login_ua": ua,
		}).Error
}

func (r *Repo) TouchUserSeen(ctx context.Context, userID string) error {
	return r.DB.WithContext(ctx).Model(&models.User{}).
		Where("id = ?", userID).
		Update("last_seen_at", gorm.Expr("NOW()")).Error
}

// 按 ID 查
func (r *Repo) FindUserByID(ctx context.Context, id string) (*models.User, error) {
	var u models.User
	if err := r.DB.WithContext(ctx).First(&u, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// 统计某用户的凭据数

func (r *Repo) TouchCredentialUsed(ctx context.Context, credID []byte) error {
	return r.DB.WithContext(ctx).Model(&models.Credential{}).
		Where("credential_id = ?", credID).
		Update("last_used_at", gorm.Expr("NOW()")).Error
}

func (r *Repo) CountCredentials(ctx context.Context, userID string) (int64, error) {
	var n int64
	err := r.DB.WithContext(ctx).Model(&models.Credential{}).
		Where("user_id = ?", userID).
		Count(&n).Error
	return n, err
}

// 列表（分页 + 关键词，关键词匹配用户名/显示名/邮箱）
type ListUsersResult struct {
	Users []models.User `json:"users"`
	Total int64         `json:"total"`
}

func (r *Repo) ListUsers(ctx context.Context, q string, page, size int) (ListUsersResult, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}

	tx := r.DB.WithContext(ctx).Model(&models.User{})
	if q = strings.TrimSpace(q); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		tx = tx.Where("LOWER(username) LIKE ? OR LOWER(display_name) LIKE ?", like, like)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return ListUsersResult{}, err
	}

	var users []models.User
	if err := tx.
		Order("created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&users).Error; err != nil {
		return ListUsersResult{}, err
	}
	return ListUsersResult{Users: users, Total: total}, nil
}

// 删除用户（凭借外键级联生效；若未设级联，这里显式删凭据）
func (r *Repo) DeleteUserByID(ctx context.Context, id string) error {
	// 显式删除凭据（保险起见）
	if err := r.DB.WithContext(ctx).Where("user_id = ?", id).Delete(&models.Credential{}).Error; err != nil {
		return err
	}
	// 再删用户
	return r.DB.WithContext(ctx).Clauses(clause.Returning{}).Delete(&models.User{ID: id}).Error
}

func (r *Repo) FindOrCreateUser(ctx context.Context, username string, newID string) (*models.User, error) {
	var u models.User
	err := r.DB.WithContext(ctx).Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		u = models.User{ID: newID, Username: username, DisplayName: username}
		if err := r.DB.WithContext(ctx).Create(&u).Error; err != nil {
			return nil, err
		}
		return &u, nil
	}
	return &u, err
}

func (r *Repo) FindUserByUsername(ctx context.Context, username string) (*models.User, error) {
	var u models.User
	if err := r.DB.WithContext(ctx).Where("username=?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repo) LoadUserCredentials(ctx context.Context, userID string) ([]models.Credential, error) {
	var cs []models.Credential
	if err := r.DB.WithContext(ctx).Where("user_id=?", userID).Find(&cs).Error; err != nil {
		return nil, err
	}
	return cs, nil
}

// Credentials
func (r *Repo) AddCredential(ctx context.Context, c *models.Credential) error {
	return r.DB.WithContext(ctx).Create(c).Error
}

func (r *Repo) UpdateCredentialCounter(ctx context.Context, credID []byte, newCount uint32, cloneWarn bool) error {
	return r.DB.WithContext(ctx).Model(&models.Credential{}).
		Where("credential_id = ?", credID).
		Updates(map[string]any{"sign_count": newCount, "clone_warning": cloneWarn}).Error
}

func (r *Repo) FindUserByCredentialID(ctx context.Context, credID []byte) (*models.User, *models.Credential, error) {
	var c models.Credential
	if err := r.DB.WithContext(ctx).Where("credential_id=?", credID).First(&c).Error; err != nil {
		return nil, nil, err
	}
	var u models.User
	if err := r.DB.WithContext(ctx).Where("id=?", c.UserID).First(&u).Error; err != nil {
		return nil, nil, err
	}
	return &u, &c, nil
}

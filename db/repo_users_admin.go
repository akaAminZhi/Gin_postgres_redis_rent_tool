// db/repo_users_admin.go
package db

import (
	"Gin_postgres_redis_rent_tool/models"
	"context"
)

func (r *Repo) SetUserAdmin(ctx context.Context, userID string, isAdmin bool) error {
	return r.DB.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", userID).
		Update("is_admin", isAdmin).Error
}

func (r *Repo) CountAdmins(ctx context.Context) (int64, error) {
	var n int64
	err := r.DB.WithContext(ctx).
		Model(&models.User{}).
		Where("is_admin = TRUE").
		Count(&n).Error
	return n, err
}

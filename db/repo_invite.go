package db

import (
	"context"
	"errors"
	"time"

	"Gin_postgres_redis_rent_tool/models"
)

func (r *Repo) CreateInvite(ctx context.Context, email, token string, expiresAt time.Time, createdBy string) (*models.Invite, error) {
	inv := &models.Invite{Email: email, Token: token, ExpiresAt: expiresAt, CreatedBy: createdBy}
	return inv, r.DB.WithContext(ctx).Create(inv).Error
}

func (r *Repo) GetInviteByToken(ctx context.Context, token string) (*models.Invite, error) {
	var inv models.Invite
	if err := r.DB.WithContext(ctx).Where("token = ?", token).First(&inv).Error; err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *Repo) MarkInviteUsed(ctx context.Context, token string) error {
	now := time.Now()
	res := r.DB.WithContext(ctx).Model(&models.Invite{}).
		Where("token = ? AND used_at IS NULL", token).
		Update("used_at", &now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("invite already used or not found")
	}
	return nil
}

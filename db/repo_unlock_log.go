package db

import (
	"Gin_postgres_redis_rent_tool/models"
	"context"
	"fmt"
)

func (r *Repo) LogUnlock(ctx context.Context, actorID, actorUsername string, reason *string) (*models.UnlockLog, error) {
	log := &models.UnlockLog{
		ActorID:       actorID,
		ActorUsername: actorUsername,
		Reason:        reason,
	}
	if err := r.DB.WithContext(ctx).Create(log).Error; err != nil {
		return nil, fmt.Errorf("insert unlock log: %w", err)
	}
	return log, nil
}

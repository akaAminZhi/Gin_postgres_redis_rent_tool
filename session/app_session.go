package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type AppSessionStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewAppSessionStore(rdb *redis.Client, ttl time.Duration) *AppSessionStore {
	return &AppSessionStore{rdb: rdb, ttl: ttl}
}

type AppSession struct {
	UserID    string `json:"uid"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

func key(id string) string         { return fmt.Sprintf("app:sess:%s", id) }
func userSetKey(uid string) string { return fmt.Sprintf("app:user_sessions:%s", uid) }

func (s *AppSessionStore) Create(ctx context.Context, id, userID string) error {
	now := time.Now()
	b, _ := json.Marshal(AppSession{
		UserID:    userID,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(s.ttl).Unix(),
	})
	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, key(id), b, s.ttl)
	pipe.SAdd(ctx, userSetKey(userID), id)
	pipe.Expire(ctx, userSetKey(userID), s.ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *AppSessionStore) Get(ctx context.Context, id string) (*AppSession, error) {
	b, err := s.rdb.Get(ctx, key(id)).Bytes()
	if err != nil {
		return nil, err
	}
	var as AppSession
	if err := json.Unmarshal(b, &as); err != nil {
		return nil, err
	}
	return &as, nil
}

func (s *AppSessionStore) Delete(ctx context.Context, id string) error {
	as, _ := s.Get(ctx, id) // 忽略失败
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, key(id))
	if as != nil {
		pipe.SRem(ctx, userSetKey(as.UserID), id)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// ✅ 关键：删除用户时，撤销该用户的所有会话
func (s *AppSessionStore) RevokeAllForUser(ctx context.Context, userID string) error {
	ids, err := s.rdb.SMembers(ctx, userSetKey(userID)).Result()
	if err != nil && err != redis.Nil {
		return err
	}

	pipe := s.rdb.TxPipeline()
	for _, sid := range ids {
		pipe.Del(ctx, key(sid))
	}
	pipe.Del(ctx, userSetKey(userID))
	_, err = pipe.Exec(ctx)
	return err
}

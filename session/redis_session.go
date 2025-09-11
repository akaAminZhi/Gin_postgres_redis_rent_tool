package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/redis/go-redis/v9"
)

type Store struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewStore(rdb *redis.Client, ttl time.Duration) *Store { return &Store{rdb: rdb, ttl: ttl} }

func regKey(username string) string { return fmt.Sprintf("webauthn:reg:%s", username) }
func authKey(sid string) string     { return fmt.Sprintf("webauthn:auth:%s", sid) }

func (s *Store) SaveReg(ctx context.Context, username string, sd *webauthn.SessionData) error {
	b, _ := json.Marshal(sd)
	return s.rdb.Set(ctx, regKey(username), b, s.ttl).Err()
}

func (s *Store) LoadReg(ctx context.Context, username string) (*webauthn.SessionData, error) {
	b, err := s.rdb.Get(ctx, regKey(username)).Bytes()
	if err != nil {
		return nil, err
	}
	var sd webauthn.SessionData
	if err := json.Unmarshal(b, &sd); err != nil {
		return nil, err
	}
	return &sd, nil
}

func (s *Store) DelReg(ctx context.Context, username string) {
	_ = s.rdb.Del(ctx, regKey(username)).Err()
}

func (s *Store) SaveAuth(ctx context.Context, sid string, sd *webauthn.SessionData) error {
	b, _ := json.Marshal(sd)
	return s.rdb.Set(ctx, authKey(sid), b, s.ttl).Err()
}

func (s *Store) LoadAuth(ctx context.Context, sid string) (*webauthn.SessionData, error) {
	b, err := s.rdb.Get(ctx, authKey(sid)).Bytes()
	if err != nil {
		return nil, err
	}
	var sd webauthn.SessionData
	if err := json.Unmarshal(b, &sd); err != nil {
		return nil, err
	}
	return &sd, nil
}

func (s *Store) DelAuth(ctx context.Context, sid string) { _ = s.rdb.Del(ctx, authKey(sid)).Err() }

func regTokenKey(token string) string { return fmt.Sprintf("webauthn:reg:inv:%s", token) }

func (s *Store) SaveRegByToken(ctx context.Context, token string, sd *webauthn.SessionData) error {
	b, _ := json.Marshal(sd)
	return s.rdb.Set(ctx, regTokenKey(token), b, s.ttl).Err()
}

func (s *Store) LoadRegByToken(ctx context.Context, token string) (*webauthn.SessionData, error) {
	b, err := s.rdb.Get(ctx, regTokenKey(token)).Bytes()
	if err != nil {
		return nil, err
	}
	var sd webauthn.SessionData
	if err := json.Unmarshal(b, &sd); err != nil {
		return nil, err
	}
	return &sd, nil
}

func (s *Store) DelRegByToken(ctx context.Context, token string) {
	_ = s.rdb.Del(ctx, regTokenKey(token)).Err()
}

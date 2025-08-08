package redisorm

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type Session struct {
	c   *Client
	ctx context.Context
}

func (c *Client) WithContext(ctx context.Context) *Session { return &Session{c: c, ctx: ctx} }

// Simplified methods that proxy to the client with the session's context.
func (s *Session) Save(v any, ttl ...time.Duration) (string, error) {
	return s.c.Save(s.ctx, v, ttl...)
}
func (s *Session) Load(dst any, id string) error { return s.c.Load(s.ctx, dst, id) }
func (s *Session) Delete(v any, id string) error { return s.c.Delete(s.ctx, v, id) }
func (s *Session) UpdateFields(dst any, id string, updates map[string]any) (string, error) {
	return s.c.UpdateFields(s.ctx, dst, id, updates)
}
func (s *Session) UpdateFieldsFast(sample any, id string, updates map[string]any) error {
	return s.c.UpdateFieldsFast(s.ctx, sample, id, updates)
}
func (s *Session) Exists(sample any, id string) (bool, error) { return s.c.Exists(s.ctx, sample, id) }
func (s *Session) Touch(sample any, id string, ttl time.Duration) error {
	return s.c.Touch(s.ctx, sample, id, ttl)
}

// Payload methods
func (s *Session) SavePayload(sample any, id string, payload any, encrypt bool, ttl ...time.Duration) error {
	return s.c.SavePayload(s.ctx, sample, id, payload, encrypt, ttl...)
}
func (s *Session) FindPayload(sample any, id string, decrypt bool) ([]byte, error) {
	return s.c.GetPayload(s.ctx, sample, id, decrypt)
}
func (s *Session) TouchPayload(sample any, id string, ttl time.Duration) error {
	return s.c.TouchPayload(s.ctx, sample, id, ttl)
}

// Index paging methods
func (s *Session) PageIDsByIndex(sample any, field, value string, cursor uint64, count int64) ([]string, uint64, error) {
	return s.c.PageIDsByIndex(s.ctx, sample, field, value, cursor, count)
}
func (s *Session) PageIDsByEncIndex(sample any, field, plainValue string, cursor uint64, count int64) ([]string, uint64, error) {
	return s.c.PageIDsByEncIndex(s.ctx, sample, field, plainValue, cursor, count)
}

// Edit loads, mutates, and saves an object. Uses optimistic locking if a Version field is present.
func (s *Session) Edit(dst any, id string, mut func() error) (string, error) {
	// >>>>>>>>> FIX <<<<<<<<<
	// Get metadata first to pass to readPrimaryKey
	meta, err := s.c.getModelMetadata(dst)
	if err != nil {
		return "", err
	}

	if id == "" {
		id, err = readPrimaryKey(dst, meta) // Pass meta object
		if err != nil || id == "" {
			return "", errors.New("empty id")
		}
	}
	if err := s.c.Load(s.ctx, dst, id); err != nil && err != redis.Nil {
		return "", err
	}
	if mut != nil {
		if err := mut(); err != nil {
			return "", err
		}
	}
	if vp, _ := versionPointer(dst); vp != nil {
		return s.c.SaveOptimistic(s.ctx, dst)
	}
	return s.c.Save(s.ctx, dst)
}

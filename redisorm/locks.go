package redisorm

import (
	"context"
	"encoding/base64"
	"errors"
	"time"
)

func (c *Client) acquireLock(ctx context.Context, model, id string, ttl time.Duration) (func(context.Context) error, error) {
	if ttl <= 0 { ttl = 5 * time.Second }
	key := c.keyLock(model, id)
	tokBytes, _ := randBytes(16)
	token := base64.StdEncoding.EncodeToString(tokBytes)
	ok, err := c.rdb.SetNX(ctx, key, token, ttl).Result()
	if err != nil { return nil, err }
	if !ok { return nil, errors.New("lock busy") }
	unlock := func(ctx context.Context) error {
		_, err := c.luaUnlock.Run(ctx, c.rdb, []string{key}, token).Result()
		return err
	}
	return unlock, nil
}

// Retryable lock
type LockRetry struct {
	Attempts int
	Backoff  time.Duration
	Jitter   bool
}

func (c *Client) acquireLockWithRetry(ctx context.Context, model, id string, ttl time.Duration, lr LockRetry) (func(context.Context) error, error) {
	if lr.Attempts <= 0 { lr.Attempts = 1 }
	if lr.Backoff <= 0 { lr.Backoff = 80 * time.Millisecond }
	var lastErr error
	for i := 0; i < lr.Attempts; i++ {
		unlock, err := c.acquireLock(ctx, model, id, ttl)
		if err == nil { return unlock, nil }
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(jitter(lr.Backoff, lr.Jitter)):
		}
		lr.Backoff *= 2
		if lr.Backoff > 2*time.Second { lr.Backoff = 2 * time.Second }
	}
	return nil, lastErr
}

func jitter(d time.Duration, enabled bool) time.Duration {
	if !enabled { return d }
	return d + (d / 10)
}

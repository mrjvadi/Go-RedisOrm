package redisorm

import (
	"errors"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
	ns  string // namespace
	kek []byte // master key (KEK) برای لفافه‌کردن DEKها

	// Lua scripts
	luaSave             *redis.Script
	luaDelete           *redis.Script
	luaPayloadSave      *redis.Script
	luaUnlock           *redis.Script
	luaUpdateFieldsFast *redis.Script // >>>>>>>>> NEW <<<<<<<<<

	// >>>>>>>>> NEW <<<<<<<<<
	// Cache for model metadata to avoid repeated reflection
	metaCache sync.Map
}

var ErrVersionConflict = errors.New("version conflict")

type Option func(*Client)

func WithNamespace(ns string) Option {
	return func(c *Client) {
		if ns != "" {
			c.ns = ns
		}
	}
}

func WithMasterKey(kek []byte) Option {
	return func(c *Client) { c.kek = kek }
}

func New(rdb *redis.Client, opts ...Option) (*Client, error) {
	c := &Client{rdb: rdb, ns: "orm"}
	for _, o := range opts {
		o(c)
	}
	if l := len(c.kek); l != 16 && l != 24 && l != 32 {
		key, err := randBytes(32)
		if err != nil {
			return nil, fmt.Errorf("generate runtime KEK: %w", err)
		}
		c.kek = key
	}
	c.luaUnlock = redis.NewScript(luaUnlock)
	c.luaSave = redis.NewScript(luaSave)
	c.luaDelete = redis.NewScript(luaDelete)
	c.luaPayloadSave = redis.NewScript(luaPayloadSave)
	c.luaUpdateFieldsFast = redis.NewScript(luaUpdateFieldsFast) // >>>>>>>>> NEW <<<<<<<<<
	return c, nil
}

// Key builders
func (c *Client) keyVal(model, id string) string { return fmt.Sprintf("%s:val:%s:%s", c.ns, model, id) }
func (c. *Client) keyVer(model, id string) string { return fmt.Sprintf("%s:ver:%s:%s", c.ns, model, id) }
func (c *Client) keyIdx(model, field, value string) string {
	return fmt.Sprintf("%s:idx:%s:%s:%s", c.ns, model, field, value)
}
func (c *Client) keyIdxEnc(model, field, mac string) string {
	return fmt.Sprintf("%s:idxenc:%s:%s:%s", c.ns, model, field, mac)
}
func (c *Client) keyUniq(model, field, value string) string {
	return fmt.Sprintf("%s:uniq:%s:%s:%s", c.ns, model, field, value)
}
func (c *Client) keyLock(model, id string) string {
	return fmt.Sprintf("%s:lock:%s:%s", c.ns, model, id)
}
func (c *Client) keyPayload(model, id string) string {
	return fmt.Sprintf("%s:pl:%s:%s", c.ns, model, id)
}
func (c *Client) keyPayloadDEK(model, id string) string {
	return fmt.Sprintf("%s:dekp:%s:%s", c.ns, model, id)
}

// >>>>>>>>> CHANGED <<<<<<<<<
// DEK key is now per-object, not per-field, for better performance.
func (c *Client) keyDEKObject(model, id string) string {
	return fmt.Sprintf("%s:dek:%s:%s", c.ns, model, id)
}
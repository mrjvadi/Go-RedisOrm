package redisorm

import (
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
	ns  string // namespace
	kek []byte // master key (KEK) برای لفافه‌کردن DEKها

	// Lua scripts
	luaSave        *redis.Script
	luaDelete      *redis.Script
	luaPayloadSave *redis.Script
	luaUnlock      *redis.Script
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

// KEK باید 16/24/32 بایتی باشه؛ اگه ندی، برای dev یه KEK تصادفی ساخته می‌شه (برای production حتما KEK امن بده).
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
		c.kek = key // فقط برای dev
	}
	c.luaUnlock = redis.NewScript(luaUnlock)
	c.luaSave = redis.NewScript(luaSave)
	c.luaDelete = redis.NewScript(luaDelete)
	c.luaPayloadSave = redis.NewScript(luaPayloadSave)
	return c, nil
}

// Key builders
func (c *Client) keyVal(model, id string) string { return fmt.Sprintf("%s:val:%s:%s", c.ns, model, id) }
func (c *Client) keyVer(model, id string) string { return fmt.Sprintf("%s:ver:%s:%s", c.ns, model, id) }
func (c *Client) keyDEKField(model, id, field string) string {
	return fmt.Sprintf("%s:dekf:%s:%s:%s", c.ns, model, id, field)
}
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

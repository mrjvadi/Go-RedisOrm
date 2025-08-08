package redisorm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/redis/go-redis/v9"
)

// Session یک wrapper آگاه از context برای کلاینت ORM است که برای انجام عملیات پایگاه داده استفاده می‌شود.
type Session struct {
	c   *Client
	ctx context.Context
}

// WithContext یک session جدید با یک context مشخص ایجاد می‌کند.
func (c *Client) WithContext(ctx context.Context) *Session { return &Session{c: c, ctx: ctx} }

// Save یک شیء را به صورت اتمی در Redis ذخیره می‌کند.
func (s *Session) Save(v any, ttl ...time.Duration) (string, error) {
	return s.c.Save(s.ctx, v, ttl...)
}

// SaveAll یک اسلایس از اشیاء را با استفاده از Redis pipeline به صورت بهینه ذخیره می‌کند.
func (s *Session) SaveAll(slice any) ([]string, error) {
	return s.c.SaveAll(s.ctx, slice)
}

// Load یک شیء را بر اساس کلید اصلی آن از Redis می‌خواند.
func (s *Session) Load(dst any, id string) error { return s.c.Load(s.ctx, dst, id) }

// Delete یک شیء را بر اساس کلید اصلی آن به صورت اتمی حذف می‌کند.
func (s *Session) Delete(v any, id string) error { return s.c.Delete(s.ctx, v, id) }

// UpdateFields شیء را می‌خواند، تغییرات را اعمال می‌کند و سپس آن را دوباره ذخیره می‌کند.
func (s *Session) UpdateFields(dst any, id string, updates map[string]any) (string, error) {
	return s.c.UpdateFields(s.ctx, dst, id, updates)
}

// UpdateFieldsFast یک آپدیت جزئی و بسیار سریع روی JSON ذخیره شده انجام می‌دهد.
func (s *Session) UpdateFieldsFast(sample any, id string, updates map[string]any) error {
	return s.c.UpdateFieldsFast(s.ctx, sample, id, updates)
}

// Exists بررسی می‌کند که آیا شیء با کلید اصلی مشخص شده وجود دارد یا خیر.
func (s *Session) Exists(sample any, id string) (bool, error) { return s.c.Exists(s.ctx, sample, id) }

// >>>>>>>>> MODIFIED: امضای متد برای پشتیبانی از گروه تغییر کرد <<<<<<<<<
// Touch زمان انقضای (TTL) یک شیء را تمدید می‌کند.
func (s *Session) Touch(sample any, id string, ttl time.Duration) error {
	return s.c.Touch(s.ctx, sample, id, ttl)
}

// SavePayload یک داده اضافی (payload) را به یک شیء اصلی متصل می‌کند.
func (s *Session) SavePayload(sample any, id string, payload any, encrypt bool, ttl ...time.Duration) error {
	return s.c.SavePayload(s.ctx, sample, id, payload, encrypt, ttl...)
}

// FindPayload داده اضافی (payload) متصل به یک شیء را بازیابی می‌کند.
func (s *Session) FindPayload(sample any, id string, decrypt bool) ([]byte, error) {
	return s.c.GetPayload(s.ctx, sample, id, decrypt)
}

// >>>>>>>>> MODIFIED: امضای متد برای پشتیبانی از گروه تغییر کرد <<<<<<<<<
// TouchPayload زمان انقضای (TTL) یک payload را تمدید می‌کند.
func (s *Session) TouchPayload(sample any, id string, ttl time.Duration) error {
	return s.c.TouchPayload(s.ctx, sample, id, ttl)
}

// ... (سایر توابع فایل بدون تغییر باقی می‌مانند) ...
// PageIDsByIndex, PageIDsByEncIndex, Edit, TransactionalOperation, Transaction, Execute
func (s *Session) PageIDsByIndex(sample any, field, value string, cursor uint64, count int64) ([]string, uint64, error) {
	return s.c.PageIDsByIndex(s.ctx, sample, field, value, cursor, count)
}

func (s *Session) PageIDsByEncIndex(sample any, field, plainValue string, cursor uint64, count int64) ([]string, uint64, error) {
	return s.c.PageIDsByEncIndex(s.ctx, sample, field, plainValue, cursor, count)
}

func (s *Session) Edit(dst any, id string, mut func() error) (string, error) {
	meta, err := s.c.getModelMetadata(dst)
	if err != nil {
		return "", err
	}
	if id == "" {
		id, err = readPrimaryKey(dst, meta)
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

type TransactionalOperation struct {
	sess   *Session
	sample any
	id     string
}

func (s *Session) Transaction(sample any, id string) *TransactionalOperation {
	return &TransactionalOperation{
		sess:   s,
		sample: sample,
		id:     id,
	}
}

func (op *TransactionalOperation) Execute(fn func(v any) error) error {
	meta, err := op.sess.c.getModelMetadata(op.sample)
	if err != nil {
		return err
	}

	modelPrefix := op.sess.c.modelPrefix(meta)
	unlock, err := op.sess.c.acquireLockWithRetry(op.sess.ctx, modelPrefix, op.id, 5*time.Second, LockRetry{
		Attempts: 3,
		Backoff:  100 * time.Millisecond,
	})
	if err != nil {
		return fmt.Errorf("could not acquire lock for %s: %w", op.id, err)
	}
	defer unlock(op.sess.ctx)

	rt := reflect.TypeOf(op.sample)
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	obj := reflect.New(rt).Interface()

	if err := op.sess.Load(obj, op.id); err != nil {
		return fmt.Errorf("could not load object inside lock: %w", err)
	}

	if err := fn(obj); err != nil {
		return err
	}

	if vp, _ := versionPointer(obj); vp != nil {
		_, err = op.sess.c.SaveOptimistic(op.sess.ctx, obj)
	} else {
		_, err = op.sess.c.Save(op.sess.ctx, obj)
	}

	if err != nil {
		return fmt.Errorf("could not save object after operation: %w", err)
	}

	return nil
}
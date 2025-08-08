package redisorm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/redis/go-redis/v9"
)

type Session struct {
	c   *Client
	ctx context.Context
}

func (c *Client) WithContext(ctx context.Context) *Session { return &Session{c: c, ctx: ctx} }

// ... (متدهای دیگر Session مانند Save, Load و غیره در اینجا قرار دارند)
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
func (s *Session) SavePayload(sample any, id string, payload any, encrypt bool, ttl ...time.Duration) error {
	return s.c.SavePayload(s.ctx, sample, id, payload, encrypt, ttl...)
}
func (s *Session) FindPayload(sample any, id string, decrypt bool) ([]byte, error) {
	return s.c.GetPayload(s.ctx, sample, id, decrypt)
}
func (s *Session) TouchPayload(sample any, id string, ttl time.Duration) error {
	return s.c.TouchPayload(s.ctx, sample, id, ttl)
}
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


// >>>>>>>>> NEW FEATURE: Transactional Pattern <<<<<<<<<

// LockedOperation یک عملیات تراکنشی load-lock-save را مدیریت می‌کند.
type LockedOperation struct {
	sess   *Session
	sample any
	id     string
}

// FindAndLock یک عملیات تراکنشی روی یک شیء را شروع می‌کند.
// این متد برای قفل کردن، خواندن و اجرای یک تابع روی شیء آماده می‌شود.
func (s *Session) FindAndLock(sample any, id string) *LockedOperation {
	return &LockedOperation{
		sess:   s,
		sample: sample,
		id:     id,
	}
}

// Do تابع ارائه شده را در داخل قفل اجرا می‌کند.
// تابع `fn` شیء خوانده شده را دریافت می‌کند. هر تغییری که روی شیء
// در داخل تابع ایجاد شود، پس از اجرای موفقیت‌آمیز، به صورت خودکار در Redis ذخیره می‌شود.
// قفل به صورت خودکار پس از عملیات آزاد می‌شود.
func (op *LockedOperation) Do(fn func(v any) error) error {
	meta, err := op.sess.c.getModelMetadata(op.sample)
	if err != nil {
		return err
	}

	// 1. قفل را بگیر
	unlock, err := op.sess.c.acquireLockWithRetry(op.sess.ctx, meta.StructName, op.id, 5*time.Second, LockRetry{
		Attempts: 3,
		Backoff:  100 * time.Millisecond,
	})
	if err != nil {
		return fmt.Errorf("could not acquire lock for %s: %w", op.id, err)
	}
	defer unlock(op.sess.ctx)

	// 2. شیء را بخوان
	rt := reflect.TypeOf(op.sample)
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	obj := reflect.New(rt).Interface()

	if err := op.sess.Load(obj, op.id); err != nil {
		return fmt.Errorf("could not load object inside lock: %w", err)
	}

	// 3. تابع کاربر را اجرا کن
	if err := fn(obj); err != nil {
		return err // اگر تابع کاربر خطا داد، ذخیره نکن
	}

	// 4. شیء تغییر یافته را ذخیره کن
	if vp, _ := versionPointer(obj); vp != nil {
		_, err = op.sess.c.SaveOptimistic(op.sess.ctx, obj)
	} else {
		_, err = op.sess.Save(obj)
	}

	if err != nil {
		return fmt.Errorf("could not save object after operation: %w", err)
	}

	return nil
}

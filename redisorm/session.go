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

// Save یک شیء را به صورت اتمی در Redis ذخیره می‌کند. این متد ایندکس‌ها و محدودیت‌های unique را نیز مدیریت می‌کند.
func (s *Session) Save(v any, ttl ...time.Duration) (string, error) {
	return s.c.Save(s.ctx, v, ttl...)
}

// SaveAll یک اسلایس از اشیاء را با استفاده از Redis pipeline به صورت بهینه ذخیره می‌کند.
// این متد اسلایسی از کلیدهای اصلی اشیاء ذخیره شده را برمی‌گرداند.
func (s *Session) SaveAll(slice any) ([]string, error) {
	return s.c.SaveAll(s.ctx, slice)
}

// Load یک شیء را بر اساس کلید اصلی آن از Redis می‌خواند.
func (s *Session) Load(dst any, id string) error { return s.c.Load(s.ctx, dst, id) }

// Delete یک شیء را بر اساس کلید اصلی آن به صورت اتمی حذف کرده و ایندکس‌های مربوطه را پاکسازی می‌کند.
func (s *Session) Delete(v any, id string) error { return s.c.Delete(s.ctx, v, id) }

// UpdateFields شیء را می‌خواند، تغییرات را اعمال می‌کند و سپس آن را دوباره ذخیره می‌کند. این متد ایندکس‌ها را نیز به‌روزرسانی می‌کند.
// برای عملکرد بهتر در آپدیت فیلدهای غیر ایندکس، از UpdateFieldsFast استفاده کنید.
func (s *Session) UpdateFields(dst any, id string, updates map[string]any) (string, error) {
	return s.c.UpdateFields(s.ctx, dst, id, updates)
}

// UpdateFieldsFast یک آپدیت جزئی و بسیار سریع روی JSON ذخیره شده انجام می‌دهد.
// هشدار: این متد ایندکس‌ها را آپدیت نمی‌کند. فقط برای فیلدهایی که ایندکس نشده‌اند استفاده شود.
func (s *Session) UpdateFieldsFast(sample any, id string, updates map[string]any) error {
	return s.c.UpdateFieldsFast(s.ctx, sample, id, updates)
}

// Exists بررسی می‌کند که آیا شیء با کلید اصلی مشخص شده وجود دارد یا خیر.
func (s *Session) Exists(sample any, id string) (bool, error) { return s.c.Exists(s.ctx, sample, id) }

// Touch زمان انقضای (TTL) یک شیء را بدون نیاز به ذخیره مجدد، تمدید می‌کند.
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

// TouchPayload زمان انقضای (TTL) یک payload را تمدید می‌کند.
func (s *Session) TouchPayload(sample any, id string, ttl time.Duration) error {
	return s.c.TouchPayload(s.ctx, sample, id, ttl)
}

// PageIDsByIndex شناسه‌های اشیاء را بر اساس یک ایندکس معمولی، به صورت صفحه‌بندی شده جستجو می‌کند.
func (s *Session) PageIDsByIndex(sample any, field, value string, cursor uint64, count int64) ([]string, uint64, error) {
	return s.c.PageIDsByIndex(s.ctx, sample, field, value, cursor, count)
}

// PageIDsByEncIndex شناسه‌های اشیاء را بر اساس یک ایندکس رمزنگاری‌شده، به صورت صفحه‌بندی شده جستجو می‌کند.
func (s *Session) PageIDsByEncIndex(sample any, field, plainValue string, cursor uint64, count int64) ([]string, uint64, error) {
	return s.c.PageIDsByEncIndex(s.ctx, sample, field, plainValue, cursor, count)
}

// Edit یک شیء را می‌خواند، تابع جهش‌دهنده (mutator) را روی آن اجرا می‌کند و سپس آن را ذخیره می‌کند.
// اگر مدل دارای فیلد Version باشد، از قفل خوش‌بینانه استفاده می‌کند.
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

// --- Transactional Pattern ---

// TransactionalOperation یک عملیات تراکنشی load-lock-execute-save را مدیریت می‌کند.
type TransactionalOperation struct {
	sess   *Session
	sample any
	id     string
}

// Transaction یک عملیات تراکنشی روی یک شیء مشخص را شروع می‌کند.
// این متد ORM را برای قفل کردن، خواندن و اجرای یک تابع آماده می‌کند.
func (s *Session) Transaction(sample any, id string) *TransactionalOperation {
	return &TransactionalOperation{
		sess:   s,
		sample: sample,
		id:     id,
	}
}

// Execute تابع ارائه شده را در داخل یک قفل توزیع‌شده اجرا می‌کند.
// تابع `fn` شیء خوانده شده از Redis را دریافت می‌کند. هر تغییری که روی شیء
// در داخل تابع ایجاد شود، پس از اجرای موفقیت‌آمیز، به صورت خودکار در Redis ذخیره می‌شود.
// قفل به صورت خودکار پس از پایان عملیات آزاد می‌شود.
func (op *TransactionalOperation) Execute(fn func(v any) error) error {
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
		return err // در صورت خطا، ذخیره نکن؛ قفل آزاد خواهد شد.
	}

	// 4. شیء تغییر یافته را ذخیره کن
	if vp, _ := versionPointer(obj); vp != nil {
		_, err = op.sess.c.SaveOptimistic(op.sess.ctx, obj)
	} else {
		// >>>>>>>>> FIX: Added missing context argument <<<<<<<<<
		_, err = op.sess.c.Save(op.sess.ctx, obj)
	}

	if err != nil {
		return fmt.Errorf("could not save object after operation: %w", err)
	}

	return nil
}

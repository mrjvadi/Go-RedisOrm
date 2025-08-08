package redisorm_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mrjvadi/Go-RedisOrm/redisorm"
	"github.com/redis/go-redis/v9"
)

// User struct for benchmarking
type User struct {
	ID        string    `json:"id" redis:"pk"`
	Version   int64     `json:"version" redis:"version"`
	Email     string    `json:"email" secret:"true" redis:",unique"`
	Country   string    `json:"country" redis:",index"`
	CreatedAt time.Time `json:"created_at" redis:",auto_create_time"`
	UpdatedAt time.Time `json:"updated_at" redis:",auto_update_time"`
}

// AuditLog یک مدل برای تست کردن قابلیت‌های سفارشی است.
type AuditLog struct {
	ID        string    `json:"id" redis:"pk" default:"uuid"`
	Action    string    `json:"action"`
	UserID    string    `json:"user_id" redis:",index"`
	Timestamp time.Time `json:"timestamp" redis:",auto_create_time"`
	Modified  time.Time `json:"modified" redis:",auto_update_time"`
}

// ModelName نام مدل را در Redis به "audit_events" تغییر می‌دهد.
func (a *AuditLog) ModelName() string {
	return "audit_events"
}

// >>>>>>>>> NEW: مدل جدید برای تست گروه‌بندی <<<<<<<<<
type Product struct {
	ID   int    `json:"id" redis:"pk"` // کلید اصلی عددی
	SKU  string `json:"sku"`
	Name string `json:"name"`
}

func (p *Product) ModelName() string { return "products" }
func (p *Product) GroupName() string { return "inventory" }

var (
	orm *redisorm.Client
	ctx = context.Background()
	rdb *redis.Client
)

// setupClient یک کلاینت ORM برای تست‌ها و بنچمارک‌ها ایجاد می‌کند.
func setupClient(t testing.TB) (*redisorm.Client, string) {
	if rdb == nil {
		rdb = redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		})
		if err := rdb.Ping(ctx).Err(); err != nil {
			t.Fatalf("could not connect to redis: %v", err)
		}
	}

	ns := fmt.Sprintf("test_%d", time.Now().UnixNano())
	client, err := redisorm.New(rdb, redisorm.WithNamespace(ns), redisorm.WithMasterKey([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatalf("failed to create orm client: %v", err)
	}
	return client, ns
}

func TestTagsAndHooks(t *testing.T) {
	orm, ns := setupClient(t)
	sess := orm.WithContext(ctx)

	t.Run("LifecycleAndCustomName", func(t *testing.T) {
		logEntry := &AuditLog{Action: "USER_LOGIN", UserID: "user-abc"}
		id, err := sess.Save(logEntry)
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		var loadedLog AuditLog
		if err := sess.Load(&loadedLog, id); err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if loadedLog.Timestamp.IsZero() || loadedLog.Modified.IsZero() {
			t.Errorf("Expected timestamps to be set automatically, but they are zero")
		}
		if loadedLog.Modified.Sub(loadedLog.Timestamp) > time.Second {
			t.Errorf("Expected Timestamp and Modified to be very close on creation")
		}

		expectedKey := fmt.Sprintf("%s:val:audit_events:%s", ns, id)
		exists, err := rdb.Exists(ctx, expectedKey).Result()
		if err != nil || exists == 0 {
			t.Errorf("Expected key '%s' to exist in Redis, but it doesn't", expectedKey)
		}

		time.Sleep(1 * time.Second)
		loadedLog.Action = "USER_LOGOUT"
		if _, err := sess.Save(&loadedLog); err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		var updatedLog AuditLog
		if err := sess.Load(&updatedLog, id); err != nil {
			t.Fatalf("Load after update failed: %v", err)
		}

		if !updatedLog.Timestamp.Equal(loadedLog.Timestamp) {
			t.Errorf("Expected Timestamp to be unchanged. Got %v, want %v", updatedLog.Timestamp, loadedLog.Timestamp)
		}
		if updatedLog.Modified.Sub(loadedLog.Timestamp) < time.Second {
			t.Errorf("Expected Modified to be updated. It appears unchanged.")
		}
		sess.Delete(&AuditLog{}, id)
	})

	t.Run("GroupingAndNumericPK", func(t *testing.T) {
		product := &Product{ID: 123, SKU: "TEST-001", Name: "Test Product"}
		id, err := sess.Save(product)
		if err != nil {
			t.Fatalf("Save failed for grouped model: %v", err)
		}
		if id != "123" {
			t.Fatalf("Expected saved ID to be '123', got '%s'", id)
		}

		expectedKey := fmt.Sprintf("%s:val:inventory:products:%s", ns, id)
		exists, err := rdb.Exists(ctx, expectedKey).Result()
		if err != nil || exists == 0 {
			t.Errorf("Expected key '%s' for grouped model to exist, but it doesn't", expectedKey)
		}

		var loadedProduct Product
		if err := sess.Load(&loadedProduct, id); err != nil {
			t.Fatalf("Failed to load grouped model: %v", err)
		}
		if loadedProduct.Name != "Test Product" {
			t.Errorf("Loaded product name is incorrect")
		}
		sess.Delete(&Product{}, id)
	})
}

// ... (سایر تست‌های بنچمارک) ...

func BenchmarkTouch(b *testing.B) {
	orm, _ := setupClient(b)
	sess := orm.WithContext(ctx)
	sampleUser := &User{Email: "touch-test@example.com", Country: "NZ"}
	id, err := sess.Save(sampleUser, 10*time.Second)
	if err != nil {
		b.Fatalf("failed to save sample user for touch benchmark: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// >>>>>>>>> MODIFIED: استفاده از امضای جدید متد Touch <<<<<<<<<
		err := sess.Touch(&User{}, id, 30*time.Second)
		if err != nil {
			b.Fatalf("failed to touch user: %v", err)
		}
	}
}
// ...
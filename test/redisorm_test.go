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

// >>>>>>>>> NEW STRUCT FOR TESTING TAGS <<<<<<<<<
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

var (
	orm *redisorm.Client
	ctx = context.Background()
	rdb *redis.Client
)

// setupClient یک کلاینت ORM برای تست‌ها و بنچمارک‌ها ایجاد می‌کند.
func setupClient(t testing.TB) {
	if rdb == nil {
		rdb = redis.NewClient(&redis.Options{
			Addr: "localhost:6379", // آدرس Redis خود را در صورت نیاز تغییر دهید
		})
		if err := rdb.Ping(ctx).Err(); err != nil {
			t.Fatalf("could not connect to redis: %v", err)
		}
	}

	ns := fmt.Sprintf("test_%d", time.Now().UnixNano())
	var err error
	orm, err = redisorm.New(rdb, redisorm.WithNamespace(ns), redisorm.WithMasterKey([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatalf("failed to create orm client: %v", err)
	}
}

// >>>>>>>>> NEW UNIT TEST FOR TAGS AND HOOKS <<<<<<<<<
func TestTagsAndHooks(t *testing.T) {
	setupClient(t)
	sess := orm.WithContext(ctx)

	// 1. یک لاگ جدید ذخیره می‌کنیم
	logEntry := &AuditLog{Action: "USER_LOGIN", UserID: "user-abc"}
	id, err := sess.Save(logEntry)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 2. لاگ را می‌خوانیم و زمان‌ها را بررسی می‌کنیم
	var loadedLog AuditLog
	if err := sess.Load(&loadedLog, id); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loadedLog.Timestamp.IsZero() || loadedLog.Modified.IsZero() {
		t.Errorf("Expected timestamps to be set automatically, but they are zero")
	}
	// در زمان ایجاد، هر دو زمان باید تقریباً برابر باشند
	if loadedLog.Modified.Sub(loadedLog.Timestamp) > time.Second {
		t.Errorf("Expected Timestamp and Modified to be very close on creation")
	}

	// 3. بررسی می‌کنیم که آیا نام مدل سفارشی در کلید Redis استفاده شده است
	expectedKey := fmt.Sprintf("%s:val:%s:%s", "test_"+orm.GetNamespace(), "audit_events", id)
	exists, err := rdb.Exists(ctx, expectedKey).Result()
	if err != nil || exists == 0 {
		t.Errorf("Expected key '%s' to exist in Redis, but it doesn't", expectedKey)
	}

	// 4. چند ثانیه صبر کرده و لاگ را آپدیت می‌کنیم
	time.Sleep(1 * time.Second)
	loadedLog.Action = "USER_LOGOUT"
	if _, err := sess.Save(&loadedLog); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// 5. دوباره لاگ را می‌خوانیم و زمان‌ها را بررسی می‌کنیم
	var updatedLog AuditLog
	if err := sess.Load(&updatedLog, id); err != nil {
		t.Fatalf("Load after update failed: %v", err)
	}

	// Timestamp (auto_create_time) نباید تغییر کرده باشد
	if !updatedLog.Timestamp.Equal(loadedLog.Timestamp) {
		t.Errorf("Expected Timestamp (auto_create_time) to be unchanged. Got %v, want %v", updatedLog.Timestamp, loadedLog.Timestamp)
	}

	// Modified (auto_update_time) باید به‌روز شده باشد
	if updatedLog.Modified.Sub(loadedLog.Timestamp) < time.Second {
		t.Errorf("Expected Modified (auto_update_time) to be updated. It appears unchanged.")
	}

	// پاکسازی
	sess.Delete(&AuditLog{}, id)
}

// --- Benchmarks ---

func BenchmarkSave(b *testing.B) {
	setupClient(b)
	b.ResetTimer()
	sess := orm.WithContext(ctx)
	for i := 0; i < b.N; i++ {
		user := &User{
			Email:   fmt.Sprintf("user%d@example.com", i),
			Country: "DE",
		}
		_, err := sess.Save(user)
		if err != nil {
			b.Fatalf("failed to save user: %v", err)
		}
	}
}

func BenchmarkLoad(b *testing.B) {
	setupClient(b)
	sess := orm.WithContext(ctx)
	sampleUser := &User{Email: "load-test@example.com", Country: "US"}
	id, err := sess.Save(sampleUser)
	if err != nil {
		b.Fatalf("failed to save sample user for load benchmark: %v", err)
	}
	b.ResetTimer()
	var u User
	for i := 0; i < b.N; i++ {
		err := sess.Load(&u, id)
		if err != nil {
			b.Fatalf("failed to load user: %v", err)
		}
	}
}

func BenchmarkUpdateFields(b *testing.B) {
	setupClient(b)
	sess := orm.WithContext(ctx)
	sampleUser := &User{Email: "update-test@example.com", Country: "CA"}
	id, err := sess.Save(sampleUser)
	if err != nil {
		b.Fatalf("failed to save sample user for update benchmark: %v", err)
	}
	b.ResetTimer()
	countries := []string{"FR", "UK", "IT", "JP"}
	for i := 0; i < b.N; i++ {
		updates := map[string]any{
			"country": countries[i%len(countries)],
		}
		_, err := sess.UpdateFields(&User{}, id, updates)
		if err != nil {
			b.Fatalf("failed to update fields: %v", err)
		}
	}
}

func BenchmarkSaveAndLoad(b *testing.B) {
	setupClient(b)
	b.ResetTimer()
	sess := orm.WithContext(ctx)
	for i := 0; i < b.N; i++ {
		userToSave := &User{
			Email:   fmt.Sprintf("roundtrip%d@example.com", i),
			Country: "AU",
		}
		id, err := sess.Save(userToSave)
		if err != nil {
			b.Fatalf("Save failed during roundtrip benchmark: %v", err)
		}
		var loadedUser User
		err = sess.Load(&loadedUser, id)
		if err != nil {
			b.Fatalf("Load failed during roundtrip benchmark: %v", err)
		}
	}
}

func BenchmarkTouch(b *testing.B) {
	setupClient(b)
	sess := orm.WithContext(ctx)
	sampleUser := &User{Email: "touch-test@example.com", Country: "NZ"}
	id, err := sess.Save(sampleUser, 10*time.Second)
	if err != nil {
		b.Fatalf("failed to save sample user for touch benchmark: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := sess.Touch("User", id, 30*time.Second)
		if err != nil {
			b.Fatalf("failed to touch user: %v", err)
		}
	}
}

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mrjvadi/Go-RedisOrm/redisorm"
	"github.com/redis/go-redis/v9"
)

// AuditLog یک مدل برای نمایش قابلیت‌های جدید است.
type AuditLog struct {
	ID        string    `json:"id" redis:"pk" default:"uuid"`
	Action    string    `json:"action"`
	UserID    string    `json:"user_id" redis:",index"`
	// به جای CreatedAt و UpdatedAt، از نام‌های سفارشی استفاده می‌کنیم
	Timestamp time.Time `json:"timestamp" redis:",auto_create_time"`
	Modified  time.Time `json:"modified" redis:",auto_update_time"`
}

// ModelName نام مدل را در Redis به "audit_events" تغییر می‌دهد.
func (a *AuditLog) ModelName() string {
	return "audit_events"
}

func main() {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	orm, err := redisorm.New(rdb,
		redisorm.WithNamespace("examples_v2"),
		redisorm.WithMasterKey([]byte("a-very-secure-32-byte-secret-key")),
	)
	if err != nil {
		log.Fatalf("Failed to create ORM client: %v", err)
	}
	sess := orm.WithContext(ctx)

	fmt.Println("--- Custom Model Name & Lifecycle Hooks Example ---")

	// 1. یک لاگ جدید ذخیره می‌کنیم. فیلدهای Timestamp و Modified به صورت خودکار پر می‌شوند.
	logEntry := &AuditLog{Action: "USER_LOGIN", UserID: "user-123"}
	id, err := sess.Save(logEntry)
	if err != nil {
		log.Fatalf("Save failed: %v", err)
	}
	fmt.Printf("✅ Log entry saved with ID: %s\n", id)

	// 2. لاگ را می‌خوانیم تا مقادیر خودکار را ببینیم
	var loadedLog AuditLog
	if err := sess.Load(&loadedLog, id); err != nil {
		log.Fatalf("Load failed: %v", err)
	}
	fmt.Printf("   - Initial Timestamp: %s\n", loadedLog.Timestamp.Format(time.RFC3339))
	fmt.Printf("   - Initial Modified:  %s\n", loadedLog.Modified.Format(time.RFC3339))

	// 3. چند ثانیه صبر کرده و لاگ را آپدیت می‌کنیم
	fmt.Println("\nUpdating log entry...")
	time.Sleep(2 * time.Second)
	loadedLog.Action = "USER_LOGOUT"
	if _, err := sess.Save(&loadedLog); err != nil {
		log.Fatalf("Update failed: %v", err)
	}

	// 4. دوباره لاگ را می‌خوانیم تا ببینیم کدام فیلدها تغییر کرده‌اند
	var updatedLog AuditLog
	if err := sess.Load(&updatedLog, id); err != nil {
		log.Fatalf("Load after update failed: %v", err)
	}
	fmt.Printf("   - Timestamp (should be unchanged): %s\n", updatedLog.Timestamp.Format(time.RFC3339))
	fmt.Printf("   - Modified (should be updated):  %s\n", updatedLog.Modified.Format(time.RFC3339))
	fmt.Println("\n✅ Lifecycle hooks worked as expected.")

	// 5. بررسی می‌کنیم که آیا کلید در Redis با نام سفارشی ذخیره شده است
	keyInRedis := fmt.Sprintf("examples_v2:val:%s:%s", loadedLog.ModelName(), id)
	exists, _ := rdb.Exists(ctx, keyInRedis).Result()
	if exists == 1 {
		fmt.Printf("✅ Verified that the key '%s' exists in Redis.\n", keyInRedis)
	}

	// پاکسازی
	sess.Delete(&AuditLog{}, id)
}

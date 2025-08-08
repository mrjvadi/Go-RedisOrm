# Go-RedisOrm

یک **ORM ساده، سریع و امن برای Go** که روی Redis ساخته شده و اجازه می‌دهد با ساختارهای `struct`ی زبان Go به‌صورت تمیز و نوع‌امن کار کنید. این کتابخانه پیچیدگی‌های مربوط به سریالایز کردن، ایندکس‌گذاری، رمزنگاری سمت‌کلاینت، عملیات اتمی و قفل خوش‌بینانه را برای شما مدیریت می‌کند.

---

## فهرست مطالب

- [ویژگی‌ها](#ویژگیها)
- [نصب](#نصب)
- [شروع سریع](#شروع-سریع)
- [استفاده و تگ‌ها](#استفاده-و-تگها)
- [شخصی‌سازی با اینترفیس‌ها](#شخصیسازی-با-اینترفیسها)
- [عملیات گروهی (Bulk)](#عملیات-گروهی-bulk)
- [الگوی تراکنشی (Get-Lock-Do)](#الگوی-تراکنشی-get-lock-do)
- [فضای نام و ساختار کلیدها](#فضای-نام-و-ساختار-کلیدها)
- [پوشه مثال‌ها](#پوشه-مثالها)
- [مجوز](#مجوز)

---

## ویژگی‌ها

- **مدل‌سازی مبتنی بر Struct**: تعریف مدل‌ها با تگ‌های ساده روی فیلدها.
- **کلید اصلی (Primary Key) منعطف**: پشتیبانی از `string` (با تولید خودکار UUID در صورت خالی بودن) و انواع عددی (`int`, `int64`, ...).
- **ایندکس‌گذاری قدرتمند**: ایندکس معمولی (`index`)، یونیک (`unique`) و **ایندکس رمزنگاری‌شده** (`index_enc`) برای جستجوی سریع و امن.
- **رمزنگاری سمت کلاینت**: با تگ `secret:"true"` فیلدهای حساس را به‌صورت خودکار با AES-GCM رمزنگاری کنید (نیازمند کلید اصلی Master Key).
- **عملیات اتمی با Lua**: نوشتن/به‌روزرسانی/حذف به‌صورت اتمی برای ثبات داده.
- **قفل خوش‌بینانه (Optimistic Locking)**: با تگ `redis:"version"` روی فیلد `int64`.
- **عملیات گروهی (Bulk)**: ذخیره مجموعه‌ای از اشیا با یک فراخوانی.
- **قلاب‌های چرخه‌عمر**: `auto_create_time` و `auto_update_time` برای مدیریت خودکار تایم‌استمپ‌ها.
- **شخصی‌سازی پیشرفته**: تغییر نام مدل، گروه‌بندی کلیدها و TTL پیش‌فرض با اینترفیس‌ها.

---

## نصب

```bash
go get github.com/mrjvadi/Go-RedisOrm
```

---

## شروع سریع

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/redis/go-redis/v9"
    "github.com/mrjvadi/Go-RedisOrm/redisorm"
)

type User struct {
    ID        string    `json:"id" redis:"pk" default:"uuid"`
    Version   int64     `json:"version" redis:"version"`
    Email     string    `json:"email" secret:"true" redis:",unique"`
    Country   string    `json:"country" redis:",index"`
    Status    string    `json:"status"`
    CreatedAt time.Time `json:"created_at" redis:",auto_create_time"`
    UpdatedAt time.Time `json:"updated_at" redis:",auto_update_time"`
}

// قرار دادن مدل در گروه "accounts"
func (u *User) GroupName() string { return "accounts" }

// TTL پیش‌فرض 1 ساعت برای رکوردهای User
func (u *User) AutoDeleteTTL() time.Duration { return time.Hour }

func main() {
    ctx := context.Background()

    rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

    orm, err := redisorm.New(
        rdb,
        redisorm.WithNamespace("myapp"),
        // کلید 32-بایتی برای AES-GCM
        redisorm.WithMasterKey([]byte("your-32-byte-master-key-here-!")),
    )
    if err != nil { log.Fatal(err) }

    sess := orm.WithContext(ctx)

    // --- ذخیره ---
    id, err := sess.Save(&User{Email: "user@example.com", Country: "IR", Status: "active"})
    if err != nil { log.Fatal(err) }

    // --- خواندن ---
    var u User
    if err := sess.Load(&u, id); err != nil { log.Fatal(err) }

    // --- حذف ---
    if err := sess.Delete(&User{}, id); err != nil { log.Fatal(err) }
}
```

---

## استفاده و تگ‌ها

| تگ                          | توضیح                                                                        | نمونه                                                           |
| --------------------------- | ---------------------------------------------------------------------------- | --------------------------------------------------------------- |
| `redis:"pk"`                | تعیین فیلد به عنوان کلید اصلی. از `string` و انواع عددی پشتیبانی می‌شود.     | \`ID string ` + "`redis:"pk"`" + `\`                            |
| `default:"uuid"`            | اگر کلید اصلی از نوع `string` و خالی باشد، به‌صورت خودکار UUID تولید می‌شود. | \`ID string ` + "`redis:"pk" default:"uuid"`" + `\`             |
| `redis:"version"`           | فعال‌سازی قفل خوش‌بینانه؛ فیلد باید `int64` باشد.                            | \`Version int64 ` + "`redis:"version"`" + `\`                   |
| `secret:"true"`             | رمزنگاری خودکار مقدار فیلد با AES-GCM (نیازمند `MasterKey`).                 | \`Email string ` + "`secret:"true"`" + `\`                      |
| `redis:",index"`            | ایجاد ایندکس برای جستجو.                                                     | \`Country string ` + "`redis:",index"`" + `\`                   |
| `redis:",unique"`           | ایجاد محدودیت یکتا.                                                          | \`Email string ` + "`redis:",unique"`" + `\`                    |
| `redis:",index_enc"`        | ایندکس **رمزنگاری‌شده (deterministic)** برای جستجوی امن.                     | \`NationalID string ` + "`redis:",index\_enc"`" + `\`           |
| `redis:",auto_create_time"` | زمان ساخت را روی `time.Time` تنظیم می‌کند.                                   | \`CreatedAt time.Time ` + "`redis:",auto\_create\_time"`" + `\` |
| `redis:",auto_update_time"` | زمان ساخت/به‌روزرسانی را تنظیم می‌کند.                                       | \`UpdatedAt time.Time ` + "`redis:",auto\_update\_time"`" + `\` |

> **نکته**: برای رمزنگاری فیلدها، حتماً `WithMasterKey(...)` را هنگام ساخت ORM تنظیم کنید.

---

## شخصی‌سازی با اینترفیس‌ها

### تغییر نام مدل (ModelNamer)

```go
type AuditLog struct { /* ... */ }

func (a *AuditLog) ModelName() string { return "audit_events" }
```

### گروه‌بندی مدل‌ها (ModelGrouper)

```go
type SessionData struct { /* ... */ }

func (s *SessionData) GroupName() string { return "sessions" }
```

### TTL پیش‌فرض مدل (AutoDeleter)

```go
type OTP struct { /* ... */ }

func (o *OTP) AutoDeleteTTL() time.Duration { return 5 * time.Minute }
```

---

## عملیات گروهی (Bulk)

```go
users := []*User{
    {Email: "bulk1@example.com", Country: "CA"},
    {Email: "bulk2@example.com", Country: "CA"},
}
ids, err := sess.SaveAll(users)
if err != nil { /* handle */ }
```

---

## الگوی تراکنشی (Get-Lock-Do)

برای عملیات حساس (مانند کم‌کردن موجودی)، از تراکنش داخلی استفاده کنید:

```go
err := sess.Transaction(&Product{}, "product-sku-123").Execute(func(v any) error {
    p := v.(*Product)
    if p.Inventory < 1 {
        return errors.New("not enough inventory")
    }
    p.Inventory--
    return nil // برگرداندن nil یعنی تغییرات ذخیره شود
})
if err != nil { /* handle */ }
```

---

## فضای نام و ساختار کلیدها

کلیدها به‌صورت زیر نام‌گذاری می‌شوند:

```
{namespace}:val:{group}:{ModelName}:{id}
```

> مثال: `myapp:val:sessions:SessionData:123`

---

## پوشه مثال‌ها

برای نمونه‌های بیشتر به پوشه [`examples`](./examples) مراجعه کنید.

---

## مجوز

این پروژه تحت مجوز [MIT](./LICENSE) منتشر شده است.

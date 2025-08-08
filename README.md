Go-RedisOrm
یک ORM (Object-Relational Mapper) ساده، سریع و امن برای زبان Go که برای کار با Redis طراحی شده است. این کتابخانه به شما اجازه می‌دهد تا با structهای Go به راحتی کار کنید و پیچیدگی‌های مربوط به سریالایز کردن، ایندکس‌گذاری، رمزنگاری و عملیات اتمی را مدیریت می‌کند.
✨ ویژگی‌ها
مدل‌سازی مبتنی بر Struct: مدل‌های خود را با استفاده از تگ‌های struct تعریف کنید.
پشتیبانی از انواع کلید اصلی: از string (با تولید خودکار UUID) و انواع عددی (int, int64, ...) به عنوان کلید اصلی استفاده کنید.
ایندکس‌گذاری قدرتمند: پشتیبانی از ایندکس‌های معمولی (index), یونیک (unique) و رمزنگاری‌شده (index_enc) برای جستجوی سریع.
رمزنگاری سمت کلاینت: فیلدهای حساس را با تگ secret:"true" مشخص کنید تا به صورت خودکار با الگوریتم AES-GCM رمزنگاری شوند.
عملیات اتمی: تمام عملیات نوشتن، آپدیت و حذف با استفاده از اسکریپت‌های Lua انجام می‌شود تا از سازگاری داده‌ها اطمینان حاصل شود.
قفل خوش‌بینانه (Optimistic Locking): با افزودن فیلد Version به مدل‌های خود، از تداخل در نوشتن‌های همزمان جلوگیری کنید.
عملیات گروهی (Bulk Operations): با استفاده از SaveAll، تعداد زیادی از اشیاء را به صورت بهینه و در یک درخواست ذخیره کنید.
قلاب‌های چرخه حیات (Lifecycle Hooks): با تگ‌های auto_create_time و auto_update_time، فیلدهای زمانی را به صورت خودکار مدیریت کنید.
شخصی‌سازی پیشرفته:
نام‌گذاری سفارشی: با اینترفیس ModelNamer نام مدل را در Redis تغییر دهید.
گروه‌بندی مدل‌ها: با اینترفیس ModelGrouper مدل‌ها را دسته‌بندی کنید.
حذف خودکار: با اینترفیس AutoDeleter یک TTL پیش‌فرض برای مدل‌ها تنظیم کنید.
🚀 نصب
go get [github.com/mrjvadi/Go-RedisOrm](https://github.com/mrjvadi/Go-RedisOrm)


📖 شروع سریع
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"[github.com/mrjvadi/Go-RedisOrm/redisorm](https://github.com/mrjvadi/Go-RedisOrm/redisorm)"
	"[github.com/redis/go-redis/v9](https://github.com/redis/go-redis/v9)"
)

// 1. مدل خود را تعریف کنید
type User struct {
	ID        string    `json:"id" redis:"pk" default:"uuid"`
	Version   int64     `json:"version" redis:"version"`
	Email     string    `json:"email" secret:"true" redis:",unique"`
	Country   string    `json:"country" redis:",index"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at" redis:",auto_create_time"`
	UpdatedAt time.Time `json:"updated_at" redis:",auto_update_time"`
}

// GroupName مدل User را در گروه 'accounts' قرار می‌دهد.
func (u *User) GroupName() string {
	return "accounts"
}

// AutoDeleteTTL یک زمان انقضای پیش‌فرض 1 ساعته برای رکوردهای User تعیین می‌کند.
func (u *User) AutoDeleteTTL() time.Duration {
    return 1 * time.Hour
}

func main() {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

	orm, err := redisorm.New(rdb,
		redisorm.WithNamespace("myapp"),
		redisorm.WithMasterKey([]byte("my-super-secret-32-byte-key-!!")),
	)
	if err != nil {
		log.Fatalf("failed to create orm: %v", err)
	}

	sess := orm.WithContext(ctx)

	// --- ذخیره و خواندن کاربر ---
	user := &User{Email: "user@example.com", Country: "IR", Status: "active"}
	id, err := sess.Save(user) // با TTL پیش‌فرض ذخیره می‌شود
	if err != nil {
		log.Fatalf("failed to save user: %v", err)
	}
	fmt.Printf("کاربر با شناسه %s در گروه 'accounts' ذخیره شد.\n", id)

	var loadedUser User
	if err := sess.Load(&loadedUser, id); err != nil {
		log.Fatalf("failed to load user: %v", err)
	}
	fmt.Printf("ایمیل کاربر خوانده شده: %s (کشور: %s)\n", loadedUser.Email, loadedUser.Country)

	// --- حذف کاربر ---
	if err := sess.Delete(&User{}, id); err != nil {
		log.Fatalf("failed to delete user: %v", err)
	}
	fmt.Println("کاربر با موفقیت حذف شد.")
}


راهنمای استفاده
تعریف مدل با تگ‌ها
تگ
توضیح
مثال
redis:"pk"
فیلد را به عنوان کلید اصلی مشخص می‌کند. از نوع string یا عددی پشتیبانی می‌کند.
ID string | redis:"pk"
default:"uuid"
اگر کلید اصلی از نوع string و خالی باشد، یک UUID برای آن تولید می‌کند.
ID string | redis:"pk" default:"uuid"
redis:"version"
قفل خوش‌بینانه را فعال می‌کند. فیلد باید از نوع int64 باشد.
Version int64 | redis:"version"
secret:"true"
مقدار فیلد را به صورت خودکار با AES-GCM رمزنگاری می‌کند.
Password string | secret:"true"
redis:",index"
برای جستجوی سریع، یک ایندکس معمولی روی فیلد ایجاد می‌کند.
Country string | redis:",index"
redis:",unique"
یک محدودیت منحصر به فرد روی فیلد ایجاد می‌کند.
Email string | redis:",unique"
redis:",index_enc"
یک ایندکس رمزنگاری‌شده (deterministic) برای جستجوی امن ایجاد می‌کند.
NationalID string | redis:",index_enc"
redis:",auto_create_time"
هنگام ساخت یک رکورد جدید، زمان فعلی را در فیلد (از نوع time.Time) قرار می‌دهد.
CreatedAt time.Time | redis:",auto_create_time"
redis:",auto_update_time"
هنگام ساخت یا به‌روزرسانی رکورد، زمان فعلی را در فیلد قرار می‌دهد.
UpdatedAt time.Time | redis:",auto_update_time"

شخصی‌سازی با اینترفیس‌ها
برای کنترل بیشتر روی رفتار مدل‌ها، می‌توانید اینترفیس‌های زیر را پیاده‌سازی کنید:
1. نام‌گذاری سفارشی (ModelNamer)
به صورت پیش‌فرض، نام struct به عنوان نام مدل در Redis استفاده می‌شود. برای تغییر این نام، اینترفیس ModelNamer را پیاده‌سازی کنید.
type AuditLog struct {
    // ...
}

// ModelName نام مدل را در Redis به "audit_events" تغییر می‌دهد.
func (a *AuditLog) ModelName() string {
    return "audit_events"
}


2. گروه‌بندی مدل‌ها (ModelGrouper)
برای سازماندهی بهتر کلیدها، می‌توانید مدل‌ها را گروه‌بندی کنید.
type SessionData struct {
    // ...
}

// GroupName این مدل را در گروه 'sessions' قرار می‌دهد.
func (s *SessionData) GroupName() string {
    return "sessions"
}


ساختار کلید در Redis به این شکل خواهد بود: {namespace}:val:sessions:SessionData:{id}
3. حذف خودکار (AutoDeleter)
می‌توانید یک زمان انقضا (TTL) پیش‌فرض برای رکوردهای یک مدل تعریف کنید.
type OTP struct {
    // ...
}

// AutoDeleteTTL باعث می‌شود رکوردهای این مدل پس از 5 دقیقه به صورت خودکار حذف شوند.
func (o *OTP) AutoDeleteTTL() time.Duration {
    return 5 * time.Minute
}


نکته: اگر هنگام فراخوانی تابع Save یک ttl به صورت مستقیم ارسال شود، آن مقدار بر این مقدار پیش‌فرض اولویت دارد.
عملیات گروهی (Bulk)
برای ذخیره تعداد زیادی شیء به صورت بهینه، از SaveAll استفاده کنید.
usersToSave := []*User{
    {Email: "bulk1@example.com", Country: "CA"},
    {Email: "bulk2@example.com", Country: "CA"},
}
ids, err := sess.SaveAll(usersToSave)
fmt.Printf("Successfully saved %d users. IDs: %v\n", len(ids), ids)


الگوی تراکنشی (Get-Lock-Do)
برای عملیات حساس که نیاز به اتمی بودن دارند (مانند کم کردن موجودی انبار)، از الگوی Transaction استفاده کنید. این الگو به صورت خودکار شیء را قفل می‌کند، می‌خواند، تابع شما را اجرا می‌کند، نتیجه را ذخیره می‌کند و قفل را آزاد می‌کند.
err := sess.Transaction(&Product{}, "product-sku-123").Execute(func(v any) error {
    p := v.(*Product) // تبدیل به نوع اصلی
    if p.Inventory < 1 {
        return errors.New("not enough inventory")
    }
    p.Inventory-- // اعمال تغییرات
    return nil // برای ذخیره تغییرات، nil را برگردانید
})


📜 مجوز
این پروژه تحت مجوز MIT منتشر شده است.

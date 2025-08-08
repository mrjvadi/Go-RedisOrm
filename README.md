Go-RedisOrm
یک ORM (Object-Relational Mapper) ساده، سریع و امن برای زبان Go که برای کار با Redis طراحی شده است. این کتابخانه به شما اجازه می‌دهد تا با structهای Go به راحتی کار کنید و پیچیدگی‌های مربوط به سریالایز کردن، ایندکس‌گذاری، رمزنگاری و عملیات اتمی را مدیریت می‌کند.

✨ ویژگی‌ها
مدل‌سازی مبتنی بر Struct: مدل‌های خود را با استفاده از تگ‌های struct تعریف کنید.

ایندکس‌گذاری قدرتمند: پشتیبانی از ایندکس‌های معمولی (index), یونیک (unique) و رمزنگاری‌شده (index_enc) برای جستجوی سریع.

رمزنگاری سمت کلاینت: فیلدهای حساس را با تگ secret:"true" مشخص کنید تا به صورت خودکار با الگوریتم AES-GCM رمزنگاری شوند.

عملیات اتمی: تمام عملیات نوشتن، آپدیت و حذف با استفاده از اسکریپت‌های Lua انجام می‌شود تا از سازگاری داده‌ها اطمینان حاصل شود.

قفل خوش‌بینانه (Optimistic Locking): با افزودن فیلد Version به مدل‌های خود، از تداخل در نوشتن‌های همزمان جلوگیری کنید.

عملیات گروهی (Bulk Operations): با استفاده از SaveAll، تعداد زیادی از اشیاء را به صورت بهینه و در یک درخواست ذخیره کنید.

الگوی تراکنشی: با استفاده از Transaction(...).Execute(...)، عملیات حساس "خواندن-تغییر-ذخیره" را در داخل یک قفل توزیع‌شده به صورت امن انجام دهید.

قلاب‌های چرخه حیات (Lifecycle Hooks): با تگ‌های auto_create_time و auto_update_time، فیلدهای زمانی را به صورت خودکار مدیریت کنید.

نام‌گذاری سفارشی مدل: با پیاده‌سازی اینترفیس ModelNamer، نام گروه کلیدها را در Redis به دلخواه خود تغییر دهید.

🚀 نصب
go get github.com/mrjvadi/Go-RedisOrm

quick-start-fa شروع سریع
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mrjvadi/Go-RedisOrm/redisorm"
	"github.com/redis/go-redis/v9"
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

	user := &User{Email: "user@example.com", Country: "IR", Status: "active"}
	id, err := sess.Save(user)
	if err != nil {
		log.Fatalf("failed to save user: %v", err)
	}
	fmt.Printf("کاربر با شناسه %s ذخیره شد.\n", id)

	var loadedUser User
	if err := sess.Load(&loadedUser, id); err != nil {
		log.Fatalf("failed to load user: %v", err)
	}
	fmt.Printf("ایمیل کاربر خوانده شده: %s (کشور: %s)\n", loadedUser.Email, loadedUser.Country)

	if err := sess.Delete(&User{}, id); err != nil {
		log.Fatalf("failed to delete user: %v", err)
	}
	fmt.Println("کاربر با موفقیت حذف شد.")
}

📖 راهنمای استفاده
تعریف مدل با تگ‌ها
تگ

توضیح

مثال

redis:"pk"

فیلد را به عنوان کلید اصلی (از نوع string) مشخص می‌کند.

ID string \redis:"pk"``

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

نام‌گذاری سفارشی مدل
به صورت پیش‌فرض، نام struct به عنوان نام مدل در Redis استفاده می‌شود. برای تغییر این نام، اینترفیس ModelNamer را پیاده‌سازی کنید.

type AuditLog struct {
    ID     string    `json:"id" redis:"pk"`
    Action string    `json:"action"`
}

// ModelName نام مدل را در Redis به "audit_events" تغییر می‌دهد.
func (a *AuditLog) ModelName() string {
    return "audit_events"
}

📜 مجوز
این پروژه تحت مجوز MIT منتشر شده است.
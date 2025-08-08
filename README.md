Go-RedisOrm
یک ORM (Object-Relational Mapper) ساده، سریع و امن برای زبان Go که برای کار با Redis طراحی شده است. این کتابخانه به شما اجازه می‌دهد تا با structهای Go به راحتی کار کنید و پیچیدگی‌های مربوط به سریالایز کردن، ایندکس‌گذاری و رمزنگاری را مدیریت می‌کند.

✨ ویژگی‌ها
مدل‌سازی مبتنی بر Struct: مدل‌های خود را با استفاده از تگ‌های struct تعریف کنید.

مدیریت خودکار فیلدها: تولید خودکار کلید اصلی (UUID) و به‌روزرسانی خودکار فیلدهای CreatedAt و UpdatedAt.

ایندکس‌گذاری قدرتمند: پشتیبانی از ایندکس‌های معمولی (index)، یونیک (unique) و رمزنگاری‌شده (index_enc) برای جستجوی سریع.

رمزنگاری سمت کلاینت: فیلدهای حساس را با تگ secret:"true" مشخص کنید تا به صورت خودکار با الگوریتم AES-GCM و با استفاده از کلید اصلی شما رمزنگاری شوند.

عملیات اتمی: تمام عملیات نوشتن، آپدیت و حذف با استفاده از اسکریپت‌های Lua انجام می‌شود تا از سازگاری داده‌ها اطمینان حاصل شود.

قفل خوش‌بینانه (Optimistic Locking): با افزودن فیلد Version به مدل‌های خود، از تداخل در نوشتن‌های همزمان جلوگیری کنید.

آپدیت‌های سریع و جزئی: با استفاده از UpdateFieldsFast، فیلدهای غیر ایندکس را با سرعت بسیار بالا و بدون نیاز به خواندن کل شیء، به‌روزرسانی کنید.

ذخیره‌سازی Payload: داده‌های حجیم یا ساختارهای JSON اضافی را به صورت جداگانه به یک شیء اصلی متصل کنید.

🚀 نصب
go get github.com/mrjvadi/Go-RedisOrm

quick-start-fa شروع سریع
در ادامه یک مثال کامل برای تعریف مدل، ذخیره، خواندن و آپدیت یک شیء آورده شده است.

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
	CreatedAt time.Time `json:"created_at" default:"now"`
	UpdatedAt time.Time `json:"updated_at"`
}

func main() {
	ctx := context.Background()

	// 2. به Redis متصل شوید
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	
	// 3. یک نمونه از ORM بسازید
	// یک کلید رمزنگاری امن (16، 24 یا 32 بایتی) ارائه دهید
	orm, err := redisorm.New(rdb, 
		redisorm.WithNamespace("myapp"), 
		redisorm.WithMasterKey([]byte("my-super-secret-32-byte-key-!!")),
	)
	if err != nil {
		log.Fatalf("failed to create orm: %v", err)
	}

	// 4. یک Session برای انجام عملیات ایجاد کنید
	sess := orm.WithContext(ctx)

	// 5. یک کاربر جدید ذخیره کنید
	user := &User{Email: "user@example.com", Country: "IR", Status: "active"}
	id, err := sess.Save(user)
	if err != nil {
		log.Fatalf("failed to save user: %v", err)
	}
	fmt.Printf("کاربر با شناسه %s ذخیره شد.\n", id)

	// 6. کاربر را بخوانید
	var loadedUser User
	if err := sess.Load(&loadedUser, id); err != nil {
		log.Fatalf("failed to load user: %v", err)
	}
	fmt.Printf("ایمیل کاربر خوانده شده: %s (کشور: %s)\n", loadedUser.Email, loadedUser.Country)

	// 7. یک فیلد را به سرعت آپدیت کنید (بدون تغییر ایندکس)
	updates := map[string]any{"status": "inactive"}
	if err := sess.UpdateFieldsFast(&User{}, id, updates); err != nil {
		log.Fatalf("failed to fast update user: %v", err)
	}
	fmt.Println("وضعیت کاربر با موفقیت به 'inactive' تغییر کرد.")

	// 8. کاربر را حذف کنید
	if err := sess.Delete(&User{}, id); err != nil {
		log.Fatalf("failed to delete user: %v", err)
	}
	fmt.Println("کاربر با موفقیت حذف شد.")
}

📖 راهنمای استفاده
تعریف مدل
از تگ‌های struct برای تعریف رفتار ORM استفاده کنید:

redis:"pk": این فیلد را به عنوان کلید اصلی (از نوع string) مشخص می‌کند.

redis:"version": این فیلد را برای قفل خوش‌بینانه (از نوع int64) فعال می‌کند.

redis:",index": یک ایندکس معمولی روی این فیلد ایجاد می‌کند.

redis:",index_enc": یک ایندکس رمزنگاری‌شده (قابل جستجو) روی این فیلد ایجاد می‌کند.

redis:",unique": یک محدودیت یکتا بودن روی این فیلد اعمال می‌کند.

secret:"true": مقدار این فیلد (از نوع string) را قبل از ذخیره‌سازی رمزنگاری می‌کند.

default:"...": یک مقدار پیش‌فرض برای فیلد در زمان ایجاد تعیین می‌کند (مثلاً uuid, now).

آپدیت سریع در مقابل آپدیت کامل
UpdateFields(user, id, updates): کندتر اما امن. ابتدا شیء را می‌خواند، تغییرات را اعمال می‌کند و سپس آن را دوباره ذخیره می‌کند. ایندکس‌ها را نیز به‌روزرسانی می‌کند.

UpdateFieldsFast(User{}, id, updates): بسیار سریع. مستقیماً JSON ذخیره شده در Redis را آپدیت می‌کند. ایندکس‌ها را آپدیت نمی‌کند. از این تابع فقط برای فیلدهایی استفاده کنید که ایندکس نشده‌اند.

جستجو با ایندکس
از توابع PageIDs... برای جستجو و صفحه‌بندی نتایج استفاده کنید.

// جستجو بر اساس فیلد ایندکس‌شده 'Country'
ids, nextCursor, err := sess.PageIDsByIndex(&User{}, "Country", "IR", 0, 50)

// جستجو بر اساس فیلد رمزنگاری‌شده 'Email'
ids, nextCursor, err := sess.PageIDsByEncIndex(&User{}, "Email", "user@example.com", 0, 50)

📜 مجوز
این پروژه تحت مجوز MIT منتشر شده است.
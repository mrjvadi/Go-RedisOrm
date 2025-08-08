package main

import (
	"context"
	"github.com/mrjvadi/Go-RedisOrm/redisorm"
	"github.com/redis/go-redis/v9"
	"time"
)

func main() {
	// init
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6380"})
	orm, _ := redisorm.New(rdb, redisorm.WithNamespace("app"), redisorm.WithMasterKey([]byte("0123456789abcdef0123456789abcdef")))

	type User struct {
		ID        string    `json:"id" redis:"pk" default:"uuid"`
		Version   int64     `json:"version" redis:"version"`
		Email     string    `json:"email" secret:"true" redis:",index_enc"`
		Country   string    `json:"country" redis:",index"`
		CreatedAt time.Time `json:"created_at" default:"now"`
		UpdatedAt time.Time `json:"updated_at"`
	}

	ctx := context.Background()
	sess := orm.WithContext(ctx)

	// Save
	id, _ := sess.Save(&User{Email: "a@b.com", Country: "DE"})

	// Load
	var u User
	_ = sess.Load(&u, id)

	// Edit (Optimistic if Version exists)
	_, _ = sess.Edit(&u, id, func() error { u.Country = "US"; return nil })

	// Encrypted index search (paged)
	ids, cursor, _ := sess.PageIDsByEncIndex(&User{}, "Email", "a@b.com", 0, 100)
	_ = cursor
	_ = ids
}

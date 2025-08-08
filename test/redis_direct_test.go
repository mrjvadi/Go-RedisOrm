package redisorm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// User struct used for direct Redis benchmarks
// Note: No ORM tags are needed here.
type BenchmarkUser struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Country   string    `json:"country"`
	CreatedAt time.Time `json:"created_at"`
}

// *** FIX: Removed the redeclaration of 'rdb'. It will use the one from redisorm_test.go ***
// var (
// 	rdb *redis.Client
// )

// setupDirect connects to Redis for benchmarking.
func setupDirect(b *testing.B) {
	if rdb != nil {
		return
	}
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // Change if your Redis is elsewhere
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		b.Fatalf("could not connect to redis: %v", err)
	}
}

// Benchmark for direct write (SET + SADD to simulate indexing).
func BenchmarkDirectRedis_Save(b *testing.B) {
	setupDirect(b)
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		userID := uuid.NewString()
		user := BenchmarkUser{
			ID:        userID,
			Email:     fmt.Sprintf("direct%d@example.com", i),
			Country:   "DE",
			CreatedAt: time.Now(),
		}

		// 1. Marshal the struct to JSON
		jsonData, err := json.Marshal(user)
		if err != nil {
			b.Fatalf("failed to marshal user: %v", err)
		}

		// 2. SET the main object data
		key := fmt.Sprintf("direct:user:%s", userID)
		if err := rdb.Set(ctx, key, jsonData, 0).Err(); err != nil {
			b.Fatalf("failed to SET user data: %v", err)
		}

		// 3. SADD to a set to simulate indexing
		indexKey := fmt.Sprintf("direct:index:country:%s", user.Country)
		if err := rdb.SAdd(ctx, indexKey, userID).Err(); err != nil {
			b.Fatalf("failed to SADD for index: %v", err)
		}
	}
}

// Benchmark for direct read (GET + Unmarshal).
func BenchmarkDirectRedis_Load(b *testing.B) {
	setupDirect(b)
	ctx := context.Background()

	// Prepare a sample user
	sampleUser := BenchmarkUser{ID: "load-test", Email: "direct-load@example.com", Country: "US"}
	jsonData, _ := json.Marshal(sampleUser)
	key := fmt.Sprintf("direct:user:%s", sampleUser.ID)
	rdb.Set(ctx, key, jsonData, 0)

	b.ResetTimer()

	var u BenchmarkUser
	for i := 0; i < b.N; i++ {
		// 1. GET the data from Redis
		val, err := rdb.Get(ctx, key).Result()
		if err != nil {
			b.Fatalf("failed to GET user: %v", err)
		}

		// 2. Unmarshal JSON into the struct
		if err := json.Unmarshal([]byte(val), &u); err != nil {
			b.Fatalf("failed to unmarshal user: %v", err)
		}
	}
}

// Benchmark for direct update (GET -> Modify -> SET).
func BenchmarkDirectRedis_Update(b *testing.B) {
	setupDirect(b)
	ctx := context.Background()

	// Prepare a sample user
	sampleUser := BenchmarkUser{ID: "update-test", Email: "direct-update@example.com", Country: "CA"}
	jsonData, _ := json.Marshal(sampleUser)
	key := fmt.Sprintf("direct:user:%s", sampleUser.ID)
	rdb.Set(ctx, key, jsonData, 0)

	b.ResetTimer()

	var u BenchmarkUser
	countries := []string{"FR", "UK", "IT", "JP"}
	for i := 0; i < b.N; i++ {
		// 1. GET
		val, err := rdb.Get(ctx, key).Result()
		if err != nil {
			b.Fatalf("failed to GET during update: %v", err)
		}

		// 2. Unmarshal
		if err := json.Unmarshal([]byte(val), &u); err != nil {
			b.Fatalf("failed to unmarshal during update: %v", err)
		}

		// 3. Modify
		u.Country = countries[i%len(countries)]

		// 4. Marshal again
		newJsonData, err := json.Marshal(u)
		if err != nil {
			b.Fatalf("failed to re-marshal during update: %v", err)
		}

		// 5. SET
		if err := rdb.Set(ctx, key, newJsonData, 0).Err(); err != nil {
			b.Fatalf("failed to SET during update: %v", err)
		}
	}
}
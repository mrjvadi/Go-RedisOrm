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
	Email     string    `json:"email" secret:"true" redis:",unique"` // Using unique for benchmark
	Country   string    `json:"country" redis:",index"`
	CreatedAt time.Time `json:"created_at" default:"now"`
	UpdatedAt time.Time `json:"updated_at"`
}

var (
	orm *redisorm.Client
	ctx = context.Background()
)

// setup an in-memory redis for testing
func setup(b *testing.B) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // Change if your Redis is elsewhere
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		b.Fatalf("could not connect to redis: %v", err)
	}

	// Using a unique namespace for each benchmark run to avoid collisions
	ns := fmt.Sprintf("benchmark_%d", time.Now().UnixNano())
	var err error
	orm, err = redisorm.New(rdb, redisorm.WithNamespace(ns), redisorm.WithMasterKey([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		b.Fatalf("failed to create orm client: %v", err)
	}
}

// Benchmark for saving new objects.
func BenchmarkSave(b *testing.B) {
	setup(b)
	b.ResetTimer() // Start timing after setup

	sess := orm.WithContext(ctx)

	// b.N is the number of iterations the benchmark will run
	for i := 0; i < b.N; i++ {
		user := &User{
			// Each email must be unique to avoid constraint violations
			Email:   fmt.Sprintf("user%d@example.com", i),
			Country: "DE",
		}
		_, err := sess.Save(user)
		if err != nil {
			b.Fatalf("failed to save user: %v", err)
		}
	}
}

// Benchmark for loading existing objects.
func BenchmarkLoad(b *testing.B) {
	setup(b)
	sess := orm.WithContext(ctx)

	// Create a sample user to load
	sampleUser := &User{Email: "load-test@example.com", Country: "US"}
	id, err := sess.Save(sampleUser)
	if err != nil {
		b.Fatalf("failed to save sample user for load benchmark: %v", err)
	}

	b.ResetTimer() // Start timing

	var u User
	for i := 0; i < b.N; i++ {
		err := sess.Load(&u, id)
		if err != nil {
			b.Fatalf("failed to load user: %v", err)
		}
	}
}

// Benchmark for updating a few fields of an object.
func BenchmarkUpdateFields(b *testing.B) {
	setup(b)
	sess := orm.WithContext(ctx)

	// Create a sample user to update
	sampleUser := &User{Email: "update-test@example.com", Country: "CA"}
	id, err := sess.Save(sampleUser)
	if err != nil {
		b.Fatalf("failed to save sample user for update benchmark: %v", err)
	}

	b.ResetTimer() // Start timing

	countries := []string{"FR", "UK", "IT", "JP"}
	for i := 0; i < b.N; i++ {
		updates := map[string]any{
			"country": countries[i%len(countries)], // Cycle through different countries
		}
		_, err := sess.UpdateFields(&User{}, id, updates)
		if err != nil {
			b.Fatalf("failed to update fields: %v", err)
		}
	}
}

// Benchmark for the full round trip: Save a new object and immediately load it.
func BenchmarkSaveAndLoad(b *testing.B) {
	setup(b)
	b.ResetTimer()
	sess := orm.WithContext(ctx)
	
	for i := 0; i < b.N; i++ {
		// Save
		userToSave := &User{
			Email: fmt.Sprintf("roundtrip%d@example.com", i),
			Country: "AU",
		}
		id, err := sess.Save(userToSave)
		if err != nil {
			b.Fatalf("Save failed during roundtrip benchmark: %v", err)
		}

		// Load
		var loadedUser User
		err = sess.Load(&loadedUser, id)
		if err != nil {
			b.Fatalf("Load failed during roundtrip benchmark: %v", err)
		}
	}
}
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mrjvadi/Go-RedisOrm/redisorm"
	"github.com/redis/go-redis/v9"
)

// User model demonstrates all major features.
type User struct {
	ID        string    `json:"id" redis:"pk" default:"uuid"`
	Version   int64     `json:"version" redis:"version"`
	Email     string    `json:"email" secret:"true" redis:",index_enc"`
	Country   string    `json:"country" redis:",index"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at" default:"now"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Profile model is used for the payload example.
type Profile struct {
	Bio      string   `json:"bio"`
	Website  string   `json:"website"`
	Interests []string `json:"interests"`
}

func main() {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}

	orm, err := redisorm.New(rdb,
		redisorm.WithNamespace("examples"),
		redisorm.WithMasterKey([]byte("a-very-secure-32-byte-secret-key")),
	)
	if err != nil {
		log.Fatalf("Failed to create ORM client: %v", err)
	}
	sess := orm.WithContext(ctx)

	fmt.Println("--- 1. Create Operation ---")
	user := &User{Email: "farhad@example.com", Country: "DE", Status: "pending"}
	id, err := sess.Save(user)
	if err != nil {
		log.Fatalf("Save failed: %v", err)
	}
	fmt.Printf("✅ User created with ID: %s\n\n", id)

	fmt.Println("--- 2. Read Operation ---")
	var loadedUser User
	if err := sess.Load(&loadedUser, id); err != nil {
		log.Fatalf("Load failed: %v", err)
	}
	fmt.Printf("✅ Loaded user: %+v\n\n", loadedUser)

	fmt.Println("--- 3. Slow Update (for indexed field) ---")
	updatesSlow := map[string]any{"country": "US"}
	if _, err := sess.UpdateFields(&User{}, id, updatesSlow); err != nil {
		log.Fatalf("UpdateFields failed: %v", err)
	}
	fmt.Println("✅ User country updated to US (indexes are now consistent).\n")

	fmt.Println("--- 4. Fast Update (for non-indexed field) ---")
	updatesFast := map[string]any{"status": "active"}
	if err := sess.UpdateFieldsFast(&User{}, id, updatesFast); err != nil {
		log.Fatalf("UpdateFieldsFast failed: %v", err)
	}
	fmt.Println("✅ User status updated to 'active' very quickly.\n")

	fmt.Println("--- 5. Search by Index ---")
	ids, _, err := sess.PageIDsByIndex(&User{}, "Country", "US", 0, 10)
	if err != nil {
		log.Fatalf("PageIDsByIndex failed: %v", err)
	}
	if len(ids) > 0 && ids[0] == id {
		fmt.Printf("✅ Found user %s in country 'US'.\n\n", ids[0])
	}

	fmt.Println("--- 6. Search by Encrypted Index ---")
	ids, _, err = sess.PageIDsByEncIndex(&User{}, "Email", "farhad@example.com", 0, 10)
	if err != nil {
		log.Fatalf("PageIDsByEncIndex failed: %v", err)
	}
	if len(ids) > 0 && ids[0] == id {
		fmt.Printf("✅ Found user %s by secret email 'farhad@example.com'.\n\n", ids[0])
	}

	fmt.Println("--- 7. Optimistic Locking ---")
	var userV1 User
	sess.Load(&userV1, id)
	fmt.Printf("Loaded user with version %d.\n", userV1.Version)
	userV1.Status = "away" // Make a change
	
	// Simulate another process updating the user in the background
	sess.UpdateFields(&User{}, id, map[string]any{"status": "busy"})
	fmt.Println("Another process updated the user, version is now higher.")

	// Now, try to save our stale userV1 object. This should fail.
	userV1.Version-- // Manually set to old version for demonstration
	if _, err := sess.Save(&userV1); err != nil {
		fmt.Printf("✅ As expected, failed to save with old version: %v\n\n", err)
	} else {
		log.Fatalf("Optimistic lock failed to prevent write!")
	}

	fmt.Println("--- 8. Payload Operations ---")
	profile := Profile{
		Bio:      "Software developer and tech enthusiast.",
		Website:  "https://example.com",
		Interests: []string{"Go", "Redis", "Distributed Systems"},
	}
	if err := sess.SavePayload(&User{}, id, profile, true); err != nil {
		log.Fatalf("SavePayload failed: %v", err)
	}
	fmt.Println("✅ User profile saved as an encrypted payload.")

	var loadedProfile Profile
	payloadBytes, err := sess.FindPayload(&User{}, id, true)
	if err != nil {
		log.Fatalf("FindPayload failed: %v", err)
	}
	json.Unmarshal(payloadBytes, &loadedProfile)
	fmt.Printf("✅ Loaded profile bio: %s\n\n", loadedProfile.Bio)

	fmt.Println("--- 9. Delete Operation ---")
	if err := sess.Delete(&User{}, id); err != nil {
		log.Fatalf("Delete failed: %v", err)
	}
	exists, _ := sess.Exists(&User{}, id)
	if !exists {
		fmt.Println("✅ User successfully deleted.")
	}
}

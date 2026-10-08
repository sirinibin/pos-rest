// Command seed prepares a throwaway database for the end-to-end suites
// (backend API e2e in e2e/api and the frontend Playwright suite in
// sirinibin/reactjs-pos). It upserts one admin user whose credentials come
// from E2E_EMAIL / E2E_PASSWORD, so the suites never depend on a real
// account's password.
//
// It refuses to run against any database whose name does not contain "e2e"
// or "test", so it cannot touch production data by accident.
package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jameskeane/bcrypt"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	defaultEmail    = "e2e-admin@startpos.test"
	defaultPassword = "E2e-Passw0rd!"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// safeDBName reports whether name is clearly a disposable test database.
func safeDBName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "e2e") || strings.Contains(n, "test")
}

func main() {
	dbName := os.Getenv("MONGO_DB")
	if !safeDBName(dbName) {
		log.Fatalf("refusing to seed database %q: set MONGO_DB to a name containing \"e2e\" or \"test\"", dbName)
	}

	email := strings.ToLower(getenv("E2E_EMAIL", defaultEmail))
	password := getenv("E2E_PASSWORD", defaultPassword)

	salt, err := bcrypt.Salt(10)
	if err != nil {
		log.Fatalf("bcrypt salt: %v", err)
	}
	hash, err := bcrypt.Hash(password, salt)
	if err != nil {
		log.Fatalf("bcrypt hash: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now()
	users := db.Client("").Database(dbName).Collection("user")
	_, err = users.UpdateOne(ctx,
		bson.M{"email": email},
		bson.M{
			"$set": bson.M{
				"name":       "E2E Admin",
				"email":      email,
				"mob":        "0500000000",
				"password":   hash,
				"role":       "Admin",
				"admin":      true,
				"deleted":    false,
				"updated_at": now,
			},
			"$setOnInsert": bson.M{"created_at": now},
		},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		log.Fatalf("upsert e2e user: %v", err)
	}
	log.Printf("[e2e-seed] admin user ready in %s: %s", dbName, email)
}

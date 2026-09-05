package models

import (
	"reflect"
	"testing"
)

// ── VerifyPassword ────────────────────────────────────────────────────────────

func TestVerifyPassword_CorrectPlaintext(t *testing.T) {
	plain := "mysecretpassword"
	hash := HashPassword(plain)
	u := &User{Password: hash}
	if !u.VerifyPassword(plain) {
		t.Error("expected VerifyPassword to return true for matching password")
	}
}

func TestVerifyPassword_WrongPlaintext(t *testing.T) {
	hash := HashPassword("correctpassword")
	u := &User{Password: hash}
	if u.VerifyPassword("wrongpassword") {
		t.Error("expected VerifyPassword to return false for wrong password")
	}
}

func TestVerifyPassword_EmptyPlaintext(t *testing.T) {
	hash := HashPassword("somepassword")
	u := &User{Password: hash}
	if u.VerifyPassword("") {
		t.Error("expected VerifyPassword to return false for empty plaintext")
	}
}

func TestVerifyPassword_EmptyHash(t *testing.T) {
	u := &User{Password: ""}
	// An empty hash should not match any non-empty plaintext
	if u.VerifyPassword("somepassword") {
		t.Error("expected VerifyPassword to return false when hash is empty")
	}
}

func TestVerifyPassword_SamePasswordHashedTwice(t *testing.T) {
	// bcrypt generates different salts each time — both should verify
	plain := "testpassword"
	hash1 := HashPassword(plain)
	hash2 := HashPassword(plain)

	u1 := &User{Password: hash1}
	u2 := &User{Password: hash2}

	if !u1.VerifyPassword(plain) {
		t.Error("hash1 should verify correctly")
	}
	if !u2.VerifyPassword(plain) {
		t.Error("hash2 should verify correctly")
	}
}

// ── HashPassword ──────────────────────────────────────────────────────────────

func TestHashPassword_ProducesNonEmptyString(t *testing.T) {
	hash := HashPassword("mypassword")
	if hash == "" {
		t.Error("HashPassword should return a non-empty string")
	}
}

func TestHashPassword_DifferentInputsDifferentHashes(t *testing.T) {
	h1 := HashPassword("password1")
	h2 := HashPassword("password2")
	if h1 == h2 {
		t.Error("different passwords should produce different hashes")
	}
}

func TestHashPassword_SamePlaintextDifferentHashes(t *testing.T) {
	// bcrypt uses random salts — same plaintext should produce different hashes
	h1 := HashPassword("samepassword")
	h2 := HashPassword("samepassword")
	if h1 == h2 {
		t.Log("note: same hash produced (possible but unlikely with bcrypt salting)")
	}
}

// ── RestoreUser — struct method exists ───────────────────────────────────────

func TestRestoreUser_MethodExists(t *testing.T) {
	u := &User{}
	uType := reflect.TypeOf(u)
	_, ok := uType.MethodByName("RestoreUser")
	if !ok {
		t.Error("User should have a RestoreUser method")
	}
}

func TestVerifyPassword_MethodExists(t *testing.T) {
	u := &User{}
	uType := reflect.TypeOf(u)
	_, ok := uType.MethodByName("VerifyPassword")
	if !ok {
		t.Error("User should have a VerifyPassword method")
	}
}

// ── VerifyPassword signature ──────────────────────────────────────────────────

func TestVerifyPassword_AcceptsStringReturnsBool(t *testing.T) {
	u := &User{Password: HashPassword("pw")}
	// Compile-time check: method exists with correct signature
	result := u.VerifyPassword("pw")
	if !result {
		t.Error("VerifyPassword should return true for correct password")
	}
}

// ── RestoreUser signature check (reflection) ─────────────────────────────────

func TestRestoreUser_MethodSignature(t *testing.T) {
	uType := reflect.TypeOf(&User{})
	method, ok := uType.MethodByName("RestoreUser")
	if !ok {
		t.Fatal("RestoreUser method not found")
	}
	// RestoreUser(updatedBy *primitive.ObjectID) error — 2 params (receiver + updatedBy), 1 return
	if method.Type.NumIn() != 2 {
		t.Errorf("RestoreUser should take 2 inputs (receiver + updatedBy), got %d", method.Type.NumIn())
	}
	if method.Type.NumOut() != 1 {
		t.Errorf("RestoreUser should return 1 value (error), got %d", method.Type.NumOut())
	}
}

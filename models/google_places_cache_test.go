package models

import (
	"testing"
	"time"
)

func TestGooglePlacesCacheTTL(t *testing.T) {
	if googlePlacesCacheTTL != 30*24*time.Hour {
		t.Errorf("expected 30-day TTL, got %v", googlePlacesCacheTTL)
	}
}

func TestCachedSupplierFields(t *testing.T) {
	s := CachedSupplier{
		Name:    "Al Salam Steel",
		Phone:   "966501234567",
		Address: "Dammam Industrial Area",
		Lat:     26.4207,
		Lng:     50.0888,
		Rating:  4.5,
		PlaceID: "ChIJ_abc123",
		Website: "https://alsalamsteel.com",
	}
	if s.Name == "" {
		t.Error("Name should not be empty")
	}
	if s.Phone == "" {
		t.Error("Phone should not be empty")
	}
	if s.PlaceID == "" {
		t.Error("PlaceID should not be empty")
	}
}

func TestGooglePlacesCacheEntryExpiry(t *testing.T) {
	now := time.Now()
	entry := GooglePlacesCacheEntry{
		Category:  "Steel Pipes",
		Market:    "Dammam",
		Suppliers: []CachedSupplier{{Name: "Test", Phone: "123"}},
		CachedAt:  now,
		ExpiresAt: now.Add(googlePlacesCacheTTL),
	}

	if !entry.ExpiresAt.After(now) {
		t.Error("ExpiresAt should be in the future")
	}
	if entry.ExpiresAt.Sub(now) < 29*24*time.Hour {
		t.Error("TTL should be at least 29 days")
	}
}

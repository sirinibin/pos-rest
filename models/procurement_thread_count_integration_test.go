//go:build integration

package models

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func seedWhatsAppMessages(t *testing.T, storeID primitive.ObjectID, phones []string, unread int) {
	t.Helper()
	now := time.Now().UTC()
	for i, p := range phones {
		for j := 0; j < 2; j++ {
			d := now.Add(time.Duration(-i*10-j) * time.Minute)
			msg := &ProcurementMessage{
				StoreID: storeID, Type: "whatsapp", Direction: "in", Provider: "test",
				From: p, To: []string{"966500000000"}, BodyText: fmt.Sprintf("msg %d-%d", i, j),
				Read: !(i < unread), MessageDate: &d,
			}
			if err := SaveProcurementMessage(msg); err != nil {
				t.Fatalf("SaveProcurementMessage: %v", err)
			}
		}
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		procurementMessageCol().DeleteMany(ctx, bson.M{"store_id": storeID})
	})
}

func TestListContactThreads_CountAndPaging(t *testing.T) {
	storeID := primitive.NewObjectID()
	phones := []string{"966511111111", "966522222222", "966533333333"}
	seedWhatsAppMessages(t, storeID, phones, 2)

	threads, total, err := ListContactThreads(storeID, "whatsapp", "", 1, 2, nil, nil, nil)
	if err != nil {
		t.Fatalf("ListContactThreads: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(threads) != 2 {
		t.Errorf("page 1 len = %d, want 2", len(threads))
	}

	threads, total, err = ListContactThreads(storeID, "whatsapp", "", 2, 2, nil, nil, nil)
	if err != nil || total != 3 || len(threads) != 1 {
		t.Errorf("page 2: len=%d total=%d err=%v, want 1/3/nil", len(threads), total, err)
	}
}

func TestListContactThreads_EmptyStoreReturnsZero(t *testing.T) {
	threads, total, err := ListContactThreads(primitive.NewObjectID(), "whatsapp", "", 1, 1000, nil, nil, nil)
	if err != nil {
		t.Fatalf("ListContactThreads: %v", err)
	}
	if total != 0 || len(threads) != 0 {
		t.Errorf("got len=%d total=%d, want 0/0", len(threads), total)
	}
}

// An invalid regex in search makes Aggregate fail server-side; this used to
// panic on the nil count cursor and must now return an error.
func TestListContactThreads_InvalidSearchRegexReturnsError(t *testing.T) {
	storeID := primitive.NewObjectID()
	seedWhatsAppMessages(t, storeID, []string{"966511111111"}, 1)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ListContactThreads panicked: %v", r)
		}
	}()
	_, _, err := ListContactThreads(storeID, "whatsapp", "(", 1, 10, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid regex search, got nil")
	}
}

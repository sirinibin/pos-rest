//go:build integration

package models

import (
	"context"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// MarkThreadMessagesRead reports how many messages it changed, so the thread
// handler pushes an unread update only when something was actually unread.
func TestMarkThreadMessagesRead_ReturnsChangedCount(t *testing.T) {
	storeID := primitive.NewObjectID()
	seedWhatsAppMessages(t, storeID, []string{"966541111111", "966542222222"}, 1) // first phone: 2 unread

	n, err := MarkThreadMessagesRead(storeID, "966541111111", "whatsapp")
	if err != nil || n != 2 {
		t.Fatalf("first read: n=%d err=%v, want 2/nil", n, err)
	}
	if n, err = MarkThreadMessagesRead(storeID, "966541111111", "whatsapp"); err != nil || n != 0 {
		t.Fatalf("second read: n=%d err=%v, want 0/nil", n, err)
	}
	if n, err = MarkThreadMessagesRead(storeID, "966542222222", "whatsapp"); err != nil || n != 0 {
		t.Fatalf("already-read thread: n=%d err=%v, want 0/nil", n, err)
	}
	if n, err = MarkThreadMessagesRead(storeID, "966541111111", "email"); err != nil || n != 0 {
		t.Fatalf("other type: n=%d err=%v, want 0/nil", n, err)
	}
}

// FindStoreUnreadFeeds reads the settings that gate the unread endpoints.
func TestFindStoreUnreadFeeds(t *testing.T) {
	col := storeCollectionForUnreadTest()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cases := []struct {
		name     string
		settings map[string]interface{}
		wa, em   bool
	}{
		{"module off", map[string]interface{}{"enable_rfq_module": false, "bot_waba_phone_number_id": "1", "rfq_email_connected": true}, false, false},
		{"module on, nothing connected", map[string]interface{}{"enable_rfq_module": true}, false, false},
		{"module on, both", map[string]interface{}{"enable_rfq_module": true, "store_rfq_waba_phone_number_id": "1", "rfq_email_accounts": []map[string]interface{}{{"_id": primitive.NewObjectID(), "email": "a@b.c"}}}, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id := primitive.NewObjectID()
			if _, err := col.InsertOne(ctx, map[string]interface{}{"_id": id, "name": "unread-feeds-" + id.Hex(), "settings": c.settings}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { col.DeleteOne(context.Background(), map[string]interface{}{"_id": id}) })
			wa, em, err := FindStoreUnreadFeeds(id)
			if err != nil || wa != c.wa || em != c.em {
				t.Fatalf("got (%v, %v, %v), want (%v, %v, nil)", wa, em, err, c.wa, c.em)
			}
		})
	}
	if _, _, err := FindStoreUnreadFeeds(primitive.NewObjectID()); err == nil {
		t.Fatal("unknown store: want an error")
	}
}

func storeCollectionForUnreadTest() *mongo.Collection {
	return db.Client("").Database(db.GetPosDB()).Collection("store")
}

package models

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// aggregateCount must return the Aggregate error instead of panicking on a nil
// cursor (regression: production panic in GetRFQWhatsAppUnreadHandler).
func TestAggregateCount_AggregateErrorReturnedNotPanic(t *testing.T) {
	// mongo.Connect is lazy, so no server is needed; a cancelled context makes
	// Aggregate fail before any network I/O.
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://127.0.0.1:1"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Disconnect(context.Background())
	col := client.Database("unit_test").Collection("procurement_messages")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("aggregateCount panicked: %v", r)
		}
	}()
	total, err := aggregateCount(ctx, col, mongo.Pipeline{bson.D{{Key: "$count", Value: "total"}}})
	if err == nil {
		t.Fatal("expected an error from Aggregate with a cancelled context, got nil")
	}
	if total != 0 {
		t.Fatalf("expected total 0 on error, got %d", total)
	}
}

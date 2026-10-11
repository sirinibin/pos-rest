package models

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/x/mongo/driver/topology"
)

func stubZatcaChainLookup(t *testing.T, results ...error) *int {
	t.Helper()
	calls := 0
	origOnce, origDelay := nextZatcaChainLinkOnce, zatcaChainRetryDelay
	nextZatcaChainLinkOnce = func(string) (ZatcaChainLink, error) {
		err := results[calls]
		calls++
		if err != nil {
			return ZatcaChainLink{}, err
		}
		return ZatcaChainLink{ICV: 42, PIH: "H-last"}, nil
	}
	zatcaChainRetryDelay = time.Millisecond
	t.Cleanup(func() { nextZatcaChainLinkOnce, zatcaChainRetryDelay = origOnce, origDelay })
	return &calls
}

// the error the live server gave: a read cut off by the query's deadline
func liveTimeoutError() error {
	return topology.ConnectionError{
		ConnectionID: "localhost:27017[-195]",
		Wrapped:      fmt.Errorf("read tcp 127.0.0.1:41040->127.0.0.1:27017: %w", context.DeadlineExceeded),
	}
}

func TestIsTransientDBError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadline exceeded on read", liveTimeoutError(), true},
		{"plain deadline", context.DeadlineExceeded, true},
		{"wrapped deadline", fmt.Errorf("finding: %w", context.DeadlineExceeded), true},
		{"no documents", mongo.ErrNoDocuments, false},
		{"bad query", errors.New("unknown operator: $foo"), false},
	}
	for _, c := range cases {
		if got := isTransientDBError(c.err); got != c.want {
			t.Errorf("%s: isTransientDBError = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNextZatcaChainLink_RetriesATimedOutLookup(t *testing.T) {
	calls := stubZatcaChainLookup(t, liveTimeoutError(), liveTimeoutError(), nil)
	link, err := NextZatcaChainLink("store")
	if err != nil {
		t.Fatalf("third attempt succeeded, got error %v", err)
	}
	if *calls != 3 || link.ICV != 42 || link.PIH != "H-last" {
		t.Fatalf("calls %d link %+v, want 3 calls and the stored chain link", *calls, link)
	}
}

func TestNextZatcaChainLink_GivesUpAfterTheLastAttempt(t *testing.T) {
	calls := stubZatcaChainLookup(t, liveTimeoutError(), liveTimeoutError(), liveTimeoutError(), nil)
	_, err := NextZatcaChainLink("store")
	if err == nil || !isTransientDBError(err) {
		t.Fatalf("want the timeout error after %d attempts, got %v", zatcaChainAttempts, err)
	}
	if *calls != zatcaChainAttempts {
		t.Fatalf("calls = %d, want %d", *calls, zatcaChainAttempts)
	}
}

func TestNextZatcaChainLink_DoesNotRetryOtherErrors(t *testing.T) {
	calls := stubZatcaChainLookup(t, errors.New("unknown operator: $foo"), nil)
	if _, err := NextZatcaChainLink("store"); err == nil {
		t.Fatal("want the query error")
	}
	if *calls != 1 {
		t.Fatalf("calls = %d, want 1 (a query error is not retried)", *calls)
	}
}

func TestZatcaChainIndexes_InPerfIndexesForEveryChainCollection(t *testing.T) {
	for _, coll := range zatcaChainCollections {
		for _, keys := range zatcaChainIndexes {
			found := false
			for _, ix := range perfStoreIndexes {
				if ix.Coll == coll && fmt.Sprint(ix.Keys) == fmt.Sprint(keys) {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: chain index %v not in perfStoreIndexes", coll, keys)
			}
		}
	}
}

package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func stubUnreadFeeds(t *testing.T, whatsapp, email bool, err error) *int {
	t.Helper()
	calls := 0
	orig := findStoreUnreadFeeds
	findStoreUnreadFeeds = func(primitive.ObjectID) (bool, bool, error) {
		calls++
		return whatsapp, email, err
	}
	t.Cleanup(func() { findStoreUnreadFeeds = orig })
	return &calls
}

var unreadHandlers = []struct {
	name    string
	path    string
	handler http.HandlerFunc
	feed    string
}{
	{"whatsapp", "/v1/rfq-whatsapp-unread", GetRFQWhatsAppUnreadHandler, "whatsapp"},
	{"email", "/v1/email-unread", GetEmailUnreadHandler, "email"},
}

func callUnread(h http.HandlerFunc, url string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, url, nil))
	return rr
}

func TestUnreadHandlers_InvalidStoreID(t *testing.T) {
	for _, h := range unreadHandlers {
		t.Run(h.name, func(t *testing.T) {
			calls := stubUnreadFeeds(t, true, true, nil)
			for _, q := range []string{"", "?store_id=", "?store_id=nope", "?store_id=123"} {
				rr := callUnread(h.handler, h.path+q)
				if rr.Code != http.StatusBadRequest {
					t.Errorf("%s%s: status %d, want 400", h.path, q, rr.Code)
				}
			}
			if *calls != 0 {
				t.Errorf("store looked up %d times for an invalid id", *calls)
			}
		})
	}
}

// With the RFQ module off or the channel not connected the handlers answer
// from the store settings alone: an empty feed, "enabled": false, status 200.
func TestUnreadHandlers_DisabledFeedAnswersEmpty(t *testing.T) {
	id := primitive.NewObjectID().Hex()
	for _, h := range unreadHandlers {
		for _, flags := range [][2]bool{{false, false}, {true, false}, {false, true}} {
			wa, em := flags[0], flags[1]
			enabled := wa
			if h.feed == "email" {
				enabled = em
			}
			if enabled {
				continue // the enabled path reads procurement messages (integration tests)
			}
			t.Run(h.name, func(t *testing.T) {
				stubUnreadFeeds(t, wa, em, nil)
				rr := callUnread(h.handler, h.path+"?store_id="+id)
				if rr.Code != http.StatusOK {
					t.Fatalf("status %d, want 200 (body %s)", rr.Code, rr.Body.String())
				}
				var body struct {
					Items       []interface{} `json:"items"`
					TotalUnread int           `json:"total_unread"`
					Enabled     *bool         `json:"enabled"`
				}
				if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
					t.Fatalf("bad JSON %q: %v", rr.Body.String(), err)
				}
				if body.Items == nil || len(body.Items) != 0 {
					t.Errorf("items = %v, want []", body.Items)
				}
				if body.TotalUnread != 0 {
					t.Errorf("total_unread = %d, want 0", body.TotalUnread)
				}
				if body.Enabled == nil || *body.Enabled {
					t.Errorf("enabled = %v, want false", body.Enabled)
				}
			})
		}
	}
}

func TestUnreadHandlers_UnknownStore404(t *testing.T) {
	for _, h := range unreadHandlers {
		t.Run(h.name, func(t *testing.T) {
			stubUnreadFeeds(t, false, false, mongo.ErrNoDocuments)
			rr := callUnread(h.handler, h.path+"?store_id="+primitive.NewObjectID().Hex())
			if rr.Code != http.StatusNotFound {
				t.Fatalf("status %d, want 404", rr.Code)
			}
		})
	}
}

func TestUnreadHandlers_StoreReadError500(t *testing.T) {
	for _, h := range unreadHandlers {
		t.Run(h.name, func(t *testing.T) {
			stubUnreadFeeds(t, false, false, errors.New("db down"))
			rr := callUnread(h.handler, h.path+"?store_id="+primitive.NewObjectID().Hex())
			if rr.Code != http.StatusInternalServerError {
				t.Fatalf("status %d, want 500", rr.Code)
			}
		})
	}
}

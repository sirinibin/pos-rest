package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestUserMayUseStore(t *testing.T) {
	a, b := primitive.NewObjectID(), primitive.NewObjectID()
	cases := []struct {
		name   string
		role   string
		stores []*primitive.ObjectID
		id     primitive.ObjectID
		want   bool
	}{
		{"admin reaches any store", "Admin", []*primitive.ObjectID{&a}, b, true},
		{"listed store", "Manager", []*primitive.ObjectID{&a, &b}, b, true},
		{"unlisted store", "Manager", []*primitive.ObjectID{&a}, b, false},
		{"no store list means every store", "User", nil, b, true},
		{"nil entries are ignored", "User", []*primitive.ObjectID{nil, &a}, b, false},
		{"role is case sensitive", "admin", []*primitive.ObjectID{&a}, b, false},
	}
	for _, c := range cases {
		if got := userMayUseStore(c.role, c.stores, c.id); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRequestedStoreIDs(t *testing.T) {
	a, b, c := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	var got []primitive.ObjectID
	var body string
	router := mux.NewRouter()
	h := func(w http.ResponseWriter, r *http.Request) {
		got = requestedStoreIDs(r)
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
	}
	router.HandleFunc("/v1/store/{id}", h)
	router.HandleFunc("/v1/order", h)
	router.HandleFunc("/v1/order/{id}", h)

	serve := func(method, target, ct, payload string) {
		got, body = nil, ""
		req := httptest.NewRequest(method, target, strings.NewReader(payload))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		router.ServeHTTP(httptest.NewRecorder(), req)
	}
	want := func(name string, ids ...primitive.ObjectID) {
		t.Helper()
		if len(got) != len(ids) {
			t.Fatalf("%s: got %v, want %v", name, got, ids)
		}
		for i := range ids {
			if got[i] != ids[i] {
				t.Fatalf("%s: got %v, want %v", name, got, ids)
			}
		}
	}

	serve("GET", "/v1/order?search[store_id]="+a.Hex(), "", "")
	want("query", a)
	serve("GET", "/v1/order?store_id="+a.Hex(), "", "")
	want("plain store_id query", a)
	serve("GET", "/v1/store/"+b.Hex(), "", "")
	want("store path", b)
	serve("GET", "/v1/order/"+b.Hex(), "", "")
	want("other paths are not store ids")

	payload := `{"store_id":"` + c.Hex() + `","name":"x"}`
	serve("POST", "/v1/order?search[store_id]="+a.Hex(), "application/json", payload)
	want("query and body", a, c)
	if body != payload {
		t.Fatalf("the handler must still read the whole body, got %q", body)
	}
	serve("POST", "/v1/order", "", payload)
	want("body without a content type", c)
	serve("POST", "/v1/order", "multipart/form-data; boundary=x", payload)
	want("multipart bodies are not parsed")
	serve("POST", "/v1/order", "application/json", `{"store_id":5}`)
	want("non-string store_id")
	serve("POST", "/v1/order", "application/json", `[1,2]`)
	want("non-object body")
	serve("GET", "/v1/order?search[store_id]=000000000000000000000000&store_id=junk", "", "")
	want("zero and malformed ids are left to the handler")
}

func TestStoreAccessMiddleware_PassesRequestsWithoutAValidToken(t *testing.T) {
	called := false
	h := StoreAccessMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	req := httptest.NewRequest("GET", "/v1/order?search[store_id]="+primitive.NewObjectID().Hex(), nil)
	req.Header.Set("Authorization", "not-a-jwt")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatalf("a request without a valid token must reach the handler (which answers 401), got HTTP %d", rec.Code)
	}
}

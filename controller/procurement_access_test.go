package controller

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func okHandler(called *bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*called = true
		w.WriteHeader(http.StatusTeapot)
	}
}

// Every procurement message route answers 401 without a valid access token
// and never reaches its handler.
func TestRequireProcurementAccess_Unauthenticated(t *testing.T) {
	for _, auth := range []string{"", "garbage", "Bearer garbage", "Bearer "} {
		called := false
		h := RequireProcurementAccess(okHandler(&called))
		req := httptest.NewRequest(http.MethodGet, "/v1/email-unread?store_id="+primitive.NewObjectID().Hex(), nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rr := httptest.NewRecorder()
		h(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Authorization %q: status %d, want 401", auth, rr.Code)
		}
		if called {
			t.Errorf("Authorization %q: handler ran without a sign-in", auth)
		}
	}
}

func TestProcurementMessageIDFromRoute(t *testing.T) {
	id := primitive.NewObjectID()
	cases := []struct {
		tpl, path string
		want      bool
	}{
		{"/v1/procurement-messages/{id}", "/v1/procurement-messages/" + id.Hex(), true},
		{"/v1/procurement-messages/{id}/extract", "/v1/procurement-messages/" + id.Hex() + "/extract", true},
		{"/v1/procurement-messages/{id}", "/v1/procurement-messages/not-an-id", false},
		{"/v1/procurement-message-threads/{phone}", "/v1/procurement-message-threads/9665", false},
		{"/v1/procurement-messages/thread", "/v1/procurement-messages/thread", false},
	}
	for _, c := range cases {
		var got primitive.ObjectID
		var ok bool
		r := mux.NewRouter()
		r.HandleFunc(c.tpl, func(w http.ResponseWriter, req *http.Request) { got, ok = procurementMessageIDFromRoute(req) })
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, c.path, nil))
		if ok != c.want || (ok && got != id) {
			t.Errorf("%s: got (%v, %v), want ok=%v", c.path, got, ok, c.want)
		}
	}
	if _, ok := procurementMessageIDFromRoute(httptest.NewRequest(http.MethodGet, "/x", nil)); ok {
		t.Error("no route: want ok=false")
	}
}

// storeAccessCase runs a request through the wrapper with the sign-in and the
// message lookup stubbed, as user with the given role and stores.
func runProcurementAccess(t *testing.T, role string, stores []primitive.ObjectID, msgStore primitive.ObjectID, msgErr error, req *http.Request, tpl string) (int, bool) {
	t.Helper()
	origUser, origMsg := userFromClaims, procurementMessageStoreID
	t.Cleanup(func() { userFromClaims, procurementMessageStoreID = origUser, origMsg })
	var ptrs []*primitive.ObjectID
	for i := range stores {
		ptrs = append(ptrs, &stores[i])
	}
	userFromClaims = func(models.TokenClaims) (*models.User, error) {
		return &models.User{Role: role, StoreIDs: ptrs}, nil
	}
	procurementMessageStoreID = func(primitive.ObjectID) (primitive.ObjectID, error) { return msgStore, msgErr }
	authenticateProcurementRequest = func(*http.Request) (models.TokenClaims, error) {
		return models.TokenClaims{Type: "access_token"}, nil
	}
	t.Cleanup(func() { authenticateProcurementRequest = models.AuthenticateByAccessToken })

	called := false
	r := mux.NewRouter()
	r.HandleFunc(tpl, RequireProcurementAccess(okHandler(&called)))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr.Code, called
}

func TestRequireProcurementAccess_StoreRules(t *testing.T) {
	mine, other := primitive.NewObjectID(), primitive.NewObjectID()
	msgID := primitive.NewObjectID().Hex()
	get := func(path string) *http.Request { return httptest.NewRequest(http.MethodGet, path, nil) }
	multipartReq := func(path, storeID string) *http.Request {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		mw.WriteField("store_id", storeID)
		mw.Close()
		req := httptest.NewRequest(http.MethodPost, path, &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		return req
	}

	cases := []struct {
		name     string
		role     string
		stores   []primitive.ObjectID
		msgStore primitive.ObjectID
		msgErr   error
		req      *http.Request
		tpl      string
		want     int
	}{
		{"own store in query", "", []primitive.ObjectID{mine}, mine, nil, get("/v1/email-unread?store_id=" + mine.Hex()), "/v1/email-unread", http.StatusTeapot},
		{"other store in query", "", []primitive.ObjectID{mine}, mine, nil, get("/v1/email-unread?store_id=" + other.Hex()), "/v1/email-unread", http.StatusForbidden},
		{"admin, other store", "Admin", []primitive.ObjectID{mine}, mine, nil, get("/v1/email-unread?store_id=" + other.Hex()), "/v1/email-unread", http.StatusTeapot},
		{"no assigned stores means every store", "", nil, mine, nil, get("/v1/email-unread?store_id=" + other.Hex()), "/v1/email-unread", http.StatusTeapot},
		{"malformed store id left to the handler", "", []primitive.ObjectID{mine}, mine, nil, get("/v1/email-unread?store_id=nope"), "/v1/email-unread", http.StatusTeapot},
		{"message of own store", "", []primitive.ObjectID{mine}, mine, nil, get("/v1/procurement-messages/" + msgID), "/v1/procurement-messages/{id}", http.StatusTeapot},
		{"message of other store", "", []primitive.ObjectID{mine}, other, nil, get("/v1/procurement-messages/" + msgID), "/v1/procurement-messages/{id}", http.StatusForbidden},
		{"message of other store, own store in query", "", []primitive.ObjectID{mine}, other, nil, get("/v1/procurement-messages/" + msgID + "?store_id=" + mine.Hex()), "/v1/procurement-messages/{id}", http.StatusForbidden},
		{"unknown message left to the handler", "", []primitive.ObjectID{mine}, primitive.NilObjectID, mongo.ErrNoDocuments, get("/v1/procurement-messages/" + msgID), "/v1/procurement-messages/{id}", http.StatusTeapot},
		{"message lookup fails", "", []primitive.ObjectID{mine}, primitive.NilObjectID, mongo.ErrClientDisconnected, get("/v1/procurement-messages/" + msgID), "/v1/procurement-messages/{id}", http.StatusInternalServerError},
		{"multipart form, own store", "", []primitive.ObjectID{mine}, mine, nil, multipartReq("/v1/procurement-message-threads/9665/send-media", mine.Hex()), "/v1/procurement-message-threads/{phone}/send-media", http.StatusTeapot},
		{"multipart form, other store", "", []primitive.ObjectID{mine}, mine, nil, multipartReq("/v1/procurement-message-threads/9665/send-media", other.Hex()), "/v1/procurement-message-threads/{phone}/send-media", http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, called := runProcurementAccess(t, c.role, c.stores, c.msgStore, c.msgErr, c.req, c.tpl)
			if code != c.want {
				t.Fatalf("status %d, want %d", code, c.want)
			}
			if called != (c.want == http.StatusTeapot) {
				t.Fatalf("handler called = %v", called)
			}
		})
	}
}

// Every procurement message route in main.go goes through RequireProcurementAccess.
func TestProcurementRoutesRequireAccess(t *testing.T) {
	src, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`router\.HandleFunc\("(/v1/(?:procurement-[^"]*|rfq-whatsapp-unread|email-unread|email-accounts/sync))",\s*([^)]*\)?)`)
	found := 0
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		found++
		if !strings.HasPrefix(m[2], "controller.RequireProcurementAccess(") {
			t.Errorf("%s is not wrapped in RequireProcurementAccess", m[1])
		}
	}
	if found < 31 {
		t.Errorf("found %d procurement routes, want at least 31", found)
	}
}

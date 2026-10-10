package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/erp/erpfixture"
	"github.com/sirinibin/startpos/backend/models"
)

// DB-backed handler tests. They run against a real MongoDB + Redis when
// ERP_TEST_DB=1 (CI's api job sets it, with MONGO_HOST/MONGO_PORT/REDIS_DSN):
//
//	ERP_TEST_DB=1 MONGO_HOST=127.0.0.1 MONGO_PORT=27017 REDIS_DSN=127.0.0.1:6379 go test ./controller/ -count=1
//
// The legacy-shaped erpfixture is seeded once per test binary into a
// throw-away database (erp_test_ctrl_<pid>) and dropped in TestMain.
var (
	dbOnce sync.Once
	dbFx   *erpfixture.Fixture
	dbErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if dbFx != nil {
		dbFx.Drop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		_ = db.GetDB("").Drop(ctx)
		cancel()
	}
	os.Exit(code)
}

// requireDB seeds the fixture on first use and returns it; without
// ERP_TEST_DB=1 the test is skipped (the no-database unit job).
func requireDB(t *testing.T) *erpfixture.Fixture {
	t.Helper()
	if os.Getenv("ERP_TEST_DB") != "1" {
		t.Skip("DB-backed: set ERP_TEST_DB=1 with MONGO_HOST/MONGO_PORT/REDIS_DSN")
	}
	dbOnce.Do(func() {
		name := os.Getenv("MONGO_DB")
		if !(strings.HasPrefix(name, "erp_test") || strings.HasPrefix(name, "t1_") || strings.HasPrefix(name, "test")) {
			name = fmt.Sprintf("erp_test_ctrl_%d", os.Getpid())
			os.Setenv("MONGO_DB", name)
		}
		if os.Getenv("ACCESS_SECRET") == "" {
			os.Setenv("ACCESS_SECRET", "ctrl-test-secret")
		}
		db.Client("")
		db.InitRedis()
		dbFx, dbErr = erpfixture.Seed(name)
	})
	if dbErr != nil {
		t.Fatalf("fixture seed failed: %v", dbErr)
	}
	return dbFx
}

var tokenMu sync.Mutex
var tokenByEmail = map[string]string{}

// tokenFor issues a real legacy access token (JWT + Redis session).
func tokenFor(t *testing.T, email string) string {
	t.Helper()
	tokenMu.Lock()
	defer tokenMu.Unlock()
	if tok, ok := tokenByEmail[email]; ok {
		return tok
	}
	tok, err := models.GenerateAccesstoken(email)
	if err != nil {
		t.Fatalf("token for %s: %v", email, err)
	}
	tokenByEmail[email] = tok.Token
	return tok.Token
}

// apiResp is a decoded legacy JSON response ({status, result, errors}).
type apiResp struct {
	Code   int
	Raw    string
	Status bool
	Result json.RawMessage
	Errors map[string]interface{}
}

// callHandler runs a legacy handler with a JSON body, an optional token and
// mux route vars (e.g. "id", "<hex>").
func callHandler(t *testing.T, h http.HandlerFunc, method, url, token string, body interface{}, vars ...string) apiResp {
	t.Helper()
	var rd *bytes.Reader
	switch b := body.(type) {
	case nil:
		rd = bytes.NewReader(nil)
	case string:
		rd = bytes.NewReader([]byte(b))
	default:
		j, _ := json.Marshal(b)
		rd = bytes.NewReader(j)
	}
	req := httptest.NewRequest(method, url, rd)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	if len(vars) > 0 {
		m := map[string]string{}
		for i := 0; i+1 < len(vars); i += 2 {
			m[vars[i]] = vars[i+1]
		}
		req = mux.SetURLVars(req, m)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	out := apiResp{Code: rec.Code, Raw: rec.Body.String()}
	var env struct {
		Status bool                   `json:"status"`
		Result json.RawMessage        `json:"result"`
		Errors map[string]interface{} `json:"errors"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &env) == nil {
		out.Status, out.Result, out.Errors = env.Status, env.Result, env.Errors
	}
	return out
}

// resultMap decodes the result object of a response.
func (r apiResp) resultMap(t *testing.T) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(r.Result, &m); err != nil {
		t.Fatalf("result is not an object: %s", r.Raw)
	}
	return m
}

// uniqName returns a name unique to this run.
func uniqName(prefix string) string {
	return fmt.Sprintf("%s %d", prefix, time.Now().UnixNano())
}

// TestDBHarness_SeedAndToken checks the harness itself: the seeded admin
// gets a token the legacy auth accepts.
func TestDBHarness_SeedAndToken(t *testing.T) {
	fx := requireDB(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", tokenFor(t, fx.AdminEmail))
	claims, err := models.AuthenticateByAccessToken(req)
	if err != nil || claims.UserID != fx.Admin.Hex() {
		t.Fatalf("auth with fixture token: claims=%+v err=%v", claims, err)
	}
}

package erp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/controller"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/erp/erpfixture"
	"go.mongodb.org/mongo-driver/bson"
)

// DB-backed tests (API happy paths, integration / old-data compatibility)
// are OPT-IN so the default `go test ./...` (CI without MongoDB/Redis) stays
// green and fast:
//
//	ERP_TEST_DB=1 MONGO_HOST=127.0.0.1 MONGO_PORT=27017 REDIS_DSN=127.0.0.1:6379 go test ./erp/ -count=1
//
// MONGO_DB is forced to a throw-away test database (erp_test_<pid>) unless it
// already has a test prefix; it is dropped afterwards.
var (
	dbEnabled  bool
	fx         *erpfixture.Fixture
	testRouter http.Handler
)

func TestMain(m *testing.M) {
	initResources()
	loginLimiter = controller.NewRateLimiter(100000, time.Minute)
	signupLimiter = controller.NewRateLimiter(100000, time.Minute)
	testRouter = NewRouter()
	code := 0
	if os.Getenv("ERP_TEST_DB") == "1" {
		name := os.Getenv("MONGO_DB")
		if !(strings.HasPrefix(name, "erp_test") || strings.HasPrefix(name, "t1_") || strings.HasPrefix(name, "test")) {
			name = fmt.Sprintf("erp_test_%d", os.Getpid())
			os.Setenv("MONGO_DB", name)
		}
		if os.Getenv("ACCESS_SECRET") == "" {
			os.Setenv("ACCESS_SECRET", "erp-test-secret")
		}
		// legacy code writes files (images, zatca xml) relative to cwd
		tmp, _ := os.MkdirTemp("", "erp-test-cwd")
		_ = os.Chdir(tmp)
		db.Client("")
		db.InitRedis()
		var err error
		fx, err = erpfixture.Seed(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, "fixture seed failed:", err)
			os.Exit(1)
		}
		dbEnabled = true
		var bi struct {
			Version string `bson:"version"`
		}
		ctx0, cancel0 := context.WithTimeout(context.Background(), 10*time.Second)
		_ = db.GetDB("").RunCommand(ctx0, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&bi)
		cancel0()
		mongoVersion = bi.Version
		code = m.Run()
		fx.Drop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		_ = db.GetDB("").Drop(ctx)
		cancel()
		_ = os.RemoveAll(tmp)
	} else {
		// no MongoDB: store reads must not try to count ZATCA documents
		storeHasZatcaDocs = func(M) bool { return false }
		code = m.Run()
	}
	os.Exit(code)
}

var mongoVersion string

// mongoAtLeast reports whether the test server supports features of the
// given MongoDB version (some legacy updates use 4.2+ pipeline updates).
func mongoAtLeast(major, minor int) bool {
	var ma, mi int
	fmt.Sscanf(mongoVersion, "%d.%d", &ma, &mi)
	return ma > major || (ma == major && mi >= minor)
}

func requireDB(t *testing.T) {
	t.Helper()
	if !dbEnabled {
		t.Skip("set ERP_TEST_DB=1 (with MONGO_*/REDIS_DSN) to run DB-backed adapter tests")
	}
}

type resp struct {
	Code   int
	Body   M
	Raw    string
	Header http.Header
}

func (r resp) errField(f string) string { return str(sub(sub(r.Body, "error"), "fields")[f]) }
func (r resp) errCode() string          { return str(get(r.Body, "error.code")) }
func (r resp) data() []interface{}      { return arr(r.Body["data"]) }

func call(t *testing.T, method, path, token string, body interface{}, headers ...string) resp {
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
	req := httptest.NewRequest(method, Prefix+path, rd)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	testRouter.ServeHTTP(rec, req)
	out := resp{Code: rec.Code, Raw: rec.Body.String(), Header: rec.Header()}
	if s := strings.TrimSpace(out.Raw); s != "" {
		var m M
		if json.Unmarshal([]byte(s), &m) == nil {
			out.Body = normNumbers(m).(M)
		}
	}
	return out
}

var tokenCache = map[string]string{}

func login(t *testing.T, email string) string {
	t.Helper()
	if tok, ok := tokenCache[email]; ok {
		return tok
	}
	r := call(t, "POST", "/auth/login", "", M{"email": email, "password": erpfixture.Password})
	if r.Code != 200 {
		t.Fatalf("login %s: %d %s", email, r.Code, r.Raw)
	}
	tok := str(r.Body["accessToken"])
	tokenCache[email] = tok
	return tok
}

// eventually polls cond (legacy side effects run in goroutines).
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

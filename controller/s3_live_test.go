package controller

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/models"
)

// Live S3 tests. They run the real SigV4 upload / read / HEAD / delete code
// against an S3-compatible server that verifies signatures (versitygw). CI's
// api job starts it over TLS with virtual-host addressing for
// s3.us-east-1.amazonaws.com and sets:
//
//	S3_TEST_ENDPOINT=https://127.0.0.1:7070 S3_TEST_CA_FILE=<its cert>
//	S3_TEST_ACCESS_KEY=... S3_TEST_SECRET_KEY=...
//
// CI also sets S3_TEST_REQUIRED=1 so a missing server fails instead of skipping.
//
// Every test runs twice: "endpoint" uses the custom-endpoint (path-style)
// settings, "aws" leaves S3Endpoint empty like a real AWS bucket and routes
// <bucket>.s3.us-east-1.amazonaws.com to the test server. The keys are
// throw-away test values from ci/s3-test-server.sh, never real AWS credentials.
// Locally: eval "$(ci/s3-test-server.sh | sed 's/^/export /')"
var s3LiveModes = []string{"endpoint", "aws"}

func requireS3(t *testing.T, mode string) models.AdminSettings {
	t.Helper()
	ep := os.Getenv("S3_TEST_ENDPOINT")
	if ep == "" {
		if os.Getenv("S3_TEST_REQUIRED") == "1" {
			t.Fatal("S3_TEST_REQUIRED=1 but S3_TEST_ENDPOINT is not set: the S3 test server did not start")
		}
		t.Skip("live S3: set S3_TEST_ENDPOINT, S3_TEST_CA_FILE, S3_TEST_ACCESS_KEY, S3_TEST_SECRET_KEY (ci/s3-test-server.sh)")
	}
	u, err := url.Parse(ep)
	if err != nil {
		t.Fatal(err)
	}
	s3TestTransport(t, u.Host)
	s := models.AdminSettings{
		S3Enabled:     true,
		S3Region:      "us-east-1",
		S3AccessKeyID: os.Getenv("S3_TEST_ACCESS_KEY"),
		S3SecretKey:   os.Getenv("S3_TEST_SECRET_KEY"),
		S3BucketName:  fmt.Sprintf("ci-%d", time.Now().UnixNano()),
	}
	if mode == "endpoint" {
		s.S3Endpoint = ep
	}
	if code := s3TestCreateBucket(t, ep, s); code != http.StatusOK {
		t.Fatalf("create bucket %s: status %d", s.S3BucketName, code)
	}
	return s
}

// s3TestTransport makes http.DefaultClient (used by the S3 code) trust the
// test server's certificate and dial it for every *.amazonaws.com host.
func s3TestTransport(t *testing.T, serverAddr string) {
	t.Helper()
	tlsCfg := &tls.Config{}
	if f := os.Getenv("S3_TEST_CA_FILE"); f != "" {
		pem, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			t.Fatalf("no certificate in %s", f)
		}
		tlsCfg.RootCAs = pool
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = tlsCfg
	tr.Proxy = nil
	var d net.Dialer
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if host, _, _ := net.SplitHostPort(addr); strings.HasSuffix(host, ".amazonaws.com") {
			addr = serverAddr
		}
		return d.DialContext(ctx, network, addr)
	}
	prev := http.DefaultTransport
	http.DefaultTransport = tr
	t.Cleanup(func() { http.DefaultTransport = prev })
}

// s3TestCreateBucket sends a signed CreateBucket (PUT /bucket). It signs with
// the request's real Host, independently of the code under test.
func s3TestCreateBucket(t *testing.T, endpoint string, s models.AdminSettings) int {
	t.Helper()
	u, err := url.Parse(strings.TrimRight(endpoint, "/") + "/" + s.S3BucketName)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	dateStr, timeStr := now.Format("20060102"), now.Format("20060102T150405Z")
	emptyHash := fmt.Sprintf("%x", sha256sum(nil))
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", u.Host, emptyHash, timeStr)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonical := strings.Join([]string{"PUT", u.Path, "", canonicalHeaders, signedHeaders, emptyHash}, "\n")
	scope := dateStr + "/" + s.S3Region + "/s3/aws4_request"
	toSign := strings.Join([]string{"AWS4-HMAC-SHA256", timeStr, scope, fmt.Sprintf("%x", sha256sum([]byte(canonical)))}, "\n")
	key := hmacSHA256(hmacSHA256(hmacSHA256(hmacSHA256([]byte("AWS4"+s.S3SecretKey), []byte(dateStr)), []byte(s.S3Region)), []byte("s3")), []byte("aws4_request"))
	req, _ := http.NewRequest("PUT", u.String(), nil)
	req.Header.Set("X-Amz-Date", timeStr)
	req.Header.Set("X-Amz-Content-Sha256", emptyHash)
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%x",
		s.S3AccessKeyID, scope, signedHeaders, hmacSHA256(key, []byte(toSign))))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// Keys as the app builds them, plus the awkward cases real uploads produce
// (spaces from WhatsApp/email file names, Arabic names, nested folders).
var s3LiveKeys = []string{
	"images/store1/expenses/receipt.jpg",
	"attachments/store1/wa_msg1/Cable Comnnection..pdf",
	"attachments/store1/msg2/فاتورة ضريبية.pdf",
	"attachments/store1/msg3/a+b (copy) #2.png",
}

func TestS3Live_UploadReadHeadDelete_RoundTrip(t *testing.T) {
	for _, mode := range s3LiveModes {
		t.Run(mode, func(t *testing.T) { s3LiveUploadReadHeadDeleteRoundTrip(t, requireS3(t, mode)) })
	}
}

func s3LiveUploadReadHeadDeleteRoundTrip(t *testing.T, s models.AdminSettings) {
	for i, key := range s3LiveKeys {
		key := key
		body := []byte(fmt.Sprintf("payload %d for %s", i, key))
		t.Run(key, func(t *testing.T) {
			u, err := uploadToS3(s, key, body, "application/pdf")
			if err != nil {
				t.Fatalf("upload: %v", err)
			}
			if u == "" {
				t.Fatal("upload returned an empty URL")
			}
			if !headS3Object(s, key) {
				t.Fatal("HEAD after upload: object not found")
			}
			got, err := downloadFromS3(s, key)
			if err != nil || !bytes.Equal(got, body) {
				t.Fatalf("download: %v, got %q want %q", err, got, body)
			}
			got, err = downloadAttachment(s, "/cdn/"+key)
			if err != nil || !bytes.Equal(got, body) {
				t.Fatalf("downloadAttachment: %v, got %q", err, got)
			}
			rec := httptest.NewRecorder()
			if !proxyS3Object(s, key, rec) {
				t.Fatal("proxy: not served")
			}
			if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), body) {
				t.Fatalf("proxy: status %d body %q", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
				t.Errorf("proxy Content-Type = %q, want application/pdf", ct)
			}

			if err := deleteFromS3(s, key); err != nil {
				t.Fatalf("delete: %v", err)
			}
			if headS3Object(s, key) {
				t.Fatal("HEAD after delete: object still exists")
			}
			if _, err := downloadFromS3(s, key); err == nil {
				t.Fatal("download after delete should fail")
			}
			if proxyS3Object(s, key, httptest.NewRecorder()) {
				t.Fatal("proxy after delete should not serve")
			}
		})
	}
}

func TestS3Live_SaveAttachment_GoesToS3NotDisk(t *testing.T) {
	for _, mode := range s3LiveModes {
		t.Run(mode, func(t *testing.T) { s3LiveSaveAttachmentGoesToS3NotDisk(t, requireS3(t, mode)) })
	}
}

func s3LiveSaveAttachmentGoesToS3NotDisk(t *testing.T, s models.AdminSettings) {
	key := fmt.Sprintf("attachments/ci/%d/quote.pdf", time.Now().UnixNano())
	if got := saveAttachment(s, key, []byte("pdf"), "application/pdf"); got != "/cdn/"+key {
		t.Fatalf("saveAttachment = %q", got)
	}
	if _, err := os.Stat("./" + key); err == nil {
		os.RemoveAll("./attachments/ci")
		t.Fatal("file was written to local disk although S3 is configured")
	}
	if !headS3Object(s, key) {
		t.Fatal("object missing in S3")
	}
	if err := deleteFromS3(s, key); err != nil {
		t.Fatal(err)
	}
}

// Full app path: S3 settings saved in admin_settings, an upload through
// models.SaveFileToStorage (store logos, vendor logos, expense images), then
// the file served back through GET /cdn/... and gone after delete.
func TestS3Live_SaveFileToStorage_ServedByCdnHandler(t *testing.T) {
	for _, mode := range s3LiveModes {
		t.Run(mode, func(t *testing.T) { s3LiveSaveFileToStorageServedByCdnHandler(t, requireS3(t, mode)) })
	}
}

func s3LiveSaveFileToStorageServedByCdnHandler(t *testing.T, s models.AdminSettings) {
	requireDB(t)
	prev, _ := models.GetAdminSettings()
	if err := models.UpsertAdminSettings(&s); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if prev == nil {
			prev = &models.AdminSettings{}
		}
		_ = models.UpsertAdminSettings(prev)
	})

	key := fmt.Sprintf("images/ci%d/logo name.png", time.Now().UnixNano())
	if got := models.SaveFileToStorage(key, []byte("png-bytes"), "image/png"); got != "/cdn/"+key {
		t.Fatalf("SaveFileToStorage = %q", got)
	}
	if _, err := os.Stat("./" + key); err == nil {
		t.Fatal("file was written to local disk although S3 is configured")
	}

	cdnGet := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		CdnFileHandler(rec, httptest.NewRequest("GET", "/cdn/"+s3URIEncodeKey(key), nil))
		return rec
	}
	if rec := cdnGet(); rec.Code != http.StatusOK || rec.Body.String() != "png-bytes" {
		t.Fatalf("GET /cdn/: status %d body %q", rec.Code, rec.Body.String())
	}
	if err := deleteFromS3(s, key); err != nil {
		t.Fatal(err)
	}
	if rec := cdnGet(); rec.Code != http.StatusNotFound {
		t.Fatalf("GET /cdn/ after delete: status %d, want 404", rec.Code)
	}
}

// The server must really check signatures, or the tests above prove nothing.
func TestS3Live_WrongSecretIsRejected(t *testing.T) {
	for _, mode := range s3LiveModes {
		t.Run(mode, func(t *testing.T) { s3LiveWrongSecretIsRejected(t, requireS3(t, mode)) })
	}
}

func s3LiveWrongSecretIsRejected(t *testing.T, s models.AdminSettings) {
	bad := s
	bad.S3SecretKey = s.S3SecretKey + "-wrong"
	key := "attachments/ci/wrong-secret.txt"
	if _, err := uploadToS3(bad, key, []byte("x"), "text/plain"); err == nil {
		t.Fatal("upload with a wrong secret succeeded")
	}
	if got := saveAttachment(bad, key, []byte("x"), "text/plain"); got != "" {
		t.Fatalf("saveAttachment with a wrong secret = %q, want \"\"", got)
	}
	if headS3Object(s, key) {
		t.Fatal("object exists after rejected uploads")
	}
}

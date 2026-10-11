package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/models"
)

func TestCdnServable(t *testing.T) {
	for p, want := range map[string]bool{
		"images/s/products/p/a.png": true, "attachments/s/1/a.pdf": true, "zatca/s/sales/xml/a.xml": true,
		"env/env.go": false, "localhost.key.pem": false, "main.go": false, "ZatcaPython/x.py": false, "": false,
	} {
		if got := cdnServable(p); got != want {
			t.Errorf("cdnServable(%q) = %v, want %v", p, got, want)
		}
	}
}

func cdnGet(path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	CdnFileHandler(rec, req)
	return rec
}

// ZATCA XML under /cdn/zatca/<storeId>/ needs a signed link or a token of a
// user of that store; files outside the stored folders are never served.
func TestCdnFileHandler_Access_Integration(t *testing.T) {
	fx := requireDB(t)
	link := "/cdn/zatca/" + fx.StoreA.Hex() + "/sales/xml/NO-SUCH-FILE.xml"
	// refused before the (missing) file is looked up
	if rec := cdnGet(link, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no login: want 401, got %d", rec.Code)
	}
	if rec := cdnGet(link, tokenFor(t, fx.UserBEmail)); rec.Code != http.StatusForbidden {
		t.Errorf("another company's user: want 403, got %d", rec.Code)
	}
	// allowed: the file then doesn't exist (404), which shows the check passed
	if rec := cdnGet(link, tokenFor(t, fx.ManagerEmail)); rec.Code != http.StatusNotFound {
		t.Errorf("the store's manager: want 404 (allowed, no file), got %d", rec.Code)
	}
	if rec := cdnGet(models.SignCdnURL(link, time.Now()), ""); rec.Code != http.StatusNotFound {
		t.Errorf("signed link: want 404 (allowed, no file), got %d", rec.Code)
	}
	if rec := cdnGet(models.SignCdnURL(link, time.Now().Add(-48*time.Hour)), ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("expired signed link: want 401, got %d", rec.Code)
	}
	if rec := cdnGet("/cdn/env/env.go", ""); rec.Code != http.StatusNotFound {
		t.Errorf("a file outside the stored folders: want 404, got %d", rec.Code)
	}
}

package controller

import (
	"encoding/base64"
	"testing"

	"github.com/sirinibin/startpos/backend/models"
)

func TestMimeForExt(t *testing.T) {
	cases := []struct {
		ext  string
		want string
	}{
		{".jpg", "image/jpeg"},
		{".jpeg", "image/jpeg"},
		{".png", "image/png"},
		{".gif", "image/gif"},
		{".webp", "image/webp"},
		{".pdf", "application/pdf"},
		{".xml", "application/xml"},
		{".bin", "application/octet-stream"},
		{"", "application/octet-stream"},
	}
	for _, tc := range cases {
		got := mimeForExt(tc.ext)
		if got != tc.want {
			t.Errorf("mimeForExt(%q) = %q, want %q", tc.ext, got, tc.want)
		}
	}
}

func TestMigrateInlineBase64Items_AlreadyCDN(t *testing.T) {
	s := models.AdminSettings{}
	items := []string{"/cdn/images/abc/expenses/file.jpg"}
	urls := migrateInlineBase64Items(s, "store1", "eid1", "expenses", "exp_", items)
	if len(urls) != 1 || urls[0] != "/cdn/images/abc/expenses/file.jpg" {
		t.Errorf("expected already-CDN item to be returned unchanged, got %v", urls)
	}
}

func TestMigrateInlineBase64Items_InvalidBase64Skipped(t *testing.T) {
	s := models.AdminSettings{}
	items := []string{"not-valid-base64!!"}
	urls := migrateInlineBase64Items(s, "store1", "eid1", "expenses", "exp_", items)
	// S3 disabled + invalid base64 → no URLs returned
	if len(urls) != 0 {
		t.Errorf("expected empty result for invalid base64, got %v", urls)
	}
}

func TestMigrateInlineBase64Items_DataURIParsed(t *testing.T) {
	// 1x1 white PNG as data URI
	pngB64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI6QAAAABJRU5ErkJggg=="
	dataURI := "data:image/png;base64," + pngB64

	s := models.AdminSettings{} // S3 disabled → writes to disk
	items := []string{dataURI}
	urls := migrateInlineBase64Items(s, "store1", "eid1", "expenses", "exp_", items)
	// With S3 disabled, saveAttachment will fall through to local disk write (returns /cdn/ URL)
	if len(urls) != 1 {
		t.Errorf("expected 1 url, got %d: %v", len(urls), urls)
	}
}

func TestMigrateInlineBase64Items_RawBase64(t *testing.T) {
	// Minimal valid JPEG bytes encoded as base64
	jpegBytes := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	raw := base64.StdEncoding.EncodeToString(jpegBytes)

	s := models.AdminSettings{}
	items := []string{raw}
	urls := migrateInlineBase64Items(s, "store1", "eid1", "expenses", "exp_", items)
	if len(urls) != 1 {
		t.Errorf("expected 1 url for raw base64, got %d", len(urls))
	}
}

func TestMigrateInlineBase64Items_MixedItems(t *testing.T) {
	existing := "/cdn/images/s/d/file.jpg"
	pngB64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI6QAAAABJRU5ErkJggg=="
	dataURI := "data:image/png;base64," + pngB64
	invalid := "not-base64!!!"

	s := models.AdminSettings{}
	items := []string{existing, dataURI, invalid}
	urls := migrateInlineBase64Items(s, "store1", "eid1", "expenses", "exp_", items)

	// existing CDN item passes through, dataURI uploads (local disk), invalid is skipped
	if len(urls) != 2 {
		t.Errorf("expected 2 urls (existing + dataURI), got %d: %v", len(urls), urls)
	}
	if urls[0] != existing {
		t.Errorf("first item should be existing CDN url, got %q", urls[0])
	}
}

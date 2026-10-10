package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Helpers shared by the DB-backed customer / guest-register / upload /
// product integration tests (group A). They build on dbtest_test.go.

// tinyPNG is a valid 1x1 transparent PNG.
var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// multipartUpload runs an Upload*Image handler with a multipart body made of
// the given form fields plus (when image != nil) an "image" file part.
func multipartUpload(t *testing.T, h http.HandlerFunc, url, token string, fields map[string]string, filename string, image []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("write field %s: %v", k, err)
		}
	}
	if image != nil {
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="%s"`, filename))
		hdr.Set("Content-Type", "image/png")
		part, err := mw.CreatePart(hdr)
		if err != nil {
			t.Fatalf("create part: %v", err)
		}
		if _, err := part.Write(image); err != nil {
			t.Fatalf("write image: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, url, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

// insertStoreDoc inserts a document into store_<storeID>.<coll> and deletes it
// when the test ends.
func insertStoreDoc(t *testing.T, storeID primitive.ObjectID, coll string, doc bson.M) primitive.ObjectID {
	t.Helper()
	id := primitive.NewObjectID()
	doc["_id"] = id
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := db.GetDB("store_" + storeID.Hex()).Collection(coll)
	if _, err := c.InsertOne(ctx, doc); err != nil {
		t.Fatalf("insert %s: %v", coll, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = c.DeleteOne(ctx, bson.M{"_id": id})
	})
	return id
}

// storeDoc reads one raw document from store_<storeID>.<coll>.
func storeDoc(t *testing.T, storeID primitive.ObjectID, coll string, id primitive.ObjectID) bson.M {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var m bson.M
	if err := db.GetDB("store_"+storeID.Hex()).Collection(coll).FindOne(ctx, bson.M{"_id": id}).Decode(&m); err != nil {
		t.Fatalf("read %s %s: %v", coll, id.Hex(), err)
	}
	return m
}

// docStrings returns m[key] as a []string (for bson arrays of strings).
func docStrings(m bson.M, key string) []string {
	var out []string
	if a, ok := m[key].(primitive.A); ok {
		for _, v := range a {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// removeUploadDir removes the local-disk upload directory of one entity
// (./images/<store>/<kind>/<id>) and then any parents left empty.
func removeUploadDir(storeID, kind, id string) {
	dir := filepath.Join("images", storeID, kind, id)
	_ = os.RemoveAll(dir)
	for _, p := range []string{filepath.Join("images", storeID, kind), filepath.Join("images", storeID), "images"} {
		_ = os.Remove(p) // only succeeds when empty
	}
}

// assertImageUpload drives one Upload*Image handler through its contract:
// form validation errors, the (missing) auth guard, and a successful upload
// that lands on the local-disk storage fallback (no S3 is configured in the
// test database, so models.SaveFileToStorage writes ./images/... relative to
// the package directory and returns a /cdn/ URL) and is $push-ed onto the
// entity's "images" array.
func assertImageUpload(t *testing.T, h http.HandlerFunc, url, token string, storeID, entityID primitive.ObjectID, coll, kind string) {
	t.Helper()
	sID, eID := storeID.Hex(), entityID.Hex()
	t.Cleanup(func() { removeUploadDir(sID, kind, eID) })

	// --- validation (all rejected before anything is written) ---
	cases := []struct {
		name   string
		fields map[string]string
		image  []byte
		want   int
	}{
		{"missing id", map[string]string{"storeID": sID}, tinyPNG, http.StatusBadRequest},
		{"missing storeID", map[string]string{"id": eID}, tinyPNG, http.StatusBadRequest},
		{"missing image", map[string]string{"id": eID, "storeID": sID}, nil, http.StatusBadRequest},
		{"bad store id", map[string]string{"id": eID, "storeID": "not-hex"}, tinyPNG, http.StatusBadRequest},
		{"unknown store", map[string]string{"id": eID, "storeID": primitive.NewObjectID().Hex()}, tinyPNG, http.StatusInternalServerError},
		{"bad entity id", map[string]string{"id": "not-hex", "storeID": sID}, tinyPNG, http.StatusBadRequest},
	}
	for _, c := range cases {
		rec := multipartUpload(t, h, url, token, c.fields, "x.png", c.image)
		if rec.Code != c.want {
			t.Errorf("%s: want %d, got %d (%s)", c.name, c.want, rec.Code, rec.Body.String())
		}
	}
	// a non-multipart body is rejected by ParseMultipartForm
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(`{"id":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-multipart body: want 400, got %d", rec.Code)
	}
	if imgs := docStrings(storeDoc(t, storeID, coll, entityID), "images"); len(imgs) != 0 {
		t.Fatalf("rejected uploads must not touch the document, images=%v", imgs)
	}

	// --- success with a real access token ---
	fields := map[string]string{"id": eID, "storeID": sID}
	rec = multipartUpload(t, h, url, token, fields, "photo.png", tinyPNG)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("upload response is not JSON: %s", rec.Body.String())
	}
	prefix := fmt.Sprintf("/cdn/images/%s/%s/%s/", sID, kind, eID)
	if !strings.HasPrefix(out.URL, prefix) || !strings.HasSuffix(out.URL, ".png") {
		t.Fatalf("url %q: want prefix %q and .png extension", out.URL, prefix)
	}
	onDisk, err := os.ReadFile("." + strings.TrimPrefix(out.URL, "/cdn"))
	if err != nil {
		t.Fatalf("uploaded file not on local disk: %v", err)
	}
	if !bytes.Equal(onDisk, tinyPNG) {
		t.Fatalf("stored file differs from the upload (%d vs %d bytes)", len(onDisk), len(tinyPNG))
	}
	if imgs := docStrings(storeDoc(t, storeID, coll, entityID), "images"); len(imgs) != 1 || imgs[0] != out.URL {
		t.Fatalf("document images = %v, want [%s]", imgs, out.URL)
	}

	// --- no token: the handler has no auth guard (see report). Accept the
	// current behaviour (200 + image appended) but also a future 401, in
	// which case nothing may have been written. ---
	rec = multipartUpload(t, h, url, "", fields, "anon.png", tinyPNG)
	imgs := docStrings(storeDoc(t, storeID, coll, entityID), "images")
	switch rec.Code {
	case http.StatusUnauthorized:
		if len(imgs) != 1 {
			t.Fatalf("401 upload must not write, images=%v", imgs)
		}
	case http.StatusOK:
		t.Logf("NOTE: %s accepts uploads without an access token", url)
		if len(imgs) != 2 {
			t.Fatalf("anonymous upload returned 200 but images=%v", imgs)
		}
	default:
		t.Fatalf("no token: want 200 (current) or 401, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// waitFor polls cond until it returns true or the timeout elapses. Used for
// work the handlers do in background goroutines (product history).
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := ""
	for {
		ok, state := cond()
		if ok {
			return
		}
		last = state
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s; last state: %s", what, last)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// productHistory returns the product_history rows referencing a product,
// ordered by date.
func productHistory(t *testing.T, storeID, productID primitive.ObjectID) []bson.M {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := db.GetDB("store_" + storeID.Hex()).Collection("product_history")
	cur, err := c.Find(ctx, bson.M{"reference_id": productID})
	if err != nil {
		t.Fatalf("find history: %v", err)
	}
	var rows []bson.M
	if err := cur.All(ctx, &rows); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	// sort by date (small slices)
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && histDate(rows[j]).Before(histDate(rows[j-1])); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
	return rows
}

func histDate(m bson.M) time.Time {
	if d, ok := m["date"].(primitive.DateTime); ok {
		return d.Time()
	}
	return time.Time{}
}

func num(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	}
	return -1e18
}

// cleanupProduct deletes a product and its history rows when the test ends
// (after waiting briefly for the background history goroutines to settle).
func cleanupProduct(t *testing.T, storeID, productID primitive.ObjectID) {
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		d := db.GetDB("store_" + storeID.Hex())
		_, _ = d.Collection("product").DeleteOne(ctx, bson.M{"_id": productID})
		_, _ = d.Collection("product_history").DeleteMany(ctx, bson.M{"reference_id": productID})
	})
}

// productBody builds a create/update body for store A.
func productBody(storeID primitive.ObjectID, name, partNumber string, category primitive.ObjectID, adjustments []map[string]interface{}) map[string]interface{} {
	if adjustments == nil {
		adjustments = []map[string]interface{}{}
	}
	b := map[string]interface{}{
		"name": name,
		"unit": "pcs",
		"product_stores": map[string]interface{}{
			storeID.Hex(): map[string]interface{}{
				"store_id":             storeID.Hex(),
				"purchase_unit_price":  50.0,
				"retail_unit_price":    80.0,
				"wholesale_unit_price": 70.0,
				"stock_adjustments":    adjustments,
			},
		},
	}
	if partNumber != "" {
		b["part_number"] = partNumber
	}
	if !category.IsZero() {
		b["category_id"] = []string{category.Hex()}
	}
	return b
}

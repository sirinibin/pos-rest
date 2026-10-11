package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestLocalImagePath(t *testing.T) {
	cases := map[string]string{
		"/images/s1/products/p1/a.png":     "images/s1/products/p1/a.png",
		"/cdn/images/s1/products/p1/a.png": "", // /cdn files were never removed from disk here
		"/images/../main.go":               "",
		"/../../etc/passwd":                "",
		"/env/env.go":                      "",
		"":                                 "",
	}
	for in, want := range cases {
		if got := localImagePath(in); got != want {
			t.Errorf("localImagePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// #1: image uploads and deletes need a login and the store: no token is 401,
// another company's user is 403, and nothing is written either way.
func TestEntityImages_AuthAndStore_Integration(t *testing.T) {
	fx := requireDB(t)
	id := insertStoreDoc(t, fx.StoreA, "customer", bson.M{"name": uniqName("Img Customer"), "store_id": fx.StoreA, "deleted": false})
	fields := map[string]string{"id": id.Hex(), "storeID": fx.StoreA.Hex()}
	t.Cleanup(func() { removeUploadDir(fx.StoreA.Hex(), "customers", id.Hex()) })
	for _, h := range []struct {
		name string
		fn   http.HandlerFunc
	}{{"customer", UploadCustomerImage}, {"vendor", UploadVendorImage}, {"product", UploadProductImage}} {
		if rec := multipartUpload(t, h.fn, "/v1/x/upload-image", "", fields, "a.png", tinyPNG); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s upload without a token: want 401, got %d", h.name, rec.Code)
		}
		if rec := multipartUpload(t, h.fn, "/v1/x/upload-image", tokenFor(t, fx.UserBEmail), fields, "a.png", tinyPNG); rec.Code != http.StatusForbidden {
			t.Errorf("%s upload into another company's store: want 403, got %d", h.name, rec.Code)
		}
	}
	if imgs := docStrings(storeDoc(t, fx.StoreA, "customer", id), "images"); len(imgs) != 0 {
		t.Fatalf("refused uploads wrote images: %v", imgs)
	}

	q := "/v1/customer/delete-image?url=/images/x.png&id=" + id.Hex() + "&storeID=" + fx.StoreA.Hex()
	for _, h := range []http.HandlerFunc{DeleteCustomerImage, DeleteVendorImage, DeleteProductImage} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodPost, q, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("delete without a token: want 401, got %d", rec.Code)
		}
		req := httptest.NewRequest(http.MethodPost, q, nil)
		req.Header.Set("Authorization", "Bearer "+tokenFor(t, fx.UserBEmail))
		rec = httptest.NewRecorder()
		h(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("delete in another company's store: want 403, got %d", rec.Code)
		}
	}
}

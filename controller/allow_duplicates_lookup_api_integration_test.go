//go:build integration

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// API test for the "Allow duplicates" lookup the quotation forms make before merging
// imported lines (fetchAllowDuplicateIds in reactjs-pos/src/quotation/quotationImport.js):
//
//	GET /v1/product?search[ids]=a,b&search[store_id]=..&limit=2&select=id,allow_duplicates
//
//	go test -tags integration ./controller/ -run TestAllowDuplicatesLookupAPI
func TestAllowDuplicatesLookupAPI(t *testing.T) {
	f := setupQuotationImportFixture(t)
	sid := f.store.ID.Hex()

	coll := db.GetDB("store_" + sid).Collection("product")
	dup, plain, other := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	mk := func(id primitive.ObjectID, name string, allow bool) models.Product {
		return models.Product{ID: id, Name: name, PartNumber: "PN-" + name, AllowDuplicates: allow, ProductStores: map[string]models.ProductStore{
			sid: {StoreID: f.store.ID, RetailUnitPrice: 9},
		}}
	}
	if _, err := coll.InsertMany(context.Background(), []interface{}{mk(dup, "Dup", true), mk(plain, "Plain", false), mk(other, "Other", true)}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })

	call := func(token, query string) (int, listProductResponse) {
		req := httptest.NewRequest(http.MethodGet, "/v1/product?"+query, nil)
		if token != "" {
			req.Header.Set("Authorization", token)
		}
		w := httptest.NewRecorder()
		ListProduct(w, req)
		var resp listProductResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v (%s)", err, w.Body.String())
		}
		return w.Code, resp
	}
	lookup := func(ids ...primitive.ObjectID) string {
		hex := make([]string, len(ids))
		for i, id := range ids {
			hex[i] = id.Hex()
		}
		return "search[ids]=" + strings.Join(hex, ",") + "&search[store_id]=" + sid + "&limit=" + strconv.Itoa(len(ids)) + "&select=id,allow_duplicates"
	}

	t.Run("returns the flag for the requested products only", func(t *testing.T) {
		_, resp := call(f.token, lookup(dup, plain))
		if !resp.Status || len(resp.Result) != 2 {
			t.Fatalf("got %+v", resp)
		}
		flags := map[primitive.ObjectID]bool{}
		for _, p := range resp.Result {
			flags[p.ID] = p.AllowDuplicates
		}
		if !flags[dup] || flags[plain] {
			t.Errorf("flags: %+v", flags)
		}
		if _, ok := flags[other]; ok {
			t.Errorf("unrequested product returned")
		}
	})
	t.Run("selects nothing but id and the flag", func(t *testing.T) {
		_, resp := call(f.token, lookup(dup))
		if len(resp.Result) != 1 {
			t.Fatalf("got %+v", resp)
		}
		p := resp.Result[0]
		if p.Name != "" || p.PartNumber != "" || len(p.ProductStores) != 0 {
			t.Errorf("unselected fields returned: name=%q part=%q stores=%v", p.Name, p.PartNumber, p.ProductStores)
		}
	})
	t.Run("unknown id returns no rows", func(t *testing.T) {
		_, resp := call(f.token, lookup(primitive.NewObjectID()))
		if !resp.Status || len(resp.Result) != 0 {
			t.Fatalf("got %+v", resp)
		}
	})
	t.Run("needs a token", func(t *testing.T) {
		code, resp := call("", lookup(dup))
		if code != http.StatusUnauthorized || resp.Status {
			t.Fatalf("got %d %+v", code, resp)
		}
	})
}

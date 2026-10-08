//go:build integration

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// API tests for the two requests behind the quotation form's
// "Import > From Purchases":
//   1. GET /v1/purchase      — the purchase search (SourceDocumentPicker)
//   2. GET /v1/product       — selling-price lookup (quotationImport.fetchRetailPrices)
// Run: go test -tags integration ./controller/ -run 'TestPurchaseImportAPI|TestRetailPriceLookupAPI'

type listPurchaseResponse struct {
	Status     bool              `json:"status"`
	TotalCount int64             `json:"total_count"`
	Result     []models.Purchase `json:"result"`
	Errors     map[string]string `json:"errors"`
}

func TestPurchaseImportAPI(t *testing.T) {
	f := setupQuotationImportFixture(t) // store + access token
	sid := f.store.ID.Hex()
	vendA, vendB := primitive.NewObjectID(), primitive.NewObjectID()

	coll := db.GetDB("store_" + sid).Collection("purchase")
	base := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	mk := func(code string, vendor primitive.ObjectID, mins int, deleted bool) models.Purchase {
		created := base.Add(time.Duration(mins) * time.Minute)
		v := vendor
		return models.Purchase{ID: primitive.NewObjectID(), Code: code, Date: &created, StoreID: &f.store.ID, VendorID: &v,
			VendorName: "vendor-" + code, NetTotal: 30, CreatedAt: &created,
			Products: []models.PurchaseProduct{{ProductID: primitive.NewObjectID(), Name: "Item " + code, Quantity: 4, PurchaseUnitPrice: 5, PurchaseUnitPriceWithVAT: 5.75}}}
	}
	if _, err := coll.InsertMany(context.Background(), []interface{}{
		mk("PI-API-001", vendA, 1, false), mk("PI-API-002", vendB, 2, false), mk("PI-API-DEL", vendA, 3, true),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Purchase has no Deleted field in Go, so soft-delete the row directly.
	if _, err := coll.UpdateOne(context.Background(), bson.M{"code": "PI-API-DEL"}, bson.M{"$set": bson.M{"deleted": true}}); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })

	call := func(token, query string) (int, listPurchaseResponse) {
		req := httptest.NewRequest(http.MethodGet, "/v1/purchase?"+query, nil)
		if token != "" {
			req.Header.Set("Authorization", token)
		}
		w := httptest.NewRecorder()
		ListPurchase(w, req)
		var resp listPurchaseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v (%s)", err, w.Body.String())
		}
		return w.Code, resp
	}
	picker := "search[store_id]=" + sid + "&select=id,code,date,net_total,vendor_name,vendor_id,customer_name,customer_id,products&limit=20&page=1&sort=-created_at"

	t.Run("no token is rejected", func(t *testing.T) {
		code, resp := call("", picker)
		if code != http.StatusUnauthorized || resp.Errors["access_token"] == "" {
			t.Fatalf("got %d %v", code, resp.Errors)
		}
	})
	t.Run("lists newest first without deleted", func(t *testing.T) {
		_, resp := call(f.token, picker)
		if !resp.Status || len(resp.Result) != 2 || resp.Result[0].Code != "PI-API-002" || resp.Result[1].Code != "PI-API-001" || resp.TotalCount != 2 {
			t.Fatalf("got %+v", resp)
		}
	})
	t.Run("filters by vendor", func(t *testing.T) {
		_, resp := call(f.token, picker+"&search[vendor_id]="+vendA.Hex())
		if len(resp.Result) != 1 || resp.Result[0].Code != "PI-API-001" {
			t.Fatalf("got %+v", resp.Result)
		}
	})
	t.Run("returns product lines with cost prices", func(t *testing.T) {
		_, resp := call(f.token, picker+"&search[code]=PI-API-001")
		if len(resp.Result) != 1 || len(resp.Result[0].Products) != 1 {
			t.Fatalf("got %+v", resp.Result)
		}
		p := resp.Result[0].Products[0]
		if p.Quantity != 4 || p.PurchaseUnitPrice != 5 || p.PurchaseUnitPriceWithVAT != 5.75 || resp.Result[0].VendorName != "vendor-PI-API-001" {
			t.Errorf("got %+v", p)
		}
	})
	t.Run("invalid vendor id reports a find error", func(t *testing.T) {
		_, resp := call(f.token, picker+"&search[vendor_id]=bad")
		if resp.Status || resp.Errors["find"] == "" {
			t.Fatalf("got %+v", resp)
		}
	})
}

type listProductResponse struct {
	Status bool              `json:"status"`
	Result []models.Product  `json:"result"`
	Errors map[string]string `json:"errors"`
}

func TestRetailPriceLookupAPI(t *testing.T) {
	f := setupQuotationImportFixture(t)
	sid := f.store.ID.Hex()

	coll := db.GetDB("store_" + sid).Collection("product")
	ids := []primitive.ObjectID{primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()}
	mk := func(id primitive.ObjectID, name string, price, priceVAT float64, deleted bool) models.Product {
		return models.Product{ID: id, Name: name, Deleted: deleted, ProductStores: map[string]models.ProductStore{
			sid: {StoreID: f.store.ID, RetailUnitPrice: price, RetailUnitPriceWithVAT: priceVAT, PurchaseUnitPrice: 1},
		}}
	}
	if _, err := coll.InsertMany(context.Background(), []interface{}{
		mk(ids[0], "Bolt", 3, 3.45, false), mk(ids[1], "Nut", 1.5, 1.73, false), mk(ids[2], "Washer", 0.5, 0.58, false),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })

	call := func(query string) listProductResponse {
		req := httptest.NewRequest(http.MethodGet, "/v1/product?"+query, nil)
		req.Header.Set("Authorization", f.token)
		w := httptest.NewRecorder()
		ListProduct(w, req)
		var resp listProductResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v (%s)", err, w.Body.String())
		}
		return resp
	}
	// Same shape as fetchRetailPrices in reactjs-pos/src/quotation/quotationImport.js
	sel := "id,product_stores." + sid + ".retail_unit_price,product_stores." + sid + ".retail_unit_price_with_vat"
	lookup := func(want ...primitive.ObjectID) string {
		s := ""
		for i, id := range want {
			if i > 0 {
				s += ","
			}
			s += id.Hex()
		}
		return "search[ids]=" + s + "&search[store_id]=" + sid + "&limit=" + strconv.Itoa(len(want)) + "&select=" + sel
	}

	t.Run("returns only the requested products with their store prices", func(t *testing.T) {
		resp := call(lookup(ids[0], ids[1]))
		if !resp.Status || len(resp.Result) != 2 {
			t.Fatalf("got %+v", resp)
		}
		byID := map[primitive.ObjectID]models.ProductStore{}
		for _, p := range resp.Result {
			byID[p.ID] = p.ProductStores[sid]
		}
		if byID[ids[0]].RetailUnitPrice != 3 || byID[ids[0]].RetailUnitPriceWithVAT != 3.45 || byID[ids[1]].RetailUnitPrice != 1.5 {
			t.Errorf("prices: %+v", byID)
		}
		if _, ok := byID[ids[2]]; ok {
			t.Errorf("unrequested product returned")
		}
		if byID[ids[0]].PurchaseUnitPrice != 0 {
			t.Errorf("unselected field purchase_unit_price leaked: %v", byID[ids[0]].PurchaseUnitPrice)
		}
	})
	t.Run("plain ids/store_id params are not understood (guards the frontend query shape)", func(t *testing.T) {
		resp := call("ids=" + ids[0].Hex() + "&store_id=" + sid)
		if resp.Status || resp.Errors["store_id"] == "" {
			t.Fatalf("expected store_id error, got %+v", resp)
		}
	})
	t.Run("invalid id is a find error", func(t *testing.T) {
		resp := call("search[ids]=bad&search[store_id]=" + sid)
		if resp.Status || resp.Errors["find"] == "" {
			t.Fatalf("got %+v", resp)
		}
	})
}

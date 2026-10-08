//go:build integration

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// API test for GET /v1/order as used by the quotation form's "Import > From Sales".
// Run: go test -tags integration ./controller/ -run TestSalesImportAPI

type listOrderResponse struct {
	Status     bool              `json:"status"`
	TotalCount int64             `json:"total_count"`
	Result     []models.Order    `json:"result"`
	Errors     map[string]string `json:"errors"`
}

func TestSalesImportAPI(t *testing.T) {
	f := setupQuotationImportFixture(t) // store + access token
	sid := f.store.ID.Hex()
	custA, custB := primitive.NewObjectID(), primitive.NewObjectID()

	coll := db.GetDB("store_" + sid).Collection("order")
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	mk := func(code string, cust primitive.ObjectID, mins int) models.Order {
		created := base.Add(time.Duration(mins) * time.Minute)
		c := cust
		return models.Order{ID: primitive.NewObjectID(), Code: code, Date: &created, StoreID: &f.store.ID, CustomerID: &c,
			CustomerName: "cust-" + code, NetTotal: 20, CreatedAt: &created,
			Products: []models.OrderProduct{{ProductID: primitive.NewObjectID(), Name: "Item " + code, Quantity: 2, UnitPrice: 9, UnitPriceWithVAT: 10.35, UnitDiscount: 1}}}
	}
	if _, err := coll.InsertMany(context.Background(), []interface{}{
		mk("SI-API-001", custA, 1), mk("SI-API-002", custB, 2), mk("SI-API-DEL", custA, 3),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := coll.UpdateOne(context.Background(), bson.M{"code": "SI-API-DEL"}, bson.M{"$set": bson.M{"deleted": true}}); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })

	call := func(token, query string) (int, listOrderResponse) {
		req := httptest.NewRequest(http.MethodGet, "/v1/order?"+query, nil)
		if token != "" {
			req.Header.Set("Authorization", token)
		}
		w := httptest.NewRecorder()
		ListOrder(w, req)
		var resp listOrderResponse
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
		if !resp.Status || len(resp.Result) != 2 || resp.Result[0].Code != "SI-API-002" || resp.Result[1].Code != "SI-API-001" || resp.TotalCount != 2 {
			t.Fatalf("got %+v", resp)
		}
	})
	t.Run("filters by customer", func(t *testing.T) {
		_, resp := call(f.token, picker+"&search[customer_id]="+custA.Hex())
		if len(resp.Result) != 1 || resp.Result[0].Code != "SI-API-001" {
			t.Fatalf("got %+v", resp.Result)
		}
	})
	t.Run("returns product lines with sale prices", func(t *testing.T) {
		_, resp := call(f.token, picker+"&search[code]=SI-API-001")
		if len(resp.Result) != 1 || len(resp.Result[0].Products) != 1 {
			t.Fatalf("got %+v", resp.Result)
		}
		p := resp.Result[0].Products[0]
		if p.Quantity != 2 || p.UnitPrice != 9 || p.UnitPriceWithVAT != 10.35 || p.UnitDiscount != 1 {
			t.Errorf("got %+v", p)
		}
	})
	t.Run("invalid customer id reports a find error", func(t *testing.T) {
		_, resp := call(f.token, picker+"&search[customer_id]=bad")
		if resp.Status || resp.Errors["find"] == "" {
			t.Fatalf("got %+v", resp)
		}
	})
}

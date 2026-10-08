//go:build integration

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// API tests for GET /v1/quotation as used by the quotation form's
// "Import > From Quotations" picker. They run the real handler against MongoDB
// and Redis:
//
//	go test -tags integration ./controller/ -run TestQuotationImportAPI
//
// Uses the same env vars as models/integration_test.go (MONGO_DB defaults to
// startpos_integration_test).
func TestMain(m *testing.M) {
	if os.Getenv("MONGO_DB") == "" {
		os.Setenv("MONGO_DB", "startpos_integration_test")
	}
	_ = db.Client(db.GetPosDB())
	db.InitRedis()
	if os.Getenv("ACCESS_SECRET") == "" {
		os.Setenv("ACCESS_SECRET", "integration-test-secret")
	}
	os.Exit(m.Run())
}

type quotationImportFixture struct {
	store *models.Store
	token string
	custA primitive.ObjectID
	custB primitive.ObjectID
}

func setupQuotationImportFixture(t *testing.T) quotationImportFixture {
	t.Helper()
	ctx := context.Background()

	store := &models.Store{
		Name:  "Quotation Import API Test Store",
		Code:  "QIAPI-" + primitive.NewObjectID().Hex()[:6],
		Email: "qiapi@example.com",
	}
	if err := store.Insert(); err != nil {
		t.Fatalf("store.Insert: %v", err)
	}
	t.Cleanup(func() { _ = store.PermanentlyDelete() })

	email := "qiapi-" + primitive.NewObjectID().Hex() + "@example.com"
	userID := primitive.NewObjectID()
	users := db.Client("").Database(db.GetPosDB()).Collection("user")
	if _, err := users.InsertOne(ctx, bson.M{"_id": userID, "name": "QI API", "email": email, "store_ids": []primitive.ObjectID{store.ID}}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = users.DeleteOne(ctx, bson.M{"_id": userID}) })

	tok, err := models.GenerateAccesstoken(email)
	if err != nil {
		t.Fatalf("GenerateAccesstoken: %v", err)
	}

	f := quotationImportFixture{store: store, token: tok.Token, custA: primitive.NewObjectID(), custB: primitive.NewObjectID()}

	coll := db.GetDB("store_" + store.ID.Hex()).Collection("quotation")
	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	mk := func(code string, cust primitive.ObjectID, mins int, deleted bool) models.Quotation {
		created := base.Add(time.Duration(mins) * time.Minute)
		c := cust
		return models.Quotation{
			ID: primitive.NewObjectID(), Code: code, Date: &created, StoreID: &store.ID,
			CustomerID: &c, CustomerName: "cust-" + code, NetTotal: 50, Deleted: deleted, CreatedAt: &created,
			Products: []models.QuotationProduct{{ProductID: primitive.NewObjectID(), Name: "Item " + code, Quantity: 3, UnitPrice: 12.5, UnitPriceWithVAT: 14.38}},
		}
	}
	docs := []interface{}{
		mk("QT-API-001", f.custA, 1, false),
		mk("QT-API-002", f.custA, 2, false),
		mk("QT-API-003", f.custB, 3, false),
		mk("QT-API-DEL", f.custA, 4, true),
	}
	if _, err := coll.InsertMany(ctx, docs); err != nil {
		t.Fatalf("seed quotations: %v", err)
	}
	t.Cleanup(func() { _ = coll.Drop(ctx) })
	return f
}

type listQuotationResponse struct {
	Status     bool               `json:"status"`
	TotalCount int64              `json:"total_count"`
	Result     []models.Quotation `json:"result"`
	Errors     map[string]string  `json:"errors"`
}

func callListQuotation(t *testing.T, token, query string) (int, listQuotationResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/quotation?"+query, nil)
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	w := httptest.NewRecorder()
	ListQuotation(w, req)
	var resp listQuotationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body %s)", err, w.Body.String())
	}
	return w.Code, resp
}

// pickerQuery mirrors the query string SourceDocumentPicker builds.
func pickerQuery(storeID string, extra string) string {
	return "search[store_id]=" + storeID +
		"&select=id,code,date,net_total,vendor_name,vendor_id,customer_name,customer_id,products" +
		"&limit=20&page=1&sort=-created_at" + extra
}

func TestQuotationImportAPI(t *testing.T) {
	f := setupQuotationImportFixture(t)
	sid := f.store.ID.Hex()

	tests := []struct {
		name       string
		token      string
		query      string
		wantHTTP   int
		wantStatus bool
		wantCodes  []string
		wantTotal  int64
		wantErrKey string
	}{
		{"no token is rejected", "", pickerQuery(sid, ""), http.StatusUnauthorized, false, nil, 0, "access_token"},
		{"bad token is rejected", "not-a-jwt", pickerQuery(sid, ""), http.StatusUnauthorized, false, nil, 0, "access_token"},
		{"missing store id", f.token, "select=id,code&limit=20", http.StatusOK, false, nil, 0, "store_id"},
		{"invalid store id", f.token, "search[store_id]=xyz", http.StatusOK, false, nil, 0, "store_id"},
		{"lists newest first without deleted", f.token, pickerQuery(sid, ""), http.StatusOK, true, []string{"QT-API-003", "QT-API-002", "QT-API-001"}, 3, ""},
		{"filters by code", f.token, pickerQuery(sid, "&search[code]=api-002"), http.StatusOK, true, []string{"QT-API-002"}, 1, ""},
		{"filters by customer list", f.token, pickerQuery(sid, "&search[customer_id]="+f.custA.Hex()), http.StatusOK, true, []string{"QT-API-002", "QT-API-001"}, 2, ""},
		{"total count ignores the page size", f.token, "search[store_id]=" + sid + "&limit=1&page=2&sort=-created_at", http.StatusOK, true, []string{"QT-API-002"}, 3, ""},
		{"invalid customer id reports a find error", f.token, pickerQuery(sid, "&search[customer_id]=bad"), http.StatusOK, false, nil, 0, "find"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, resp := callListQuotation(t, tc.token, tc.query)
			if code != tc.wantHTTP {
				t.Fatalf("HTTP status: got %d, want %d", code, tc.wantHTTP)
			}
			if resp.Status != tc.wantStatus {
				t.Fatalf("status: got %v, want %v (errors %v)", resp.Status, tc.wantStatus, resp.Errors)
			}
			if tc.wantErrKey != "" {
				if _, ok := resp.Errors[tc.wantErrKey]; !ok {
					t.Errorf("expected error key %q, got %v", tc.wantErrKey, resp.Errors)
				}
				return
			}
			codes := []string{}
			for _, q := range resp.Result {
				codes = append(codes, q.Code)
			}
			if len(codes) != len(tc.wantCodes) {
				t.Fatalf("codes: got %v, want %v", codes, tc.wantCodes)
			}
			for i := range codes {
				if codes[i] != tc.wantCodes[i] {
					t.Fatalf("codes: got %v, want %v", codes, tc.wantCodes)
				}
			}
			if resp.TotalCount != tc.wantTotal {
				t.Errorf("total_count: got %d, want %d", resp.TotalCount, tc.wantTotal)
			}
		})
	}

	t.Run("returns product lines for the picker", func(t *testing.T) {
		_, resp := callListQuotation(t, f.token, pickerQuery(sid, "&search[code]=QT-API-001"))
		if len(resp.Result) != 1 || len(resp.Result[0].Products) != 1 {
			t.Fatalf("expected one quotation with one product, got %+v", resp.Result)
		}
		p := resp.Result[0].Products[0]
		if p.Name != "Item QT-API-001" || p.Quantity != 3 || p.UnitPrice != 12.5 || p.UnitPriceWithVAT != 14.38 {
			t.Errorf("product line: %+v", p)
		}
	})
}

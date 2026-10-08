//go:build integration

package models

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// These tests cover the GET /v1/quotation query that the quotation form's
// "Import > From Quotations" picker sends:
//
//	search[store_id], search[code], search[customer_id]=id1,id2,
//	select=id,code,date,net_total,customer_name,customer_id,products,
//	limit, page, sort=-created_at

func seedImportQuotations(t *testing.T, store *Store, custA, custB primitive.ObjectID) {
	t.Helper()
	coll := db.GetDB("store_" + store.ID.Hex()).Collection("quotation")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(code string, cust *primitive.ObjectID, mins int, deleted bool, products []QuotationProduct) Quotation {
		created := base.Add(time.Duration(mins) * time.Minute)
		return Quotation{
			ID:           primitive.NewObjectID(),
			Code:         code,
			Date:         &created,
			StoreID:      &store.ID,
			CustomerID:   cust,
			CustomerName: "cust-" + code,
			Products:     products,
			NetTotal:     100,
			Deleted:      deleted,
			CreatedAt:    &created,
		}
	}
	productID := primitive.NewObjectID()
	docs := []interface{}{
		mk("QT-IMP-001", &custA, 1, false, []QuotationProduct{{
			ProductID: productID, Name: "Oil Filter", NameInArabic: "فلتر", ItemCode: "IC-1",
			PrefixPartNumber: "AB", PartNumber: "100", Quantity: 2, Unit: "pcs",
			UnitPrice: 10, UnitPriceWithVAT: 11.5, PurchaseUnitPrice: 6, PurchaseUnitPriceWithVAT: 6.9,
			UnitDiscount: 1, UnitDiscountWithVAT: 1.15, UnitDiscountPercent: 10, UnitDiscountPercentWithVAT: 10,
		}}),
		mk("QT-IMP-002", &custA, 2, false, []QuotationProduct{{ProductID: primitive.NewObjectID(), Name: "Air Filter", Quantity: 1, UnitPrice: 20}}),
		mk("QT-IMP-003", &custB, 3, false, nil),
		mk("QT-IMP-004", nil, 4, false, nil),
		mk("QT-IMP-DEL", &custA, 5, true, nil),
	}
	if _, err := coll.InsertMany(context.Background(), docs); err != nil {
		t.Fatalf("seed InsertMany: %v", err)
	}
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })
}

func searchQuotationCodes(t *testing.T, store *Store, query string) ([]Quotation, []string, error) {
	t.Helper()
	r := httptest.NewRequest("GET", "/v1/quotation?"+query, nil)
	w := httptest.NewRecorder()
	quotations, _, err := store.SearchQuotation(w, r)
	codes := []string{}
	for _, q := range quotations {
		codes = append(codes, q.Code)
	}
	return quotations, codes, err
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestQuotationImportSearch(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { _ = store.PermanentlyDelete() })
	custA, custB := primitive.NewObjectID(), primitive.NewObjectID()
	seedImportQuotations(t, store, custA, custB)

	sel := "select=id,code,date,net_total,customer_name,customer_id,products"

	tests := []struct {
		name    string
		query   string
		want    []string
		wantErr bool
	}{
		{"newest first, deleted excluded", sel + "&limit=20&page=1&sort=-created_at", []string{"QT-IMP-004", "QT-IMP-003", "QT-IMP-002", "QT-IMP-001"}, false},
		{"plain created_at sorts oldest first", sel + "&limit=20&sort=created_at", []string{"QT-IMP-001", "QT-IMP-002", "QT-IMP-003", "QT-IMP-004"}, false},
		{"code partial, case-insensitive", sel + "&sort=-created_at&search[code]=qt-imp-00", []string{"QT-IMP-004", "QT-IMP-003", "QT-IMP-002", "QT-IMP-001"}, false},
		{"code exact", sel + "&search[code]=QT-IMP-002", []string{"QT-IMP-002"}, false},
		{"code no match", sel + "&search[code]=NOPE", []string{}, false},
		{"single customer", sel + "&sort=-created_at&search[customer_id]=" + custA.Hex(), []string{"QT-IMP-002", "QT-IMP-001"}, false},
		{"multiple customers", sel + "&sort=-created_at&search[customer_id]=" + custA.Hex() + "," + custB.Hex(), []string{"QT-IMP-003", "QT-IMP-002", "QT-IMP-001"}, false},
		{"unknown customer matches quotations without customer", sel + "&search[customer_id]=unknown_customer", []string{"QT-IMP-004"}, false},
		{"customer and code combined", sel + "&search[customer_id]=" + custA.Hex() + "&search[code]=001", []string{"QT-IMP-001"}, false},
		{"page size 2, page 2", sel + "&sort=-created_at&limit=2&page=2", []string{"QT-IMP-002", "QT-IMP-001"}, false},
		{"page past the end", sel + "&sort=-created_at&limit=2&page=5", []string{}, false},
		{"invalid customer id", sel + "&search[customer_id]=not-an-id", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, codes, err := searchQuotationCodes(t, store, tc.query)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got codes %v", codes)
				}
				return
			}
			if err != nil {
				t.Fatalf("SearchQuotation: %v", err)
			}
			if !equalStrings(codes, tc.want) {
				t.Errorf("codes: got %v, want %v", codes, tc.want)
			}
		})
	}
}

func TestQuotationImportSearch_SelectReturnsProductLines(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { _ = store.PermanentlyDelete() })
	custA, custB := primitive.NewObjectID(), primitive.NewObjectID()
	seedImportQuotations(t, store, custA, custB)

	quotations, _, err := searchQuotationCodes(t, store, "select=id,code,date,net_total,customer_name,customer_id,products&search[code]=QT-IMP-001")
	if err != nil {
		t.Fatalf("SearchQuotation: %v", err)
	}
	if len(quotations) != 1 {
		t.Fatalf("got %d quotations, want 1", len(quotations))
	}
	q := quotations[0]
	if q.CustomerName != "cust-QT-IMP-001" || q.NetTotal != 100 || q.Date == nil {
		t.Errorf("header fields not selected: %+v", q)
	}
	if len(q.Products) != 1 {
		t.Fatalf("got %d products, want 1", len(q.Products))
	}
	p := q.Products[0]
	if p.Name != "Oil Filter" || p.NameInArabic != "فلتر" || p.ItemCode != "IC-1" || p.PrefixPartNumber != "AB" || p.PartNumber != "100" || p.Unit != "pcs" {
		t.Errorf("identity fields: %+v", p)
	}
	if p.Quantity != 2 || p.UnitPrice != 10 || p.UnitPriceWithVAT != 11.5 || p.PurchaseUnitPrice != 6 || p.PurchaseUnitPriceWithVAT != 6.9 {
		t.Errorf("price fields: %+v", p)
	}
	if p.UnitDiscount != 1 || p.UnitDiscountWithVAT != 1.15 || p.UnitDiscountPercent != 10 || p.UnitDiscountPercentWithVAT != 10 {
		t.Errorf("discount fields: %+v", p)
	}

	// Fields outside the select list stay empty, keeping the picker payload small.
	if q.StoreID != nil {
		t.Errorf("store_id should not be selected, got %v", q.StoreID)
	}
}

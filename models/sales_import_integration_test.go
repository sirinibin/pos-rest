//go:build integration

package models

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Covers GET /v1/order as the quotation form's "Import > From Sales" picker
// calls it (search[store_id], search[code], search[customer_id]=id1,id2,
// select=...,products, limit, page, sort=-created_at).

func TestSalesImportSearch(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { _ = store.PermanentlyDelete() })
	custA, custB := primitive.NewObjectID(), primitive.NewObjectID()

	coll := db.GetDB("store_" + store.ID.Hex()).Collection("order")
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	mk := func(code string, cust *primitive.ObjectID, mins int, products []OrderProduct) Order {
		created := base.Add(time.Duration(mins) * time.Minute)
		return Order{ID: primitive.NewObjectID(), Code: code, Date: &created, StoreID: &store.ID, CustomerID: cust,
			CustomerName: "cust-" + code, NetTotal: 60, CreatedAt: &created, Products: products}
	}
	docs := []interface{}{
		mk("SI-IMP-001", &custA, 1, []OrderProduct{{
			ProductID: primitive.NewObjectID(), Name: "Oil Filter", NameInArabic: "فلتر", ItemCode: "IC-1", PrefixPartNumber: "AB",
			PartNumber: "100", Quantity: 3, Unit: "pcs", UnitPrice: 10, UnitPriceWithVAT: 11.5, PurchaseUnitPrice: 6,
			UnitDiscount: 1, UnitDiscountWithVAT: 1.15, UnitDiscountPercent: 10, UnitDiscountPercentWithVAT: 10,
		}}),
		mk("SI-IMP-002", &custA, 2, nil),
		mk("SI-IMP-003", &custB, 3, nil),
		mk("SI-IMP-DEL", &custA, 4, nil),
	}
	if _, err := coll.InsertMany(context.Background(), docs); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Order has no Deleted field in Go, so soft-delete the row directly.
	if _, err := coll.UpdateOne(context.Background(), bson.M{"code": "SI-IMP-DEL"}, bson.M{"$set": bson.M{"deleted": true}}); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })

	sel := "select=id,code,date,net_total,vendor_name,vendor_id,customer_name,customer_id,products"
	search := func(query string) ([]Order, []string, error) {
		r := httptest.NewRequest("GET", "/v1/order?"+query, nil)
		orders, _, err := store.SearchOrder(httptest.NewRecorder(), r)
		codes := []string{}
		for _, o := range orders {
			codes = append(codes, o.Code)
		}
		return orders, codes, err
	}

	tests := []struct {
		name    string
		query   string
		want    []string
		wantErr bool
	}{
		{"newest first, deleted excluded", sel + "&limit=20&page=1&sort=-created_at", []string{"SI-IMP-003", "SI-IMP-002", "SI-IMP-001"}, false},
		{"code partial, case-insensitive", sel + "&sort=-created_at&search[code]=si-imp-00", []string{"SI-IMP-003", "SI-IMP-002", "SI-IMP-001"}, false},
		{"code no match", sel + "&search[code]=NOPE", []string{}, false},
		{"single customer", sel + "&sort=-created_at&search[customer_id]=" + custA.Hex(), []string{"SI-IMP-002", "SI-IMP-001"}, false},
		{"several customers", sel + "&sort=-created_at&search[customer_id]=" + custA.Hex() + "," + custB.Hex(), []string{"SI-IMP-003", "SI-IMP-002", "SI-IMP-001"}, false},
		{"customer and code", sel + "&search[customer_id]=" + custA.Hex() + "&search[code]=001", []string{"SI-IMP-001"}, false},
		{"paging", sel + "&sort=-created_at&limit=2&page=2", []string{"SI-IMP-001"}, false},
		{"invalid customer id", sel + "&search[customer_id]=bad", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, codes, err := search(tc.query)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %v", codes)
				}
				return
			}
			if err != nil {
				t.Fatalf("SearchOrder: %v", err)
			}
			if !equalStrings(codes, tc.want) {
				t.Errorf("codes: got %v, want %v", codes, tc.want)
			}
		})
	}

	t.Run("selected product lines carry sale prices and discounts", func(t *testing.T) {
		orders, _, err := search(sel + "&search[code]=SI-IMP-001")
		if err != nil || len(orders) != 1 || len(orders[0].Products) != 1 {
			t.Fatalf("got %+v, err %v", orders, err)
		}
		p := orders[0].Products[0]
		if p.Name != "Oil Filter" || p.NameInArabic != "فلتر" || p.ItemCode != "IC-1" || p.PrefixPartNumber != "AB" || p.PartNumber != "100" || p.Unit != "pcs" || p.Quantity != 3 {
			t.Errorf("identity fields: %+v", p)
		}
		if p.UnitPrice != 10 || p.UnitPriceWithVAT != 11.5 || p.UnitDiscount != 1 || p.UnitDiscountWithVAT != 1.15 || p.UnitDiscountPercent != 10 {
			t.Errorf("price fields: %+v", p)
		}
	})
}

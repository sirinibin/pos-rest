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

// Covers GET /v1/purchase as the quotation form's "Import > From Purchases"
// picker calls it (search[store_id], search[code], search[vendor_id]=id1,id2,
// select=...,products, limit, page, sort=-created_at).

func TestPurchaseImportSearch(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { _ = store.PermanentlyDelete() })
	vendA, vendB := primitive.NewObjectID(), primitive.NewObjectID()

	coll := db.GetDB("store_" + store.ID.Hex()).Collection("purchase")
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	mk := func(code string, vendor *primitive.ObjectID, mins int, deleted bool, products []PurchaseProduct) Purchase {
		created := base.Add(time.Duration(mins) * time.Minute)
		return Purchase{ID: primitive.NewObjectID(), Code: code, Date: &created, StoreID: &store.ID, VendorID: vendor,
			VendorName: "vendor-" + code, NetTotal: 80, CreatedAt: &created, Products: products}
	}
	docs := []interface{}{
		mk("PI-IMP-001", &vendA, 1, false, []PurchaseProduct{{
			ProductID: primitive.NewObjectID(), Name: "Bolt", NameInArabic: "برغي", ItemCode: "IC-B", PrefixPartNumber: "AB",
			PartNumber: "B1", Quantity: 10, Unit: "pcs", PurchaseUnitPrice: 2, PurchaseUnitPriceWithVAT: 2.3,
			RetailUnitPrice: 3, RetailUnitPriceWithVAT: 3.45, UnitDiscount: 0.5,
		}}),
		mk("PI-IMP-002", &vendA, 2, false, nil),
		mk("PI-IMP-003", &vendB, 3, false, nil),
		mk("PI-IMP-DEL", &vendA, 4, true, nil),
	}
	if _, err := coll.InsertMany(context.Background(), docs); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Purchase has no Deleted field in Go, so soft-delete the row directly.
	if _, err := coll.UpdateOne(context.Background(), bson.M{"code": "PI-IMP-DEL"}, bson.M{"$set": bson.M{"deleted": true}}); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })

	sel := "select=id,code,date,net_total,vendor_name,vendor_id,customer_name,customer_id,products"
	search := func(query string) ([]Purchase, []string, error) {
		r := httptest.NewRequest("GET", "/v1/purchase?"+query, nil)
		purchases, _, err := store.SearchPurchase(httptest.NewRecorder(), r)
		codes := []string{}
		for _, p := range purchases {
			codes = append(codes, p.Code)
		}
		return purchases, codes, err
	}

	tests := []struct {
		name    string
		query   string
		want    []string
		wantErr bool
	}{
		{"newest first, deleted excluded", sel + "&limit=20&page=1&sort=-created_at", []string{"PI-IMP-003", "PI-IMP-002", "PI-IMP-001"}, false},
		{"code partial, case-insensitive", sel + "&sort=-created_at&search[code]=pi-imp-00", []string{"PI-IMP-003", "PI-IMP-002", "PI-IMP-001"}, false},
		{"code no match", sel + "&search[code]=NOPE", []string{}, false},
		{"single vendor", sel + "&sort=-created_at&search[vendor_id]=" + vendA.Hex(), []string{"PI-IMP-002", "PI-IMP-001"}, false},
		{"several vendors", sel + "&sort=-created_at&search[vendor_id]=" + vendA.Hex() + "," + vendB.Hex(), []string{"PI-IMP-003", "PI-IMP-002", "PI-IMP-001"}, false},
		{"vendor and code", sel + "&search[vendor_id]=" + vendA.Hex() + "&search[code]=002", []string{"PI-IMP-002"}, false},
		{"paging", sel + "&sort=-created_at&limit=2&page=2", []string{"PI-IMP-001"}, false},
		{"invalid vendor id", sel + "&search[vendor_id]=bad", nil, true},
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
				t.Fatalf("SearchPurchase: %v", err)
			}
			if !equalStrings(codes, tc.want) {
				t.Errorf("codes: got %v, want %v", codes, tc.want)
			}
		})
	}

	t.Run("selected product lines carry cost and saved retail prices", func(t *testing.T) {
		purchases, _, err := search(sel + "&search[code]=PI-IMP-001")
		if err != nil || len(purchases) != 1 || len(purchases[0].Products) != 1 {
			t.Fatalf("got %+v, err %v", purchases, err)
		}
		p := purchases[0].Products[0]
		if p.Name != "Bolt" || p.NameInArabic != "برغي" || p.ItemCode != "IC-B" || p.PrefixPartNumber != "AB" || p.PartNumber != "B1" || p.Unit != "pcs" || p.Quantity != 10 {
			t.Errorf("identity fields: %+v", p)
		}
		if p.PurchaseUnitPrice != 2 || p.PurchaseUnitPriceWithVAT != 2.3 || p.RetailUnitPrice != 3 || p.RetailUnitPriceWithVAT != 3.45 {
			t.Errorf("price fields: %+v", p)
		}
		if purchases[0].VendorName != "vendor-PI-IMP-001" {
			t.Errorf("vendor_name: %q", purchases[0].VendorName)
		}
	})
}

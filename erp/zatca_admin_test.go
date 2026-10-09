package erp

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Pure-function tests for the platform-admin ZATCA re-connection tools.

func TestZatcaReconnectFilter(t *testing.T) {
	f := zatcaReconnectFilter("")
	if f["zatca.zatca_reconnect_required"] != true {
		t.Fatalf("must match marked stores only: %v", f)
	}
	if d, _ := f["deleted"].(bson.M); d == nil || d["$ne"] != true {
		t.Fatalf("must skip deleted stores: %v", f)
	}
	if g := zatcaReconnectFilter("   "); g["$and"] != nil {
		t.Fatalf("blank q adds no search: %v", g)
	}
	g := zatcaReconnectFilter(" Gulf (Riyadh) ")
	and, _ := g["$and"].(bson.A)
	if len(and) != 2 {
		t.Fatalf("q must be ANDed with the mark: %v", g)
	}
	or, _ := and[1].(bson.M)["$or"].(bson.A)
	if len(or) != 5 {
		t.Fatalf("q searches name, Arabic name, code, VAT and CR: %v", and[1])
	}
	rx := or[0].(bson.M)["name"].(primitive.Regex)
	if rx.Pattern != `Gulf \(Riyadh\)` || rx.Options != "i" {
		t.Fatalf("q must be trimmed, escaped and case-insensitive: %+v", rx)
	}
}

func TestZatcaReconnectLimit(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		bad  bool
	}{
		{"", 50, false}, {" ", 50, false}, {"1", 1, false}, {"100", 100, false}, {" 20 ", 20, false},
		{"0", 0, true}, {"-3", 0, true}, {"101", 0, true}, {"abc", 0, true}, {"2.5", 0, true},
	}
	for _, c := range cases {
		got, err := zatcaReconnectLimit(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("limit %q: want error", c.in)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("limit %q = %d, %v; want %d", c.in, got, err, c.want)
		}
	}
}

func TestZatcaReconnectRow(t *testing.T) {
	oid := primitive.NewObjectID()
	row := zatcaReconnectRow(M{"_id": oid, "name": "Gulf Union", "name_in_arabic": "الخليج", "branch_name": "Main",
		"code": "GU", "vat_no": "300000000000003", "registration_number": "1010", "business_category": "Retail",
		"country_code": "SA", "zatca": M{"phase": "2", "env": "Production", "connected": true,
			"private_key": "SECRET", "production_secret": "SECRET"}})
	if row["id"] != oid.Hex() || row["nameEn"] != "Gulf Union" || row["nameAr"] != "الخليج" || row["vatNo"] != "300000000000003" ||
		row["crNo"] != "1010" || row["category"] != "Retail" || row["countryCode"] != "SA" || row["code"] != "GU" {
		t.Fatalf("row: %v", row)
	}
	z := row["zatca"].(M)
	if z["phase"] != "2" || z["env"] != "Production" || z["connected"] != true || z["reconnectNeeded"] != true || len(z) != 4 {
		t.Fatalf("zatca block must carry no secrets: %v", z)
	}
	if empty := zatcaReconnectRow(M{"_id": oid}); empty["countryCode"] != "SA" || empty["nameEn"] != "" {
		t.Fatalf("missing fields: %v", empty)
	}
}

// Platform-admin-only endpoints refuse a signed-in non-admin before touching
// the database (the DB-backed flow is in zatca_admin_api_test.go).
func TestZatcaAdmin_RequiresPlatformAdmin(t *testing.T) {
	if err, _ := requirePlatformAdmin(&Ctx{}).(*APIError); err == nil || err.Status != 403 {
		t.Fatalf("non-admin: %v", err)
	}
	if err := requirePlatformAdmin(&Ctx{Admin: true}); err != nil {
		t.Fatalf("admin: %v", err)
	}
}

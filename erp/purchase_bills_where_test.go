package erp

import (
	"testing"
	"time"
)

func TestPurchaseBillsWhereKeys(t *testing.T) {
	b := newPurchaseBillsResource().Backend.(*nativeBackend)
	for _, k := range []string{"status", "vendorId"} {
		if !b.whereKeys[k] {
			t.Fatalf("purchase bills cannot be filtered by %s", k)
		}
	}
	if b.whereKeys["amount"] {
		t.Fatal("amount is not a filter")
	}
}

// The sidebar badge counts new purchase bills with ?where.status=new&limit=1.
func TestAPI_PurchaseBillsByStatus(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	st := "&storeId=" + storeA()
	dt := time.Now().Format("2006-01-02T15:04")
	before := call(t, "GET", "/purchase-bills?limit=1&where.status=new"+st, tok, nil)
	if before.Code != 200 {
		t.Fatalf("where.status: %d %s", before.Code, before.Raw)
	}
	n0 := int(num(before.Body["total"]))
	mk := func(status string) string {
		r := call(t, "POST", "/purchase-bills", tok, M{"storeId": storeA(), "vendorId": fx.VendorA1.Hex(), "receivedAt": dt,
			"fileName": "b.pdf", "amount": 115, "vat": 15, "status": status})
		if r.Code != 201 && r.Code != 200 {
			t.Fatalf("create bill: %d %s", r.Code, r.Raw)
		}
		return str(r.Body["id"])
	}
	mk("new")
	mk("new")
	mk("converted")
	after := call(t, "GET", "/purchase-bills?limit=1&where.status=new"+st, tok, nil)
	if int(num(after.Body["total"])) != n0+2 {
		t.Fatalf("new bills %v, want %d", after.Body["total"], n0+2)
	}
	for _, r := range call(t, "GET", "/purchase-bills?limit=500&where.status=new"+st, tok, nil).data() {
		if s := str(r.(M)["status"]); s != "new" {
			t.Fatalf("status filter returned %q", s)
		}
	}
	if bad := call(t, "GET", "/purchase-bills?where.amount=115"+st, tok, nil); bad.Code != 400 {
		t.Fatalf("unknown filter: %d", bad.Code)
	}
}

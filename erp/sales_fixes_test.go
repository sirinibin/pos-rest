package erp

import (
	"math"
	"testing"

	"github.com/sirinibin/startpos/backend/models"
)

// NEW-line-error-masked: a line refused for its amounts skips the payments
// check, so the error is reported on the line (by linesToLegacy).
func TestBadLineAmounts(t *testing.T) {
	line := func(q, p, d float64) M { return M{"qty": q, "unitPrice": p, "unitDiscount": d} }
	for _, c := range []struct {
		items []interface{}
		bad   bool
	}{
		{[]interface{}{line(1, 5, 0)}, false},
		{[]interface{}{line(1, 0, 0)}, false},
		{[]interface{}{line(-1, 5, 0)}, true},
		{[]interface{}{line(0, 5, 0)}, true},
		{[]interface{}{line(1, -5, 0)}, true},
		{[]interface{}{line(1, 5, 6)}, true},
		{[]interface{}{line(1, 5, 0), line(2, -1, 0)}, true},
		{nil, false},
	} {
		if got := badLineAmounts(M{"items": c.items}); got != c.bad {
			t.Errorf("%v: bad=%v, want %v", c.items, got, c.bad)
		}
	}
	b := newSalesResource().Backend.(*legacyBackend)
	sid := hexID()
	x := testX(sid, M{"_id": sid, "vat_percent": 15.0})
	rec := M{"date": "2026-10-05T11:30", "items": []interface{}{line(-1, 5, 0)},
		"payments": []interface{}{M{"amount": 5.75, "method": "cash"}}}
	if e := b.validate(x, rec, nil); e["payments"] != "" {
		t.Errorf("a negative line is reported on payments: %v", e)
	}
}

// NEW-idem-scope: an Idempotency-Key is scoped to the user, method and path.
func TestIdemDocID(t *testing.T) {
	a := idemDocID("u1", "POST", "/v1/erp/sales", "k")
	for _, other := range []string{
		idemDocID("u1", "POST", "/v1/erp/sales-returns", "k"),
		idemDocID("u1", "PATCH", "/v1/erp/sales", "k"),
		idemDocID("u2", "POST", "/v1/erp/sales", "k"),
		idemDocID("u1", "POST", "/v1/erp/sales", "k2"),
	} {
		if other == a {
			t.Errorf("%q is not scoped", other)
		}
	}
	if a != idemDocID("u1", "POST", "/v1/erp/sales", "k") {
		t.Error("same request, other id")
	}
}

// NEW-sales-unknown-customer / NEW-foreign-customer: a customer id that is
// not one of the store's customers is refused (another store's customers live
// in another store DB).
func TestRefCustomerMustExist(t *testing.T) {
	sid := hexID()
	x := testX(sid, M{"_id": sid})
	known := testCustomer(x)
	unknown := hexID()
	x.cache["customer|"+unknown] = nil // looked up: not in this store
	if h, err := x.ref("customer", known); err != nil || h != known {
		t.Errorf("known customer: %v %v", h, err)
	}
	if _, err := x.ref("customer", unknown); err == nil {
		t.Error("unknown customer accepted")
	}
	if h, err := x.ref("customer", ""); err != nil || h != nil {
		t.Errorf("no customer: %v %v", h, err)
	}
	b := newSalesResource().Backend.(*legacyBackend)
	_, err := b.toL(x, M{"date": "2026-10-05T11:30", "customerId": unknown, "items": []interface{}{}}, nil, knownSet("customerId"), false)
	if ae, ok := err.(*APIError); !ok || ae.Fields["customerId"] == "" {
		t.Errorf("unknown customer on a sale: %v", err)
	}
	v := newVehiclesResource().Backend.(*legacyBackend)
	_, err = v.toL(x, M{"plate": "ABJ 1234", "make": "Toyota", "customerId": unknown}, nil, knownSet("plate", "make", "customerId"), true)
	if ae, ok := err.(*APIError); !ok || ae.Fields["customerId"] == "" {
		t.Errorf("unknown customer on a vehicle: %v", err)
	}
}

// NEW-vat-half-cent / NEW-fin-3dec: money rounds half away from zero on the
// decimal value, at the currency's decimals; the adapter and the legacy save
// agree.
func TestMoneyRoundingDecimals(t *testing.T) {
	for _, c := range []struct {
		x    float64
		d    int
		want float64
	}{
		{3.3 * 0.15, 2, 0.50}, {0.495, 2, 0.50}, {-0.495, 2, -0.50}, {1.005, 2, 1.01}, {0.494999, 2, 0.49},
		{3.717 * 0.05, 3, 0.186}, {0.0005, 3, 0.001}, {1.2345, 3, 1.235}, {2.0004, 3, 2.0}, {-1.2345, 3, -1.235},
	} {
		if got := roundD(c.x, c.d); got != c.want {
			t.Errorf("roundD(%v, %d) = %v, want %v", c.x, c.d, got, c.want)
		}
		if got := models.RoundMoney(c.x, c.d); got != c.want {
			t.Errorf("models.RoundMoney(%v, %d) = %v, want %v", c.x, c.d, got, c.want)
		}
	}
	if models.RoundTo2Decimals(3.3*0.15) != 0.5 {
		t.Error("legacy RoundTo2Decimals still rounds 0.495 down")
	}
	rec := M{"items": []interface{}{M{"qty": 3.0, "unitPrice": 1.239}}}
	if got := ksTotalsD(rec, 5, 3); math.Abs(got-3.903) > 1e-9 {
		t.Errorf("3-decimal net %v, want 3.903", got)
	}
	if got := legacyBeforeRoundingD(rec, 5, 3); math.Abs(got-3.903) > 1e-9 {
		t.Errorf("3-decimal legacy net %v, want 3.903", got)
	}
	if fix := roundingCentFixD(rec, 5, 3); fix != 0 {
		t.Errorf("3-decimal cent fix %v", fix)
	}
	om := models.CountryProfileFor("OM")
	if om == nil || om.Decimals != 3 {
		t.Fatalf("Oman profile: %v", om)
	}
	if r := autoRounding(3.903, om); math.Abs(r-(-0.003)) > 1e-9 {
		t.Errorf("Oman auto rounding of 3.903 = %v, want -0.003", r)
	}
	if x := testX("", M{"country_code": "OM"}); x.decimals() != 3 {
		t.Errorf("Oman store decimals %d", x.decimals())
	}
	if x := testX("", M{}); x.decimals() != 2 {
		t.Errorf("Saudi store decimals %d", x.decimals())
	}
}

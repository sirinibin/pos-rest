package erp

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// docLineErrors reports bad lines under their own fields (a purchase used to
// report them as a payments error, NEW-PUR-PAY-MASK).
func TestDocLineErrors(t *testing.T) {
	for _, c := range []struct {
		name  string
		item  M
		field string
	}{
		{"negative price", M{"qty": 1, "unitPrice": -2}, "items.0.unitPrice"},
		{"negative qty", M{"qty": -1, "unitPrice": 5}, "items.0.qty"},
		{"zero qty", M{"qty": 0, "unitPrice": 5}, "items.0.qty"},
		{"discount above price", M{"qty": 1, "unitPrice": 5, "unitDiscount": 6}, "items.0.unitDiscount"},
		{"good line", M{"qty": 1, "unitPrice": 5, "unitDiscount": 1}, ""},
	} {
		e := docLineErrors(M{"items": []interface{}{c.item}})
		if c.field == "" && len(e) != 0 {
			t.Errorf("%s: unexpected errors %v", c.name, e)
		}
		if c.field != "" && e[c.field] == "" {
			t.Errorf("%s: want %s, got %v", c.name, c.field, e)
		}
	}
}

// Unknown or other-company ids are refused with the contract field
// (NEW-PUR-UNKNOWN-VENDOR, NEW-BILL-VENDOR, NEW-fin-expense-vendor,
// NEW-fin-note-customer), and legacy error keys are mapped
// (NEW-fin-expcat-field).
func TestFix_UnknownReferencesRefused(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	sA, dt, ms := storeA(), "2026-10-05T10:00", "ms_"+storeA()
	ghost := primitive.NewObjectID().Hex()
	for _, c := range []struct {
		path, field string
		body        M
	}{
		{"purchases", "vendorId", M{"storeId": sA, "date": dt, "vendorId": ghost,
			"items": []M{{"productId": fx.ProductA2.Hex(), "qty": 1, "unitPrice": 5, "warehouseId": ms}}}},
		{"purchase-bills", "vendorId", M{"storeId": sA, "vendorId": ghost, "receivedAt": dt, "fileName": "b.pdf", "amount": 1}},
		{"expenses", "vendorId", M{"storeId": sA, "date": dt, "categoryId": fx.ExpenseCatA.Hex(), "description": "x", "amount": 5,
			"method": "cash", "vendorId": ghost}},
		{"expenses", "categoryId", M{"storeId": sA, "date": dt, "categoryId": ghost, "description": "x", "amount": 5, "method": "cash"}},
		{"deposits", "customerId", M{"storeId": sA, "date": dt, "customerId": ghost, "amount": 5, "method": "cash"}},
		{"withdrawals", "customerId", M{"storeId": sA, "date": dt, "customerId": ghost, "amount": 5, "method": "cash"}},
		{"products", "categoryIds", M{"storeId": sA, "nameEn": "Ghost category item", "categoryIds": []string{ghost}}},
		{"products", "components", M{"storeId": sA, "nameEn": "Ghost kit", "isSet": true, "components": []M{{"productId": ghost, "qty": 1}}}},
		{"products", "stock", M{"storeId": sA, "nameEn": "Foreign main store", "stock": M{"ms_" + ghost: M{"qty": 1}}}},
	} {
		r := call(t, "POST", "/"+c.path, tok, c.body)
		if r.Code != 400 || r.errField(c.field) == "" {
			t.Errorf("%s with an unknown %s: want 400 %s, got %d %s", c.path, c.field, c.field, r.Code, r.Raw)
		}
	}
	// a bad purchase line is reported under the line, not as payments
	r := call(t, "POST", "/purchases", tok, M{"storeId": sA, "date": dt, "vendorId": fx.VendorA1.Hex(),
		"items": []M{{"productId": fx.ProductA2.Hex(), "qty": 1, "unitPrice": -2, "warehouseId": ms}}})
	if r.Code != 400 || r.errField("items.0.unitPrice") == "" || r.errField("payments") != "" {
		t.Errorf("negative line price: %d %s", r.Code, r.Raw)
	}
}

// The VAT entered on an expense is kept, with or without a vendor
// (NEW-fin-expense-vat).
func TestFix_ExpenseVATKept(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	for _, vendor := range []interface{}{nil, fx.VendorA1.Hex()} {
		r := call(t, "POST", "/expenses", tok, M{"storeId": storeA(), "date": "2026-10-05T10:00", "categoryId": fx.ExpenseCatA.Hex(),
			"description": "Power", "amount": 100, "vatAmount": 7.5, "method": "cash", "vendorId": vendor})
		if r.Code != 201 || num(r.Body["amount"]) != 100 || num(r.Body["vatAmount"]) != 7.5 {
			t.Fatalf("expense (vendor %v): %d %s", vendor, r.Code, r.Raw)
		}
		p := call(t, "PATCH", "/expenses/"+str(r.Body["id"]), tok, M{"description": "Power bill"}, "If-Match", str(r.Body["version"]))
		if p.Code != 200 || num(p.Body["vatAmount"]) != 7.5 {
			t.Errorf("expense edit (vendor %v) changed the VAT: %d %s", vendor, p.Code, p.Raw)
		}
	}
}

// A warehouse keeps the code sent on create; a used code is refused
// (NEW-WH-CODE). A product barcode belongs to one product (NEW-BARCODE-DUP).
func TestFix_WarehouseCodeAndBarcode(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	code := "FX" + uniq("")[7:]
	code = strings.ReplaceAll(code, ".", "")
	r := call(t, "POST", "/warehouses", tok, M{"storeId": storeA(), "nameEn": "Fix WH", "code": code})
	if r.Code != 201 || str(r.Body["code"]) != code {
		t.Fatalf("warehouse code: %d %s", r.Code, r.Raw)
	}
	if d := call(t, "POST", "/warehouses", tok, M{"storeId": storeA(), "nameEn": "Fix WH 2", "code": code}); d.Code != 400 || d.errField("code") == "" {
		t.Errorf("duplicate warehouse code: %d %s", d.Code, d.Raw)
	}
	bc := "9" + strings.ReplaceAll(uniq(""), ".", "")[:11]
	p := call(t, "POST", "/products", tok, M{"storeId": storeA(), "nameEn": "Barcode one", "barcode": bc})
	if p.Code != 201 {
		t.Fatalf("product: %d %s", p.Code, p.Raw)
	}
	if d := call(t, "POST", "/products", tok, M{"storeId": storeA(), "nameEn": "Barcode two", "barcode": bc}); d.Code != 400 || d.errField("barcode") == "" {
		t.Errorf("duplicate barcode: %d %s", d.Code, d.Raw)
	}
	// the product itself keeps its barcode on edit
	if e := call(t, "PATCH", "/products/"+str(p.Body["id"]), tok, M{"note": "x", "barcode": bc}, "If-Match", str(p.Body["version"])); e.Code != 200 {
		t.Errorf("edit with own barcode: %d %s", e.Code, e.Raw)
	}
}

// Capital withdrawals have their own CWD- series and are posted to the
// ledger; a delete takes the posting away (NEW-fin-capwd-number,
// NEW-fin-capwd-ledger, NEW-fin-delete-ledger).
func TestFix_CapitalWithdrawalSeriesAndLedger(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.AdminEmail)
	r := call(t, "POST", "/capital-withdrawals", tok, M{"storeId": storeA(), "date": "2026-10-05T10:00", "investor": "Admin T1", "amount": 50, "method": "cash"})
	if r.Code != 201 || !strings.HasPrefix(str(r.Body["code"]), "CWD-") {
		t.Fatalf("capital withdrawal: %d %s", r.Code, r.Raw)
	}
	oid, _ := primitive.ObjectIDFromHex(str(r.Body["id"]))
	ledgers := func() int64 {
		ctx, cancel := dbctx()
		defer cancel()
		n, _ := storeDB(storeA()).Collection("ledger").CountDocuments(ctx, bson.M{"reference_id": oid})
		return n
	}
	if ledgers() != 1 {
		t.Fatalf("capital withdrawal not posted to the ledger")
	}
	if d := call(t, "DELETE", "/capital-withdrawals/"+str(r.Body["id"]), tok, nil); d.Code != 200 {
		t.Fatalf("delete: %d %s", d.Code, d.Raw)
	}
	if ledgers() != 0 {
		t.Errorf("deleted capital withdrawal still on the ledger")
	}
	// deposits too
	dep := call(t, "POST", "/deposits", tok, M{"storeId": storeA(), "date": "2026-10-05T10:00", "customerId": fx.CustomerA1.Hex(), "amount": 12, "method": "cash"})
	if dep.Code != 201 {
		t.Fatalf("deposit: %d %s", dep.Code, dep.Raw)
	}
	doid, _ := primitive.ObjectIDFromHex(str(dep.Body["id"]))
	if d := call(t, "DELETE", "/deposits/"+str(dep.Body["id"]), tok, nil); d.Code != 200 {
		t.Fatalf("delete deposit: %d %s", d.Code, d.Raw)
	}
	ctx, cancel := dbctx()
	defer cancel()
	if n, _ := storeDB(storeA()).Collection("posting").CountDocuments(ctx, bson.M{"reference_id": doid}); n != 0 {
		t.Errorf("deleted deposit keeps %d postings", n)
	}
}

// A PUT without code keeps a native record's number (NEW-NATIVE-PUT-RENUMBER).
func TestFix_NativePutKeepsCode(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	b := M{"storeId": storeA(), "vendorId": fx.VendorA1.Hex(), "receivedAt": "2026-10-05T10:00", "fileName": "b.pdf", "amount": 10, "status": "new"}
	r := call(t, "POST", "/purchase-bills", tok, b)
	if r.Code != 201 || str(r.Body["code"]) == "" {
		t.Fatalf("bill: %d %s", r.Code, r.Raw)
	}
	delete(b, "storeId")
	b["amount"] = 11
	p := call(t, "PUT", "/purchase-bills/"+str(r.Body["id"]), tok, b, "If-Match", str(r.Body["version"]))
	if p.Code != 200 || str(p.Body["code"]) != str(r.Body["code"]) {
		t.Errorf("PUT renumbered the bill: %v -> %d %s", r.Body["code"], p.Code, p.Raw)
	}
}

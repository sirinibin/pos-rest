package erp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/controller"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Integration / old-data compatibility tests (opt-in, ERP_TEST_DB=1).
//
// They compare what the adapter does with what the OLD app does through the
// existing v1 handlers (hand-written legacy payloads, exactly the shape the
// old Vue client posts): Redis counters, stock, ledger postings, and that the
// v1 read endpoints return adapter-written documents correctly.

// v1 calls an existing v1 handler the way the old app does (HTTP + legacy
// access token). It returns the legacy envelope.
func v1(t *testing.T, h http.HandlerFunc, method, path string, vars map[string]string, q url.Values, body interface{}, tok string) M {
	t.Helper()
	var rd *strings.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	} else {
		rd = strings.NewReader("")
	}
	target := path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	req := httptest.NewRequest(method, target, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	if vars != nil {
		req = mux.SetURLVars(req, vars)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	var out M
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("v1 %s %s: %d %s", method, target, rec.Code, rec.Body.String())
	}
	return out
}

func v1OK(t *testing.T, what string, env M) M {
	t.Helper()
	if env["status"] != true {
		t.Fatalf("v1 %s failed: %v", what, env)
	}
	r, _ := env["result"].(map[string]interface{})
	return M(r)
}

func storeQ(s string) url.Values { return url.Values{"search[store_id]": {s}} }

func redisCounter(t *testing.T, key string) int64 {
	t.Helper()
	v, err := db.RedisClient.Get(key).Result()
	if err != nil {
		return -1
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

func rawDoc(t *testing.T, storeHex, coll, idHex string) M {
	t.Helper()
	oid, _ := primitive.ObjectIDFromHex(idHex)
	ctx, cancel := dbctx()
	defer cancel()
	var d bson.M
	if err := storeDB(storeHex).Collection(coll).FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
		t.Fatalf("raw %s/%s: %v", coll, idHex, err)
	}
	return normDoc(d)
}

func rawStock(t *testing.T, storeHex, productHex string) float64 {
	t.Helper()
	d := rawDoc(t, storeHex, "product", productHex)
	return num(get(d, "product_stores."+storeHex+".stock"))
}

// ledgerSig is the order-independent signature of all ledger postings of a
// reference: "account|debit|credit" per journal line.
func ledgerSig(t *testing.T, storeHex, refHex string) []string {
	t.Helper()
	oid, _ := primitive.ObjectIDFromHex(refHex)
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := storeDB(storeHex).Collection("ledger").Find(ctx, bson.M{"reference_id": oid})
	if err != nil {
		t.Fatal(err)
	}
	var docs []bson.M
	_ = cur.All(ctx, &docs)
	var sig []string
	for _, d := range docs {
		for _, j := range arr(d["journals"]) {
			jm := normDoc(j)
			sig = append(sig, fmt.Sprintf("%s|%.2f|%.2f", str(jm["account_name"]), num(jm["debit"]), num(jm["credit"])))
		}
	}
	sort.Strings(sig)
	return sig
}

// stableLedger waits until the ledger of a reference stops changing (legacy
// posts it from goroutines).
func stableLedger(t *testing.T, storeHex, refHex string) []string {
	t.Helper()
	var last []string
	stable := 0
	eventually(t, "ledger for "+refHex, func() bool {
		cur := ledgerSig(t, storeHex, refHex)
		if len(cur) > 0 && strings.Join(cur, ",") == strings.Join(last, ",") {
			stable++
		} else {
			stable = 0
		}
		last = cur
		return stable >= 3
	})
	return last
}

func codeSeq(t *testing.T, code string) int {
	t.Helper()
	m := regexp.MustCompile(`(\d+)$`).FindStringSubmatch(code)
	if m == nil {
		t.Fatalf("code without serial: %q", code)
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func keysOf(d M) map[string]bool {
	out := map[string]bool{}
	for k := range d {
		out[k] = true
	}
	return out
}

// legacySalePayload is what the old Vue app posts to POST /v1/order.
func legacySalePayload(store, customer, product primitive.ObjectID, qty, price, paid float64, at, payAt time.Time) M {
	return M{
		"store_id": store.Hex(), "customer_id": customer.Hex(), "date_str": at.UTC().Format(time.RFC3339), "vat_percent": 15.0,
		"discount": 0.0, "discount_with_vat": 0.0, "shipping_handling_fees": 0.0, "rounding_amount": 0.0, "auto_rounding_amount": false,
		"products": []M{{"product_id": product.Hex(), "name": "Oil Filter", "quantity": qty, "unit": "pcs",
			"unit_price": price, "unit_price_with_vat": price * 1.15, "unit_discount": 0.0, "unit_discount_with_vat": 0.0, "unit_discount_percent": 0.0}},
		"payments_input": []M{{"date_str": payAt.UTC().Format(time.RFC3339), "amount": paid, "method": "cash"}},
	}
}

func legacyReturnPayload(store, customer primitive.ObjectID, orderHex string, product primitive.ObjectID, qty, price, refund float64, at time.Time) M {
	now := at.UTC().Format(time.RFC3339)
	p := M{
		"store_id": store.Hex(), "customer_id": customer.Hex(), "order_id": orderHex, "date_str": now, "vat_percent": 15.0,
		"discount": 0.0, "discount_with_vat": 0.0, "shipping_handling_fees": 0.0, "rounding_amount": 0.0, "auto_rounding_amount": false,
		"products": []M{{"product_id": product.Hex(), "name": "Oil Filter", "quantity": qty, "unit": "pcs", "selected": true,
			"unit_price": price, "unit_price_with_vat": price * 1.15, "unit_discount": 0.0, "unit_discount_with_vat": 0.0, "unit_discount_percent": 0.0}},
		"payments_input": []M{},
	}
	if refund > 0 {
		p["payments_input"] = []M{{"date_str": now, "amount": refund, "method": "cash"}}
	}
	return p
}

// TestIntegration_LegacyDataReadable: fixture documents written in the OLD
// shapes (missing fields, int32/string numbers, no embedded payments, old
// store without settings) are read by the adapter as-is.
func TestIntegration_LegacyDataReadable(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)

	g := call(t, "GET", "/sales/"+fx.OrderA2.Hex(), tok, nil)
	if g.Code != 200 || g.Body["code"] != "S-INV-000" {
		t.Fatalf("old order: %d %s", g.Code, g.Raw)
	}
	it := arr(g.Body["items"])[0].(M)
	if it["warehouseId"] != fx.WarehouseA.Hex() || it["qty"] != 1.0 || it["unitPrice"] != 20.0 {
		t.Errorf("old order line: %v", it)
	}
	if p := arr(g.Body["payments"]); len(p) != 1 || p[0].(M)["amount"] != 23.0 {
		t.Errorf("old order payments come from sales_payment: %v", g.Body["payments"])
	}
	if g.Body["vatPercent"] != 15.0 {
		t.Errorf("int32 vat_percent: %v", g.Body["vatPercent"])
	}

	g = call(t, "GET", "/sales/"+fx.OrderA1.Hex(), tok, nil)
	if g.Code != 200 || get(g.Body, "zatca.status") != "cleared" || get(g.Body, "zatca.qr") != "QRDATA" {
		t.Fatalf("modern order: %d %s", g.Code, g.Raw)
	}
	g = call(t, "GET", "/sales-returns/"+fx.SalesReturnA1.Hex(), tok, nil)
	if g.Code != 200 || g.Body["orderId"] != fx.OrderA1.Hex() || len(arr(g.Body["items"])) != 1 {
		t.Fatalf("salesreturn: %d %s", g.Code, g.Raw)
	}
	g = call(t, "GET", "/products/"+fx.ProductA3.Hex(), tok, nil)
	if g.Code != 200 || g.Body["nameEn"] != "Legacy Brake Pad" {
		t.Fatalf("old product: %d %s", g.Code, g.Raw)
	}
	g = call(t, "GET", "/customers/"+fx.CustomerA2.Hex(), tok, nil)
	if g.Code != 200 || g.Body["nameEn"] != "Walk In Old" {
		t.Fatalf("old customer: %d %s", g.Code, g.Raw)
	}
	for path, id := range map[string]string{"deposits": fx.DepositA1.Hex(), "withdrawals": fx.WithdrawalA1.Hex(), "capitals": fx.CapitalA1.Hex(),
		"dividends": fx.DividentA1.Hex(), "salaries": fx.SalaryA1.Hex(), "expenses": fx.ExpenseA1.Hex(), "purchases": fx.PurchaseA1.Hex(),
		"quotations": fx.QuotationA1.Hex(), "delivery-notes": fx.DeliveryNoteA1.Hex(), "stock-transfers": fx.TransferA1.Hex(),
		"repair-jobs": fx.RepairJobA1.Hex(), "vehicles": fx.VehicleA1.Hex(), "employees": fx.EmployeeA1.Hex(), "packages": fx.PackageA1.Hex(),
		"accounts": fx.AccountA1.Hex()} {
		if g := call(t, "GET", "/"+path+"/"+id, tok, nil); g.Code != 200 || g.Body["id"] != id {
			t.Errorf("legacy %s: %d %s", path, g.Code, g.Raw)
		}
	}

	// old store (store B): string unit price, int32 qty, no settings
	tokB := login(t, fx.UserBEmail)
	g = call(t, "GET", "/sales/"+fx.OrderB1.Hex(), tokB, nil)
	if g.Code != 200 {
		t.Fatalf("store B order: %d %s", g.Code, g.Raw)
	}
	if it := arr(g.Body["items"])[0].(M); it["qty"] != 3.0 || it["unitPrice"] != 10.0 {
		t.Errorf("store B line: %v", it)
	}
	if g := call(t, "GET", "/stores/"+storeB(), tokB, nil); g.Code != 200 || g.Body["nameEn"] != "Old Branch" {
		t.Fatalf("old store: %d %s", g.Code, g.Raw)
	}
	// reading never writes: the legacy documents are byte-for-byte unchanged
	if d := rawDoc(t, storeA(), "order", fx.OrderA2.Hex()); d["erp"] != nil {
		t.Error("a GET must not add the erp envelope")
	}
}

// TestIntegration_SaleReturnPayment_MatchesOldApp creates the same sale →
// payment → return flow once through the OLD app path (direct v1 handlers,
// legacy payloads) and once through the adapter, then asserts identical
// counters / stock / ledger effects and that v1 reads the adapter documents.
func TestIntegration_SaleReturnPayment_MatchesOldApp(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	sA := storeA()
	ms := "ms_" + sA
	invKey, retKey := sA+"_invoice_counter", sA+"_return_invoice_counter"
	prod := fx.ProductA2
	// identical, minute-aligned business times for both paths (the contract
	// has minute resolution; legacy AdjustPayments shifts equal timestamps)
	base := time.Now().In(riyadh).Truncate(time.Minute).Add(-time.Hour)
	saleAt, pay1At, pay2At, retAt := base, base.Add(2*time.Minute), base.Add(10*time.Minute), base.Add(20*time.Minute)
	cdt := func(tm time.Time) string { return tm.In(riyadh).Format("2006-01-02T15:04") }

	// ---------- OLD APP: sale (qty 2 × 25, paid 10) ----------
	stock0 := rawStock(t, sA, prod.Hex())
	oldSale := v1OK(t, "create order", v1(t, controller.CreateOrder, "POST", "/v1/order", nil, storeQ(sA),
		legacySalePayload(fx.StoreA, fx.CustomerA1, prod, 2, 25, 10, saleAt, pay1At), tok))
	oldID := str(oldSale["id"])
	eventually(t, "old-app stock", func() bool { return rawStock(t, sA, prod.Hex()) == stock0-2 })
	inv1 := redisCounter(t, invKey)
	oldSaleLedger := stableLedger(t, sA, oldID)

	// ---------- ADAPTER: same sale ----------
	stock1 := rawStock(t, sA, prod.Hex())
	cr := call(t, "POST", "/sales", tok, M{"storeId": sA, "date": cdt(saleAt), "customerId": fx.CustomerA1.Hex(),
		"items":    []M{{"productId": prod.Hex(), "qty": 2, "unitPrice": 25, "unitDiscount": 0, "warehouseId": ms, "vatPercent": 15}},
		"payments": []M{{"date": cdt(pay1At), "amount": 10, "method": "cash"}}},
		"Idempotency-Key", uniq("op_int_sale"))
	if cr.Code != 201 {
		t.Fatalf("adapter sale: %d %s", cr.Code, cr.Raw)
	}
	newID := str(cr.Body["id"])
	// (b) counters: exactly +1, same serial sequence as the old app
	if inv2 := redisCounter(t, invKey); inv2 != inv1+1 {
		t.Errorf("invoice counter %d → %d, want +1", inv1, inv2)
	}
	if codeSeq(t, str(cr.Body["code"])) != codeSeq(t, str(oldSale["code"]))+1 {
		t.Errorf("adapter code %v must follow old-app code %v", cr.Body["code"], oldSale["code"])
	}
	// (b) stock: same delta
	eventually(t, "adapter stock", func() bool { return rawStock(t, sA, prod.Hex()) == stock1-2 })
	// (b) ledger: same journal lines
	if got := stableLedger(t, sA, newID); strings.Join(got, ",") != strings.Join(oldSaleLedger, ",") {
		t.Errorf("sale ledger differs\n old: %v\n new: %v", oldSaleLedger, got)
	}
	// (c) same legacy document shape: adapter doc keys == old-app doc keys (+ erp only)
	oldRaw, newRaw := rawDoc(t, sA, "order", oldID), rawDoc(t, sA, "order", newID)
	for k := range keysOf(oldRaw) {
		if _, ok := newRaw[k]; !ok {
			t.Errorf("adapter order lacks legacy field %q", k)
		}
	}
	for k := range keysOf(newRaw) {
		if _, ok := oldRaw[k]; !ok && k != envKey {
			t.Errorf("adapter order has non-legacy top-level field %q", k)
		}
	}

	// (a) old app reads the adapter sale
	view := v1OK(t, "view order", v1(t, controller.ViewOrder, "GET", "/v1/order/"+newID, map[string]string{"id": newID}, storeQ(sA), nil, tok))
	if view["code"] != cr.Body["code"] || num(view["net_total"]) != 57.5 || num(view["total_payment_received"]) != 10 || view["payment_status"] != "paid_partially" {
		t.Errorf("v1 view of adapter sale: code=%v net=%v paid=%v status=%v", view["code"], view["net_total"], view["total_payment_received"], view["payment_status"])
	}
	pq := storeQ(sA)
	pq.Set("search[order_id]", newID)
	pl := v1(t, controller.ListSalesPayment, "GET", "/v1/sales-payment", nil, pq, nil, tok)
	if rs := arr(pl["result"]); len(rs) != 1 || num(rs[0].(map[string]interface{})["amount"]) != 10 {
		t.Errorf("v1 sales-payment list for adapter sale: %v", pl["result"])
	}

	// ---------- payment (old app: PUT order with payments_input) ----------
	oldUpd := legacySalePayload(fx.StoreA, fx.CustomerA1, prod, 2, 25, 10, saleAt, pay1At)
	oldPays := arr(rawDoc(t, sA, "order", oldID)["payments"])
	pin := []M{}
	for _, p := range oldPays {
		pm := normDoc(p)
		pin = append(pin, M{"id": hexOf(pm["_id"]), "date_str": legacyTS(pm["date"]), "amount": num(pm["amount"]), "method": str(pm["method"])})
	}
	pin = append(pin, M{"date_str": pay2At.UTC().Format(time.RFC3339), "amount": 20.0, "method": "bank_transfer"})
	oldUpd["payments_input"] = pin
	oldUpd["code"] = oldSale["code"]
	v1OK(t, "update order payments", v1(t, controller.UpdateOrder, "PUT", "/v1/order/"+oldID, map[string]string{"id": oldID}, storeQ(sA), oldUpd, tok))
	eventually(t, "old-app payment", func() bool { return num(rawDoc(t, sA, "order", oldID)["total_payment_received"]) == 30 })
	oldPayLedger := stableLedger(t, sA, oldID)

	g := call(t, "GET", "/sales/"+newID, tok, nil)
	pays := append(arr(g.Body["payments"]), M{"date": cdt(pay2At), "amount": 20, "method": "bank_transfer"})
	p := call(t, "PATCH", "/sales/"+newID, tok, M{"payments": pays}, "If-Match", str(g.Body["version"]))
	if p.Code != 200 || len(arr(p.Body["payments"])) != 2 {
		t.Fatalf("adapter payment: %d %s", p.Code, p.Raw)
	}
	eventually(t, "adapter payment", func() bool { return num(rawDoc(t, sA, "order", newID)["total_payment_received"]) == 30 })
	if got := stableLedger(t, sA, newID); strings.Join(got, ",") != strings.Join(oldPayLedger, ",") {
		t.Errorf("payment ledger differs\n old: %v\n new: %v", oldPayLedger, got)
		for _, id := range []string{oldID, newID} {
			d := rawDoc(t, sA, "order", id)
			t.Logf("%s date=%v payments=%v", id, d["date"], d["payments"])
		}
	}
	if got := redisCounter(t, invKey); got != inv1+1 {
		t.Errorf("a payment must not allocate an invoice number (counter %d)", got)
	}
	pl = v1(t, controller.ListSalesPayment, "GET", "/v1/sales-payment", nil, pq, nil, tok)
	if len(arr(pl["result"])) != 2 {
		t.Errorf("v1 sees 2 payments for the adapter sale: %v", pl["result"])
	}

	// ---------- return (qty 1, refund 28.75) ----------
	stock2 := rawStock(t, sA, prod.Hex())
	oldRet := v1OK(t, "create sales return", v1(t, controller.CreateSalesReturn, "POST", "/v1/sales-return", nil, storeQ(sA),
		legacyReturnPayload(fx.StoreA, fx.CustomerA1, oldID, prod, 1, 25, 28.75, retAt), tok))
	oldRetID := str(oldRet["id"])
	eventually(t, "old-app return stock", func() bool { return rawStock(t, sA, prod.Hex()) == stock2+1 })
	ret1 := redisCounter(t, retKey)
	oldRetLedger := stableLedger(t, sA, oldRetID)

	stock3 := rawStock(t, sA, prod.Hex())
	rr := call(t, "POST", "/sales-returns", tok, M{"storeId": sA, "date": cdt(retAt), "orderId": newID,
		"customerId": fx.CustomerA1.Hex(),
		"items":      []M{{"productId": prod.Hex(), "qty": 1, "unitPrice": 25, "unitDiscount": 0, "warehouseId": ms, "vatPercent": 15}},
		"payments":   []M{{"date": cdt(retAt), "amount": 28.75, "method": "cash"}}},
		"Idempotency-Key", uniq("op_int_ret"))
	if rr.Code != 201 {
		t.Fatalf("adapter return: %d %s", rr.Code, rr.Raw)
	}
	newRetID := str(rr.Body["id"])
	if got := redisCounter(t, retKey); got != ret1+1 {
		t.Errorf("return counter %d → %d, want +1", ret1, got)
	}
	if codeSeq(t, str(rr.Body["code"])) != codeSeq(t, str(oldRet["code"]))+1 {
		t.Errorf("return code %v must follow %v", rr.Body["code"], oldRet["code"])
	}
	eventually(t, "adapter return stock", func() bool { return rawStock(t, sA, prod.Hex()) == stock3+1 })
	if got := stableLedger(t, sA, newRetID); strings.Join(got, ",") != strings.Join(oldRetLedger, ",") {
		t.Errorf("return ledger differs\n old: %v\n new: %v", oldRetLedger, got)
	}
	oldRetRaw, newRetRaw := rawDoc(t, sA, "salesreturn", oldRetID), rawDoc(t, sA, "salesreturn", newRetID)
	for k := range keysOf(oldRetRaw) {
		if _, ok := newRetRaw[k]; !ok {
			t.Errorf("adapter return lacks legacy field %q", k)
		}
	}
	for k := range keysOf(newRetRaw) {
		if _, ok := oldRetRaw[k]; !ok && k != envKey {
			t.Errorf("adapter return has non-legacy top-level field %q", k)
		}
	}
	// the sale's returned quantities were updated by the legacy code, for both
	for _, id := range []string{oldID, newID} {
		eventually(t, "quantity_returned on "+id, func() bool {
			ln := normDoc(arr(rawDoc(t, sA, "order", id)["products"])[0])
			return num(ln["quantity_returned"]) == 1
		})
	}
	// (a) old app reads the adapter return
	rv := v1OK(t, "view sales return", v1(t, controller.ViewSalesReturn, "GET", "/v1/sales-return/"+newRetID, map[string]string{"id": newRetID}, storeQ(sA), nil, tok))
	if rv["order_id"] != newID || rv["code"] != rr.Body["code"] || num(rv["net_total"]) != 28.75 {
		t.Errorf("v1 view of adapter return: order=%v code=%v net=%v", rv["order_id"], rv["code"], rv["net_total"])
	}
	// adapter reads the old-app documents back with the same numbers
	if g := call(t, "GET", "/sales/"+oldID, tok, nil); g.Code != 200 || len(arr(g.Body["payments"])) != 2 || get(g.Body, "legacyTotals.net") != 57.5 {
		t.Errorf("adapter view of old-app sale: %d %s", g.Code, g.Raw)
	}
	if g := call(t, "GET", "/sales-returns/"+oldRetID, tok, nil); g.Code != 200 || g.Body["orderId"] != oldID {
		t.Errorf("adapter view of old-app return: %d %s", g.Code, g.Raw)
	}
}

// TestIntegration_AdapterEditKeepsLegacyFields: an adapter PATCH of a
// document the old app wrote never removes, renames or retypes a field.
func TestIntegration_AdapterEditKeepsLegacyFields(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	sA := storeA()
	cases := []struct {
		path, coll, id string
		patch          M
	}{
		{"customers", "customer", fx.CustomerA2.Hex(), M{"remarks": "edited by adapter"}},
		{"products", "product", fx.ProductA3.Hex(), M{"note": "edited by adapter"}},
		{"expenses", "expense", fx.ExpenseA1.Hex(), M{"description": "Shop rent (Oct)"}},
	}
	for _, c := range cases {
		before := rawDoc(t, sA, c.coll, c.id)
		g := call(t, "GET", "/"+c.path+"/"+c.id, tok, nil)
		p := call(t, "PATCH", "/"+c.path+"/"+c.id, tok, c.patch, "If-Match", str(g.Body["version"]))
		if p.Code != 200 {
			t.Fatalf("%s patch: %d %s", c.path, p.Code, p.Raw)
		}
		time.Sleep(300 * time.Millisecond)
		after := rawDoc(t, sA, c.coll, c.id)
		if after[envKey] == nil {
			t.Errorf("%s: erp envelope missing", c.path)
		}
		for k, v := range before {
			av, ok := after[k]
			if !ok {
				t.Errorf("%s: legacy field %q removed", c.path, k)
				continue
			}
			if v != nil && av != nil && fmt.Sprintf("%T", v) != fmt.Sprintf("%T", av) {
				// numeric widening by the legacy model itself is allowed (int32 → float64 / int64)
				_, vNum := toNumberKind(v)
				_, aNum := toNumberKind(av)
				if !(vNum && aNum) {
					t.Errorf("%s: field %q changed type %T → %T", c.path, k, v, av)
				}
			}
		}
		// the misspelled legacy keys stay misspelled
		if c.coll == "product" {
			set := normDoc(after["set"])
			if ps := arr(set["products"]); len(ps) == 0 || normDoc(ps[0])["produc_id"] == nil {
				t.Errorf("product set.products[].produc_id must be preserved: %v", after["set"])
			}
		}
	}
	if s := rawDoc0(t, "store", storeA()); sub(sub(s, "settings"), "invoice")["receivabale_title"] == nil {
		t.Error("store settings.invoice.receivabale_title must keep its legacy spelling")
	}
}

func toNumberKind(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	case primitive.Decimal128:
		return num(x), true
	}
	return 0, false
}

func rawDoc0(t *testing.T, coll, idHex string) M {
	t.Helper()
	oid, _ := primitive.ObjectIDFromHex(idHex)
	ctx, cancel := dbctx()
	defer cancel()
	var d bson.M
	if err := mainDB().Collection(coll).FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
		t.Fatalf("raw main %s/%s: %v", coll, idHex, err)
	}
	return normDoc(d)
}

// TestIntegration_DraftsNeverTouchCountersOrStock (§2.1b regression).
func TestIntegration_DraftsNeverTouchCountersOrStock(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	sA := storeA()
	ms := "ms_" + sA
	invKey := sA + "_invoice_counter"
	prod := fx.ProductA1.Hex()
	countOrders := func() int64 {
		ctx, cancel := dbctx()
		defer cancel()
		n, _ := storeDB(sA).Collection("order").CountDocuments(ctx, bson.M{})
		return n
	}
	countLedger := func() int64 {
		ctx, cancel := dbctx()
		defer cancel()
		n, _ := storeDB(sA).Collection("ledger").CountDocuments(ctx, bson.M{})
		return n
	}
	// make sure the counter exists so "unchanged" is meaningful
	if redisCounter(t, invKey) < 0 {
		_ = db.RedisClient.Set(invKey, 1000, 0).Err()
	}
	c0, s0, o0, l0 := redisCounter(t, invKey), rawStock(t, sA, prod), countOrders(), countLedger()
	payload := M{"storeId": sA, "date": "2026-10-05T10:00", "customerId": fx.CustomerA1.Hex(),
		"items": []M{{"productId": prod, "qty": 3, "unitPrice": 120, "warehouseId": ms, "vatPercent": 15}}}
	d := call(t, "POST", "/drafts/sales", tok, M{"storeId": sA, "payload": payload})
	if d.Code != 201 {
		t.Fatalf("draft create: %d %s", d.Code, d.Raw)
	}
	id := str(d.Body["id"])
	payload["remarks"] = "edited"
	if r := call(t, "PUT", "/drafts/sales/"+id+"?storeId="+sA, tok, M{"storeId": sA, "payload": payload}); r.Code != 200 {
		t.Fatalf("draft put: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "GET", "/drafts/sales?storeId="+sA, tok, nil); r.Code != 200 || len(r.data()) == 0 {
		t.Fatalf("draft list: %d %s", r.Code, r.Raw)
	}
	time.Sleep(500 * time.Millisecond)
	if c, s, o, l := redisCounter(t, invKey), rawStock(t, sA, prod), countOrders(), countLedger(); c != c0 || s != s0 || o != o0 || l != l0 {
		t.Fatalf("drafts touched real data: counter %d→%d stock %v→%v orders %d→%d ledger %d→%d", c0, c, s0, s, o0, o, l0, l)
	}
	// a second draft, deleted: still nothing
	d2 := call(t, "POST", "/drafts/sales", tok, M{"storeId": sA, "payload": payload})
	if r := call(t, "DELETE", "/drafts/sales/"+str(d2.Body["id"])+"?storeId="+sA, tok, nil); r.Code != 204 && r.Code != 200 {
		t.Fatalf("draft delete: %d %s", r.Code, r.Raw)
	}
	if c, s, o := redisCounter(t, invKey), rawStock(t, sA, prod), countOrders(); c != c0 || s != s0 || o != o0 {
		t.Fatal("deleting a draft touched real data")
	}
	// finalize = the normal create (exactly one number, stock moves once) and the draft is gone
	f := call(t, "POST", "/drafts/sales/"+id+"/finalize?storeId="+sA, tok, M{})
	if f.Code != 201 || str(f.Body["code"]) == "" {
		t.Fatalf("finalize: %d %s", f.Code, f.Raw)
	}
	if c := redisCounter(t, invKey); c != c0+1 {
		t.Errorf("finalize must allocate exactly one number: %d → %d", c0, c)
	}
	eventually(t, "finalized stock", func() bool {
		if v := rawStock(t, sA, prod); v != s0-3 {
			t.Logf("stock %v (start %v)", v, s0)
			return false
		}
		return true
	})
	if o := countOrders(); o != o0+1 {
		t.Errorf("finalize creates exactly one order: %d → %d", o0, o)
	}
	if g := call(t, "GET", "/drafts/sales/"+id+"?storeId="+sA, tok, nil); g.Code != 404 {
		t.Errorf("finalized draft must be deleted: %d", g.Code)
	}
	// draft collections are separate from the real ones
	ctx, cancel := dbctx()
	defer cancel()
	if n, _ := storeDB(sA).Collection("order").CountDocuments(ctx, bson.M{"status": "draft", "erp": bson.M{"$exists": true}}); n != 0 {
		t.Error("adapter drafts must never be written to the order collection")
	}
}

func legacyTS(v interface{}) string {
	if tm, ok := toTime(v); ok {
		return tm.UTC().Format(time.RFC3339)
	}
	return time.Now().UTC().Format(time.RFC3339)
}

// TestIntegration_OrgUnionListsEachIDOnce: a store DB copied from another keeps
// the records' ids, so an org-wide list (union of store DBs) must not repeat
// them — the dashboard showed one expense category twice.
func TestIntegration_OrgUnionListsEachIDOnce(t *testing.T) {
	requireDB(t)
	d := rawDoc(t, storeA(), "expense_category", fx.ExpenseCatA.Hex())
	ctx, cancel := dbctx()
	defer cancel()
	cp := bson.M{}
	for k, v := range d {
		cp[k] = v
	}
	cp["_id"] = fx.ExpenseCatA
	cp["store_id"] = fx.StoreB
	if _, err := storeDB(storeB()).Collection("expense_category").InsertOne(ctx, cp); err != nil {
		t.Fatal(err)
	}
	defer storeDB(storeB()).Collection("expense_category").DeleteOne(context.Background(), bson.M{"_id": fx.ExpenseCatA})
	tok := login(t, fx.AdminEmail) // sees both stores
	g := call(t, "GET", "/expense-categories?limit=500", tok, nil)
	if g.Code != 200 {
		t.Fatalf("list: %d %s", g.Code, g.Raw)
	}
	n := 0
	for _, r := range arr(g.Body["data"]) {
		if r.(M)["id"] == fx.ExpenseCatA.Hex() {
			n++
		}
	}
	if n != 1 {
		t.Errorf("expense category listed %d times, want once: %s", n, g.Raw)
	}
}

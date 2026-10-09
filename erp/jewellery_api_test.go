package erp

import (
	"testing"
	"time"
)

// Integration (needs MongoDB + Redis): a Jewellery store signs up in Saudi
// Arabia and in India, gets its weight-priced starter pieces with their
// jewellery details and tax rates, and products with bad jewellery details
// are refused.
func TestAPI_Jewellery_StoreAndProducts(t *testing.T) {
	requireDB(t)
	sa := validSignup()
	sa["owner"].(M)["email"] = "jewel-sa+" + time.Now().Format("150405.000000") + "@signup.example"
	sa["company"].(M)["type"] = "Jewellery"
	r := call(t, "POST", "/auth/signup", "", sa)
	if r.Code != 201 {
		t.Fatalf("Saudi Jewellery signup: %d %s", r.Code, r.Raw)
	}
	sid := str(get(r.Body, "store.id"))
	defer cleanupStore(t, sid)
	tok := str(r.Body["accessToken"])
	g := call(t, "GET", "/stores/"+sid, tok, nil)
	if g.Body["posTerminal"] != "jewellery" {
		t.Errorf("posTerminal %v", g.Body["posTerminal"])
	}
	plan, _ := planStarterCatalog("jewellery", "SA")
	rows := listAll(t, tok, "/products?limit=1000&storeId="+sid)
	if len(rows) != len(plan.Items) {
		t.Fatalf("sign-up seeded %d of %d jewellery items", len(rows), len(plan.Items))
	}
	byKey := map[string]M{}
	for _, x := range rows {
		p, _ := x.(M)
		byKey[str(p["posKey"])] = p
	}
	ring := byKey["jw-ring-21"]
	if ring == nil || str(get(ring, "jewel.purity")) != "21K" || num(get(ring, "jewel.gross")) != 3.6 || ring["posTerminal"] != "jewellery" {
		t.Errorf("21K ring: %v", ring)
	}
	if bar := byKey["cn-5g"]; bar == nil || num(bar["vatPercent"]) != 0 {
		t.Errorf("investment bar zero-rated: %v", bar)
	}

	// the terminal filters and counts pieces by purity on the server
	want21 := 0
	for _, it := range plan.Items {
		if it.Jewel != nil && str(it.Jewel["purity"]) == "21K" {
			want21++
		}
	}
	if l := call(t, "GET", "/products?storeId="+sid+"&where.onlyTerminal=jewellery&where.jewelPurity=21K&limit=100&page=1&select=jewel", tok, nil); l.Code != 200 || int(num(l.Body["total"])) != want21 {
		t.Errorf("where.jewelPurity=21K: %d total %v, want %d", l.Code, l.Body["total"], want21)
	}
	fc := call(t, "GET", "/products/facets?storeId="+sid+"&where.onlyTerminal=jewellery", tok, nil)
	found := false
	for _, x := range arr(fc.Body["jewelPurity"]) {
		if m, _ := x.(M); str(m["id"]) == "21K" && int(num(m["count"])) == want21 {
			found = true
		}
	}
	if !found {
		t.Errorf("facets jewelPurity: %s", fc.Raw)
	}

	// a product with bad jewellery details is refused; good ones round-trip
	bad := M{"nameEn": "Bad ring", "unit": "Pcs", "storeId": sid, "posTerminal": "jewellery",
		"pricing": M{"retail": 0.0}, "jewel": M{"metal": "gold", "purity": "9K", "gross": 2.0}}
	if c := call(t, "POST", "/products", tok, bad); c.Code != 400 || c.errField("jewel.purity") == "" {
		t.Errorf("bad purity accepted: %d %s", c.Code, c.Raw)
	}
	good := M{"nameEn": "Test bangle 22K", "unit": "Pcs", "storeId": sid, "posTerminal": "jewellery", "posSection": "bangles",
		"pricing": M{"retail": 0.0}, "jewel": M{"metal": "gold", "purity": "22K", "pricing": "weight", "gross": 15.5,
			"stone": 0.0, "net": 15.5, "makingType": "percent", "making": 9.0, "wastage": 0.0, "huid": "ZX81Q2", "hallmark": true}}
	c := call(t, "POST", "/products", tok, good)
	if c.Code != 201 {
		t.Fatalf("good jewel product: %d %s", c.Code, c.Raw)
	}
	if str(get(c.Body, "jewel.huid")) != "ZX81Q2" || num(get(c.Body, "jewel.making")) != 9 {
		t.Errorf("jewel round-trip: %v", c.Body["jewel"])
	}
	if f := call(t, "GET", "/products/"+str(c.Body["id"])+"?select=jewel,posSection", tok, nil); str(get(f.Body, "jewel.purity")) != "22K" {
		t.Errorf("select=jewel: %d %s", f.Code, f.Raw)
	}

	// India: 3% GST on jewellery, 5% on job work, rupee making charges
	in := indiaSignup()
	in["owner"].(M)["email"] = "jewel-in+" + time.Now().Format("150405.000000") + "@signup.example"
	in["company"].(M)["type"] = "Jewellery"
	ri := call(t, "POST", "/auth/signup", "", in)
	if ri.Code != 201 {
		t.Fatalf("India Jewellery signup: %d %s", ri.Code, ri.Raw)
	}
	isid := str(get(ri.Body, "store.id"))
	defer cleanupStore(t, isid)
	itok := str(ri.Body["accessToken"])
	irows := listAll(t, itok, "/products?limit=1000&storeId="+isid)
	ib := map[string]M{}
	for _, x := range irows {
		p, _ := x.(M)
		ib[str(p["posKey"])] = p
	}
	if p := ib["in-ring-22"]; p == nil || num(p["vatPercent"]) != 3 || num(get(p, "jewel.making")) != 350 {
		t.Errorf("India ring: %v", p)
	}
	if p := ib["rp-resize"]; p == nil || num(p["vatPercent"]) != 5 {
		t.Errorf("India repair 5%%: %v", p)
	}
	if ib["jw-ring-21"] != nil {
		t.Error("India store got a Gulf 21K piece")
	}

	// a jewellery bill at 3% GST with the piece's breakdown kept on the sale,
	// paid partly by an old-gold exchange the shop records as a purchase of
	// old gold by weight (no vendor, no tax) paid out in cash
	inRing := ib["in-ring-22"]
	sale := call(t, "POST", "/sales", itok, M{"storeId": isid, "date": "2026-10-09T11:00", "vatPercent": 3, "roundingAuto": true,
		"items":   []M{{"productId": str(inRing["id"]), "qty": 1, "unitPrice": 31250.5, "warehouseId": "ms_" + isid, "nameEn": "Ladies ring 22K – floral (22K · net 3.850 g)"}},
		"posType": "jewellery", "posMeta": M{"jewel": []M{{"purity": "22K", "net": 3.85, "rate": 7000, "making": 1347.5}}},
		"payments": []M{{"date": "2026-10-09T11:00", "amount": 20000, "method": "cash", "description": "Old gold OG-1001"}}})
	if sale.Code != 201 {
		t.Fatalf("jewellery sale at 3%%: %d %s", sale.Code, sale.Raw)
	}
	if num(sale.Body["vatPercent"]) != 3 || len(arr(get(sale.Body, "posMeta.jewel"))) != 1 {
		t.Errorf("sale tax / breakdown: %v %v", sale.Body["vatPercent"], sale.Body["posMeta"])
	}
	og := ib["og-22k"]
	pur := call(t, "POST", "/purchases", itok, M{"storeId": isid, "date": "2026-10-09T11:00", "vatPercent": 0,
		"vendorName": "Old gold – Anil Kumar", "remarks": "Old gold OG-1001 · sale " + str(sale.Body["code"]),
		"items":    []M{{"productId": str(og["id"]), "qty": 2.857, "unitPrice": 7000, "warehouseId": "ms_" + isid, "unit": "g"}},
		"payments": []M{{"date": "2026-10-09T11:00", "amount": 19999, "method": "cash"}}})
	if pur.Code != 201 {
		t.Fatalf("old gold purchase without vendor, 0%% tax: %d %s", pur.Code, pur.Raw)
	}
	if num(pur.Body["vatPercent"]) != 0 {
		t.Errorf("old gold purchase tax %v", pur.Body["vatPercent"])
	}
}

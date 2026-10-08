package erp

import (
	"strings"
	"testing"
	"time"
)

// resourceCase is one contract resource exercised end to end: create, get,
// patch (+version), delete/restore/hard-delete (or the documented 409 when
// the existing system does not support the operation).
type resourceCase struct {
	path        string
	body        func() M
	patch       M
	check       func(t *testing.T, rec M)
	noDelete    bool   // legacy has no (or a no-op) delete → 409 unsupported_legacy
	minMongo    [2]int // legacy update needs this MongoDB version for the PATCH step
	noPatch     bool   // mapped-field PATCH unsupported by the legacy update → 409
	noRestore   bool
	asAdmin     bool
	skipHard    bool
	afterCreate func(t *testing.T, rec M)
}

func uniq(prefix string) string { return prefix + time.Now().Format("150405.000000") }

// digits returns n pseudo-unique digits (time based).
func digits(n int) string {
	s := strings.ReplaceAll(time.Now().Format("150405.000000"), ".", "")
	for len(s) < n {
		s += s
	}
	return s[len(s)-n:]
}

func resourceCases() []resourceCase {
	sA := storeA()
	ms := "ms_" + sA
	day := "2026-10-05"
	dt := "2026-10-05T10:00"
	line := func(pid string, qty, price float64) M {
		return M{"productId": pid, "qty": qty, "unitPrice": price, "unitDiscount": 0, "warehouseId": ms, "vatPercent": 15}
	}
	p1, p2 := fx.ProductA1.Hex(), fx.ProductA2.Hex()
	return []resourceCase{
		{path: "warehouses", body: func() M { return M{"storeId": sA, "nameEn": "WH " + uniq(""), "code": uniq("W")[:9]} }, patch: M{"nameEn": "Renamed WH"}, noRestore: true},
		{path: "products", body: func() M {
			return M{"storeId": sA, "nameEn": "Product " + uniq(""), "pricing": M{"purchase": 5, "retail": 9}, "stock": M{ms: M{"qty": 3, "min": 1}}}
		}, patch: M{"note": "patched", "pricing": M{"purchase": 6, "retail": 10, "min": 2}},
			check: func(t *testing.T, rec M) {
				if get(rec, "stock."+ms+".qty") != 3.0 || get(rec, "stock."+ms+".min") != 1.0 {
					t.Errorf("product stock/min: %v", rec["stock"])
				}
				if !strings.Contains(str(rec["code"]), "-P-0") {
					t.Errorf("blank SKU → P- serial, got %q", rec["code"])
				}
			}},
		{path: "customers", body: func() M { return M{"storeId": sA, "nameEn": "Cust " + uniq("")} }, patch: M{"remarks": "vip"}},
		{path: "vendors", body: func() M {
			return M{"storeId": sA, "nameEn": "Vendor " + uniq(""), "bank": M{"iban": "SA0380000000608010167519"}}
		},
			patch: M{"remarks": "x"}, check: func(t *testing.T, rec M) {
				if get(rec, "bank.iban") == nil {
					t.Error("vendor bank preserved in extras")
				}
			}},
		{path: "categories", body: func() M { return M{"nameEn": "Cat " + uniq(""), "nameAr": "فئة"} }, patch: M{"nameEn": "Cat renamed"}},
		{path: "brands", body: func() M { return M{"name": "Brand " + uniq("")} }, patch: M{"name": "Brand renamed"}, minMongo: [2]int{4, 2}},
		{path: "vendor-categories", body: func() M { return M{"name": "VCat " + uniq("")} }, patch: M{"name": "VCat 2"}, noRestore: true},
		{path: "expense-categories", body: func() M { return M{"nameEn": "ECat " + uniq("")} }, patch: M{"nameEn": "ECat 2"}, noRestore: true},
		{path: "customer-categories", body: func() M { return M{"name": "CC " + uniq("")} }, patch: M{"name": "CC2"}},
		{path: "employees", body: func() M {
			return M{"storeId": sA, "nameEn": "Emp " + uniq(""), "joinDate": "2026-01-01", "basicSalary": 3000, "status": "active", "phone": "05" + digits(8)}
		}, patch: M{"jobTitle": "Mechanic"}, noRestore: true},
		{path: "vehicles", body: func() M {
			return M{"storeId": sA, "plate": "ABC " + digits(4), "make": "Toyota", "model": "Camry", "customerId": fx.CustomerA1.Hex(), "year": 2021}
		}, patch: M{"color": "White"}, noRestore: true},
		{path: "packages", body: func() M {
			return M{"storeId": sA, "customerId": fx.CustomerA1.Hex(), "nameEn": "Pkg " + uniq(""), "price": 100, "visits": 5, "used": 0, "validFrom": day, "validDays": 30, "status": "active"}
		}, patch: M{"used": 1}, noRestore: true},
		{path: "rfq-suppliers", body: func() M {
			return M{"storeId": sA, "name": "Sup " + uniq(""), "phone": "05" + digits(8), "categories": []string{"filters"}}
		}, patch: M{"rating": 4}},
		{path: "signatures", asAdmin: true, body: func() M {
			return M{"storeId": sA, "name": "Sig " + uniq(""), "image": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==", "default": true, "docs": []string{"sales"}}
		}, patch: M{"name": "Sig renamed"}, noRestore: true},
		{path: "sales", body: func() M {
			return M{"storeId": sA, "date": dt, "customerId": fx.CustomerA1.Hex(), "items": []M{line(p2, 1, 25)}, "payments": []M{{"date": dt, "amount": 10, "method": "cash"}}}
		}, patch: M{"remarks": "patched sale"}, noDelete: true},
		{path: "quotations", body: func() M {
			return M{"storeId": sA, "date": dt, "customerId": fx.CustomerA1.Hex(), "type": "quotation", "status": "created", "validityDays": 7, "deliveryDays": 3, "items": []M{line(p1, 2, 118)}}
		}, patch: M{"status": "accepted"}, noRestore: true},
		{path: "proformas", body: func() M {
			return M{"storeId": sA, "date": dt, "customerId": fx.CustomerA1.Hex(), "validityDays": 7, "deliveryDays": 3, "items": []M{line(p1, 1, 118)}}
		}, patch: M{"remarks": "x"}},
		{path: "delivery-notes", body: func() M {
			return M{"storeId": sA, "date": dt, "customerId": fx.CustomerA1.Hex(), "estDelivery": day, "items": []M{line(p1, 1, 0)}}
		}, patch: M{"remarks": "deliver fast"}, noDelete: true},
		{path: "nonvat-sales", body: func() M {
			return M{"storeId": sA, "date": dt, "customerId": fx.CustomerA1.Hex(), "items": []M{line(p2, 1, 30)}}
		}, patch: M{"remarks": "x"}, noRestore: true},
		{path: "purchases", body: func() M {
			return M{"storeId": sA, "date": dt, "vendorId": fx.VendorA1.Hex(), "vendorInvoiceNo": uniq("V"), "items": []M{line(p2, 3, 9)}}
		}, patch: M{"vendorInvoiceNo": "VX-1"}, noDelete: true}, // legacy DeletePurchase is a no-op
		{path: "purchase-orders", body: func() M {
			return M{"storeId": sA, "date": dt, "vendorId": fx.VendorA1.Hex(), "expectedDate": "2026-10-10", "status": "draft", "items": []M{line(p2, 3, 9)}}
		}, patch: M{"status": "sent"}},
		{path: "purchase-requests", body: func() M {
			return M{"storeId": sA, "date": dt, "status": "pending", "notes": "need", "items": []M{line(p2, 3, 9)}}
		}, patch: M{"status": "accepted"}},
		{path: "purchase-bills", body: func() M {
			return M{"storeId": sA, "vendorId": fx.VendorA1.Hex(), "receivedAt": dt, "fileName": "b.pdf", "amount": 115, "vat": 15, "status": "new"}
		}, patch: M{"status": "converted"}},
		{path: "stock-transfers", body: func() M {
			return M{"storeId": sA, "date": dt, "fromWarehouseId": ms, "toWarehouseId": fx.WarehouseA.Hex(), "status": "completed", "items": []M{{"productId": p2, "qty": 1, "unitPrice": 9}}}
		}, patch: M{"remarks": "moved"}, noDelete: true},
		{path: "expenses", body: func() M {
			return M{"storeId": sA, "date": dt, "categoryId": fx.ExpenseCatA.Hex(), "description": "Water", "amount": 50, "vatAmount": 0, "method": "cash"}
		}, patch: M{"description": "Water bill"}, noRestore: true},
		{path: "deposits", body: func() M {
			return M{"storeId": sA, "date": dt, "customerId": fx.CustomerA1.Hex(), "amount": 75, "method": "cash", "notes": "adv"}
		}, patch: M{"notes": "advance payment"}, noRestore: true},
		{path: "withdrawals", body: func() M {
			return M{"storeId": sA, "date": dt, "customerId": fx.CustomerA1.Hex(), "amount": 5, "method": "cash", "type": "refund", "notes": "r"}
		}, patch: M{"notes": "refund 2"}, noRestore: true},
		{path: "capitals", body: func() M { return M{"storeId": sA, "date": dt, "investor": "Admin T1", "amount": 500, "method": "cash"} },
			patch: M{"notes": "more capital"}, noRestore: true},
		{path: "capital-withdrawals", body: func() M { return M{"storeId": sA, "date": dt, "investor": "Admin T1", "amount": 50, "method": "cash"} },
			patch: M{"notes": "drawing"}, noRestore: true, noPatch: true},
		{path: "dividends", body: func() M { return M{"storeId": sA, "date": dt, "recipient": "Admin T1", "amount": 20, "method": "cash"} },
			patch: M{"notes": "q3"}, noRestore: true},
		{path: "salaries", body: func() M {
			return M{"storeId": sA, "employeeId": fx.EmployeeA1.Hex(), "period": "2025-" + time.Now().Format("01"), "netSalary": 3800, "paymentDate": day, "method": "cash", "housing": 500}
		}, patch: M{"notes": "paid"}, noRestore: true},
		{path: "repair-jobs", body: func() M {
			return M{"storeId": sA, "date": dt, "status": "open", "vehicleId": fx.VehicleA1.Hex(), "customerId": fx.CustomerA1.Hex(), "complaint": "Noise", "parts": []M{{"productId": p2, "qty": 1, "unitPrice": 25}}, "labour": 100}
		}, patch: M{"status": "in_progress"}, noRestore: true},
		{path: "rfqs", body: func() M {
			return M{"storeId": sA, "customerName": "RFQ " + uniq(""), "source": "manual", "receivedAt": dt, "message": "need filters", "items": []M{{"name": "Oil filter", "qty": 3, "unit": "pcs"}}}
		}, patch: M{"status": "processing"}, check: func(t *testing.T, rec M) {
			if len(arr(rec["items"])) != 1 {
				t.Errorf("rfq items: %v", rec["items"])
			}
		}},
		{path: "threads", body: func() M { return M{"storeId": sA, "name": "T " + uniq(""), "channel": "whatsapp", "messages": []M{}} }, patch: M{"pinned": true}},
		{path: "notifications", body: func() M {
			return M{"storeId": sA, "type": "stock", "tone": "warning", "titleEn": "Low", "at": dt, "read": false}
		}, patch: M{"read": true}},
		{path: "product-specs", body: func() M { return M{"storeId": sA, "kind": "class", "name": "CL" + digits(3)} },
			patch: M{"name": "CL900"}, check: func(t *testing.T, rec M) {
				if rec["kind"] != "class" {
					t.Errorf("spec kind: %v", rec["kind"])
				}
			}},
		{path: "roles", asAdmin: true, body: func() M { return M{"name": "Role " + uniq(""), "perms": M{"sales": M{"view": true}}} }, patch: M{"description": "d"}},
		{path: "users", asAdmin: true, body: func() M {
			return M{"name": "U " + uniq(""), "email": uniq("u") + "@t1.example", "phone": "05" + digits(8), "role": "r_viewer", "storeIds": []string{sA}}
		}, patch: M{"jobTitle": "Clerk"}, noRestore: true},
	}
}

func TestAPI_AllResources_CRUD(t *testing.T) {
	requireDB(t)
	for _, rc := range resourceCases() {
		rc := rc
		t.Run(rc.path, func(t *testing.T) {
			tok := login(t, fx.ManagerEmail)
			if rc.asAdmin {
				tok = login(t, fx.AdminEmail)
			}
			cr := call(t, "POST", "/"+rc.path, tok, rc.body(), "Idempotency-Key", uniq("op_"+rc.path))
			if cr.Code != 201 {
				t.Fatalf("create: %d %s", cr.Code, cr.Raw)
			}
			id := str(cr.Body["id"])
			if id == "" || cr.Body["version"] != 1.0 || cr.Body["deleted"] != false || len(arr(cr.Body["history"])) == 0 {
				t.Fatalf("created envelope: %v", cr.Body)
			}
			if rc.check != nil {
				rc.check(t, cr.Body)
			}
			g := call(t, "GET", "/"+rc.path+"/"+id, tok, nil)
			if g.Code != 200 || g.Body["id"] != id {
				t.Fatalf("get: %d %s", g.Code, g.Raw)
			}
			v := str(g.Body["version"])
			if rc.minMongo[0] > 0 && !mongoAtLeast(rc.minMongo[0], rc.minMongo[1]) {
				t.Skipf("legacy update for %s needs MongoDB >= %d.%d (server %s)", rc.path, rc.minMongo[0], rc.minMongo[1], mongoVersion)
			}
			p := call(t, "PATCH", "/"+rc.path+"/"+id, tok, rc.patch, "If-Match", v, "X-Change-Reason", "updated")
			if rc.noPatch {
				if p.Code != 409 || p.errCode() != "unsupported_legacy" {
					t.Fatalf("patch should be unsupported: %d %s", p.Code, p.Raw)
				}
				p = g
				rc.patch = M{}
			} else if p.Code != 200 {
				t.Fatalf("patch: %d %s", p.Code, p.Raw)
			}
			if !rc.noPatch && num(p.Body["version"]) <= num(g.Body["version"]) {
				t.Fatalf("version must increase: %v → %v", g.Body["version"], p.Body["version"])
			}
			for k, want := range rc.patch {
				if wm, ok := want.(M); ok {
					for sk, sv := range wm {
						if num(get(p.Body, k+"."+sk)) != num(sv) && str(get(p.Body, k+"."+sk)) != str(sv) {
							t.Errorf("patched %s.%s=%v want %v", k, sk, get(p.Body, k+"."+sk), sv)
						}
					}
					continue
				}
				if str(p.Body[k]) != str(want) && !strings.EqualFold(str(p.Body[k]), str(want)) {
					t.Errorf("patched %s=%v want %v", k, p.Body[k], want)
				}
			}
			d := call(t, "DELETE", "/"+rc.path+"/"+id, tok, nil)
			if rc.noDelete {
				if d.Code != 409 || d.errCode() != "unsupported_legacy" {
					t.Fatalf("delete should be unsupported: %d %s", d.Code, d.Raw)
				}
				return
			}
			if d.Code != 200 || d.Body["deleted"] != true {
				t.Fatalf("delete: %d %s", d.Code, d.Raw)
			}
			rs := call(t, "POST", "/"+rc.path+"/"+id+"/restore", tok, M{})
			if rc.noRestore {
				if rs.Code != 409 {
					t.Fatalf("restore should be unsupported: %d %s", rs.Code, rs.Raw)
				}
			} else if rs.Code != 200 || rs.Body["deleted"] != false {
				t.Fatalf("restore: %d %s", rs.Code, rs.Raw)
			}
			if !rc.skipHard {
				h := call(t, "DELETE", "/"+rc.path+"/"+id+"?hard=1", tok, nil)
				if h.Code != 204 {
					t.Fatalf("hard delete: %d %s", h.Code, h.Raw)
				}
				if g := call(t, "GET", "/"+rc.path+"/"+id, tok, nil); g.Code != 404 {
					t.Fatalf("after hard delete: %d", g.Code)
				}
			}
		})
	}
}

func TestAPI_StockTransferPendingToCompleted(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	ms := "ms_" + storeA()
	stockOf := func(wh string) float64 {
		g := call(t, "GET", "/products/"+fx.ProductA2.Hex(), tok, nil)
		return num(get(g.Body, "stock."+wh+".qty"))
	}
	beforeMain, beforeWH := stockOf(ms), stockOf(fx.WarehouseA.Hex())
	cr := call(t, "POST", "/stock-transfers", tok, M{"storeId": storeA(), "date": "2026-10-05T10:00", "fromWarehouseId": ms,
		"toWarehouseId": fx.WarehouseA.Hex(), "status": "pending", "items": []M{{"productId": fx.ProductA2.Hex(), "qty": 2, "unitPrice": 9}}})
	if cr.Code != 201 || cr.Body["status"] != "pending" || !strings.HasPrefix(str(cr.Body["id"]), "sto_") {
		t.Fatalf("pending: %d %s", cr.Code, cr.Raw)
	}
	time.Sleep(500 * time.Millisecond)
	if stockOf(ms) != beforeMain {
		t.Fatal("a pending transfer must not move stock")
	}
	id := str(cr.Body["id"])
	done := call(t, "PATCH", "/stock-transfers/"+id, tok, M{"status": "completed"}, "X-Change-Reason", "completed")
	if done.Code != 200 || done.Body["status"] != "completed" || done.Body["replacedId"] != id {
		t.Fatalf("complete: %d %s", done.Code, done.Raw)
	}
	eventually(t, "transfer stock", func() bool {
		return stockOf(ms) == beforeMain-2 && stockOf(fx.WarehouseA.Hex()) == beforeWH+2
	})
	if g := call(t, "GET", "/stock-transfers/"+id, tok, nil); g.Code != 404 {
		t.Fatalf("pending record is replaced: %d", g.Code)
	}
	if r := call(t, "PATCH", "/stock-transfers/"+str(done.Body["id"]), tok, M{"status": "pending"}); r.Code != 409 {
		t.Fatalf("reopen completed: %d", r.Code)
	}
}

// TestAPI_ReturnDocuments: purchase / non-VAT / quotation returns through the
// adapter (each needs its source document and the legacy-required fields).
func TestAPI_ReturnDocuments(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	sA, ms, dt := storeA(), "ms_"+storeA(), "2026-10-05T10:00"
	line := func(pid string, qty, price, vat float64) []M {
		return []M{{"productId": pid, "qty": qty, "unitPrice": price, "unitDiscount": 0, "warehouseId": ms, "vatPercent": vat}}
	}
	pr := call(t, "POST", "/purchase-returns", tok, M{"storeId": sA, "date": dt, "purchaseId": fx.PurchaseA1.Hex(), "vendorId": fx.VendorA1.Hex(),
		"items": line(fx.ProductA1.Hex(), 1, 80, 15)})
	if pr.Code != 201 || pr.Body["purchaseId"] != fx.PurchaseA1.Hex() {
		t.Fatalf("purchase return: %d %s", pr.Code, pr.Raw)
	}
	if d := rawDoc(t, sA, "purchasereturn", str(pr.Body["id"])); hexOf(d["purchase_returned_by"]) != fx.ManagerA.Hex() {
		t.Errorf("purchase_returned_by defaults to the caller: %v", d["purchase_returned_by"])
	}
	ns := call(t, "POST", "/nonvat-sales", tok, M{"storeId": sA, "date": dt, "customerId": fx.CustomerA1.Hex(), "items": line(fx.ProductA2.Hex(), 2, 30, 0)})
	if ns.Code != 201 {
		t.Fatalf("nonvat sale: %d %s", ns.Code, ns.Raw)
	}
	nr := call(t, "POST", "/nonvat-returns", tok, M{"storeId": sA, "date": dt, "orderId": ns.Body["id"], "customerId": fx.CustomerA1.Hex(),
		"items": line(fx.ProductA2.Hex(), 1, 30, 0)})
	if nr.Code != 201 {
		t.Fatalf("nonvat return: %d %s", nr.Code, nr.Raw)
	}
	q := call(t, "POST", "/quotations", tok, M{"storeId": sA, "date": dt, "customerId": fx.CustomerA1.Hex(), "type": "invoice", "status": "created",
		"validityDays": 7, "deliveryDays": 3, "items": line(fx.ProductA2.Hex(), 2, 25, 15)})
	if q.Code != 201 {
		t.Fatalf("quotation invoice: %d %s", q.Code, q.Raw)
	}
	qr := call(t, "POST", "/quotation-returns", tok, M{"storeId": sA, "date": dt, "quotationId": q.Body["id"], "customerId": fx.CustomerA1.Hex(),
		"items": line(fx.ProductA2.Hex(), 1, 25, 15)})
	if qr.Code != 201 {
		t.Fatalf("quotation return: %d %s", qr.Code, qr.Raw)
	}
}

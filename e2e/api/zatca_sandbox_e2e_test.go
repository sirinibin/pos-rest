//go:build e2e && zatca

package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ZATCA Phase 2 against ZATCA's real non-production (developer-portal)
// sandbox: device onboarding with a new store, then reporting and clearance of
// every document type the app sends. ZATCA's test taxpayer is VAT
// 399999999900003 / CRN 4030360927; the developer-portal sandbox accepts any
// OTP (E2E_ZATCA_OTP, default 12345, the sandbox OTP the owner gave). The server needs ZatcaPython/venv and the ZATCA Java SDK in
// ZatcaPython/utilities/fatoora-cli-simulation (ci/zatca-sdk.sh) and internet
// access. CI runs it in a report-only job so a sandbox outage can't block a
// deploy.

func zatcaSignupBody() M {
	b := SignupBody("SA")
	c := b["company"].(M)
	c["nameEn"] = "Maximum Speed Tech Supply LTD"
	c["nameAr"] = "شركة السرعة القصوى للتوريد"
	c["vatNo"] = "399999999900003"
	c["crNo"] = "4030360927"
	c["address"] = M{"buildingNo": "1234", "streetEn": "King Faisal Rd", "streetAr": "طريق الملك فيصل", "districtEn": "Al Safa",
		"districtAr": "الصفا", "cityEn": "Riyadh", "cityAr": "الرياض", "postalCode": "12345", "additionalNo": "6789", "shortAddress": "RRRD2929"}
	return b
}

func zatcaReport(t *testing.T, s *Store, path, id string) M {
	t.Helper()
	r := Must(t, Call(t, "POST", "/"+path+"/"+id+"/zatca/report", s.Token, M{}), 200, "report "+path)
	z, _ := r.Body["zatca"].(M)
	if z == nil {
		t.Fatalf("report %s: no zatca block: %s", path, r)
	}
	return z
}

// zatcaAccepted fails unless ZATCA accepted the document (reported or cleared)
// with a hash, QR and UUID.
func zatcaAccepted(t *testing.T, what string, z M, wantStatus string) {
	t.Helper()
	if S(z["status"]) != wantStatus || S(z["error"]) != "" || S(z["hash"]) == "" || S(z["qr"]) == "" || S(z["uuid"]) == "" {
		t.Fatalf("%s: want %s by ZATCA, got %v", what, wantStatus, z)
	}
}

func TestZatcaSandbox_OnboardingAndReporting(t *testing.T) {
	otp := envOr("E2E_ZATCA_OTP", "12345")
	s := SignupWith(t, zatcaSignupBody())
	if S(Get(s.Rec, "zatca.phase")) != "2" || Get(s.Rec, "zatca.connected") != false {
		t.Fatalf("new Saudi store: zatca %v", s.Rec["zatca"])
	}
	p := s.Product(t, 10, 20, 100)
	pid := S(p["id"])
	line := func(qty, price float64) M { return s.Line(pid, qty, price) }

	// the environment can only be chosen while the store has no documents
	cur := Read(t, s.Token, "stores", s.ID)
	Must(t, Call(t, "PATCH", "/stores/"+s.ID, s.Token, M{"zatca": M{"env": "NonProduction"}},
		"If-Match", Num(cur["version"]).String(), "X-Change-Reason", "e2e"), 200, "set NonProduction")

	// a sale before onboarding can't be reported
	early := Create(t, s.Token, "sales", M{"storeId": s.ID, "date": s.Now(), "items": []M{line(1, 20)},
		"payments": []M{{"date": s.Now(), "amount": 23, "method": "cash"}}})
	if r := Call(t, "POST", "/sales/"+S(early["id"])+"/zatca/report", s.Token, M{}); r.Code != 409 || r.ErrCode() != "zatca_not_connected" {
		t.Fatalf("report before onboarding: %s", r)
	}

	t.Run("onboarding", func(t *testing.T) {
		// OTP format is checked before calling ZATCA
		if r := Call(t, "POST", "/stores/"+s.ID+"/zatca/connect", s.Token, M{"otp": "12"}); r.Code != 400 || r.ErrCode() != "invalid_otp" {
			t.Fatalf("short OTP: %s", r)
		}
		// a staff member without settings rights can't onboard
		_, cashier := s.User(t, "r_cashier")
		if r := Call(t, "POST", "/stores/"+s.ID+"/zatca/connect", cashier, M{"otp": otp}); r.Code != 403 {
			t.Fatalf("cashier onboarding: %s", r)
		}
		r := Must(t, Call(t, "POST", "/stores/"+s.ID+"/zatca/connect", s.Token, M{"otp": otp}), 200, "onboarding")
		z := r.Body["zatca"].(M)
		if z["connected"] != true || S(z["pcsid"]) == "" || S(z["env"]) != "NonProduction" || S(z["connectedAt"]) == "" || z["reconnectNeeded"] != false {
			t.Fatalf("after onboarding: %v", z)
		}
		// secrets never leave the server
		for _, k := range []string{"privateKey", "secret", "binarySecurityToken", "production_secret", "private_key"} {
			if strings.Contains(r.Raw, `"`+k+`"`) {
				t.Errorf("store answer exposes %s", k)
			}
		}
	})
	if t.Failed() {
		return
	}

	var first M
	t.Run("simplified invoice reported", func(t *testing.T) {
		first = zatcaReport(t, s, "sales", S(early["id"]))
		zatcaAccepted(t, "simplified sale", first, "reported")
		if S(first["invoiceType"]) != "simplified" || Num(first["icv"]) < 1 {
			t.Fatalf("simplified sale: %v", first)
		}
		// reporting again changes nothing (no second submission, same chain position)
		again := zatcaReport(t, s, "sales", S(early["id"]))
		if S(again["hash"]) != S(first["hash"]) || Num(again["icv"]) != Num(first["icv"]) || S(again["status"]) != "reported" {
			t.Fatalf("re-report: %v vs %v", again, first)
		}
	})

	t.Run("invoice chain: ICV +1 and PIH = previous hash", func(t *testing.T) {
		if first == nil {
			t.Fatal("no first invoice")
		}
		sale := Create(t, s.Token, "sales", M{"storeId": s.ID, "date": s.Now(), "items": []M{line(3, 33.33)},
			"payments": []M{}})
		z := zatcaReport(t, s, "sales", S(sale["id"]))
		zatcaAccepted(t, "second simplified sale", z, "reported")
		if Num(z["icv"]) != Num(first["icv"])+1 || S(z["pih"]) != S(first["hash"]) {
			t.Fatalf("chain: icv %v→%v, pih %v, previous hash %v", first["icv"], z["icv"], z["pih"], first["hash"])
		}
		first = z
	})

	var b2b M
	t.Run("standard (B2B) invoice cleared", func(t *testing.T) {
		cust := Create(t, s.Token, "customers", M{"storeId": s.ID, "nameEn": "Buyer Trading Co", "nameAr": "شركة المشتري", "vatNo": "300000000000003",
			"crNo": "1010101010", "phone": "0512345679",
			"address": M{"buildingNo": "1111", "streetEn": "Tahlia", "streetAr": "التحلية", "districtEn": "Rawdah", "districtAr": "الروضة",
				"cityEn": "Jeddah", "cityAr": "جدة", "postalCode": "23434", "additionalNo": "2222"}})
		b2b = Create(t, s.Token, "sales", M{"storeId": s.ID, "date": s.Now(), "customerId": cust["id"],
			"items": []M{line(2, 150), line(1, 0.5)}, "payments": []M{}})
		z := zatcaReport(t, s, "sales", S(b2b["id"]))
		zatcaAccepted(t, "standard sale", z, "cleared")
		if S(z["invoiceType"]) != "standard" {
			t.Fatalf("B2B sale type: %v", z["invoiceType"])
		}
		// the cleared invoice is stored and still reads as cleared
		if got := Read(t, s.Token, "sales", S(b2b["id"])); S(Get(got, "zatca.status")) != "cleared" {
			t.Fatalf("re-read: %v", got["zatca"])
		}
		b2b["customer"] = cust

		// a return of the standard invoice is a standard credit note, cleared too
		ret := Create(t, s.Token, "sales-returns", M{"storeId": s.ID, "date": s.Now(), "orderId": b2b["id"],
			"items":    []M{{"productId": pid, "qty": 1, "unitPrice": 150, "unitDiscount": 0, "warehouseId": s.MS, "vatPercent": 15, "selected": true}},
			"payments": []M{}})
		zr := zatcaReport(t, s, "sales-returns", S(ret["id"]))
		zatcaAccepted(t, "standard credit note", zr, "cleared")
	})

	t.Run("credit note (sales return) reported", func(t *testing.T) {
		it := Objs(Read(t, s.Token, "sales", S(early["id"]))["items"])
		ret := Create(t, s.Token, "sales-returns", M{"storeId": s.ID, "date": s.Now(), "orderId": early["id"],
			"items":    []M{{"productId": pid, "qty": 1, "unitPrice": F(it[0], "unitPrice"), "unitDiscount": 0, "warehouseId": s.MS, "vatPercent": 15, "selected": true}},
			"payments": []M{{"date": s.Now(), "amount": 23, "method": "cash"}}})
		z := zatcaReport(t, s, "sales-returns", S(ret["id"]))
		zatcaAccepted(t, "credit note", z, "reported")
		if !strings.HasPrefix(S(z["invoiceType"]), "credit") {
			t.Fatalf("return type: %v", z["invoiceType"])
		}
	})

	t.Run("debit note (deposit) and credit note (withdrawal)", func(t *testing.T) {
		if b2b == nil {
			t.Fatal("no B2B customer")
		}
		cid := S(b2b["customer"].(M)["id"])
		dep := Create(t, s.Token, "deposits", M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "amount": 50, "method": "cash", "notes": "debit note"})
		zd := zatcaReport(t, s, "deposits", S(dep["id"]))
		if S(zd["status"]) != "reported" && S(zd["status"]) != "cleared" || S(zd["error"]) != "" || S(zd["hash"]) == "" {
			t.Fatalf("debit note: %v", zd)
		}
		wd := Create(t, s.Token, "withdrawals", M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "amount": 20, "method": "cash", "type": "refund", "notes": "credit note"})
		zw := zatcaReport(t, s, "withdrawals", S(wd["id"]))
		if S(zw["status"]) != "reported" && S(zw["status"]) != "cleared" || S(zw["error"]) != "" || S(zw["hash"]) == "" {
			t.Fatalf("credit note: %v", zw)
		}
	})

	t.Run("environment locked once documents exist", func(t *testing.T) {
		cur := Read(t, s.Token, "stores", s.ID)
		if Get(cur, "zatca.envLocked") != true {
			t.Fatalf("envLocked: %v", cur["zatca"])
		}
		r := Call(t, "PATCH", "/stores/"+s.ID, s.Token, M{"zatca": M{"env": "Production"}}, "If-Match", Num(cur["version"]).String(), "X-Change-Reason", "e2e")
		if r.Code < 400 {
			t.Fatalf("env change with documents: %s", r)
		}
	})

	t.Run("branch name change needs re-onboarding", func(t *testing.T) {
		cur := Read(t, s.Token, "stores", s.ID)
		Must(t, Call(t, "PATCH", "/stores/"+s.ID, s.Token, M{"branchEn": "Riyadh Branch 2"},
			"If-Match", Num(cur["version"]).String(), "X-Change-Reason", "e2e"), 200, "branch name change")
		after := Read(t, s.Token, "stores", s.ID)
		if Get(after, "zatca.reconnectNeeded") != true {
			t.Fatalf("sensitive change did not mark the store: %v", after["zatca"])
		}
		sale := Create(t, s.Token, "sales", M{"storeId": s.ID, "date": s.Now(), "items": []M{line(1, 20)},
			"payments": []M{{"date": s.Now(), "amount": 23, "method": "cash"}}})
		if r := Call(t, "POST", "/sales/"+S(sale["id"])+"/zatca/report", s.Token, M{}); r.Code != 409 {
			t.Fatalf("report while re-onboarding is needed: %s", r)
		}
		// onboarding again clears the mark and the chain continues
		Must(t, Call(t, "POST", "/stores/"+s.ID+"/zatca/connect", s.Token, M{"otp": otp}), 200, "re-onboarding")
		if after := Read(t, s.Token, "stores", s.ID); Get(after, "zatca.reconnectNeeded") != false || Get(after, "zatca.connected") != true {
			t.Fatalf("after re-onboarding: %v", after["zatca"])
		}
		z := zatcaReport(t, s, "sales", S(sale["id"]))
		zatcaAccepted(t, "sale after re-onboarding", z, "reported")
	})

	t.Run("disconnect", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/stores/"+s.ID+"/zatca/disconnect", s.Token, M{}), 200, "disconnect")
		if Get(r.Body, "zatca.connected") != false {
			t.Fatalf("after disconnect: %v", r.Body["zatca"])
		}
		sale := Create(t, s.Token, "sales", M{"storeId": s.ID, "date": s.Now(), "items": []M{line(1, 20)},
			"payments": []M{{"date": s.Now(), "amount": 23, "method": "cash"}}})
		if r := Call(t, "POST", "/sales/"+S(sale["id"])+"/zatca/report", s.Token, M{}); r.Code != 409 || r.ErrCode() != "zatca_not_connected" {
			t.Fatalf("report after disconnect: %s", r)
		}
	})
	if os.Getenv("E2E_ZATCA_KEEP") == "" {
		t.Logf("sandbox store %s (VAT 399999999900003) done", s.ID)
	}
}

// zatcaOnboarded signs up a sandbox store and onboards it.
func zatcaOnboarded(t *testing.T) *Store {
	t.Helper()
	s := SignupWith(t, zatcaSignupBody())
	cur := Read(t, s.Token, "stores", s.ID)
	Must(t, Call(t, "PATCH", "/stores/"+s.ID, s.Token, M{"zatca": M{"env": "NonProduction"}},
		"If-Match", Num(cur["version"]).String(), "X-Change-Reason", "e2e"), 200, "set NonProduction")
	Must(t, Call(t, "POST", "/stores/"+s.ID+"/zatca/connect", s.Token, M{"otp": envOr("E2E_ZATCA_OTP", "12345")}), 200, "onboarding")
	return s
}

// zatcaQRPublicKey is TLV tag 8 (the signing certificate's public key) of a
// Phase 2 QR code.
func zatcaQRPublicKey(t *testing.T, qr string) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(qr)
	if err != nil {
		t.Fatalf("QR is not base64: %v", err)
	}
	for i := 0; i+1 < len(b); {
		tag, n := b[i], int(b[i+1])
		if i+2+n > len(b) {
			break
		}
		if tag == 8 {
			return base64.StdEncoding.EncodeToString(b[i+2 : i+2+n])
		}
		i += 2 + n
	}
	t.Fatalf("QR has no public key (tag 8)")
	return ""
}

// Several users of the same store, and users of two different stores, report
// at the same time: every invoice is accepted once, each store's ICVs are
// consecutive with no gap or duplicate, every PIH is the previous invoice's
// hash, and each store signs with its own certificate.
func TestZatcaSandbox_ConcurrentReporting(t *testing.T) {
	const perStore = 6
	type inv struct {
		store *Store
		id    string
		tok   string
	}
	var stores []*Store
	var invs []inv
	for k := 0; k < 2; k++ {
		s := zatcaOnboarded(t)
		stores = append(stores, s)
		_, mgr := s.User(t, "r_manager")
		_, sales := s.User(t, "r_salesman")
		users := []string{s.Token, mgr, sales}
		p := s.Product(t, 10, 20, 1000)
		for i := 0; i < perStore; i++ {
			sale := Create(t, s.Token, "sales", M{"storeId": s.ID, "date": s.Now(), "items": []M{s.Line(S(p["id"]), float64(i+1), 20)},
				"payments": []M{}})
			invs = append(invs, inv{s, S(sale["id"]), users[i%len(users)]})
		}
	}
	// plus the same invoice reported by two users at once
	dup := invs[0]
	_, otherUser := dup.store.User(t, "r_manager")
	jobs := append([]inv{}, invs...)
	jobs = append(jobs, inv{dup.store, dup.id, otherUser})

	results := make([]Resp, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func(i int, j inv) {
			defer wg.Done()
			results[i] = Call(t, "POST", "/sales/"+j.id+"/zatca/report", j.tok, M{})
		}(i, j)
	}
	wg.Wait()
	for i, r := range results {
		if r.Code != 200 {
			t.Errorf("report %d (%s): %s", i, jobs[i].id, r)
		}
	}
	if t.Failed() {
		return
	}
	for _, s := range stores {
		var zs []M
		lost := []string{}
		for _, j := range invs {
			if j.store == s {
				z, _ := Read(t, s.Token, "sales", j.id)["zatca"].(M)
				if S(z["status"]) != "reported" {
					// the report call answered 200 but the invoice isn't reported
					lost = append(lost, fmt.Sprintf("%s (%v, %v)", j.id, z["status"], z["error"]))
					continue
				}
				zatcaAccepted(t, "concurrent sale "+j.id, z, "reported")
				zs = append(zs, z)
			}
		}
		zatcaRaceBug(t, "NEW-ZATCA-LOST-REPORT", fmt.Sprintf("store %s: %d invoice(s) not reported after concurrent reporting answered 200: %v", s.ID, len(lost), lost), len(lost) > 0)
		sort.Slice(zs, func(a, b int) bool { return Num(zs[a]["icv"]) < Num(zs[b]["icv"]) })
		broken := 0
		for i, z := range zs {
			if i > 0 {
				if d := Num(z["icv"]) - Num(zs[i-1]["icv"]); d != 1 {
					// a gap is the lost invoice's ICV; a duplicate is never acceptable
					if d < 1 || int(d-1) > len(lost) {
						t.Errorf("store %s: ICV %v follows %v (gap or duplicate)", s.ID, z["icv"], zs[i-1]["icv"])
					}
				}
				if S(z["pih"]) != S(zs[i-1]["hash"]) {
					broken++
				}
			}
			// every invoice carries a signing certificate (developer-portal
			// certificates are ZATCA's shared test certificate)
			_ = zatcaQRPublicKey(t, S(z["qr"]))
		}
		if len(zs)+len(lost) != perStore || len(zs) == 0 {
			t.Errorf("store %s: %d reported + %d lost of %d", s.ID, len(zs), len(lost), perStore)
			continue
		}
		// models/sales_zatca.go takes the PIH from the last reported invoice
		// with no lock, so invoices reported at the same moment chain to the
		// same predecessor
		zatcaRaceBug(t, "NEW-ZATCA-PIH-RACE", fmt.Sprintf("store %s: %d of %d PIH links broken by concurrent reporting", s.ID, broken, perStore-1), broken > 0)
	}
	// each store signs with its own private key (generated at onboarding)
	if a, b := zatcaPrivateKey(t, stores[0].ID), zatcaPrivateKey(t, stores[1].ID); a == "" || a == b {
		t.Errorf("stores share or lack a ZATCA private key (same=%v)", a == b)
	}
	// the invoice reported twice at once is submitted to ZATCA once
	h1, h2 := S(Get(results[0].Body, "zatca.hash")), S(Get(results[len(results)-1].Body, "zatca.hash"))
	zatcaRaceBug(t, "NEW-ZATCA-DOUBLE-REPORT", "the same invoice reported by two users at once was signed and submitted twice ("+h1+" / "+h2+")", h1 != h2)
}

// zatcaRaceBug is KnownBug for a race: it may not show on every run, so a run
// that doesn't reproduce it passes quietly.
func zatcaRaceBug(t *testing.T, id, what string, seen bool) {
	t.Helper()
	if seen {
		KnownBug(t, id, what, true)
	} else {
		t.Logf("%s not reproduced on this run", id)
	}
}

// zatcaPrivateKey reads the store's ZATCA private key straight from MongoDB
// (E2E_MONGO_URI, main database E2E_MONGO_DB, default t1_e2e).
func zatcaPrivateKey(t *testing.T, storeID string) string {
	t.Helper()
	uri := os.Getenv("E2E_MONGO_URI")
	if uri == "" {
		t.Fatal("E2E_MONGO_URI is needed to compare the stores' ZATCA keys")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("mongo: %v", err)
	}
	defer c.Disconnect(ctx)
	oid, _ := primitive.ObjectIDFromHex(storeID)
	var st struct {
		Zatca struct {
			PrivateKey string `bson:"private_key"`
		} `bson:"zatca"`
	}
	if err := c.Database(envOr("E2E_MONGO_DB", "t1_e2e")).Collection("store").FindOne(ctx, bson.M{"_id": oid}).Decode(&st); err != nil {
		t.Fatalf("read store %s: %v", storeID, err)
	}
	return st.Zatca.PrivateKey
}

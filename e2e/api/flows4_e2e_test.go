//go:build e2e

package apie2e

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// Functional tests, part 4: workshop (vehicles, repair jobs), customer
// packages and signatures.

func TestFlow_VehicleBelongsToACustomer(t *testing.T) {
	sid := newStore(t)
	customer := createCustomer(t, sid, "Car Owner")
	good := func() map[string]interface{} {
		return map[string]interface{}{"store_id": sid, "customer_id": customer, "brand": "Toyota", "model": "Camry",
			"year": 2022, "vehicle_number": "ABC 1234", "current_km": 15000}
	}
	code, res := in(t, sid, "POST", "/v1/vehicle", good())
	v := mustOK(t, "create vehicle", code, res)
	if !strings.EqualFold(str(v, "customer_name"), "Car Owner") {
		t.Fatalf("customer_name = %q, want the owner's name", str(v, "customer_name"))
	}

	for name, edit := range map[string]func(map[string]interface{}){
		"no brand":         func(b map[string]interface{}) { delete(b, "brand") },
		"no model":         func(b map[string]interface{}) { delete(b, "model") },
		"no customer":      func(b map[string]interface{}) { delete(b, "customer_id") },
		"unknown customer": func(b map[string]interface{}) { b["customer_id"] = "5f1d7f3e9b1e8a3f4c2b1a00" },
	} {
		b := good()
		edit(b)
		code, res := in(t, sid, "POST", "/v1/vehicle", b)
		mustReject(t, name, code, res)
	}

	// Editing a vehicle changes its details but not the odometer, which the
	// edit form can't lower (it is kept from the stored record).
	b := good()
	b["color"] = "Red"
	b["current_km"] = 1
	code, res = in(t, sid, "PUT", "/v1/vehicle/"+str(v, "id"), b)
	u := mustOK(t, "update vehicle", code, res)
	if str(u, "color") != "Red" {
		t.Fatalf("color = %q, want Red", str(u, "color"))
	}
	wantNum(t, "km after edit", num(u, "current_km"), 15000)

	code, res = in(t, sid, "GET", "/v1/vehicle?search[customer_id]="+customer, nil)
	if code != http.StatusOK || !strings.Contains(string(res.Result), str(v, "id")) {
		t.Fatalf("vehicle missing from the customer's vehicles: HTTP %d", code)
	}

	code, res = in(t, sid, "DELETE", "/v1/vehicle/"+str(v, "id"), nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("delete vehicle: HTTP %d %v", code, res.Errors)
	}
}

func TestFlow_RepairJobTotalsAndUniqueNumbers(t *testing.T) {
	sid := newStore(t)
	customer := createCustomer(t, sid, "Workshop Customer")
	widget := createProduct(t, sid, "Brake Pad", 100, 60)
	job := func(title string) (int, apiResponse) {
		return in(t, sid, "POST", "/v1/repair-job", map[string]interface{}{
			"store_id": sid, "customer_id": customer, "title": title, "vat_percent": 15,
			"labour_charge": 115, // labour is entered VAT-inclusive
			"parts":         []map[string]interface{}{{"product_id": widget, "name": "Brake Pad", "qty": 2, "unit_price": 100, "unit_price_with_vat": 115}},
		})
	}
	code, res := job("")
	mustReject(t, "job without a title", code, res, "title")

	code, res = job("Brakes")
	first := mustOK(t, "first job", code, res)
	wantNum(t, "parts_total", num(first, "parts_total"), 200)
	wantNum(t, "parts_total_with_vat", num(first, "parts_total_with_vat"), 230)
	// 200 parts + 100 labour before VAT, plus 15% VAT.
	wantNum(t, "total_with_vat", num(first, "total_with_vat"), 345)

	code, res = job("Oil change")
	second := mustOK(t, "second job", code, res)

	// Deleting a job must not let its number, or a live job's, be reissued.
	code, res = in(t, sid, "DELETE", "/v1/repair-job/"+str(first, "id"), nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("delete job: HTTP %d %v", code, res.Errors)
	}
	seen := map[string]bool{str(first, "job_number"): true, str(second, "job_number"): true}
	if len(seen) != 2 {
		t.Fatalf("first two jobs share the number %s", str(first, "job_number"))
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var dup []string
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, res := job(fmt.Sprintf("Concurrent %d", i))
			n := str(mustOK(t, "concurrent job", code, res), "job_number")
			mu.Lock()
			if seen[n] {
				dup = append(dup, n)
			}
			seen[n] = true
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	if len(dup) > 0 {
		t.Fatalf("repair job numbers reissued: %v", dup)
	}
}

func TestFlow_CustomerPackageNamesAreUnique(t *testing.T) {
	sid := newStore(t)
	customer := createCustomer(t, sid, "Package Customer")
	body := map[string]interface{}{"store_id": sid, "customer_id": customer, "name": "10 Washes", "price": 300, "visits": 10, "valid_days": 90}
	code, res := in(t, sid, "POST", "/v1/customer-package", body)
	p := mustOK(t, "create package", code, res)

	code, res = in(t, sid, "POST", "/v1/customer-package", body)
	mustReject(t, "duplicate name", code, res, "name")
	code, res = in(t, sid, "POST", "/v1/customer-package", map[string]interface{}{"store_id": sid, "price": 10})
	mustReject(t, "no name", code, res, "name")

	code, res = in(t, sid, "GET", "/v1/customer-package/"+str(p, "id"), nil)
	if got := str(mustOK(t, "view package", code, res), "name"); got != "10 Washes" {
		t.Fatalf("name = %q", got)
	}
	code, res = in(t, sid, "DELETE", "/v1/customer-package/"+str(p, "id"), nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("delete package: HTTP %d %v", code, res.Errors)
	}
}

func TestFlow_SignatureNeedsValidImageData(t *testing.T) {
	sid := newStore(t)
	const png = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="
	code, res := in(t, sid, "POST", "/v1/signature", map[string]interface{}{"store_id": sid, "name": "Manager", "signature_content": png})
	s := mustOK(t, "create signature", code, res)

	for name, body := range map[string]map[string]interface{}{
		"no name":        {"store_id": sid, "signature_content": png},
		"no content":     {"store_id": sid, "name": "Cashier"},
		"not base64":     {"store_id": sid, "name": "Cashier", "signature_content": "data:image/png;base64,***"},
		"duplicate name": {"store_id": sid, "name": "Manager", "signature_content": png},
	} {
		code, res := in(t, sid, "POST", "/v1/signature", body)
		mustReject(t, name, code, res)
	}

	code, res = in(t, sid, "GET", "/v1/signature/"+str(s, "id"), nil)
	mustOK(t, "view signature", code, res)
}

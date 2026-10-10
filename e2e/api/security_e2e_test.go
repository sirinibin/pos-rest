//go:build e2e

package apie2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A user limited to some stores must not read or write any other store by
// passing its id, in the query (search[store_id]) or in the body (store_id).
func TestSecurity_UserCannotReachAnotherStore(t *testing.T) {
	tok := authToken(t)
	mine, theirs := newStore(t), newStore(t)
	widget := createProduct(t, theirs, "Their Widget", 100, 60)
	customer := createCustomer(t, theirs, "Their Customer", map[string]interface{}{"credit_limit": 1000})
	code, res := in(t, theirs, "POST", "/v1/order", saleBody(theirs, customer, agoStr(time.Hour),
		[]line{{id: widget, name: "Their Widget", qty: 1, price: 100, cost: 60}}))
	theirSale := str(mustOK(t, "their sale", code, res), "id")

	mail := fmt.Sprintf("e2e-scoped-%s@startpos.test", runID)
	code, res = call(t, "POST", "/v1/user", tok, map[string]interface{}{
		"name": "Scoped Manager", "email": mail, "mob": "0501234568", "password": "Scoped-Passw0rd!",
		"role": "Manager", "store_ids": []string{mine},
	})
	mustOK(t, "create scoped user", code, res)
	_, _, scoped := login(t, mail, "Scoped-Passw0rd!")
	if scoped == "" {
		t.Fatalf("scoped user cannot log in")
	}

	// Their own store works.
	code, res = call(t, "GET", "/v1/order?search[store_id]="+mine, scoped, nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("own store: HTTP %d %v", code, res.Errors)
	}
	code, res = call(t, "GET", "/v1/store/"+mine, scoped, nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("own store details: HTTP %d %v", code, res.Errors)
	}

	denied := func(what string, code int, res apiResponse) {
		t.Helper()
		if code != http.StatusForbidden || res.Status {
			t.Fatalf("%s in another store: HTTP %d status=%v, want 403", what, code, res.Status)
		}
	}
	for _, p := range []string{"/v1/order", "/v1/product", "/v1/customer", "/v1/ledger", "/v1/order/" + theirSale} {
		code, res = call(t, "GET", p+"?search[store_id]="+theirs, scoped, nil)
		denied("GET "+p, code, res)
		if strings.Contains(string(res.Result), theirSale) {
			t.Fatalf("GET %s leaked the other store's sale", p)
		}
	}
	code, res = call(t, "POST", "/v1/customer", scoped, map[string]interface{}{"store_id": theirs, "name": "Planted Customer"})
	denied("POST /v1/customer (store in body)", code, res)
	code, res = call(t, "POST", "/v1/customer?search[store_id]="+mine, scoped, map[string]interface{}{"store_id": theirs, "name": "Planted Customer"})
	denied("POST /v1/customer (own store in query, other in body)", code, res)
	code, res = call(t, "PUT", "/v1/order/"+theirSale+"?search[store_id]="+theirs, scoped, map[string]interface{}{"store_id": theirs})
	denied("PUT their sale", code, res)

	// The admin who created the user still reaches both.
	for _, s := range []string{mine, theirs} {
		code, res = call(t, "GET", "/v1/order?search[store_id]="+s, tok, nil)
		if code != http.StatusOK || !res.Status {
			t.Fatalf("admin GET orders: HTTP %d %v", code, res.Errors)
		}
	}
}

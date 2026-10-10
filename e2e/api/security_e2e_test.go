//go:build e2e

package apie2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

	code, res = call(t, "POST", "/v1/store/zatca/disconnect", scoped, map[string]interface{}{"id": theirs})
	denied("ZATCA disconnect (store as id)", code, res)
	code, res = call(t, "PUT", "/v1/store/"+theirs, scoped, map[string]interface{}{"name": "Taken Over"})
	denied("PUT their store", code, res)

	// The admin who created the user still reaches both.
	for _, s := range []string{mine, theirs} {
		code, res = call(t, "GET", "/v1/order?search[store_id]="+s, tok, nil)
		if code != http.StatusOK || !res.Status {
			t.Fatalf("admin GET orders: HTTP %d %v", code, res.Errors)
		}
	}
}

// No endpoint returns password hashes; the user list used to return all of them.
func TestSecurity_NoPasswordHashesInResponses(t *testing.T) {
	tok := authToken(t)
	code, res := call(t, "GET", "/v1/me", tok, nil)
	me := mustOK(t, "me", code, res)
	for _, p := range []string{"/v1/me", "/v1/user?limit=100", "/v1/user/" + str(me, "id"), "/v1/mcp/me"} {
		code, res := call(t, "GET", p, tok, nil)
		if code != http.StatusOK {
			t.Fatalf("GET %s: HTTP %d", p, code)
		}
		if strings.Contains(string(res.Result), `"password"`) || strings.Contains(string(res.Result), "$2a$") {
			t.Fatalf("GET %s returns a password hash", p)
		}
	}
}

// The MCP store list and login only show the stores the user may use.
func TestSecurity_MCPListsOnlyTheUsersStores(t *testing.T) {
	tok := authToken(t)
	mine, theirs := newStore(t), newStore(t)
	mail := fmt.Sprintf("e2e-mcp-%s@startpos.test", runID)
	code, res := call(t, "POST", "/v1/user", tok, map[string]interface{}{
		"name": "MCP Manager", "email": mail, "mob": "0501234569", "password": "Mcp-Passw0rd!",
		"role": "Manager", "store_ids": []string{mine},
	})
	mustOK(t, "create scoped user", code, res)

	code, raw := rawJSON(t, "POST", "/v1/mcp/login", "", map[string]interface{}{"email": mail, "password": "Mcp-Passw0rd!"})
	if code != http.StatusOK {
		t.Fatalf("MCP login: HTTP %d %s", code, raw)
	}
	if strings.Contains(raw, theirs) || !strings.Contains(raw, mine) {
		t.Fatalf("MCP login store list must hold the user's store and only it")
	}
	var login struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal([]byte(raw), &login)
	if login.AccessToken == "" {
		t.Fatalf("MCP login returned no access_token: %s", raw)
	}
	code, raw = rawJSON(t, "GET", "/v1/mcp/stores", login.AccessToken, nil)
	if code != http.StatusOK || strings.Contains(raw, theirs) || !strings.Contains(raw, mine) {
		t.Fatalf("MCP stores: HTTP %d; must hold the user's store and only it", code)
	}
	code, _ = rawJSON(t, "GET", "/v1/mcp/orders?store_id="+theirs, login.AccessToken, nil)
	if code != http.StatusForbidden {
		t.Fatalf("MCP orders of another store: HTTP %d, want 403", code)
	}

	code, _ = rawJSON(t, "POST", "/v1/mcp/login", "", map[string]interface{}{"email": mail, "password": "wrong"})
	if code == http.StatusOK {
		t.Fatalf("MCP login with a wrong password succeeded")
	}
}

// rawJSON calls an endpoint whose answer is not the usual {status, result}
// envelope (the MCP layer) and returns the body as text.
func rawJSON(t *testing.T, method, path, auth string, body interface{}) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, baseURL()+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	for k, v := range freshIP() {
		req.Header.Set(k, v)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(out)
}

// A Manager limited to their stores can manage their own staff but not
// other companies' users, admins, or grant access they don't have.
func TestSecurity_ManagersOnlyManageTheirOwnStaff(t *testing.T) {
	tok := authToken(t)
	mine, theirs := newStore(t), newStore(t)
	newUser := func(by, label, role string, stores []string, extra ...map[string]interface{}) (int, apiResponse) {
		body := map[string]interface{}{
			"name": "E2E " + label, "email": fmt.Sprintf("e2e-%s-%s@startpos.test", label, runID),
			"mob": "0501234570", "password": "Staff-Passw0rd!", "role": role, "store_ids": stores,
		}
		for _, e := range extra {
			for k, v := range e {
				body[k] = v
			}
		}
		return call(t, "POST", "/v1/user", by, body)
	}
	code, res := newUser(tok, "mgr-a", "Manager", []string{mine})
	managerA := mustOK(t, "manager A", code, res)
	code, res = newUser(tok, "mgr-b", "Manager", []string{theirs})
	managerB := mustOK(t, "manager B", code, res)
	_, _, mgr := login(t, str(managerA, "email"), "Staff-Passw0rd!")
	if mgr == "" {
		t.Fatalf("manager A cannot log in")
	}
	_, res = call(t, "GET", "/v1/me", tok, nil)
	adminID := str(resultMap(t, res), "id")

	forbidden := func(what string, code int, res apiResponse) {
		t.Helper()
		if code != http.StatusForbidden || res.Status {
			t.Fatalf("%s: HTTP %d status=%v %v, want 403", what, code, res.Status, res.Errors)
		}
	}
	code, res = newUser(mgr, "x1", "SalesMan", []string{theirs})
	forbidden("create a user in another store", code, res)
	code, res = newUser(mgr, "x2", "SalesMan", []string{})
	forbidden("create a user with no store list (every store)", code, res)
	code, res = newUser(mgr, "x3", "SalesMan", []string{mine}, map[string]interface{}{"admin": true})
	forbidden("create an admin-flagged user", code, res)

	code, res = call(t, "DELETE", "/v1/user/"+adminID, mgr, nil)
	forbidden("delete the admin", code, res)
	code, res = call(t, "DELETE", "/v1/user/"+str(managerB, "id"), mgr, nil)
	forbidden("delete another company's manager", code, res)
	edit := map[string]interface{}{"name": "Hijacked", "email": str(managerB, "email"), "mob": "0501234570",
		"role": "Manager", "store_ids": []string{theirs}, "password": "Hijack-Passw0rd!"}
	code, res = call(t, "PUT", "/v1/user/"+str(managerB, "id"), mgr, edit)
	forbidden("edit another company's manager", code, res)
	code, res = call(t, "GET", "/v1/user/"+str(managerB, "id"), mgr, nil)
	forbidden("view another company's manager", code, res)
	if _, _, tokB := login(t, str(managerB, "email"), "Staff-Passw0rd!"); tokB == "" {
		t.Fatalf("manager B's password was changed by manager A")
	}

	self := map[string]interface{}{"name": "E2E mgr-a", "email": str(managerA, "email"), "mob": "0501234570",
		"role": "Admin", "store_ids": []string{mine}}
	code, res = call(t, "PUT", "/v1/user/"+str(managerA, "id"), mgr, self)
	if res.Status {
		t.Fatalf("a manager made themselves Admin: HTTP %d", code)
	}

	// Their own staff: create, edit, delete.
	code, res = newUser(mgr, "staff-a", "SalesMan", []string{mine})
	staff := mustOK(t, "manager creates staff in their store", code, res)
	code, res = call(t, "PUT", "/v1/user/"+str(staff, "id"), mgr, map[string]interface{}{"name": "E2E Staff Renamed",
		"email": str(staff, "email"), "mob": "0501234570", "role": "SalesMan", "store_ids": []string{mine}})
	mustOK(t, "manager edits their staff", code, res)
	code, res = call(t, "GET", "/v1/user/"+str(staff, "id"), mgr, nil)
	mustOK(t, "manager views their staff", code, res)
	code, res = call(t, "DELETE", "/v1/user/"+str(staff, "id"), mgr, nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("manager deletes their staff: HTTP %d %v", code, res.Errors)
	}
}

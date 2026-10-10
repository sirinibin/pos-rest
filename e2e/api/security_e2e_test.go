//go:build e2e

package api

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/erp"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ---------- helpers (sec*) ----------

// secStore signs up a company without the starter catalog (faster; these
// tests don't need products).
func secStore(t testing.TB) *Store {
	t.Helper()
	b := SignupBody("")
	b["company"].(M)["starterCatalog"] = false
	return SignupWith(t, b)
}

// secNoSecrets fails when an answer carries a password field or a bcrypt hash.
func secNoSecrets(t testing.TB, what string, r Resp) {
	t.Helper()
	if strings.Contains(r.Raw, "$2a$") || strings.Contains(r.Raw, "$2b$") || strings.Contains(r.Raw, `"password":`) {
		t.Errorf("%s: answer leaks a password/hash: %s", what, r)
	}
}

// secRefused: a request for another company's data must be refused (403 or
// 404), never answered.
func secRefused(t testing.TB, what string, r Resp) {
	t.Helper()
	if r.Code != 403 && r.Code != 404 {
		t.Errorf("%s: want 403/404, got %s", what, r)
	}
}

func secLoginResp(t testing.TB, email, pw string) Resp {
	t.Helper()
	return Call(t, "POST", "/auth/login", "", M{"email": email, "password": pw})
}

// secStaff creates a staff user and returns its id, e-mail, password and token.
func secStaff(t testing.TB, s *Store, role string) (id, email, pw, tok string) {
	t.Helper()
	email = "staff" + Uniq() + Digits(4) + "@e2e.example"
	pw = "Staff@" + Digits(6)
	u := Create(t, s.Token, "users", M{"name": "Staff " + role, "email": email, "phone": "05" + Digits(8), "role": role,
		"storeIds": []string{s.ID}, "password": pw})
	return S(u["id"]), email, pw, Login(t, email, pw)
}

var (
	secDBOnce sync.Once
	secDBVal  *mongo.Database
	secDBErr  error
)

// secDB is the server's MongoDB (E2E_MONGO_URI, E2E_MONGO_DB). Only the
// platform-admin checks need it: a platform admin can't be made through the
// API, so the test promotes a fresh user directly.
func secDB() (*mongo.Database, error) {
	secDBOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cl, err := mongo.Connect(ctx, options.Client().ApplyURI(envOr("E2E_MONGO_URI", "mongodb://127.0.0.1:27017")).
			SetServerSelectionTimeout(4*time.Second))
		if err == nil {
			err = cl.Ping(ctx, nil)
		}
		if err != nil {
			secDBErr = err
			return
		}
		secDBVal = cl.Database(envOr("E2E_MONGO_DB", envOr("MONGO_DB", "t1_e2e")))
	})
	return secDBVal, secDBErr
}

// secPlatformAdmin signs up a company and makes its owner a platform admin
// (legacy admin). Returns nil when the database can't be reached.
func secPlatformAdmin(t testing.TB) (tok string, db *mongo.Database) {
	t.Helper()
	db, err := secDB()
	if err != nil {
		t.Logf("platform-admin checks need the server's MongoDB (E2E_MONGO_URI): %v", err)
		return "", nil
	}
	p := secStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := db.Collection("user").UpdateOne(ctx, bson.M{"email": p.Email}, bson.M{"$set": bson.M{"admin": true, "role": "Admin"}})
	if err != nil || res.MatchedCount != 1 {
		t.Logf("cannot promote a platform admin in %s (wrong E2E_MONGO_DB?): %v %+v", db.Name(), err, res)
		return "", nil
	}
	return Login(t, p.Email, p.Password), db
}

// secMultipart posts a multipart form (legacy image uploads) with an optional token.
func secMultipart(t testing.TB, url, token string, fields map[string]string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("image", "e2e.png")
	// 1x1 PNG
	_, _ = fw.Write([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89"))
	_ = mw.Close()
	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: no answer: %v", url, err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func secHas(rows []M, id string) bool {
	for _, r := range rows {
		if S(r["id"]) == id {
			return true
		}
	}
	return false
}

// ---------- sign-up ----------

func TestAuth_SignupValidation(t *testing.T) {
	t.Parallel()
	set := func(b M, path string, v interface{}) {
		parts := strings.Split(path, ".")
		cur := b
		for _, p := range parts[:len(parts)-1] {
			cur = cur[p].(M)
		}
		if v == nil {
			delete(cur, parts[len(parts)-1])
		} else {
			cur[parts[len(parts)-1]] = v
		}
	}
	cases := []struct {
		country, path string
		value         interface{}
		field         string
	}{
		{"", "owner.name", "  ", "owner.name"},
		{"", "owner.email", nil, "owner.email"},
		{"", "owner.email", "not-an-email", "owner.email"},
		{"", "owner.mobile", "", "owner.mobile"},
		{"", "owner.mobile", "12345", "owner.mobile"},
		{"", "owner.password", "Ab1!", "owner.password"},
		{"", "owner.password", "alllowercase", "owner.password"},
		{"", "company.countryCode", "ZZ", "company.countryCode"},
		{"", "company.nameEn", "", "company.nameEn"},
		{"", "company.nameAr", "", "company.nameAr"},
		{"", "company.nameAr", "English only", "company.nameAr"},
		{"", "company.vatNo", "", "company.vatNo"},
		{"", "company.vatNo", "12345", "company.vatNo"},
		{"", "company.crNo", "", "company.crNo"},
		{"", "company.crNo", "12", "company.crNo"},
		{"", "company.mobile", "", "company.mobile"},
		{"", "company.mobile", "abc", "company.mobile"},
		{"", "company.type", "spaceship", "company.type"},
		{"", "company.plan", "gold", "company.plan"},
		{"", "company.address.buildingNo", "12", "company.address.buildingNo"},
		{"", "company.address.streetEn", "", "company.address.streetEn"},
		{"", "company.address.streetAr", "Olaya", "company.address.streetAr"},
		{"", "company.address.districtEn", "", "company.address.districtEn"},
		{"", "company.address.districtAr", "", "company.address.districtAr"},
		{"", "company.address.cityEn", "", "company.address.cityEn"},
		{"", "company.address.postalCode", "1234", "company.address.postalCode"},
		{"", "company.address.additionalNo", "x", "company.address.additionalNo"},
		{"", "company.address.shortAddress", "12345678", "company.address.shortAddress"},
		{"AE", "company.address.streetEn", "", "company.address.streetEn"},
		{"AE", "company.address.cityEn", "", "company.address.cityEn"},
		{"AE", "company.vatNo", "123", "company.vatNo"},
		{"IN", "company.address.stateCode", nil, "company.address.stateCode"},
		{"IN", "company.address.postalCode", nil, "company.address.postalCode"},
	}
	for _, tc := range cases {
		b := SignupBody(tc.country)
		set(b, tc.path, tc.value)
		r := Call(t, "POST", "/auth/signup", "", b)
		if r.Code != 400 || r.ErrCode() != "validation" || r.ErrField(tc.field) == "" {
			t.Errorf("%s %s=%v: want 400 with error.fields[%s], got %s", tc.country, tc.path, tc.value, tc.field, r)
		}
		if r.Body["accessToken"] != nil {
			t.Errorf("%s: a rejected sign-up issued a token", tc.path)
		}
	}
	// empty and broken bodies
	r := Call(t, "POST", "/auth/signup", "", M{})
	if r.Code != 400 || r.ErrField("owner.email") == "" || r.ErrField("company.nameEn") == "" || r.ErrField("company.plan") == "" {
		t.Errorf("empty sign-up: %s", r)
	}
	if r := Call(t, "POST", "/auth/signup", "", "{"); r.Code != 400 || r.ErrCode() != "malformed_json" {
		t.Errorf("malformed sign-up: %s", r)
	}
	// India: the Arabic name is optional (SignupBody("IN") has none) but must be Arabic when given
	b := SignupBody("IN")
	b["company"].(M)["nameAr"] = "Latin"
	if r := Call(t, "POST", "/auth/signup", "", b); r.Code != 400 || r.ErrField("company.nameAr") == "" {
		t.Errorf("IN latin nameAr: %s", r)
	}
}

func TestAuth_SignupSuccessAndDuplicates(t *testing.T) {
	t.Parallel()
	b := SignupBody("")
	b["company"].(M)["starterCatalog"] = false
	mixed := "  E2E.Mixed" + Uniq() + Digits(3) + "@E2E.Example "
	b["owner"].(M)["email"] = mixed
	r := Must(t, Call(t, "POST", "/auth/signup", "", b), 201, "sign-up")
	secNoSecrets(t, "sign-up", r)
	lower := strings.ToLower(strings.TrimSpace(mixed))
	if S(Get(r.Body, "user.email")) != lower || S(Get(r.Body, "user.role")) != "r_admin" || S(r.Body["accessToken"]) == "" ||
		S(r.Body["refreshToken"]) == "" || S(Get(r.Body, "store.id")) == "" {
		t.Fatalf("sign-up answer: %s", r)
	}
	sid := S(Get(r.Body, "store.id"))
	dropStoreDBAfter(t, sid)
	if ids, _ := Get(r.Body, "user.storeIds").([]interface{}); len(ids) != 1 || S(ids[0]) != sid {
		t.Errorf("owner storeIds: %v", Get(r.Body, "user.storeIds"))
	}
	if S(Get(r.Body, "store.nameEn")) != S(Get(b, "company.nameEn")) || S(Get(r.Body, "store.vatNo")) != "310122393500003" {
		t.Errorf("store fields: %v", r.Body["store"])
	}
	// the new owner token works straight away
	me := Must(t, Call(t, "GET", "/auth/me", S(r.Body["accessToken"]), nil), 200, "me")
	if S(Get(me.Body, "user.email")) != lower {
		t.Errorf("me email: %v", Get(me.Body, "user.email"))
	}

	// the same e-mail again: exact, other case, padded → 409 email_taken
	for _, em := range []string{lower, strings.ToUpper(lower), " " + strings.Title(lower) + " "} {
		d := SignupBody("")
		d["owner"].(M)["email"] = em
		dr := Call(t, "POST", "/auth/signup", "", d)
		if dr.Code == 201 {
			KnownBug(t, "#14", "sign-up accepts an e-mail that differs only in case: "+em, true)
			continue
		}
		if dr.Code != 409 || dr.ErrCode() != "email_taken" || dr.ErrField("owner.email") == "" {
			t.Errorf("duplicate %q: want 409 email_taken, got %s", em, dr)
		}
	}
	// a staff user's e-mail is taken too
	s := &Store{ID: sid, Token: S(r.Body["accessToken"])}
	_, staffEmail, _, _ := secStaff(t, s, "r_viewer")
	d := SignupBody("")
	d["owner"].(M)["email"] = strings.ToUpper(staffEmail)
	if dr := Call(t, "POST", "/auth/signup", "", d); dr.Code != 409 {
		t.Errorf("sign-up with a staff e-mail: %s", dr)
	}
}

// ---------- login, refresh, logout, me ----------

func TestAuth_LoginRefreshLogoutMe(t *testing.T) {
	t.Parallel()
	s := secStore(t)

	t.Run("login", func(t *testing.T) {
		for _, em := range []string{s.Email, strings.ToUpper(s.Email), "  " + s.Email + "\t"} {
			r := secLoginResp(t, em, s.Password)
			if r.Code != 200 {
				t.Errorf("login %q: %s", em, r)
				continue
			}
			secNoSecrets(t, "login", r)
			if S(r.Body["accessToken"]) == "" || S(r.Body["refreshToken"]) == "" || r.Body["expiresAt"] == nil ||
				S(Get(r.Body, "user.email")) != strings.ToLower(s.Email) || S(Get(r.Body, "user.role")) != "r_admin" {
				t.Errorf("login answer: %s", r)
			}
		}
		// a wrong password and an unknown user answer the same (no user enumeration)
		wp := secLoginResp(t, s.Email, s.Password+"x")
		uu := secLoginResp(t, "nobody"+Uniq()+"@e2e.example", s.Password)
		if wp.Code != 401 || wp.ErrCode() != "invalid_credentials" || uu.Code != 401 || uu.ErrCode() != "invalid_credentials" ||
			S(Get(wp.Body, "error.message")) != S(Get(uu.Body, "error.message")) {
			t.Errorf("wrong password %s / unknown user %s", wp, uu)
		}
		if r := secLoginResp(t, s.Email, strings.ToUpper(s.Password)); r.Code != 401 {
			t.Errorf("password must be case-sensitive: %s", r)
		}
		if r := Call(t, "POST", "/auth/login", "", M{}); r.Code != 400 || r.ErrField("email") == "" || r.ErrField("password") == "" {
			t.Errorf("empty login: %s", r)
		}
		if r := Call(t, "POST", "/auth/login", "", M{"email": "   ", "password": "x"}); r.Code != 400 || r.ErrField("email") == "" {
			t.Errorf("blank e-mail: %s", r)
		}
		if r := Call(t, "POST", "/auth/login", "", "{"); r.Code != 400 || r.ErrCode() != "malformed_json" {
			t.Errorf("malformed login: %s", r)
		}
		// a regex in the e-mail must not match other accounts
		if r := secLoginResp(t, ".*", s.Password); r.Code != 401 {
			t.Errorf("regex e-mail: %s", r)
		}
	})

	t.Run("me", func(t *testing.T) {
		r := Must(t, Call(t, "GET", "/auth/me", s.Token, nil), 200, "me")
		secNoSecrets(t, "me", r)
		if S(Get(r.Body, "user.email")) != strings.ToLower(s.Email) || S(Get(r.Body, "user.role")) != "r_admin" ||
			Get(r.Body, "user.platformAdmin") != false || S(Get(r.Body, "user.id")) == "" {
			t.Errorf("me user: %v", r.Body["user"])
		}
		for _, m := range []string{"sales", "purchases", "inventory", "customers", "vendors", "finance", "hr", "workshop", "reports", "settings"} {
			for _, v := range []string{"view", "create", "edit", "delete"} {
				if Get(r.Body, "user.perms."+m+"."+v) != true {
					t.Errorf("owner perms %s.%s: %v", m, v, Get(r.Body, "user.perms."+m))
				}
			}
		}
		st := Objs(r.Body["stores"])
		if len(st) != 1 || S(st[0]["id"]) != s.ID {
			t.Errorf("me stores: %v", r.Body["stores"])
		}
		// token without the Bearer word and with a lower-case scheme
		if r := Call(t, "GET", "/auth/me", "", nil, "Authorization", s.Token); r.Code != 200 {
			t.Errorf("raw token: %s", r)
		}
		if r := Call(t, "GET", "/auth/me", "", nil, "Authorization", "bearer "+s.Token); r.Code != 200 {
			t.Errorf("lower-case bearer: %s", r)
		}
		for name, h := range map[string]string{"none": "", "garbage": "Bearer abc.def.ghi", "basic": "Basic " + s.Token,
			"tampered": "Bearer " + s.Token[:len(s.Token)-3] + "abc"} {
			r := Call(t, "GET", "/auth/me", "", nil, "Authorization", h)
			if r.Code != 401 || r.ErrCode() != "unauthorized" {
				t.Errorf("me with %s token: %s", name, r)
			}
		}
	})

	t.Run("refresh", func(t *testing.T) {
		l := Must(t, secLoginResp(t, s.Email, s.Password), 200, "login")
		rt := S(l.Body["refreshToken"])
		n := Must(t, Call(t, "POST", "/auth/refresh", "", M{"refreshToken": rt}), 200, "refresh")
		secNoSecrets(t, "refresh", n)
		if S(n.Body["accessToken"]) == "" || S(n.Body["refreshToken"]) == "" || S(n.Body["refreshToken"]) == rt {
			t.Fatalf("refresh answer: %s", n)
		}
		Must(t, Call(t, "GET", "/auth/me", S(n.Body["accessToken"]), nil), 200, "me with refreshed token")
		// rotation: the used refresh token is revoked
		if r := Call(t, "POST", "/auth/refresh", "", M{"refreshToken": rt}); r.Code != 401 {
			t.Errorf("re-used refresh token: %s", r)
		}
		// the new one works once
		Must(t, Call(t, "POST", "/auth/refresh", "", M{"refreshToken": S(n.Body["refreshToken"])}), 200, "second refresh")
		for name, body := range map[string]interface{}{"empty": M{}, "garbage": M{"refreshToken": "x.y.z"},
			"access token": M{"refreshToken": S(l.Body["accessToken"])}, "number": M{"refreshToken": 42}} {
			if r := Call(t, "POST", "/auth/refresh", "", body); r.Code != 401 {
				t.Errorf("refresh with %s: %s", name, r)
			}
		}
		if r := Call(t, "POST", "/auth/refresh", "", "{"); r.Code != 400 {
			t.Errorf("malformed refresh: %s", r)
		}
	})

	t.Run("logout", func(t *testing.T) {
		l := Must(t, secLoginResp(t, s.Email, s.Password), 200, "login")
		at, rt := S(l.Body["accessToken"]), S(l.Body["refreshToken"])
		other := Login(t, s.Email, s.Password)
		if r := Call(t, "POST", "/auth/logout", "", nil); r.Code != 401 {
			t.Errorf("logout without token: %s", r)
		}
		Must(t, Call(t, "POST", "/auth/logout", at, M{"refreshToken": rt}), 204, "logout")
		if r := Call(t, "GET", "/auth/me", at, nil); r.Code != 401 {
			t.Errorf("access token after logout: %s", r)
		}
		if r := Call(t, "POST", "/auth/refresh", "", M{"refreshToken": rt}); r.Code != 401 {
			t.Errorf("refresh token after logout: %s", r)
		}
		if r := Call(t, "POST", "/auth/logout", at, nil); r.Code != 401 {
			t.Errorf("second logout: %s", r)
		}
		// other sessions of the same user stay signed in
		Must(t, Call(t, "GET", "/auth/me", other, nil), 200, "other session")
		// someone else's refresh token in the body is not revoked
		o := Must(t, secLoginResp(t, s.Email, s.Password), 200, "login")
		b := secStore(t)
		Must(t, Call(t, "POST", "/auth/logout", Login(t, b.Email, b.Password), M{"refreshToken": S(o.Body["refreshToken"])}), 204, "logout B")
		Must(t, Call(t, "POST", "/auth/refresh", "", M{"refreshToken": S(o.Body["refreshToken"])}), 200, "A's refresh after B's logout")
	})

	t.Run("inactive and deleted users", func(t *testing.T) {
		id, email, pw, tok := secStaff(t, s, "r_cashier")
		rt := S(Must(t, secLoginResp(t, email, pw), 200, "login").Body["refreshToken"])
		u := Patch(t, s.Token, "users", id, M{"status": "inactive"})
		if u["status"] != "inactive" {
			t.Fatalf("status: %v", u["status"])
		}
		if r := secLoginResp(t, email, pw); r.Code != 403 || r.ErrCode() != "inactive" {
			t.Errorf("inactive login: %s", r)
		}
		// a wrong password still answers 401 (inactive is told only to the owner of the password)
		if r := secLoginResp(t, email, pw+"x"); r.Code != 401 {
			t.Errorf("inactive wrong password: %s", r)
		}
		if r := Call(t, "GET", "/auth/me", tok, nil); r.Code != 401 {
			t.Errorf("inactive user's token: %s", r)
		}
		if r := Call(t, "POST", "/auth/refresh", "", M{"refreshToken": rt}); r.Code != 401 {
			t.Errorf("inactive refresh: %s", r)
		}
		Patch(t, s.Token, "users", id, M{"status": "active"})
		tok = Login(t, email, pw)
		Must(t, Call(t, "GET", "/auth/me", tok, nil), 200, "reactivated")
		Must(t, Call(t, "DELETE", "/users/"+id, s.Token, nil), 200, "delete user")
		if r := Call(t, "GET", "/auth/me", tok, nil); r.Code != 401 {
			t.Errorf("deleted user's token: %s", r)
		}
		if r := secLoginResp(t, email, pw); r.Code != 401 {
			t.Errorf("deleted user login: %s", r)
		}
	})
}

func TestAuth_ChangeOwnPassword(t *testing.T) {
	t.Parallel()
	s := secStore(t)
	_, email, pw, tok := secStaff(t, s, "r_cashier")
	call := func(body interface{}) Resp { return Call(t, "POST", "/auth/password", tok, body) }
	if r := Call(t, "POST", "/auth/password", "", M{"currentPassword": pw, "newPassword": "New@" + Digits(8)}); r.Code != 401 {
		t.Errorf("no token: %s", r)
	}
	cases := []struct {
		body  M
		field string
	}{
		{M{}, "currentPassword"},
		{M{"currentPassword": pw}, "newPassword"},
		{M{"currentPassword": pw, "newPassword": "Ab1!x"}, "newPassword"},
		{M{"currentPassword": pw, "newPassword": pw}, "newPassword"},
		{M{"currentPassword": pw + "x", "newPassword": "Other@12345"}, "currentPassword"},
		{M{"currentPassword": "", "newPassword": "Other@12345"}, "currentPassword"},
	}
	for _, c := range cases {
		if r := call(c.body); r.Code != 400 || r.ErrField(c.field) == "" {
			t.Errorf("%v: want 400 fields[%s], got %s", c.body, c.field, r)
		}
	}
	if r := call("{"); r.Code != 400 {
		t.Errorf("malformed: %s", r)
	}
	// the wrong current password changed nothing
	Login(t, email, pw)

	// a weak but 8+ character password: sign-up refuses it as "too weak"
	weak := "weakpassword"
	r := call(M{"currentPassword": pw, "newPassword": weak})
	if r.Code == 200 {
		KnownBug(t, "NEW-AUTH-WEAKPW", "POST /auth/password accepts a password sign-up rejects as too weak (lower-case letters only)", true)
		pw = weak
	} else if r.Code != 400 || r.ErrField("newPassword") == "" {
		t.Errorf("weak new password: %s", r)
	}

	np := "New@" + Digits(8)
	r = Must(t, call(M{"currentPassword": pw, "newPassword": np}), 200, "change password")
	secNoSecrets(t, "change password", r)
	if S(Get(r.Body, "user.email")) != strings.ToLower(email) {
		t.Errorf("answer user: %v", r.Body["user"])
	}
	if lr := secLoginResp(t, email, pw); lr.Code != 401 {
		t.Errorf("old password still works: %s", lr)
	}
	Login(t, email, np)
	// the owner's password is untouched
	Login(t, s.Email, s.Password)
}

// ---------- store isolation ----------

func TestSecurity_StoreIsolation(t *testing.T) {
	t.Parallel()
	a, b := Signup(t, ""), secStore(t)
	// A's records
	cust := a.Customer(t, "")
	prod := a.Product(t, 10, 20, 50)
	vend := a.Vendor(t)
	sale := Create(t, a.Token, "sales", M{"storeId": a.ID, "date": a.Now(), "customerId": cust["id"],
		"items": []M{a.Line(S(prod["id"]), 1, 20)}, "payments": []M{{"date": a.Now(), "amount": 23, "method": "cash"}}})
	emp := Create(t, a.Token, "employees", M{"storeId": a.ID, "nameEn": "Emp " + Uniq(), "joinDate": a.Today(), "basicSalary": 3000,
		"status": "active", "phone": "05" + Digits(8)})
	spec := Create(t, a.Token, "product-specs", M{"storeId": a.ID, "kind": "class", "name": "CL" + Digits(4)})
	notif := Create(t, a.Token, "notifications", M{"storeId": a.ID, "type": "stock", "tone": "warning", "titleEn": "Low", "at": a.Now(), "read": false})
	storeIDs := map[string]string{"customers": S(cust["id"]), "products": S(prod["id"]), "vendors": S(vend["id"]), "sales": S(sale["id"]),
		"employees": S(emp["id"]), "product-specs": S(spec["id"]), "notifications": S(notif["id"])}
	orgBodies := map[string]M{
		"categories": {"nameEn": "SecretCat " + Uniq(), "nameAr": "فئة"}, "brands": {"name": "SecretBrand " + Uniq()},
		"expense-categories": {"nameEn": "SecretECat " + Uniq()}, "vendor-categories": {"name": "SecretVCat " + Uniq()},
		"customer-categories": {"name": "SecretCCat " + Uniq()}, "roles": {"name": "SecretRole " + Uniq(), "perms": M{"sales": M{"view": true}}},
	}
	orgIDs := map[string]string{}
	orgNames := map[string]string{}
	for p, body := range orgBodies {
		rec := Create(t, a.Token, p, body)
		orgIDs[p] = S(rec["id"])
		orgNames[p] = S(body["nameEn"]) + S(body["name"])
	}
	aStaffID, _, _, aStaff := secStaff(t, a, "r_salesman")

	t.Run("store resources", func(t *testing.T) {
		versions := map[string]Number{}
		for p, id := range storeIDs {
			versions[p] = Num(Read(t, a.Token, p, id)["version"])
		}
		for _, res := range erp.Resources() {
			if res.Scope != "store" {
				continue
			}
			p := res.Path
			id := storeIDs[p]
			if id == "" {
				id = S(cust["id"]) // still A's record
			}
			secRefused(t, "B lists "+p+" of A", Call(t, "GET", "/"+p+"?storeId="+a.ID, b.Token, nil))
			secRefused(t, "B stats "+p+" of A", Call(t, "GET", "/"+p+"/stats?storeId="+a.ID, b.Token, nil))
			if r := Call(t, "POST", "/"+p, b.Token, M{"storeId": a.ID, "nameEn": "x", "name": "x"}); r.Code != 403 {
				t.Errorf("B creates %s in A: want 403, got %s", p, r)
			}
			for _, q := range []string{"", "?storeId=" + a.ID} {
				secRefused(t, "B reads "+p+"/"+id+q, Call(t, "GET", "/"+p+"/"+id+q, b.Token, nil))
				secRefused(t, "B patches "+p+q, Call(t, "PATCH", "/"+p+"/"+id+q, b.Token, M{"remarks": "hacked", "nameEn": "hacked"}))
				secRefused(t, "B puts "+p+q, Call(t, "PUT", "/"+p+"/"+id+q, b.Token, M{"nameEn": "hacked"}))
				secRefused(t, "B deletes "+p+q, Call(t, "DELETE", "/"+p+"/"+id+q, b.Token, nil))
				secRefused(t, "B hard-deletes "+p+q, Call(t, "DELETE", "/"+p+"/"+id+"?hard=1"+strings.Replace(q, "?", "&", 1), b.Token, nil))
				secRefused(t, "B restores "+p+q, Call(t, "POST", "/"+p+"/"+id+"/restore"+q, b.Token, nil))
			}
			// B's list of its own store never shows A's record
			if r := Call(t, "GET", "/"+p+"?storeId="+b.ID+"&ids="+id, b.Token, nil); r.Code == 200 && secHas(r.Data(), id) {
				t.Errorf("B's %s list shows A's record", p)
			}
		}
		// A's records are untouched
		for p, id := range storeIDs {
			rec := Read(t, a.Token, p, id)
			if Num(rec["version"]) != versions[p] || rec["deleted"] == true || strings.Contains(S(rec["remarks"])+S(rec["nameEn"]), "hacked") {
				t.Errorf("A's %s changed by B: %v", p, rec)
			}
		}
		// a record of B can't be moved into A
		bc := b.Customer(t, "")
		if r := PatchResp(t, b.Token, "customers", S(bc["id"]), M{"storeId": a.ID}); r.Code != 403 {
			t.Errorf("move B's customer to A: %s", r)
		}
		if r := PatchResp(t, a.Token, "customers", S(cust["id"]), M{"storeId": b.ID}); r.Code != 403 {
			t.Errorf("move A's customer to B: %s", r)
		}
	})

	t.Run("org resources", func(t *testing.T) {
		for p, id := range orgIDs {
			r := Must(t, Call(t, "GET", "/"+p+"?limit=5000", b.Token, nil), 200, "B lists "+p)
			leak := secHas(r.Data(), id) || strings.Contains(r.Raw, orgNames[p])
			g := Call(t, "GET", "/"+p+"/"+id, b.Token, nil)
			pa := Call(t, "PATCH", "/"+p+"/"+id, b.Token, M{"description": "seen by B"})
			switch p {
			case "customer-categories", "roles":
				// stored in one main-DB collection with no tenant filter
				KnownBug(t, "NEW-ISO-"+strings.ToUpper(p), p+" of company A are listed, read and changed by company B (main-DB native collection without a tenant filter, erp/native.go List/load)",
					leak || g.Code == 200 || pa.Code == 200)
			default:
				if leak {
					t.Errorf("B's %s list leaks A's record %s", p, orgNames[p])
				}
				secRefused(t, "B reads "+p, g)
				secRefused(t, "B patches "+p, pa)
				secRefused(t, "B deletes "+p, Call(t, "DELETE", "/"+p+"/"+id, b.Token, nil))
			}
		}
	})

	t.Run("users and stores", func(t *testing.T) {
		r := Must(t, Call(t, "GET", "/users", b.Token, nil), 200, "B lists users")
		secNoSecrets(t, "users list", r)
		if strings.Contains(r.Raw, strings.ToLower(a.Email)) || secHas(r.Data(), aStaffID) {
			t.Errorf("B's users list shows A's users")
		}
		for _, m := range []string{"GET", "PATCH", "PUT", "DELETE"} {
			secRefused(t, "B "+m+" A's user", Call(t, m, "/users/"+aStaffID, b.Token, M{"name": "hacked"}))
		}
		// a user of B can't be granted A's store
		if r := Call(t, "POST", "/users", b.Token, M{"name": "X", "email": "x" + Uniq() + "@e2e.example", "phone": "05" + Digits(8),
			"role": "r_viewer", "storeIds": []string{a.ID}}); r.Code != 403 {
			t.Errorf("grant A's store: %s", r)
		}
		st := Must(t, Call(t, "GET", "/stores", b.Token, nil), 200, "B lists stores").Data()
		if len(st) != 1 || S(st[0]["id"]) != b.ID {
			t.Errorf("B's stores: %v", st)
		}
		for _, m := range []string{"GET", "PATCH", "PUT"} {
			secRefused(t, "B "+m+" A's store", Call(t, m, "/stores/"+a.ID, b.Token, M{"nameEn": "hacked"}))
		}
		secRefused(t, "B ZATCA-connects A", Call(t, "POST", "/stores/"+a.ID+"/zatca/connect", b.Token, M{"otp": "123345"}))
		secRefused(t, "B ZATCA-disconnects A", Call(t, "POST", "/stores/"+a.ID+"/zatca/disconnect", b.Token, nil))
		secRefused(t, "B reports A's sale", Call(t, "POST", "/sales/"+S(sale["id"])+"/zatca/report", b.Token, nil))
		secRefused(t, "B reports A's sale (storeId)", Call(t, "POST", "/sales/"+S(sale["id"])+"/zatca/report?storeId="+a.ID, b.Token, nil))
		secRefused(t, "B starter catalog of A", Call(t, "GET", "/stores/"+a.ID+"/starter-catalog", b.Token, nil))
		secRefused(t, "B seeds A", Call(t, "POST", "/stores/"+a.ID+"/starter-catalog", b.Token, M{}))
		if Read(t, a.Token, "stores", a.ID)["nameEn"] == "hacked" {
			t.Errorf("B renamed A's store")
		}
		// A's salesman can't reach B either
		secRefused(t, "A staff lists B", Call(t, "GET", "/customers?storeId="+b.ID, aStaff, nil))
	})

	t.Run("dashboards, stats, history, drafts, pos numbers", func(t *testing.T) {
		for _, k := range []string{"total-expense", "vat", "revenue", "net-profit", "salary-balance", "bi", "feed"} {
			secRefused(t, "B dashboard "+k+" of A", Call(t, "GET", "/dashboard/"+k+"?storeId="+a.ID, b.Token, nil))
		}
		secRefused(t, "product history", Call(t, "GET", "/products/"+S(prod["id"])+"/history?kind=all&storeId="+a.ID, b.Token, nil))
		if r := Call(t, "GET", "/products/"+S(prod["id"])+"/history?kind=all&storeId="+b.ID, b.Token, nil); r.Code == 200 && len(r.Data()) > 0 {
			t.Errorf("B reads A's product history through its own store: %s", r)
		}
		secRefused(t, "facets", Call(t, "GET", "/products/facets?storeId="+a.ID, b.Token, nil))
		secRefused(t, "pos next number", Call(t, "POST", "/pos-records/next-number", b.Token, M{"storeId": a.ID, "terminal": "restaurant", "key": "order"}))
		d := Must(t, Call(t, "POST", "/drafts/sales", a.Token, M{"storeId": a.ID, "payload": M{"storeId": a.ID, "remarks": "secret"}}), 201, "A draft")
		did := S(d.Body["id"])
		secRefused(t, "B lists A's drafts", Call(t, "GET", "/drafts/sales?storeId="+a.ID, b.Token, nil))
		for _, q := range []string{"?storeId=" + a.ID, "?storeId=" + b.ID} {
			secRefused(t, "B reads draft"+q, Call(t, "GET", "/drafts/sales/"+did+q, b.Token, nil))
			secRefused(t, "B puts draft"+q, Call(t, "PUT", "/drafts/sales/"+did+q, b.Token, M{"payload": M{"remarks": "hacked"}}))
			secRefused(t, "B finalizes draft"+q, Call(t, "POST", "/drafts/sales/"+did+"/finalize"+q, b.Token, nil))
			secRefused(t, "B deletes draft"+q, Call(t, "DELETE", "/drafts/sales/"+did+q, b.Token, nil))
		}
		if r := Must(t, Call(t, "GET", "/drafts/sales/"+did+"?storeId="+a.ID, a.Token, nil), 200, "A reads draft"); S(Get(r.Body, "payload.remarks")) != "secret" {
			t.Errorf("A's draft changed: %s", r)
		}
		// an Idempotency-Key of A replayed by B never returns A's answer
		key := "sec-" + Uniq()
		Must(t, Call(t, "POST", "/customers", a.Token, M{"storeId": a.ID, "nameEn": "Idem A"}, "Idempotency-Key", key), 201, "A idem")
		r := Call(t, "POST", "/customers", b.Token, M{"storeId": b.ID, "nameEn": "Idem B"}, "Idempotency-Key", key)
		if r.Code != 201 || !strings.EqualFold(S(r.Body["nameEn"]), "Idem B") || S(r.Body["storeId"]) != b.ID {
			t.Errorf("B's request with A's idempotency key: %s", r)
		}
	})

	t.Run("legacy routes", func(t *testing.T) {
		// #3: any signed-in user can read and delete any user through /v1/user/{id}
		r := CallURL(t, "GET", baseURL+"/v1/user/"+aStaffID, b.Token, nil)
		KnownBug(t, "#3", "B's owner reads A's user through GET /v1/user/{id}", r.Code == 200 && strings.Contains(r.Raw, aStaffID))
		victim, _, _, _ := secStaff(t, a, "r_viewer")
		r = CallURL(t, "DELETE", baseURL+"/v1/user/"+victim, b.Token, nil)
		g := Call(t, "GET", "/users/"+victim, a.Token, nil)
		KnownBug(t, "#3", "B's owner deletes A's user through DELETE /v1/user/{id}", r.Code == 200 && (g.Code == 404 || g.Body["deleted"] == true))
		// #2: the legacy user list returns bcrypt hashes
		l := CallURL(t, "GET", baseURL+"/v1/user", a.Token, nil)
		KnownBug(t, "#2", "GET /v1/user returns password hashes", strings.Contains(l.Raw, "$2a$"))
		// #1: image uploads need no login
		for _, k := range []struct{ path, id string }{{"customer", S(cust["id"])}, {"vendor", S(vend["id"])}, {"product", S(prod["id"])}} {
			code, body := secMultipart(t, baseURL+"/v1/"+k.path+"/upload-image", "", map[string]string{"id": k.id, "storeID": a.ID})
			KnownBug(t, "#1", "POST /v1/"+k.path+"/upload-image without a token answers "+http.StatusText(code), code < 300)
			if code >= 500 {
				t.Errorf("upload %s: %d %s", k.path, code, body)
			}
		}
		code, _ := secMultipart(t, baseURL+"/v1/customer/upload-image", "", map[string]string{"id": primitive.NewObjectID().Hex(), "storeID": a.ID})
		KnownBug(t, "#1", "image upload for a customer that doesn't exist answers 200 (orphan file)", code == 200)
	})
}

// ---------- RBAC ----------

// secExpected is each system role's grants (view, create, edit, delete) per
// module, written from the role descriptions independently of erp/rbac.go.
var secExpected = map[string]map[string]string{
	"r_manager": {"sales": "vced", "purchases": "vced", "inventory": "vced", "customers": "vced", "vendors": "vced",
		"finance": "vced", "hr": "vced", "workshop": "vced", "reports": "vced", "settings": "v"},
	"r_salesman": {"sales": "vce", "customers": "vce", "inventory": "v"},
	"r_cashier":  {"sales": "vc", "customers": "v", "inventory": "v"},
	"r_accountant": {"sales": "v", "purchases": "v", "inventory": "v", "customers": "v", "vendors": "v",
		"finance": "vced", "hr": "vced", "workshop": "v", "reports": "vced", "settings": "v"},
	"r_viewer": {"sales": "v", "purchases": "v", "inventory": "v", "customers": "v", "vendors": "v",
		"finance": "v", "hr": "v", "workshop": "v", "reports": "v", "settings": "v"},
}

// secModule is a resource of a module with a valid create body and a patch.
type secModule struct {
	module, path string
	body         func() M
	patch        func() M
}

func secModules(s *Store, custID, prodID string) []secModule {
	return []secModule{
		{"sales", "quotations", func() M {
			return M{"storeId": s.ID, "date": s.Now(), "customerId": custID, "type": "quotation", "status": "created", "validityDays": 7,
				"deliveryDays": 3, "items": []M{s.Line(prodID, 1, 50)}}
		}, func() M { return M{"remarks": "r " + Uniq()} }},
		{"purchases", "rfq-suppliers", func() M {
			return M{"storeId": s.ID, "name": "Sup " + Uniq(), "phone": "05" + Digits(8), "categories": []string{"filters"}}
		}, func() M { return M{"rating": 4} }},
		{"inventory", "products", func() M {
			// an explicit code: concurrent creates without one can share a code (TestRobustness_ConcurrentCreateCodes)
			return M{"storeId": s.ID, "nameEn": "Prod " + Uniq(), "code": "RB" + Digits(8), "pricing": M{"purchase": 5, "retail": 9}}
		}, func() M { return M{"note": "n " + Uniq()} }},
		{"customers", "customers", func() M { return M{"storeId": s.ID, "nameEn": "Cust " + Uniq()} }, func() M { return M{"remarks": "r"} }},
		{"vendors", "vendors", func() M { return M{"storeId": s.ID, "nameEn": "Vend " + Uniq()} }, func() M { return M{"remarks": "r"} }},
		{"finance", "expense-categories", func() M { return M{"nameEn": "ECat " + Uniq()} }, func() M { return M{"nameEn": "ECat " + Uniq()} }},
		{"hr", "employees", func() M {
			return M{"storeId": s.ID, "nameEn": "Emp " + Uniq(), "joinDate": s.Today(), "basicSalary": 3000, "status": "active", "phone": "05" + Digits(8)}
		}, func() M { return M{"jobTitle": "Mechanic"} }},
		{"workshop", "vehicles", func() M {
			return M{"storeId": s.ID, "plate": "ABC " + Digits(4), "make": "Toyota", "model": "Camry", "customerId": custID, "year": 2021}
		}, func() M { return M{"color": "White"} }},
		{"settings", "roles", func() M { return M{"name": "Role " + Uniq() + Digits(3), "perms": M{"sales": M{"view": true}}} },
			func() M { return M{"description": "d " + Uniq()} }},
	}
}

func TestRBAC_SystemRoleMatrix(t *testing.T) {
	t.Parallel()
	s := secStore(t)
	cust := s.Customer(t, "")
	prod := s.Product(t, 10, 50, 100)
	mods := secModules(s, S(cust["id"]), S(prod["id"]))
	for role, grants := range secExpected {
		role, grants := role, grants
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			_, tok := s.User(t, role)
			me := Must(t, Call(t, "GET", "/auth/me", tok, nil), 200, "me")
			if S(Get(me.Body, "user.role")) != role {
				t.Errorf("role: %v", Get(me.Body, "user.role"))
			}
			for _, m := range mods {
				g := grants[m.module]
				q := ""
				if m.path != "roles" && m.path != "expense-categories" {
					q = "?storeId=" + s.ID
				}
				// the owner's target records (one to edit, one to delete)
				editID := S(Create(t, s.Token, m.path, m.body())["id"])
				delID := S(Create(t, s.Token, m.path, m.body())["id"])
				check := func(verb, letter string, r Resp, okCodes ...int) {
					t.Helper()
					allowed := strings.Contains(g, letter)
					if !allowed {
						if r.Code != 403 || r.ErrCode() != "forbidden" {
							t.Errorf("%s %s %s: want 403, got %s", role, verb, m.path, r)
						}
						return
					}
					for _, c := range okCodes {
						if r.Code == c {
							return
						}
					}
					t.Errorf("%s %s %s: want %v, got %s", role, verb, m.path, okCodes, r)
				}
				check("view", "v", Call(t, "GET", "/"+m.path+q, tok, nil), 200)
				check("read", "v", Call(t, "GET", "/"+m.path+"/"+editID, tok, nil), 200)
				check("create", "c", Call(t, "POST", "/"+m.path, tok, m.body()), 201)
				check("edit", "e", Call(t, "PATCH", "/"+m.path+"/"+editID, tok, m.patch()), 200)
				// some legacy records have no delete (409 unsupported_legacy)
				check("delete", "d", Call(t, "DELETE", "/"+m.path+"/"+delID, tok, nil), 200, 409)
				check("restore", "e", Call(t, "POST", "/"+m.path+"/"+delID+"/restore", tok, nil), 200, 409)
			}
			// reports: the dashboards
			check := strings.Contains(grants["reports"], "v")
			r := Call(t, "GET", "/dashboard/revenue?storeId="+s.ID, tok, nil)
			if check && r.Code != 200 || !check && r.Code != 403 {
				t.Errorf("%s dashboard: %s", role, r)
			}
			// users and stores are the admin's
			if r := Call(t, "POST", "/users", tok, M{"name": "X", "email": "x" + Uniq() + Digits(3) + "@e2e.example", "phone": "05" + Digits(8),
				"role": "r_viewer", "storeIds": []string{s.ID}, "password": "Staff@123456"}); r.Code != 403 {
				t.Errorf("%s creates a user: %s", role, r)
			}
			if r := PatchResp(t, s.Token, "stores", s.ID, M{"branchEn": "x"}); r.Code != 200 {
				t.Errorf("owner patches store: %s", r)
			}
			if r := Call(t, "PATCH", "/stores/"+s.ID, tok, M{"branchEn": "by " + role}); r.Code != 403 {
				t.Errorf("%s patches the store: %s", role, r)
			}
			ul := Call(t, "GET", "/users", tok, nil)
			if strings.Contains(grants["settings"], "v") != (ul.Code == 200) {
				t.Errorf("%s lists users: %s", role, ul)
			}
			if ul.Code == 200 {
				secNoSecrets(t, role+" users list", ul)
			}
		})
	}
}

func TestRBAC_CashierDiscountLimit(t *testing.T) {
	t.Parallel()
	s := secStore(t)
	_, tok := s.User(t, "r_cashier")
	p := s.Product(t, 10, 100, 20)
	c := s.Customer(t, "")
	// the Cashier role's maxDiscount is 5 %: a 50 % line discount must be refused
	ln := s.Line(S(p["id"]), 1, 100)
	ln["unitDiscount"] = 50
	r := Call(t, "POST", "/sales", tok, M{"storeId": s.ID, "date": s.Now(), "customerId": c["id"], "items": []M{ln},
		"payments": []M{{"date": s.Now(), "amount": 57.5, "method": "cash"}}})
	if r.Code >= 500 {
		t.Fatalf("sale: %s", r)
	}
	KnownBug(t, "NEW-RBAC-MAXDISC", "a Cashier (role maxDiscount 5%) saves a sale with a 50% discount: role maxDiscount is never enforced", r.Code == 201)
	// a sale without a discount is fine
	Must(t, Call(t, "POST", "/sales", tok, M{"storeId": s.ID, "date": s.Now(), "customerId": c["id"], "items": []M{s.Line(S(p["id"]), 1, 100)},
		"payments": []M{{"date": s.Now(), "amount": 115, "method": "cash"}}}), 201, "cashier sale")
}

func TestRBAC_CustomRole(t *testing.T) {
	t.Parallel()
	s := secStore(t)
	name := "Counter " + Uniq() + Digits(3)

	// validation
	for _, c := range []struct {
		body  M
		field string
	}{
		{M{"perms": M{}}, "name"},
		{M{"name": "  "}, "name"},
		{M{"name": "admin"}, "name"},
		{M{"name": "Cashier"}, "name"},
		{M{"name": name + "x", "maxDiscount": 101}, "maxDiscount"},
		{M{"name": name + "y", "maxDiscount": -1}, "maxDiscount"},
	} {
		if r := Call(t, "POST", "/roles", s.Token, c.body); r.Code != 400 || r.ErrField(c.field) == "" {
			t.Errorf("role %v: want 400 fields[%s], got %s", c.body, c.field, r)
		}
	}
	if r := Call(t, "POST", "/roles", s.Token, M{"id": "r_admin", "name": name + "z"}); r.Code != 409 || r.ErrCode() != "id_conflict" {
		t.Errorf("role with a system id: %s", r)
	}

	role := Create(t, s.Token, "roles", M{"name": name, "description": "front desk", "maxDiscount": 15,
		"perms": M{"customers": M{"view": true, "create": true}, "sales": M{"view": true}, "bogus": M{"view": true}}})
	rid := S(role["id"])
	if role["system"] != false || Get(role, "perms.customers.create") != true || Get(role, "perms.customers.edit") != false ||
		Get(role, "perms.vendors.view") != false || Get(role, "bogus") != nil || Get(role, "perms.bogus") != nil || Num(role["maxDiscount"]) != 15 {
		t.Errorf("created role: %v", role)
	}
	// the same name in another case
	if r := Call(t, "POST", "/roles", s.Token, M{"name": strings.ToUpper(name)}); r.Code != 400 || r.ErrField("name") == "" {
		t.Errorf("duplicate role name: %s", r)
	}
	l := List(t, s.Token, "roles", "")
	sys := 0
	for _, r := range l {
		if r["system"] == true {
			sys++
		}
	}
	if sys != 6 || !secHas(l, rid) {
		t.Errorf("roles list: %d system, custom present %v", sys, secHas(l, rid))
	}
	if r := Call(t, "GET", "/roles?limit=2&page=2", s.Token, nil); r.Code != 200 || len(r.Data()) != 2 || Num(r.Body["total"]) < 7 {
		t.Errorf("roles page 2: %s", r)
	}

	uid, email, pw, tok := secStaff(t, s, rid)
	if S(Read(t, s.Token, "users", uid)["role"]) != rid {
		t.Errorf("user role")
	}
	me := Must(t, Call(t, "GET", "/auth/me", tok, nil), 200, "me")
	if Get(me.Body, "user.perms.customers.create") != true || Get(me.Body, "user.perms.customers.edit") != false {
		t.Errorf("me perms: %v", Get(me.Body, "user.perms"))
	}
	cust := Must(t, Call(t, "POST", "/customers", tok, M{"storeId": s.ID, "nameEn": "By role " + Uniq()}), 201, "role creates customer").Body
	if r := Call(t, "PATCH", "/customers/"+S(cust["id"]), tok, M{"remarks": "x"}); r.Code != 403 {
		t.Errorf("edit without the grant: %s", r)
	}
	if r := Call(t, "DELETE", "/customers/"+S(cust["id"]), tok, nil); r.Code != 403 {
		t.Errorf("delete without the grant: %s", r)
	}
	for _, p := range []string{"vendors", "products", "expenses"} {
		if r := Call(t, "GET", "/"+p+"?storeId="+s.ID, tok, nil); r.Code != 403 {
			t.Errorf("view %s without the grant: %s", p, r)
		}
	}
	if r := Call(t, "GET", "/sales?storeId="+s.ID, tok, nil); r.Code != 200 {
		t.Errorf("view sales: %s", r)
	}
	if r := Call(t, "POST", "/roles", tok, M{"name": "Mine " + Uniq()}); r.Code != 403 {
		t.Errorf("role user creates a role: %s", r)
	}

	// grant edit: the next request has it (perms are read per request)
	up := Patch(t, s.Token, "roles", rid, M{"perms": M{"customers": M{"view": true, "create": true, "edit": true}, "sales": M{"view": true}}})
	if Num(up["version"]) != 2 {
		t.Errorf("role version: %v", up["version"])
	}
	Must(t, Call(t, "PATCH", "/customers/"+S(cust["id"]), tok, M{"remarks": "now allowed"}), 200, "edit with the grant")
	if r := Call(t, "PATCH", "/roles/"+rid, s.Token, M{"name": "Viewer"}, "If-Match", "1"); r.Code != 409 {
		t.Errorf("stale If-Match: %s", r)
	}
	if r := PatchResp(t, s.Token, "roles", rid, M{"name": "viewer"}); r.Code != 400 || r.ErrField("name") == "" {
		t.Errorf("rename to a system name: %s", r)
	}
	// PUT: full replace
	cur := Read(t, s.Token, "roles", rid)
	cur["description"] = "replaced"
	pr := Must(t, Call(t, "PUT", "/roles/"+rid, s.Token, cur), 200, "put role").Body
	if pr["description"] != "replaced" || Get(pr, "perms.customers.edit") != true {
		t.Errorf("put role: %v", pr)
	}

	// a role user may not hand out the Admin role even with settings rights
	adminish := Create(t, s.Token, "roles", M{"name": "Usermgr " + Uniq() + Digits(3),
		"perms": M{"settings": M{"view": true, "create": true, "edit": true}}})
	_, _, _, mtok := secStaff(t, s, S(adminish["id"]))
	r := Call(t, "POST", "/users", mtok, M{"name": "Esc", "email": "esc" + Uniq() + Digits(3) + "@e2e.example", "phone": "05" + Digits(8),
		"role": "r_admin", "storeIds": []string{s.ID}, "password": "Staff@123456"})
	if r.Code != 403 {
		t.Errorf("non-admin assigns r_admin: %s", r)
	}
	if r := Call(t, "PATCH", "/users/"+uid, mtok, M{"role": "r_admin"}); r.Code != 403 {
		t.Errorf("non-admin promotes to r_admin: %s", r)
	}
	// ... nor change the ZATCA environment (admins only)
	envR := Call(t, "PATCH", "/stores/"+s.ID, mtok, M{"zatca": M{"env": "Simulation"}})
	if S(Get(Read(t, s.Token, "stores", s.ID), "zatca.env")) == "Simulation" && envR.Code == 200 {
		t.Errorf("non-admin changed the ZATCA environment: %s", envR)
	}

	// system roles are read-only
	Must(t, Call(t, "GET", "/roles/r_cashier", s.Token, nil), 200, "system role")
	for _, m := range []string{"PATCH", "PUT", "DELETE"} {
		if r := Call(t, m, "/roles/r_manager", s.Token, M{"description": "x"}); r.Code != 403 {
			t.Errorf("%s system role: %s", m, r)
		}
	}
	if r := Call(t, "POST", "/roles/r_admin/restore", s.Token, nil); r.Code != 200 && r.Code != 403 {
		t.Errorf("restore system role: %s", r)
	}
	if r := Call(t, "DELETE", "/roles/r_admin?hard=1", s.Token, nil); r.Code != 403 {
		t.Errorf("hard-delete system role: %s", r)
	}
	if r := Call(t, "GET", "/roles/rol_nope"+Uniq(), s.Token, nil); r.Code != 404 {
		t.Errorf("unknown role: %s", r)
	}

	// delete the custom role: its users must not gain rights
	d := Must(t, Call(t, "DELETE", "/roles/"+rid, s.Token, nil), 200, "delete role").Body
	if d["deleted"] != true {
		t.Errorf("deleted role: %v", d)
	}
	if secHas(List(t, s.Token, "roles", ""), rid) || !secHas(List(t, s.Token, "roles", "includeDeleted=1"), rid) {
		t.Errorf("deleted role listing")
	}
	tok = Login(t, email, pw)
	sr := Call(t, "POST", "/sales", tok, M{"storeId": s.ID})
	KnownBug(t, "NEW-RBAC-DELROLE", "users of a deleted custom role fall back to Salesman rights (they gain sales create; erp/ctx.go effectivePerms)",
		sr.Code != 403)
	// a user can't be given a deleted role
	if r := Call(t, "POST", "/users", s.Token, M{"name": "X", "email": "x" + Uniq() + Digits(3) + "@e2e.example", "phone": "05" + Digits(8),
		"role": rid, "storeIds": []string{s.ID}}); r.Code != 400 || r.ErrField("role") == "" {
		t.Errorf("user with a deleted role: %s", r)
	}
	rs := Must(t, Call(t, "POST", "/roles/"+rid+"/restore", s.Token, nil), 200, "restore role").Body
	if rs["deleted"] != false {
		t.Errorf("restored role: %v", rs)
	}
	Must(t, Call(t, "PATCH", "/customers/"+S(cust["id"]), tok, M{"remarks": "again"}), 200, "restored role's grant")
	if r := Call(t, "POST", "/sales", tok, M{"storeId": s.ID}); r.Code != 403 {
		t.Errorf("restored role: sales create must be refused: %s", r)
	}
	// hard delete hides the role for good
	Must(t, Call(t, "DELETE", "/roles/"+S(adminish["id"])+"?hard=1", s.Token, nil), 204, "hard delete role")
	if r := Call(t, "GET", "/roles/"+S(adminish["id"]), s.Token, nil); r.Code != 404 {
		t.Errorf("hard-deleted role: %s", r)
	}
}

func TestRBAC_UsersCRUD(t *testing.T) {
	t.Parallel()
	s := secStore(t)
	email := "User" + Uniq() + Digits(3) + "@E2E.example"
	pw := "Staff@" + Digits(6)
	valid := func() M {
		return M{"name": "Clerk", "email": email, "phone": "05" + Digits(8), "role": "r_cashier", "storeIds": []string{s.ID}, "password": pw}
	}
	other := secStore(t)
	for _, c := range []struct {
		key   string
		value interface{}
		field string
		code  int
	}{
		{"name", "", "name", 400},
		{"email", "", "email", 400},
		{"email", "not-mail", "email", 400},
		{"email", strings.ToUpper(s.Email), "email", 400},
		{"phone", "", "phone", 400},
		{"phone", "123", "phone", 400},
		{"role", "", "role", 400},
		{"role", "r_superuser", "role", 400},
		{"storeIds", []string{}, "storeIds", 400},
		{"status", "sleeping", "status", 400},
		{"password", "short", "password", 400},
		{"storeIds", []string{other.ID}, "", 403},
	} {
		b := valid()
		b[c.key] = c.value
		r := Call(t, "POST", "/users", s.Token, b)
		if r.Code != c.code || (c.field != "" && r.ErrField(c.field) == "") {
			t.Errorf("user %s=%v: want %d fields[%s], got %s", c.key, c.value, c.code, c.field, r)
		}
	}
	if r := Call(t, "POST", "/users", s.Token, "{"); r.Code != 400 {
		t.Errorf("malformed: %s", r)
	}

	cr := Must(t, Call(t, "POST", "/users", s.Token, valid()), 201, "create user")
	secNoSecrets(t, "create user", cr)
	u := cr.Body
	id := S(u["id"])
	if u["email"] != strings.ToLower(email) || u["role"] != "r_cashier" || u["status"] != "active" || Num(u["version"]) != 1 {
		t.Errorf("created user: %v", u)
	}
	tok := Login(t, email, pw)
	g := Must(t, Call(t, "GET", "/users/"+id, s.Token, nil), 200, "get user")
	secNoSecrets(t, "get user", g)
	l := Must(t, Call(t, "GET", "/users?limit=1000", s.Token, nil), 200, "list users")
	secNoSecrets(t, "list users", l)
	if !secHas(l.Data(), id) || len(l.Data()) != 2 || Num(l.Body["total"]) != 2 {
		t.Errorf("users list (owner + new user): %s", l)
	}
	if r := Call(t, "GET", "/users?q=x", s.Token, nil); r.Code != 400 || r.ErrField("q") == "" {
		t.Errorf("users search (not supported): %s", r)
	}
	if r := Call(t, "POST", "/users", s.Token, valid()); r.Code != 400 || r.ErrField("email") == "" {
		t.Errorf("duplicate user e-mail: %s", r)
	}

	// role change takes effect on the next request
	p := PatchResp(t, s.Token, "users", id, M{"role": "r_manager"})
	secNoSecrets(t, "patch user", p)
	if p.Code != 200 || p.Body["role"] != "r_manager" {
		t.Fatalf("role change: %s", p)
	}
	me := Must(t, Call(t, "GET", "/auth/me", tok, nil), 200, "me")
	if Get(me.Body, "user.perms.vendors.delete") != true || Get(me.Body, "user.perms.settings.edit") != false {
		t.Errorf("manager perms: %v", Get(me.Body, "user.perms"))
	}
	// stale version
	if r := Call(t, "PATCH", "/users/"+id, s.Token, M{"name": "x"}, "If-Match", "1"); r.Code != 409 || r.ErrCode() != "version_conflict" {
		t.Errorf("stale If-Match: %s", r)
	}
	if r := PatchResp(t, s.Token, "users", id, M{"email": strings.ToUpper(s.Email)}); r.Code != 400 || r.ErrField("email") == "" {
		t.Errorf("take the owner's e-mail: %s", r)
	}
	if r := PatchResp(t, s.Token, "users", id, M{"storeIds": []string{other.ID}}); r.Code != 403 {
		t.Errorf("grant another company's store: %s", r)
	}
	// the owner can't change or delete their own role/account
	me2 := Must(t, Call(t, "GET", "/auth/me", s.Token, nil), 200, "owner me")
	ownerID := S(Get(me2.Body, "user.id"))
	if r := PatchResp(t, s.Token, "users", ownerID, M{"role": "r_viewer"}); r.Code != 403 {
		t.Errorf("own role change: %s", r)
	}
	if r := Call(t, "DELETE", "/users/"+ownerID, s.Token, nil); r.Code != 403 {
		t.Errorf("own delete: %s", r)
	}
	// a manager can't manage users (settings: view only)
	if r := Call(t, "PATCH", "/users/"+ownerID, tok, M{"role": "r_viewer"}); r.Code != 403 {
		t.Errorf("manager edits the owner: %s", r)
	}
	if r := Call(t, "DELETE", "/users/"+ownerID, tok, nil); r.Code != 403 {
		t.Errorf("manager deletes the owner: %s", r)
	}

	// password change by the admin
	np := "Reset@" + Digits(6)
	pp := Must(t, PatchResp(t, s.Token, "users", id, M{"password": np}), 200, "password reset")
	secNoSecrets(t, "password patch", pp)
	// the record's history must not keep the new password in clear text
	KnownBug(t, "NEW-SEC-PWHIST", "PATCH /users/{id} {password} writes the plain-text password into the user's history "+
		"(history[].changes {field: password, to: <password>}), returned by every /users read (erp/envelope.go diff)",
		strings.Contains(pp.Raw, np) || strings.Contains(Must(t, Call(t, "GET", "/users?limit=1000", s.Token, nil), 200, "users").Raw, np))
	if r := secLoginResp(t, email, pw); r.Code != 401 {
		t.Errorf("old password after reset: %s", r)
	}
	tok = Login(t, email, np)
	if r := PatchResp(t, s.Token, "users", id, M{"password": "short"}); r.Code != 400 || r.ErrField("password") == "" {
		t.Errorf("short password: %s", r)
	}

	// status
	if r := PatchResp(t, s.Token, "users", id, M{"status": "paused"}); r.Code != 400 || r.ErrField("status") == "" {
		t.Errorf("bad status: %s", r)
	}
	Patch(t, s.Token, "users", id, M{"status": "inactive"})
	if r := secLoginResp(t, email, np); r.Code != 403 {
		t.Errorf("inactive login: %s", r)
	}
	if r := Call(t, "GET", "/auth/me", tok, nil); r.Code != 401 {
		t.Errorf("inactive token: %s", r)
	}
	Patch(t, s.Token, "users", id, M{"status": "active"})

	// PUT: full replace from the current record
	cur := Read(t, s.Token, "users", id)
	cur["name"] = "Replaced Name"
	cur["role"] = "r_viewer"
	pr := Call(t, "PUT", "/users/"+id, s.Token, cur)
	secNoSecrets(t, "put user", pr)
	if pr.Code != 200 || pr.Body["name"] != "Replaced Name" || pr.Body["role"] != "r_viewer" {
		t.Errorf("put user: %s", pr)
	}
	Login(t, email, np) // PUT without a password keeps it

	// a user created without a password gets a random one and must change it
	nb := valid()
	nb["email"] = "nopw" + Uniq() + Digits(3) + "@e2e.example"
	delete(nb, "password")
	Must(t, Call(t, "POST", "/users", s.Token, nb), 201, "user without password")

	// delete / restore
	d := Must(t, Call(t, "DELETE", "/users/"+id, s.Token, nil), 200, "delete user")
	secNoSecrets(t, "delete user", d)
	if d.Body["deleted"] != true {
		t.Errorf("deleted user: %s", d)
	}
	if r := secLoginResp(t, email, np); r.Code != 401 {
		t.Errorf("deleted user login: %s", r)
	}
	if secHas(List(t, s.Token, "users", ""), id) {
		t.Errorf("deleted user still listed")
	}
	if r := Call(t, "POST", "/users/"+id+"/restore", s.Token, nil); r.Code != 409 && r.Code != 404 {
		t.Errorf("restore user (unsupported by the legacy system): %s", r)
	} else if r.Code == 409 && r.ErrCode() != "unsupported_legacy" {
		t.Errorf("restore user code: %s", r)
	}
	if r := Call(t, "DELETE", "/users/"+primitive.NewObjectID().Hex(), s.Token, nil); r.Code != 404 {
		t.Errorf("delete unknown user: %s", r)
	}
	if r := Call(t, "GET", "/users/nope", s.Token, nil); r.Code != 404 {
		t.Errorf("get bad id: %s", r)
	}
	// the legacy user check still counts the deleted user's e-mail as taken
	if r := Call(t, "POST", "/users", s.Token, valid()); r.Code != 400 || r.ErrField("email") == "" {
		t.Errorf("re-create user with a deleted user's e-mail: %s", r)
	}

	// legacy create returns the hash (#2)
	lr := CallURL(t, "POST", baseURL+"/v1/user", s.Token, M{"name": "Legacy", "email": "legacy" + Uniq() + Digits(3) + "@e2e.example",
		"mob": "05" + Digits(8), "password": "Staff@123456", "role": "SalesMan", "store_ids": []string{s.ID}})
	KnownBug(t, "#2", "POST /v1/user returns the new user's bcrypt hash", strings.Contains(lr.Raw, "$2a$"))
}

// ---------- stores ----------

func TestSecurity_StoresSettings(t *testing.T) {
	t.Parallel()
	s := secStore(t)

	r := Must(t, Call(t, "GET", "/stores", s.Token, nil), 200, "list stores")
	if len(r.Data()) != 1 || S(r.Data()[0]["id"]) != s.ID || Num(r.Body["total"]) != 1 {
		t.Fatalf("stores: %s", r)
	}
	st := Read(t, s.Token, "stores", s.ID)
	if st["countryCode"] != "SA" || Num(st["vatPercent"]) != 15 || Get(st, "zatca.phase") != 2.0 || Get(st, "zatca.connected") != false ||
		Get(st, "zatca.envLocked") != false || Get(st, "country.locked") != false {
		t.Errorf("new store: %v %v", st["zatca"], st["country"])
	}
	for _, secret := range []string{"private_key", "binary_security_token", "production_secret", "smtp_password"} {
		if strings.Contains(Must(t, Call(t, "GET", "/stores/"+s.ID, s.Token, nil), 200, "store").Raw, secret) {
			t.Errorf("store answer has %s", secret)
		}
	}
	if r := Call(t, "GET", "/stores/"+primitive.NewObjectID().Hex(), s.Token, nil); r.Code != 404 {
		t.Errorf("unknown store: %s", r)
	}

	t.Run("validation", func(t *testing.T) {
		for _, c := range []struct {
			body  M
			field string
		}{
			{M{"nameEn": ""}, "nameEn"},
			{M{"category": "spaceship"}, "category"},
			{M{"countryCode": "ZZ"}, "countryCode"},
			{M{"vatPercent": 101}, "vatPercent"},
			{M{"vatPercent": -1}, "vatPercent"},
			{M{"email": "bad"}, "email"},
			{M{"vatNo": "123"}, "vatNo"},
			{M{"crNo": "12"}, "crNo"},
			{M{"short": "abc"}, "short"},
			{M{"zatca": M{"env": "Moon"}}, "zatca.env"},
			{M{"serials": M{"sales": M{"prefix": "IN V", "start": 1}}}, "serials.sales.prefix"},
			{M{"serials": M{"sales": M{"prefix": "INV", "start": 0}}}, "serials.sales.start"},
			{M{"serials": M{"sales": M{"prefix": "INV", "start": 1.5}}}, "serials.sales.start"},
			{M{"titles": M{"invoiceEn": strings.Repeat("T", 101)}}, "titles.invoiceEn"},
			{M{"titles": M{"invoiceAr": 5}}, "titles.invoiceAr"},
		} {
			if r := PatchResp(t, s.Token, "stores", s.ID, c.body); r.Code != 400 || r.ErrField(c.field) == "" {
				t.Errorf("store %v: want 400 fields[%s], got %s", c.body, c.field, r)
			}
		}
		if r := Call(t, "PATCH", "/stores/"+s.ID, s.Token, M{"branchEn": "x"}, "If-Match", "999"); r.Code != 409 || r.ErrCode() != "version_conflict" {
			t.Errorf("stale If-Match: %s", r)
		}
		if Num(Read(t, s.Token, "stores", s.ID)["version"]) != Num(st["version"]) {
			t.Errorf("a refused save changed the version")
		}
	})

	t.Run("settings, serials, flags", func(t *testing.T) {
		before := Read(t, s.Token, "stores", s.ID)
		u := Patch(t, s.Token, "stores", s.ID, M{"branchEn": "فرع العليا", "flags": M{"enable_rbac_module": true},
			"serials": M{"sales": M{"prefix": "INV-E2E", "start": 7}, "quotation": M{"prefix": "QT", "start": 1}},
			"titles":  M{"invoiceEn": "Tax Invoice", "invoiceAr": "فاتورة ضريبية"}, "plan": "enterprise", "trialEndsAt": "2099-01-01"})
		if u["branchEn"] != "فرع العليا" || Get(u, "flags.enable_rbac_module") != true || Get(u, "serials.sales.prefix") != "INV-E2E-" ||
			Get(u, "serials.sales.start") != 7.0 || Get(u, "titles.invoiceAr") != "فاتورة ضريبية" || Num(u["version"]) != Num(before["version"])+1 {
			t.Errorf("patched store: %v %v %v", u["flags"], Get(u, "serials.sales"), u["titles"])
		}
		// billing fields are server-owned
		if u["plan"] != before["plan"] || u["trialEndsAt"] != before["trialEndsAt"] {
			t.Errorf("billing fields changed by PATCH: %v %v", u["plan"], u["trialEndsAt"])
		}
		// the next sale uses the new serial
		p := s.Product(t, 5, 10, 10)
		c := s.Customer(t, "")
		sale := Create(t, s.Token, "sales", M{"storeId": s.ID, "date": s.Now(), "customerId": c["id"], "items": []M{s.Line(S(p["id"]), 1, 10)},
			"payments": []M{{"date": s.Now(), "amount": 11.5, "method": "cash"}}})
		if !strings.Contains(S(sale["code"]), "INV-E2E") {
			t.Errorf("sale code after serial change: %v", sale["code"])
		}
		// PUT: the full record back (Undo)
		cur := Read(t, s.Token, "stores", s.ID)
		cur["branchEn"] = "Main Branch"
		pr := Must(t, Call(t, "PUT", "/stores/"+s.ID, s.Token, cur), 200, "put store").Body
		if pr["branchEn"] != "Main Branch" || Get(pr, "serials.sales.prefix") != "INV-E2E-" {
			t.Errorf("put store: %v", pr["branchEn"])
		}
		if r := Call(t, "PUT", "/stores/"+s.ID, s.Token, M{"nameEn": ""}); r.Code != 400 {
			t.Errorf("put an empty store: %s", r)
		}
	})

	t.Run("zatca environment", func(t *testing.T) {
		// first value: always allowed; changing it: admin and no documents yet
		s2 := secStore(t)
		e := Patch(t, s2.Token, "stores", s2.ID, M{"zatca": M{"env": "nonproduction", "connected": true, "pcsid": "fake"}})
		if Get(e, "zatca.env") != "NonProduction" || Get(e, "zatca.connected") != false || Get(e, "zatca.pcsid") != "" {
			t.Errorf("env set: %v", e["zatca"])
		}
		e = Patch(t, s2.Token, "stores", s2.ID, M{"zatca": M{"env": "Simulation"}})
		if Get(e, "zatca.env") != "Simulation" {
			t.Errorf("env change before documents: %v", e["zatca"])
		}
		Patch(t, s2.Token, "stores", s2.ID, M{"zatca": M{"env": "NonProduction"}})
		// a manager has no settings edit at all
		_, mtok := s2.User(t, "r_manager")
		if r := Call(t, "PATCH", "/stores/"+s2.ID, mtok, M{"zatca": M{"env": "Simulation"}}); r.Code != 403 {
			t.Errorf("manager env change: %s", r)
		}
		// after the first sale the environment and the country are locked
		p := s2.Product(t, 5, 10, 10)
		c := s2.Customer(t, "")
		Create(t, s2.Token, "sales", M{"storeId": s2.ID, "date": s2.Now(), "customerId": c["id"], "items": []M{s2.Line(S(p["id"]), 1, 10)},
			"payments": []M{{"date": s2.Now(), "amount": 11.5, "method": "cash"}}})
		cur := Read(t, s2.Token, "stores", s2.ID)
		if Get(cur, "zatca.envLocked") != true || Get(cur, "country.locked") != true {
			t.Errorf("locks after a sale: %v %v", cur["zatca"], cur["country"])
		}
		if r := PatchResp(t, s2.Token, "stores", s2.ID, M{"zatca": M{"env": "Simulation"}}); r.Code != 400 || r.ErrField("zatca.env") == "" {
			t.Errorf("env change after a sale: %s", r)
		}
		if r := PatchResp(t, s2.Token, "stores", s2.ID, M{"countryCode": "AE"}); r.Code != 400 || r.ErrField("countryCode") == "" {
			t.Errorf("country change after a sale: %s", r)
		}
		// the same value is not a change
		Must(t, PatchResp(t, s2.Token, "stores", s2.ID, M{"zatca": M{"env": "NonProduction"}}), 200, "same env")
		if Get(Read(t, s2.Token, "stores", s2.ID), "zatca.env") != "NonProduction" {
			t.Errorf("env changed after lock")
		}
	})

	t.Run("delete and restore", func(t *testing.T) {
		for _, q := range []string{"", "?hard=1"} {
			if r := Call(t, "DELETE", "/stores/"+s.ID+q, s.Token, nil); r.Code != 403 {
				t.Errorf("delete store%s: %s", q, r)
			}
		}
		g := Read(t, s.Token, "stores", s.ID)
		if g["deleted"] != false {
			t.Errorf("store deleted")
		}
		if r := Call(t, "POST", "/stores/"+s.ID+"/restore", s.Token, nil); r.Code != 200 || r.Body["deleted"] != false {
			t.Errorf("restore a live store: %s", r)
		}
		if r := Call(t, "POST", "/stores", s.Token, M{"nameEn": "Second"}); r.Code != 403 {
			t.Errorf("owner creates a store: %s", r)
		}
	})

	t.Run("platform admin", func(t *testing.T) {
		// a store owner is not a platform admin
		if r := Call(t, "GET", "/admin/zatca-reconnects", s.Token, nil); r.Code != 403 {
			t.Errorf("owner reconnect list: %s", r)
		}
		if r := Call(t, "POST", "/stores/"+s.ID+"/zatca/unmark", s.Token, nil); r.Code != 403 {
			t.Errorf("owner unmark: %s", r)
		}
		if r := Call(t, "GET", "/admin/zatca-reconnects", "", nil); r.Code != 401 {
			t.Errorf("no token: %s", r)
		}
		admin, db := secPlatformAdmin(t)
		if db == nil {
			return
		}
		me := Must(t, Call(t, "GET", "/auth/me", admin, nil), 200, "admin me")
		if Get(me.Body, "user.platformAdmin") != true {
			t.Fatalf("not a platform admin: %v", me.Body["user"])
		}
		for _, q := range []string{"limit=0", "limit=101", "limit=abc"} {
			if r := Call(t, "GET", "/admin/zatca-reconnects?"+q, admin, nil); r.Code != 400 || r.ErrField("limit") == "" {
				t.Errorf("reconnects %s: %s", q, r)
			}
		}
		// mark s for re-connection (what a ZATCA-sensitive change of a connected store does)
		oid, _ := primitive.ObjectIDFromHex(s.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := db.Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"zatca.zatca_reconnect_required": true}}); err != nil {
			t.Fatal(err)
		}
		name := S(st["nameEn"])
		l := Must(t, Call(t, "GET", "/admin/zatca-reconnects?limit=100&q="+strings.ReplaceAll(name, " ", "+"), admin, nil), 200, "reconnects")
		var row M
		for _, it := range Objs(l.Body["items"]) {
			if S(it["id"]) == s.ID {
				row = it
			}
		}
		if row == nil || row["nameEn"] != name || Get(row, "zatca.reconnectNeeded") != true || Num(l.Body["total"]) < 1 {
			t.Errorf("reconnect list: %s", l)
		}
		if Get(Read(t, s.Token, "stores", s.ID), "zatca.reconnectNeeded") != true {
			t.Errorf("store shows no mark")
		}
		u := Must(t, Call(t, "POST", "/stores/"+s.ID+"/zatca/unmark", admin, nil), 200, "unmark")
		if Get(u.Body, "zatca.reconnectNeeded") != false || S(Get(u.Body, "zatca.reconnectClearedAt")) == "" {
			t.Errorf("unmark: %s", u)
		}
		if Get(Read(t, s.Token, "stores", s.ID), "zatca.reconnectNeeded") != false {
			t.Errorf("mark not cleared")
		}
		if r := Call(t, "POST", "/stores/"+s.ID+"/zatca/unmark", admin, nil); r.Code != 409 || r.ErrCode() != "zatca_not_marked" {
			t.Errorf("unmark twice: %s", r)
		}
		for _, id := range []string{"x", primitive.NewObjectID().Hex()} {
			if r := Call(t, "POST", "/stores/"+id+"/zatca/unmark", admin, nil); r.Code != 404 {
				t.Errorf("unmark %s: %s", id, r)
			}
		}
		// a platform admin adds a store; a broken body names the fields
		if r := Call(t, "POST", "/stores", admin, M{"nameEn": ""}); r.Code != 400 || r.ErrField("nameEn") == "" || r.ErrField("category") == "" {
			t.Errorf("admin creates an empty store: %s", r)
		}
		ns := Call(t, "POST", "/stores", admin, M{"nameEn": "Admin Store " + Uniq(), "nameAr": "متجر", "category": "Grocery", "vatNo": "310122393500003",
			"crNo": "1010101010", "email": "store" + Uniq() + "@e2e.example", "phone": "0112345678", "starterCatalog": false,
			"address": M{"buildingNo": "1234", "streetEn": "Olaya", "streetAr": "العليا", "districtEn": "Olaya", "districtAr": "العليا",
				"cityEn": "Riyadh", "cityAr": "الرياض", "postalCode": "12345", "additionalNo": "6789"}})
		if ns.Code == 201 {
			dropStoreDBAfter(t, S(ns.Body["id"]))
		}
		if ns.Code != 201 || Get(ns.Body, "zatca.phase") != 2.0 || S(ns.Body["id"]) == "" {
			t.Errorf("admin creates a store: %s", ns)
		} else if r := Call(t, "GET", "/stores/"+S(ns.Body["id"]), s.Token, nil); r.Code != 404 && r.Code != 403 {
			t.Errorf("an owner reads the admin's new store: %s", r)
		}
	})
}

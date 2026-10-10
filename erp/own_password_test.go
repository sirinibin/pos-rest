package erp

import (
	"net/http"
	"testing"
)

func TestOwnPasswordErrors(t *testing.T) {
	cases := []struct {
		name, cur, next string
		want            map[string]string
	}{
		{"ok", "Old@12345", "New@12345", map[string]string{}},
		{"both missing", "", "", map[string]string{"currentPassword": "required", "newPassword": "required"}},
		{"current missing", "", "New@12345", map[string]string{"currentPassword": "required"}},
		{"too short", "Old@12345", "Ab1!", map[string]string{"newPassword": "at least 8 characters"}},
		{"exactly 8", "Old@12345", "Abcd123!", map[string]string{}},
		{"same as current", "Same@1234", "Same@1234", map[string]string{"newPassword": "must differ from the current password"}},
	}
	for _, c := range cases {
		got := ownPasswordErrors(c.cur, c.next)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
			continue
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("%s: %s = %q want %q", c.name, k, got[k], v)
			}
		}
	}
}

// The legacy user model refuses a user without a mobile number ("Mob is required"), so
// the contract validation says so on the phone field before the write is attempted.
func TestUserValidate_PhoneRequired(t *testing.T) {
	for _, c := range []struct {
		phone, want string
	}{
		{"", "required"},
		{"   ", "required"},
		{"123", "invalid phone"},
		{"0551234567", ""},
		{"+96599123456", ""},
	} {
		e := userValidate(nil, M{"name": "A", "phone": c.phone, "role": "r_viewer", "storeIds": []string{"x"}}, nil)
		if e["phone"] != c.want {
			t.Errorf("phone %q: got %q want %q", c.phone, e["phone"], c.want)
		}
	}
}

func TestOwnPassword_Unauthenticated(t *testing.T) {
	for _, tok := range []string{"", "not-a-jwt"} {
		if r := call(t, "POST", "/auth/password", tok, M{"currentPassword": "a", "newPassword": "b"}); r.Code != http.StatusUnauthorized {
			t.Errorf("token %q: got %d %s", tok, r.Code, r.Raw)
		}
	}
}

// A signed-up owner changes their own password: wrong current password is refused on
// the currentPassword field, then the new password signs in and the old one does not.
func TestAPI_ChangeOwnPassword(t *testing.T) {
	requireDB(t)
	body := validSignup()
	email := "ownpw+" + nowFn().Format("150405.000000") + "@signup.example"
	body["owner"].(M)["email"] = email
	r := call(t, "POST", "/auth/signup", "", body)
	if r.Code != 201 {
		t.Fatalf("signup: %d %s", r.Code, r.Raw)
	}
	tok, sid := str(r.Body["accessToken"]), str(get(r.Body, "store.id"))
	defer cleanupStore(t, sid)
	old := str(body["owner"].(M)["password"])

	if r := call(t, "POST", "/auth/password", tok, M{"currentPassword": "wrong-one", "newPassword": "Fresh@2026"}); r.Code != 400 || r.errField("currentPassword") == "" {
		t.Fatalf("wrong current: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/auth/password", tok, M{"currentPassword": old, "newPassword": "short"}); r.Code != 400 || r.errField("newPassword") == "" {
		t.Fatalf("short new: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/auth/password", tok, M{"currentPassword": old, "newPassword": "Fresh@2026"}); r.Code != 200 || str(get(r.Body, "user.email")) == "" {
		t.Fatalf("change: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/auth/login", "", M{"email": email, "password": "Fresh@2026"}); r.Code != 200 {
		t.Fatalf("login with new password: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/auth/login", "", M{"email": email, "password": old}); r.Code != 401 {
		t.Fatalf("old password still works: %d %s", r.Code, r.Raw)
	}
}

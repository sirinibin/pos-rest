package controller

import (
	"os"
	"strings"
	"testing"
)

// Source-level tests for the Admin-role assignment guards in user.go (controller layer).
// These verify code structure without a live database or HTTP server.

var userControllerSrc = func() string {
	b, err := os.ReadFile("user.go")
	if err != nil {
		panic("cannot read controller/user.go: " + err.Error())
	}
	return string(b)
}()

// ── CreateUser guard ──────────────────────────────────────────────────────────

func TestCreateUser_HasAdminRoleGuard(t *testing.T) {
	if !strings.Contains(userControllerSrc, `user.Role == "Admin"`) {
		t.Error("CreateUser must check if the new user's role is Admin before allowing it")
	}
}

func TestCreateUser_GuardUsesRequestingUserRole(t *testing.T) {
	if !strings.Contains(userControllerSrc, `requestingUser.Role != "Admin"`) {
		t.Error("CreateUser guard must block by requestingUser.Role != Admin")
	}
}

func TestCreateUser_GuardDoesNotRelyOnAdminFlag(t *testing.T) {
	// The guard must NOT use requestingUser.Admin — role field is the sole authority.
	// Using the admin flag would allow users with admin=true but role≠Admin to bypass.
	src := userControllerSrc
	guardIdx := strings.Index(src, `Only Admins (role=Admin) can create`)
	if guardIdx == -1 {
		t.Fatal("could not locate CreateUser Admin role guard comment")
	}
	// Find the closing brace of the guard block (within next 300 chars)
	guardBlock := src[guardIdx : guardIdx+300]
	if strings.Contains(guardBlock, "requestingUser.Admin") {
		t.Error("CreateUser guard must NOT reference requestingUser.Admin — role field only")
	}
}

func TestCreateUser_GuardReturnsRoleError(t *testing.T) {
	if !strings.Contains(userControllerSrc, `"Only admins can assign the Admin role"`) {
		t.Error("CreateUser guard must return a descriptive role error message")
	}
}

func TestCreateUser_GuardFiresBeforeValidation(t *testing.T) {
	// The Admin role guard must appear before the Validate call so it can't be
	// bypassed by submitting data that passes validation but has role=Admin.
	guardIdx := strings.Index(userControllerSrc, `Only Admins (role=Admin) can create`)
	validateIdx := strings.Index(userControllerSrc, `user.Validate(w, r, "create")`)
	if guardIdx == -1 || validateIdx == -1 {
		t.Fatal("could not locate guard or validate call in CreateUser")
	}
	if guardIdx > validateIdx {
		t.Error("Admin role guard must appear BEFORE the Validate call in CreateUser")
	}
}

// ── UpdateUser guard ──────────────────────────────────────────────────────────

func TestUpdateUser_HasAdminRoleGuard(t *testing.T) {
	if !strings.Contains(userControllerSrc, `Only Admins (role=Admin) can set role=Admin`) {
		t.Error("UpdateUser must have an Admin role guard comment")
	}
}

func TestUpdateUser_GuardUsesAccessingUserRole(t *testing.T) {
	if !strings.Contains(userControllerSrc, `accessingUser.Role != "Admin"`) {
		t.Error("UpdateUser guard must block by accessingUser.Role != Admin")
	}
}

func TestUpdateUser_GuardDoesNotRelyOnAdminFlag(t *testing.T) {
	src := userControllerSrc
	guardIdx := strings.Index(src, `Only Admins (role=Admin) can set role=Admin`)
	if guardIdx == -1 {
		t.Fatal("could not locate UpdateUser Admin role guard comment")
	}
	guardBlock := src[guardIdx : guardIdx+300]
	if strings.Contains(guardBlock, "accessingUser.Admin") {
		t.Error("UpdateUser guard must NOT reference accessingUser.Admin — role field only")
	}
}

func TestUpdateUser_GuardFiresBeforeValidation(t *testing.T) {
	guardIdx := strings.Index(userControllerSrc, `Only Admins (role=Admin) can set role=Admin`)
	validateIdx := strings.Index(userControllerSrc, `user.Validate(w, r, "update")`)
	if guardIdx == -1 || validateIdx == -1 {
		t.Fatal("could not locate guard or validate call in UpdateUser")
	}
	if guardIdx > validateIdx {
		t.Error("Admin role guard must appear BEFORE the Validate call in UpdateUser")
	}
}

func TestUpdateUser_GuardReturnsRoleError(t *testing.T) {
	// Both Create and Update return the same error string — one occurrence is sufficient
	count := strings.Count(userControllerSrc, `"Only admins can assign the Admin role"`)
	if count < 2 {
		t.Errorf("expected Admin role error message in both CreateUser and UpdateUser, found %d occurrence(s)", count)
	}
}

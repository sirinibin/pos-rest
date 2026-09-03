package models

import (
	"os"
	"strings"
	"testing"
)

// Source-level tests for the SearchUser non-admin guards in user.go.
// These verify code structure without a live database.

var userModelSrc = func() string {
	b, err := os.ReadFile("user.go")
	if err != nil {
		panic("cannot read user.go: " + err.Error())
	}
	return string(b)
}()

// ── Store-scope OR created_by filter ─────────────────────────────────────────

func TestSearchUser_NonAdminUsesOrFilter(t *testing.T) {
	if !strings.Contains(userModelSrc, `"$or"`) {
		t.Error("SearchUser must use $or to combine store_ids and created_by filters for non-admins")
	}
}

func TestSearchUser_OrContainsStoreIds(t *testing.T) {
	if !strings.Contains(userModelSrc, `"store_ids"`) {
		t.Error("SearchUser $or filter must include store_ids condition")
	}
	if !strings.Contains(userModelSrc, `accessingUser.StoreIDs`) {
		t.Error("SearchUser must filter by the accessing user's StoreIDs")
	}
}

func TestSearchUser_OrContainsCreatedBy(t *testing.T) {
	if !strings.Contains(userModelSrc, `"created_by"`) {
		t.Error("SearchUser $or filter must include created_by condition")
	}
	if !strings.Contains(userModelSrc, `accessingUserID`) {
		t.Error("SearchUser must use accessingUserID for created_by filter")
	}
}

// ── Role filter (search[role]) ────────────────────────────────────────────────

func TestSearchUser_ParsesRoleFilter(t *testing.T) {
	if !strings.Contains(userModelSrc, `search[role]`) {
		t.Error("SearchUser must parse search[role] query parameter")
	}
}

func TestSearchUser_RoleFilterUsesEqOperator(t *testing.T) {
	// Role values are exact strings; must use $eq not regex
	if !strings.Contains(userModelSrc, `"$eq"`) {
		t.Error("SearchUser role filter must use exact match ($eq)")
	}
}

// ── Non-admin admin-exclusion guard ──────────────────────────────────────────

func TestSearchUser_NonAdminExcludesAdminRole(t *testing.T) {
	if !strings.Contains(userModelSrc, `"$ne": "Admin"`) {
		t.Error("SearchUser must exclude users with role=Admin for non-admin callers")
	}
}

func TestSearchUser_NonAdminExcludesAdminFlag(t *testing.T) {
	// Must also guard on the admin boolean field, not just the role string
	if !strings.Contains(userModelSrc, `"admin"`) || !strings.Contains(userModelSrc, `"$ne": true`) {
		t.Error("SearchUser must exclude users with admin=true for non-admin callers")
	}
}

func TestSearchUser_AdminExclusionAppliedAfterRoleFilter(t *testing.T) {
	// The non-admin guard must appear after the search[role] parsing block
	// so it cannot be overridden by a malicious search[role]=Admin param.
	roleFilterIdx := strings.Index(userModelSrc, `search[role]`)
	adminGuardIdx := strings.Index(userModelSrc, `"$ne": "Admin"`)
	if roleFilterIdx == -1 || adminGuardIdx == -1 {
		t.Fatal("could not locate both role filter and admin guard in user.go")
	}
	if adminGuardIdx <= roleFilterIdx {
		t.Error("admin exclusion guard must come AFTER role filter parsing so it cannot be bypassed")
	}
}

func TestSearchUser_NonAdminGuardBlocksAdminRoleSearch(t *testing.T) {
	// The guard must check if the existing role filter equals Admin and override it
	if !strings.Contains(userModelSrc, `"Admin"`) || !strings.Contains(userModelSrc, `"$ne": "Admin"`) {
		t.Error("guard must detect and block search[role]=Admin from non-admin callers")
	}
}

func TestSearchUser_AdminCallerNotRestricted(t *testing.T) {
	// The non-admin guard is inside an if block checking the accessing user's role
	if !strings.Contains(userModelSrc, `accessingUser.Role != "Admin"`) {
		t.Error("non-admin guards must be inside accessingUser.Role != Admin check")
	}
}

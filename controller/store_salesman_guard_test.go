package controller

import (
	"os"
	"strings"
	"testing"
)

// Source-level tests for the SalesMan role guard in UpdateStore (store.go).
// These verify code structure without a live database or HTTP server.

var storeControllerSrc = func() string {
	b, err := os.ReadFile("store.go")
	if err != nil {
		panic("cannot read controller/store.go: " + err.Error())
	}
	return string(b)
}()

func TestUpdateStore_HasSalesManGuard(t *testing.T) {
	if !strings.Contains(storeControllerSrc, `accessingUser.Role == "SalesMan"`) {
		t.Error("UpdateStore must check if the accessing user's role is SalesMan and block them")
	}
}

func TestUpdateStore_SalesManGuardUsesRoleField(t *testing.T) {
	// Guard must use Role field, not the legacy admin boolean flag
	idx := strings.Index(storeControllerSrc, `accessingUser.Role == "SalesMan"`)
	if idx == -1 {
		t.Fatal("could not locate SalesMan guard in UpdateStore")
	}
	block := storeControllerSrc[idx : idx+300]
	if strings.Contains(block, "accessingUser.Admin") {
		t.Error("SalesMan guard must NOT reference accessingUser.Admin — role field only")
	}
}

func TestUpdateStore_SalesManGuardReturnsError(t *testing.T) {
	if !strings.Contains(storeControllerSrc, `"SalesMan users cannot update store settings"`) {
		t.Error("UpdateStore SalesMan guard must return a descriptive error message")
	}
}

func TestUpdateStore_SalesManGuardReturnsForbidden(t *testing.T) {
	// The guard must set StatusForbidden (403), not StatusInternalServerError
	idx := strings.Index(storeControllerSrc, `SalesMan cannot update store settings`)
	if idx == -1 {
		t.Fatal("could not locate SalesMan guard comment in UpdateStore")
	}
	block := storeControllerSrc[idx : idx+300]
	if !strings.Contains(block, "StatusForbidden") {
		t.Error("SalesMan guard must respond with http.StatusForbidden (403)")
	}
}

func TestUpdateStore_SalesManGuardFiresAfterMembershipCheck(t *testing.T) {
	// The membership check ("unauthorized access") must come before the SalesMan guard
	// so that a SalesMan who is not even a store member hits the membership check first.
	membershipIdx := strings.Index(storeControllerSrc, `"unauthorized access"`)
	salesManIdx := strings.Index(storeControllerSrc, `accessingUser.Role == "SalesMan"`)
	if membershipIdx == -1 || salesManIdx == -1 {
		t.Fatal("could not locate membership check or SalesMan guard in store.go")
	}
	if membershipIdx > salesManIdx {
		t.Error("membership check must appear BEFORE SalesMan role guard in UpdateStore")
	}
}

func TestUpdateStore_SalesManGuardFiresBeforeUpdate(t *testing.T) {
	// Guard must appear before store.UpdatedBy is set in UpdateStore.
	// Use LastIndex for store.UpdatedBy because CreateStore also sets it earlier in the file.
	guardIdx := strings.Index(storeControllerSrc, `SalesMan cannot update store settings`)
	updateIdx := strings.LastIndex(storeControllerSrc, `store.UpdatedBy`)
	if guardIdx == -1 || updateIdx == -1 {
		t.Fatal("could not locate SalesMan guard or UpdatedBy assignment in store.go")
	}
	if guardIdx > updateIdx {
		t.Error("SalesMan guard must appear BEFORE store.UpdatedBy assignment in UpdateStore")
	}
}

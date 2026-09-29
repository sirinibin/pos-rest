package models

import (
	"os"
	"strings"
	"testing"
)

// TestFindVendorByNameByVatNo_NoStoreIDFilter verifies that FindVendorByNameByVatNo
// does NOT include store_id in its MongoDB query filter.
//
// Background: the collection is already store-scoped (db "store_<id>"), so old vendor
// records created without store_id in the document are still reachable.
// IsVendorExistsByVatNoByName uses the same approach (no store_id filter in the query).
// Including store_id in the FindVendorByNameByVatNo filter silently misses those records,
// which triggers a 409 on creation (vendor exists) but then can't resolve the vendor.
func TestFindVendorByNameByVatNo_NoStoreIDFilter(t *testing.T) {
	src, err := os.ReadFile("vendor.go")
	if err != nil {
		t.Fatalf("could not read vendor.go: %v", err)
	}
	text := string(src)

	funcMarker := "func (store *Store) FindVendorByNameByVatNo("
	start := strings.Index(text, funcMarker)
	if start < 0 {
		t.Fatal("FindVendorByNameByVatNo function not found in vendor.go")
	}

	// Grab everything from the function start to the next top-level func declaration
	rest := text[start+len(funcMarker):]
	nextFunc := strings.Index(rest, "\nfunc ")
	var funcBody string
	if nextFunc < 0 {
		funcBody = rest
	} else {
		funcBody = rest[:nextFunc]
	}

	if strings.Contains(funcBody, `"store_id"`) {
		t.Errorf(
			"FindVendorByNameByVatNo must NOT include \"store_id\" in its filter.\n"+
				"The collection is already store-scoped via db.GetDB(\"store_<id>\").\n"+
				"Old vendor records created without the store_id field would be missed,\n"+
				"causing 409 on create but a null lookup — breaking Create Purchase auto-fill.",
		)
	}
}

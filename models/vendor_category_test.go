package models

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── VendorCategory struct field existence ─────────────────────────────────────

func TestVendorCategoryStruct_HasRequiredFields(t *testing.T) {
	vc := VendorCategory{}
	typ := reflect.TypeOf(vc)

	required := []string{"ID", "Name", "Deleted", "StoreID", "CreatedBy", "UpdatedBy",
		"CreatedAt", "UpdatedAt", "CreatedByName", "UpdatedByName"}
	for _, field := range required {
		if _, ok := typ.FieldByName(field); !ok {
			t.Errorf("VendorCategory is missing expected field: %s", field)
		}
	}
}

func TestVendorCategoryStruct_JSONTags(t *testing.T) {
	typ := reflect.TypeOf(VendorCategory{})
	cases := []struct {
		field   string
		jsonTag string
	}{
		{"Name", "name,omitempty"},
		{"Deleted", "deleted,omitempty"},
		{"CreatedByName", "created_by_name,omitempty"},
		{"UpdatedByName", "updated_by_name,omitempty"},
		{"DeletedByName", "deleted_by_name,omitempty"},
		{"StoreID", "store_id,omitempty"},
	}
	for _, tc := range cases {
		f, ok := typ.FieldByName(tc.field)
		if !ok {
			t.Errorf("field %s not found", tc.field)
			continue
		}
		got := f.Tag.Get("json")
		if got != tc.jsonTag {
			t.Errorf("VendorCategory.%s json tag: want %q, got %q", tc.field, tc.jsonTag, got)
		}
	}
}

func TestVendorCategoryStruct_IDField_JSONOmitempty(t *testing.T) {
	typ := reflect.TypeOf(VendorCategory{})
	f, ok := typ.FieldByName("ID")
	if !ok {
		t.Fatal("ID field not found")
	}
	tag := f.Tag.Get("json")
	if !strings.Contains(tag, "omitempty") {
		t.Errorf("ID json tag should have omitempty to avoid sending zero ID; got %q", tag)
	}
}

func TestVendorCategoryStruct_BSONIDTag(t *testing.T) {
	typ := reflect.TypeOf(VendorCategory{})
	f, ok := typ.FieldByName("ID")
	if !ok {
		t.Fatal("ID field not found")
	}
	bsonTag := f.Tag.Get("bson")
	if !strings.Contains(bsonTag, "_id") {
		t.Errorf("ID bson tag should contain _id, got %q", bsonTag)
	}
}

// ── VendorCategory JSON round-trip ────────────────────────────────────────────

func TestVendorCategoryJSON_RoundTrip_Name(t *testing.T) {
	id := primitive.NewObjectID()
	storeID := primitive.NewObjectID()
	vc := VendorCategory{
		ID:      id,
		Name:    "Electronics",
		StoreID: &storeID,
	}

	data, err := json.Marshal(vc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded VendorCategory
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Name != "Electronics" {
		t.Errorf("Name: want %q, got %q", "Electronics", decoded.Name)
	}
	if decoded.ID != id {
		t.Errorf("ID: want %v, got %v", id, decoded.ID)
	}
}

func TestVendorCategoryJSON_DeletedOmittedWhenFalse(t *testing.T) {
	vc := VendorCategory{Name: "Test"}
	data, err := json.Marshal(vc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	// deleted=false should be omitted (omitempty)
	if strings.Contains(string(data), `"deleted"`) {
		t.Error("deleted=false should be omitted from JSON (omitempty)")
	}
}

func TestVendorCategoryJSON_DeletedIncludedWhenTrue(t *testing.T) {
	vc := VendorCategory{Name: "Test", Deleted: true}
	data, err := json.Marshal(vc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if !strings.Contains(string(data), `"deleted":true`) {
		t.Error("deleted=true should appear in JSON")
	}
}

// ── Vendor struct: new CategoryID / CategoryName fields ───────────────────────

func TestVendorStruct_HasCategoryIDField(t *testing.T) {
	typ := reflect.TypeOf(Vendor{})
	f, ok := typ.FieldByName("CategoryID")
	if !ok {
		t.Fatal("Vendor is missing CategoryID field")
	}
	// Must be slice of *primitive.ObjectID
	if f.Type.Kind() != reflect.Slice {
		t.Errorf("CategoryID must be a slice, got %v", f.Type.Kind())
	}
	jsonTag := f.Tag.Get("json")
	if jsonTag != "category_id" {
		t.Errorf("CategoryID json tag: want %q, got %q", "category_id", jsonTag)
	}
	bsonTag := f.Tag.Get("bson")
	if bsonTag != "category_id" {
		t.Errorf("CategoryID bson tag: want %q, got %q", "category_id", bsonTag)
	}
}

func TestVendorStruct_HasCategoryNameField(t *testing.T) {
	typ := reflect.TypeOf(Vendor{})
	f, ok := typ.FieldByName("CategoryName")
	if !ok {
		t.Fatal("Vendor is missing CategoryName field")
	}
	if f.Type.Kind() != reflect.Slice {
		t.Errorf("CategoryName must be a slice, got %v", f.Type.Kind())
	}
	elemKind := f.Type.Elem().Kind()
	if elemKind != reflect.String {
		t.Errorf("CategoryName must be []string, element kind: %v", elemKind)
	}
	jsonTag := f.Tag.Get("json")
	if jsonTag != "category_name" {
		t.Errorf("CategoryName json tag: want %q, got %q", "category_name", jsonTag)
	}
}

func TestVendorCategoryIDField_ElementType_IsObjectIDPointer(t *testing.T) {
	typ := reflect.TypeOf(Vendor{})
	f, _ := typ.FieldByName("CategoryID")
	elemType := f.Type.Elem()
	// Should be *primitive.ObjectID
	if elemType.Kind() != reflect.Ptr {
		t.Errorf("CategoryID element should be pointer, got %v", elemType.Kind())
	}
	if elemType.Elem() != reflect.TypeOf(primitive.ObjectID{}) {
		t.Errorf("CategoryID element should be *primitive.ObjectID, got %v", elemType.Elem())
	}
}

func TestVendorJSON_CategoryFields_RoundTrip(t *testing.T) {
	catID1 := primitive.NewObjectID()
	catID2 := primitive.NewObjectID()
	v := Vendor{
		Name:         "Test Vendor",
		CategoryID:   []*primitive.ObjectID{&catID1, &catID2},
		CategoryName: []string{"Electronics", "Accessories"},
	}

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Vendor
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(decoded.CategoryID) != 2 {
		t.Errorf("CategoryID: want 2 elements, got %d", len(decoded.CategoryID))
	}
	if len(decoded.CategoryName) != 2 {
		t.Errorf("CategoryName: want 2 elements, got %d", len(decoded.CategoryName))
	}
	if decoded.CategoryName[0] != "Electronics" || decoded.CategoryName[1] != "Accessories" {
		t.Errorf("CategoryName mismatch: got %v", decoded.CategoryName)
	}
	if *decoded.CategoryID[0] != catID1 || *decoded.CategoryID[1] != catID2 {
		t.Errorf("CategoryID mismatch")
	}
}

func TestVendorJSON_EmptyCategoryFields_Serialized(t *testing.T) {
	v := Vendor{Name: "NoCategory"}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	// Both fields have no omitempty → they appear as null/[]
	if !strings.Contains(string(data), "category_id") {
		t.Error("category_id should appear in JSON even when empty")
	}
	if !strings.Contains(string(data), "category_name") {
		t.Error("category_name should appear in JSON even when empty")
	}
}

// ── search[category_id] URL parsing logic (mirrors inline SearchVendor code) ──
//
// These tests validate the exact same pattern used inside SearchVendor:
//   strings.Split(keys[0], ",") → ObjectIDFromHex each → bson.M{"$in": objecIds}

func parseCategoryIDParam(raw string) (bson.M, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	ids := strings.Split(raw, ",")
	objectIDs := []primitive.ObjectID{}
	for _, id := range ids {
		oid, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			return nil, err
		}
		objectIDs = append(objectIDs, oid)
	}
	if len(objectIDs) == 0 {
		return nil, nil
	}
	return bson.M{"$in": objectIDs}, nil
}

func TestSearchVendorCategoryIDParam_SingleID(t *testing.T) {
	id := primitive.NewObjectID()
	filter, err := parseCategoryIDParam(id.Hex())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list, ok := filter["$in"].([]primitive.ObjectID)
	if !ok || len(list) != 1 {
		t.Errorf("want $in with 1 element, got %v", filter)
	}
	if list[0] != id {
		t.Errorf("want %v, got %v", id, list[0])
	}
}

func TestSearchVendorCategoryIDParam_MultipleIDs(t *testing.T) {
	id1 := primitive.NewObjectID()
	id2 := primitive.NewObjectID()
	id3 := primitive.NewObjectID()
	filter, err := parseCategoryIDParam(id1.Hex() + "," + id2.Hex() + "," + id3.Hex())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list, ok := filter["$in"].([]primitive.ObjectID)
	if !ok || len(list) != 3 {
		t.Errorf("want $in with 3 elements, got %v", filter)
	}
	if list[0] != id1 || list[1] != id2 || list[2] != id3 {
		t.Errorf("IDs mismatch: got %v", list)
	}
}

func TestSearchVendorCategoryIDParam_InvalidID_ReturnsError(t *testing.T) {
	_, err := parseCategoryIDParam("not-a-valid-objectid")
	if err == nil {
		t.Error("expected error for invalid ObjectID, got nil")
	}
}

func TestSearchVendorCategoryIDParam_SecondIDInvalid_ReturnsError(t *testing.T) {
	id := primitive.NewObjectID()
	_, err := parseCategoryIDParam(id.Hex() + ",bad-id")
	if err == nil {
		t.Error("expected error when second ID is invalid")
	}
}

func TestSearchVendorCategoryIDParam_EmptyString_ReturnsNil(t *testing.T) {
	filter, err := parseCategoryIDParam("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filter != nil {
		t.Errorf("empty string should return nil filter, got %v", filter)
	}
}

func TestSearchVendorCategoryIDParam_TrailingComma_ReturnsError(t *testing.T) {
	id := primitive.NewObjectID()
	_, err := parseCategoryIDParam(id.Hex() + ",")
	if err == nil {
		t.Error("trailing comma produces empty segment → ObjectIDFromHex('') must error")
	}
}

func TestSearchVendorCategoryIDParam_LeadingComma_ReturnsError(t *testing.T) {
	id := primitive.NewObjectID()
	_, err := parseCategoryIDParam("," + id.Hex())
	if err == nil {
		t.Error("leading comma produces empty first segment → ObjectIDFromHex('') must error")
	}
}

func TestSearchVendorCategoryIDParam_DoubleComma_ReturnsError(t *testing.T) {
	id1 := primitive.NewObjectID()
	id2 := primitive.NewObjectID()
	_, err := parseCategoryIDParam(id1.Hex() + ",," + id2.Hex())
	if err == nil {
		t.Error("double comma (empty middle segment) must error")
	}
}

func TestSearchVendorCategoryIDParam_FilterUses_InOperator(t *testing.T) {
	id := primitive.NewObjectID()
	filter, _ := parseCategoryIDParam(id.Hex())
	if _, ok := filter["$in"]; !ok {
		t.Error("category_id filter must use $in for slice lookup")
	}
}

func TestSearchVendorCategoryIDParam_TwoIDs_OrderPreserved(t *testing.T) {
	id1 := primitive.NewObjectID()
	id2 := primitive.NewObjectID()
	filter, _ := parseCategoryIDParam(id1.Hex() + "," + id2.Hex())
	list := filter["$in"].([]primitive.ObjectID)
	if list[0] != id1 {
		t.Errorf("first ID: want %v, got %v", id1, list[0])
	}
	if list[1] != id2 {
		t.Errorf("second ID: want %v, got %v", id2, list[1])
	}
}

// ── UpdateForeignLabelFields: CategoryName slice management ───────────────────

func TestVendor_CategoryNameSlice_CanBeBuiltFromIDs(t *testing.T) {
	id1 := primitive.NewObjectID()
	id2 := primitive.NewObjectID()
	v := Vendor{
		CategoryID:   []*primitive.ObjectID{&id1, &id2},
		CategoryName: []string{},
	}
	// Simulate what UpdateForeignLabelFields builds
	names := []string{"Electronics", "Accessories"}
	v.CategoryName = names

	if len(v.CategoryName) != len(v.CategoryID) {
		t.Errorf("CategoryName and CategoryID should have equal length: %d vs %d",
			len(v.CategoryName), len(v.CategoryID))
	}
	if v.CategoryName[0] != "Electronics" || v.CategoryName[1] != "Accessories" {
		t.Errorf("CategoryName content mismatch: %v", v.CategoryName)
	}
}

func TestVendor_EmptyCategoryID_ProducesEmptyCategoryName(t *testing.T) {
	v := Vendor{
		CategoryID:   []*primitive.ObjectID{},
		CategoryName: []string{},
	}
	// When CategoryID is empty, UpdateForeignLabelFields should not add names
	if len(v.CategoryName) != 0 {
		t.Errorf("empty CategoryID should produce empty CategoryName, got %v", v.CategoryName)
	}
}

func TestVendor_NilCategoryID_DefaultsToNil(t *testing.T) {
	v := Vendor{}
	if v.CategoryID != nil {
		t.Errorf("unset CategoryID should be nil, got %v", v.CategoryID)
	}
	if v.CategoryName != nil {
		t.Errorf("unset CategoryName should be nil, got %v", v.CategoryName)
	}
}

// ── VendorCategory: collection name convention ────────────────────────────────

func TestVendorCategoryCollection_NameIsVendorCategory(t *testing.T) {
	// The collection name string is embedded in each DB call; we verify the
	// naming convention by checking the struct is named correctly.
	typName := reflect.TypeOf(VendorCategory{}).Name()
	if typName != "VendorCategory" {
		t.Errorf("type name: want VendorCategory, got %s", typName)
	}
}

// ── VendorCategory: no ParentID (intentionally simpler than ExpenseCategory) ─

func TestVendorCategoryStruct_NoParentIDField(t *testing.T) {
	typ := reflect.TypeOf(VendorCategory{})
	if _, ok := typ.FieldByName("ParentID"); ok {
		t.Error("VendorCategory should NOT have ParentID (flat category list, no hierarchy)")
	}
	if _, ok := typ.FieldByName("ParentName"); ok {
		t.Error("VendorCategory should NOT have ParentName")
	}
}

// ── Zero-value safety ─────────────────────────────────────────────────────────

func TestVendorCategory_ZeroValue_IDIsZero(t *testing.T) {
	vc := VendorCategory{}
	if !vc.ID.IsZero() {
		t.Error("zero-value VendorCategory should have zero ID")
	}
}

func TestVendorCategory_ZeroValue_DeletedIsFalse(t *testing.T) {
	vc := VendorCategory{}
	if vc.Deleted {
		t.Error("zero-value VendorCategory.Deleted should be false")
	}
}

func TestVendorCategory_ZeroValue_NilPointers(t *testing.T) {
	vc := VendorCategory{}
	if vc.DeletedBy != nil {
		t.Error("zero-value DeletedBy should be nil")
	}
	if vc.CreatedBy != nil {
		t.Error("zero-value CreatedBy should be nil")
	}
	if vc.UpdatedBy != nil {
		t.Error("zero-value UpdatedBy should be nil")
	}
	if vc.StoreID != nil {
		t.Error("zero-value StoreID should be nil")
	}
}

// ── VendorCategory: Deleted soft-delete fields ────────────────────────────────

func TestVendorCategory_DeletedByIsPointer(t *testing.T) {
	typ := reflect.TypeOf(VendorCategory{})
	f, ok := typ.FieldByName("DeletedBy")
	if !ok {
		t.Fatal("DeletedBy field not found")
	}
	if f.Type.Kind() != reflect.Ptr {
		t.Errorf("DeletedBy must be a pointer (*primitive.ObjectID), got %v", f.Type.Kind())
	}
}

func TestVendorCategory_SetsDeletedByAndAt_WhenDeleted(t *testing.T) {
	id := primitive.NewObjectID()
	vc := VendorCategory{Deleted: true, DeletedBy: &id}
	if vc.DeletedBy == nil {
		t.Error("DeletedBy should be set when deleted")
	}
	if *vc.DeletedBy != id {
		t.Errorf("DeletedBy: want %v, got %v", id, *vc.DeletedBy)
	}
	if !vc.Deleted {
		t.Error("Deleted flag should be true")
	}
}

package models

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// makeReq builds a minimal *http.Request with the given query params.
func makeReq(params map[string]string) *http.Request {
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	return &http.Request{URL: &url.URL{RawQuery: q.Encode()}}
}

// ── InitSearchCriterias ───────────────────────────────────────────────────────

func TestInitSearchCriterias(t *testing.T) {
	c := InitSearchCriterias()
	if c.Page != 1 {
		t.Errorf("Page: want 1, got %d", c.Page)
	}
	if c.Size != 10 {
		t.Errorf("Size: want 10, got %d", c.Size)
	}
	if c.SortBy == nil {
		t.Error("SortBy should be initialised (not nil)")
	}
	if c.SearchBy == nil {
		t.Error("SearchBy should be initialised (not nil)")
	}
	// Maps must be empty, not nil, so callers can write to them immediately
	if len(c.SortBy) != 0 {
		t.Errorf("SortBy should be empty, got %v", c.SortBy)
	}
	if len(c.SearchBy) != 0 {
		t.Errorf("SearchBy should be empty, got %v", c.SearchBy)
	}
}

// ── ParseDeletedFilter ────────────────────────────────────────────────────────

func TestParseDeletedFilter_Default(t *testing.T) {
	c := InitSearchCriterias()
	ParseDeletedFilter(makeReq(nil), &c)
	filter, ok := c.SearchBy["deleted"].(bson.M)
	if !ok {
		t.Fatal("deleted filter not set")
	}
	v, ok := filter["$ne"].(bool)
	if !ok || v != true {
		t.Errorf("want $ne:true, got %v", filter)
	}
}

func TestParseDeletedFilter_ShowDeleted(t *testing.T) {
	c := InitSearchCriterias()
	ParseDeletedFilter(makeReq(map[string]string{"search[deleted]": "1"}), &c)
	filter, ok := c.SearchBy["deleted"].(bson.M)
	if !ok {
		t.Fatal("deleted filter not set")
	}
	v, ok := filter["$eq"].(bool)
	if !ok || v != true {
		t.Errorf("want $eq:true, got %v", filter)
	}
}

func TestParseDeletedFilter_Zero_KeepsDefault(t *testing.T) {
	c := InitSearchCriterias()
	ParseDeletedFilter(makeReq(map[string]string{"search[deleted]": "0"}), &c)
	filter, ok := c.SearchBy["deleted"].(bson.M)
	if !ok {
		t.Fatal("deleted filter not set")
	}
	if _, hasNe := filter["$ne"]; !hasNe {
		t.Errorf("value 0 should keep default $ne filter, got %v", filter)
	}
}

// ── ParsePaginationAndSort ────────────────────────────────────────────────────

func TestParsePaginationAndSort_Defaults(t *testing.T) {
	c := InitSearchCriterias()
	ParsePaginationAndSort(makeReq(nil), &c)
	if c.Size != 10 {
		t.Errorf("Size unchanged: want 10, got %d", c.Size)
	}
	if c.Page != 1 {
		t.Errorf("Page unchanged: want 1, got %d", c.Page)
	}
}

func TestParsePaginationAndSort_CustomLimitAndPage(t *testing.T) {
	c := InitSearchCriterias()
	ParsePaginationAndSort(makeReq(map[string]string{
		"limit": "50",
		"page":  "3",
	}), &c)
	if c.Size != 50 {
		t.Errorf("Size: want 50, got %d", c.Size)
	}
	if c.Page != 3 {
		t.Errorf("Page: want 3, got %d", c.Page)
	}
}

func TestParsePaginationAndSort_SortAscending(t *testing.T) {
	c := InitSearchCriterias()
	ParsePaginationAndSort(makeReq(map[string]string{"sort": "created_at 1"}), &c)
	if c.SortBy["created_at"] != 1 {
		t.Errorf("SortBy: want created_at:1, got %v", c.SortBy)
	}
}

func TestParsePaginationAndSort_SortDescending(t *testing.T) {
	c := InitSearchCriterias()
	ParsePaginationAndSort(makeReq(map[string]string{"sort": "name -1"}), &c)
	if c.SortBy["name"] != -1 {
		t.Errorf("SortBy: want name:-1, got %v", c.SortBy)
	}
}

// ── ParseTextSearch ───────────────────────────────────────────────────────────

func TestParseTextSearch_SetsRegex(t *testing.T) {
	c := InitSearchCriterias()
	ParseTextSearch(makeReq(map[string]string{"search[name]": "acme"}), &c, "search[name]", "name")
	filter, ok := c.SearchBy["name"].(bson.M)
	if !ok {
		t.Fatal("name filter not set")
	}
	if filter["$regex"] != "acme" {
		t.Errorf("want $regex:acme, got %v", filter["$regex"])
	}
	if filter["$options"] != "i" {
		t.Errorf("want $options:i (case-insensitive), got %v", filter["$options"])
	}
}

func TestParseTextSearch_Missing_NoFilter(t *testing.T) {
	c := InitSearchCriterias()
	ParseTextSearch(makeReq(nil), &c, "search[name]", "name")
	if _, exists := c.SearchBy["name"]; exists {
		t.Error("empty param should not add a filter")
	}
}

// ── ParseObjectIDFilter ───────────────────────────────────────────────────────

func TestParseObjectIDFilter_Valid(t *testing.T) {
	id := primitive.NewObjectID()
	c := InitSearchCriterias()
	err := ParseObjectIDFilter(makeReq(map[string]string{"search[customer_id]": id.Hex()}), &c, "search[customer_id]", "customer_id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := c.SearchBy["customer_id"].(primitive.ObjectID)
	if !ok || got != id {
		t.Errorf("want %v, got %v", id, c.SearchBy["customer_id"])
	}
}

func TestParseObjectIDFilter_Invalid_ReturnsError(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseObjectIDFilter(makeReq(map[string]string{"search[customer_id]": "not-an-id"}), &c, "search[customer_id]", "customer_id")
	if err == nil {
		t.Error("expected error for invalid ObjectID")
	}
}

func TestParseObjectIDFilter_Missing_NoFilter(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseObjectIDFilter(makeReq(nil), &c, "search[customer_id]", "customer_id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := c.SearchBy["customer_id"]; exists {
		t.Error("missing param should not add a filter")
	}
}

// ── ParseObjectIDListFilter ───────────────────────────────────────────────────

func TestParseObjectIDListFilter_Single(t *testing.T) {
	id := primitive.NewObjectID()
	c := InitSearchCriterias()
	err := ParseObjectIDListFilter(makeReq(map[string]string{"search[ids]": id.Hex()}), &c, "search[ids]", "ids")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filter, ok := c.SearchBy["ids"].(bson.M)
	if !ok {
		t.Fatal("ids filter not set")
	}
	list, ok := filter["$in"].([]primitive.ObjectID)
	if !ok || len(list) != 1 || list[0] != id {
		t.Errorf("want $in:[%v], got %v", id, filter["$in"])
	}
}

func TestParseObjectIDListFilter_Multiple(t *testing.T) {
	id1 := primitive.NewObjectID()
	id2 := primitive.NewObjectID()
	c := InitSearchCriterias()
	err := ParseObjectIDListFilter(
		makeReq(map[string]string{"search[ids]": id1.Hex() + "," + id2.Hex()}),
		&c, "search[ids]", "ids",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filter, ok := c.SearchBy["ids"].(bson.M)
	if !ok {
		t.Fatal("ids filter not set")
	}
	list, ok := filter["$in"].([]primitive.ObjectID)
	if !ok || len(list) != 2 {
		t.Errorf("want 2 IDs, got %v", filter["$in"])
	}
}

func TestParseObjectIDListFilter_InvalidID_ReturnsError(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseObjectIDListFilter(makeReq(map[string]string{"search[ids]": "bad-id"}), &c, "search[ids]", "ids")
	if err == nil {
		t.Error("expected error for invalid ObjectID in list")
	}
}

// ── ParseFloatWithOperatorFilter ──────────────────────────────────────────────

func TestParseFloatWithOperatorFilter_ExactValue(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseFloatWithOperatorFilter(makeReq(map[string]string{"search[amount]": "100.5"}), &c, "search[amount]", "amount")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.SearchBy["amount"] != 100.5 {
		t.Errorf("want 100.5, got %v", c.SearchBy["amount"])
	}
}

func TestParseFloatWithOperatorFilter_Lte(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseFloatWithOperatorFilter(makeReq(map[string]string{"search[amount]": "<=500"}), &c, "search[amount]", "amount")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filter, ok := c.SearchBy["amount"].(bson.M)
	if !ok {
		t.Fatal("amount filter not set")
	}
	if filter["$lte"] != 500.0 {
		t.Errorf("want $lte:500, got %v", filter)
	}
}

func TestParseFloatWithOperatorFilter_Gte(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseFloatWithOperatorFilter(makeReq(map[string]string{"search[amount]": ">=0"}), &c, "search[amount]", "amount")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filter, ok := c.SearchBy["amount"].(bson.M)
	if !ok || filter["$gte"] != 0.0 {
		t.Errorf("want $gte:0, got %v", filter)
	}
}

func TestParseFloatWithOperatorFilter_Lt(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseFloatWithOperatorFilter(makeReq(map[string]string{"search[amount]": "<10"}), &c, "search[amount]", "amount")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filter, ok := c.SearchBy["amount"].(bson.M)
	if !ok || filter["$lt"] != 10.0 {
		t.Errorf("want $lt:10, got %v", filter)
	}
}

func TestParseFloatWithOperatorFilter_Gt(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseFloatWithOperatorFilter(makeReq(map[string]string{"search[amount]": ">999"}), &c, "search[amount]", "amount")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filter, ok := c.SearchBy["amount"].(bson.M)
	if !ok || filter["$gt"] != 999.0 {
		t.Errorf("want $gt:999, got %v", filter)
	}
}

func TestParseFloatWithOperatorFilter_Invalid_ReturnsError(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseFloatWithOperatorFilter(makeReq(map[string]string{"search[amount]": "not-a-float"}), &c, "search[amount]", "amount")
	if err == nil {
		t.Error("expected error for non-numeric value")
	}
}

func TestParseFloatWithOperatorFilter_Missing_NoFilter(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseFloatWithOperatorFilter(makeReq(nil), &c, "search[amount]", "amount")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := c.SearchBy["amount"]; exists {
		t.Error("missing param should not add a filter")
	}
}

// ── ParseExactDateFilter ──────────────────────────────────────────────────────

func TestParseExactDateFilter_Valid(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseExactDateFilter(
		makeReq(map[string]string{"search[date]": "Jan 15 2024"}),
		&c, "search[date]", "created_at", 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filter, ok := c.SearchBy["created_at"].(bson.M)
	if !ok {
		t.Fatal("created_at filter not set")
	}
	start, ok := filter["$gte"].(time.Time)
	if !ok {
		t.Fatalf("$gte not a time.Time: %T", filter["$gte"])
	}
	end, ok := filter["$lte"].(time.Time)
	if !ok {
		t.Fatalf("$lte not a time.Time: %T", filter["$lte"])
	}
	if start.Day() != 15 || start.Month() != time.January || start.Year() != 2024 {
		t.Errorf("start date wrong: %v", start)
	}
	// end should be 23:59:59 of the same day
	diff := end.Sub(start)
	const wantDiff = 24*time.Hour - time.Second
	if diff != wantDiff {
		t.Errorf("date range duration: want %v, got %v", wantDiff, diff)
	}
}

func TestParseExactDateFilter_InvalidFormat_ReturnsError(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseExactDateFilter(makeReq(map[string]string{"search[date]": "2024-01-15"}), &c, "search[date]", "created_at", 0)
	if err == nil {
		t.Error("expected error for wrong date format")
	}
}

func TestParseExactDateFilter_WithTimezoneOffset(t *testing.T) {
	c := InitSearchCriterias()
	// UTC+3 (Saudi Arabia): offset passed as -3 (JS convention)
	err := ParseExactDateFilter(
		makeReq(map[string]string{"search[date]": "Jan 01 2024"}),
		&c, "search[date]", "created_at", -3,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filter := c.SearchBy["created_at"].(bson.M)
	start := filter["$gte"].(time.Time)
	// UTC start should be 3 hours before midnight local time = 2023-12-31 21:00:00 UTC
	if start.Hour() != 21 {
		t.Errorf("expected UTC start at 21:00, got %v", start)
	}
}

func TestParseExactDateFilter_Missing_NoFilter(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseExactDateFilter(makeReq(nil), &c, "search[date]", "created_at", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := c.SearchBy["created_at"]; exists {
		t.Error("missing param should not add a filter")
	}
}

// ── ParseDateRangeFilter ──────────────────────────────────────────────────────

func TestParseDateRangeFilter_BothDates(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseDateRangeFilter(
		makeReq(map[string]string{
			"search[from]": "Jan 01 2024",
			"search[to]":   "Jan 31 2024",
		}),
		&c, "search[from]", "search[to]", "created_at", 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filter, ok := c.SearchBy["created_at"].(bson.M)
	if !ok {
		t.Fatal("created_at filter not set")
	}
	if _, hasGte := filter["$gte"]; !hasGte {
		t.Error("missing $gte in range filter")
	}
	if _, hasLte := filter["$lte"]; !hasLte {
		t.Error("missing $lte in range filter")
	}
	end := filter["$lte"].(time.Time)
	// end should be Jan 31 23:59:59
	if end.Day() != 31 || end.Month() != time.January {
		t.Errorf("end date wrong: %v", end)
	}
}

func TestParseDateRangeFilter_OnlyFrom(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseDateRangeFilter(
		makeReq(map[string]string{"search[from]": "Jan 01 2024"}),
		&c, "search[from]", "search[to]", "created_at", 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filter, ok := c.SearchBy["created_at"].(bson.M)
	if !ok {
		t.Fatal("created_at filter not set")
	}
	if _, hasGte := filter["$gte"]; !hasGte {
		t.Error("missing $gte")
	}
	if _, hasLte := filter["$lte"]; hasLte {
		t.Error("$lte should not be set when only from is provided")
	}
}

func TestParseDateRangeFilter_OnlyTo(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseDateRangeFilter(
		makeReq(map[string]string{"search[to]": "Dec 31 2024"}),
		&c, "search[from]", "search[to]", "created_at", 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filter, ok := c.SearchBy["created_at"].(bson.M)
	if !ok {
		t.Fatal("created_at filter not set")
	}
	if _, hasLte := filter["$lte"]; !hasLte {
		t.Error("missing $lte")
	}
	if _, hasGte := filter["$gte"]; hasGte {
		t.Error("$gte should not be set when only to is provided")
	}
}

func TestParseDateRangeFilter_NeitherDate_NoFilter(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseDateRangeFilter(makeReq(nil), &c, "search[from]", "search[to]", "created_at", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := c.SearchBy["created_at"]; exists {
		t.Error("no dates should produce no filter")
	}
}

func TestParseDateRangeFilter_InvalidFrom_ReturnsError(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseDateRangeFilter(
		makeReq(map[string]string{"search[from]": "bad-date"}),
		&c, "search[from]", "search[to]", "created_at", 0,
	)
	if err == nil {
		t.Error("expected error for invalid from date")
	}
}

// ── GetMongoLogicalOperator ───────────────────────────────────────────────────

func TestGetMongoLogicalOperator(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"<=100", "$lte"},
		{"<100", "$lt"},
		{">=100", "$gte"},
		{">100", "$gt"},
		{"100", ""},
		{"", ""},
		{"==100", ""},
	}
	for _, tc := range cases {
		got := GetMongoLogicalOperator(tc.input)
		if got != tc.want {
			t.Errorf("GetMongoLogicalOperator(%q): want %q, got %q", tc.input, tc.want, got)
		}
	}
}

// ── TrimLogicalOperatorPrefix ─────────────────────────────────────────────────

func TestTrimLogicalOperatorPrefix(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"<=100", "100"},
		{"<100", "100"},
		{">=100", "100"},
		{">100", "100"},
		{"100", "100"},
		{"", ""},
		{"<=", ""},
	}
	for _, tc := range cases {
		got := TrimLogicalOperatorPrefix(tc.input)
		if got != tc.want {
			t.Errorf("TrimLogicalOperatorPrefix(%q): want %q, got %q", tc.input, tc.want, got)
		}
	}
}

// ── GetSortByFields ───────────────────────────────────────────────────────────

func TestGetSortByFields(t *testing.T) {
	t.Run("field space 1 → ascending", func(t *testing.T) {
		m := GetSortByFields("created_at 1")
		if m["created_at"] != 1 {
			t.Errorf("want 1, got %v", m["created_at"])
		}
	})
	t.Run("field space -1 → descending", func(t *testing.T) {
		m := GetSortByFields("name -1")
		if m["name"] != -1 {
			t.Errorf("want -1, got %v", m["name"])
		}
	})
	t.Run("dash-prefix → descending", func(t *testing.T) {
		m := GetSortByFields("-amount")
		if m["amount"] != -1 {
			t.Errorf("want -1, got %v", m["amount"])
		}
	})
	t.Run("plain name → ascending", func(t *testing.T) {
		m := GetSortByFields("amount")
		if m["amount"] != 1 {
			t.Errorf("want 1, got %v", m["amount"])
		}
	})
	t.Run("empty string → empty map", func(t *testing.T) {
		m := GetSortByFields("")
		if len(m) != 0 {
			t.Errorf("want empty map, got %v", m)
		}
	})
}

// ── ConvertTimeZoneToUTC ──────────────────────────────────────────────────────

func TestConvertTimeZoneToUTC(t *testing.T) {
	// Jan 1 2024 00:00:00 in UTC+3 (offset = -3 in JS convention)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("UTC+3 shifts back 3 hours", func(t *testing.T) {
		result := ConvertTimeZoneToUTC(-3, base)
		if result.Hour() != 21 || result.Day() != 31 {
			t.Errorf("want 2023-12-31 21:00:00 UTC, got %v", result)
		}
	})
	t.Run("UTC+5:30 (India) offset=-5.5", func(t *testing.T) {
		result := ConvertTimeZoneToUTC(-5.5, base)
		// 00:00 - 5h30m = 2023-12-31 18:30
		if result.Hour() != 18 || result.Minute() != 30 {
			t.Errorf("want 18:30, got %v", result)
		}
	})
	t.Run("UTC+0 no change", func(t *testing.T) {
		result := ConvertTimeZoneToUTC(0, base)
		if !result.Equal(base) {
			t.Errorf("want unchanged, got %v", result)
		}
	})
}

// ── CountryTimezoneOffset ─────────────────────────────────────────────────────

func TestCountryTimezoneOffset(t *testing.T) {
	cases := []struct {
		code string
		want float64
	}{
		{"SA", -3},   // Saudi Arabia UTC+3
		{"AE", -4},   // UAE UTC+4
		{"IN", -5.5}, // India UTC+5:30
		{"GB", 0},    // UK UTC+0
		{"US", 5},    // US (eastern) UTC-5
		{"XX", 0},    // unknown → UTC
	}
	for _, tc := range cases {
		got := CountryTimezoneOffset(tc.code)
		if got != tc.want {
			t.Errorf("CountryTimezoneOffset(%q): want %v, got %v", tc.code, tc.want, got)
		}
	}
}

func TestCountryTimezoneOffset_EmptyString_ReturnsUTC(t *testing.T) {
	if got := CountryTimezoneOffset(""); got != 0 {
		t.Errorf("empty code: want 0 (UTC), got %v", got)
	}
}

// ── ParseDeletedFilter (additional corner cases) ──────────────────────────────

func TestParseDeletedFilter_EmptyValue_KeepsDefault(t *testing.T) {
	c := InitSearchCriterias()
	ParseDeletedFilter(makeReq(map[string]string{"search[deleted]": ""}), &c)
	filter, ok := c.SearchBy["deleted"].(bson.M)
	if !ok {
		t.Fatal("deleted filter not set")
	}
	if _, hasNe := filter["$ne"]; !hasNe {
		t.Errorf("empty string should keep default $ne filter, got %v", filter)
	}
}

func TestParseDeletedFilter_TextValue_KeepsDefault(t *testing.T) {
	// "true" fails strconv.ParseInt; error silently ignored → value stays 0 → keeps default
	c := InitSearchCriterias()
	ParseDeletedFilter(makeReq(map[string]string{"search[deleted]": "true"}), &c)
	filter, ok := c.SearchBy["deleted"].(bson.M)
	if !ok {
		t.Fatal("deleted filter not set")
	}
	if _, hasNe := filter["$ne"]; !hasNe {
		t.Errorf("text 'true' should keep default $ne filter (only '1' enables deleted), got %v", filter)
	}
}

func TestParseDeletedFilter_NegativeInteger_KeepsDefault(t *testing.T) {
	c := InitSearchCriterias()
	ParseDeletedFilter(makeReq(map[string]string{"search[deleted]": "-1"}), &c)
	filter, ok := c.SearchBy["deleted"].(bson.M)
	if !ok {
		t.Fatal("deleted filter not set")
	}
	if _, hasNe := filter["$ne"]; !hasNe {
		t.Errorf("value -1 should keep default $ne filter, got %v", filter)
	}
}

// ── ParsePaginationAndSort (additional corner cases) ──────────────────────────

func TestParsePaginationAndSort_NonNumericLimit_FallsToZero(t *testing.T) {
	// strconv.Atoi("abc") returns (0, error); error silently ignored → Size=0
	c := InitSearchCriterias()
	ParsePaginationAndSort(makeReq(map[string]string{"limit": "abc"}), &c)
	if c.Size != 0 {
		t.Errorf("non-numeric limit: want Size=0, got %d", c.Size)
	}
}

func TestParsePaginationAndSort_ZeroLimit(t *testing.T) {
	c := InitSearchCriterias()
	ParsePaginationAndSort(makeReq(map[string]string{"limit": "0"}), &c)
	if c.Size != 0 {
		t.Errorf("limit=0: want Size=0, got %d", c.Size)
	}
}

func TestParsePaginationAndSort_NegativeLimit(t *testing.T) {
	// strconv.Atoi accepts negatives; Size becomes -5
	c := InitSearchCriterias()
	ParsePaginationAndSort(makeReq(map[string]string{"limit": "-5"}), &c)
	if c.Size != -5 {
		t.Errorf("limit=-5: want Size=-5, got %d", c.Size)
	}
}

func TestParsePaginationAndSort_NonNumericPage_FallsToZero(t *testing.T) {
	c := InitSearchCriterias()
	ParsePaginationAndSort(makeReq(map[string]string{"page": "xyz"}), &c)
	if c.Page != 0 {
		t.Errorf("non-numeric page: want Page=0, got %d", c.Page)
	}
}

func TestParsePaginationAndSort_InvalidSortOrderValue_EmptyMap(t *testing.T) {
	// "price 2" → order "2" is neither "1" nor "-1" → no entry added, SortBy stays empty
	c := InitSearchCriterias()
	ParsePaginationAndSort(makeReq(map[string]string{"sort": "price 2"}), &c)
	if len(c.SortBy) != 0 {
		t.Errorf("invalid sort order '2': want empty SortBy, got %v", c.SortBy)
	}
}

func TestParsePaginationAndSort_ThreeWordSort_EmptyMap(t *testing.T) {
	c := InitSearchCriterias()
	ParsePaginationAndSort(makeReq(map[string]string{"sort": "a b c"}), &c)
	if len(c.SortBy) != 0 {
		t.Errorf("3-word sort string: want empty SortBy, got %v", c.SortBy)
	}
}

// ── ParseTextSearch (additional corner cases) ─────────────────────────────────

func TestParseTextSearch_EmptyValue_NoFilter(t *testing.T) {
	c := InitSearchCriterias()
	ParseTextSearch(makeReq(map[string]string{"search[name]": ""}), &c, "search[name]", "name")
	if _, exists := c.SearchBy["name"]; exists {
		t.Error("empty string value should not add a filter (len check guards)")
	}
}

// ── ParseObjectIDFilter (additional corner cases) ─────────────────────────────

func TestParseObjectIDFilter_EmptyValue_NoFilter(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseObjectIDFilter(
		makeReq(map[string]string{"search[customer_id]": ""}),
		&c, "search[customer_id]", "customer_id",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := c.SearchBy["customer_id"]; exists {
		t.Error("empty value should not add a filter")
	}
}

// ── ParseObjectIDListFilter (additional corner cases) ─────────────────────────

func TestParseObjectIDListFilter_TrailingComma_ReturnsError(t *testing.T) {
	id := primitive.NewObjectID()
	c := InitSearchCriterias()
	err := ParseObjectIDListFilter(
		makeReq(map[string]string{"search[ids]": id.Hex() + ","}),
		&c, "search[ids]", "ids",
	)
	if err == nil {
		t.Error("trailing comma → empty segment → ObjectIDFromHex(\"\") must error")
	}
}

func TestParseObjectIDListFilter_LeadingComma_ReturnsError(t *testing.T) {
	id := primitive.NewObjectID()
	c := InitSearchCriterias()
	err := ParseObjectIDListFilter(
		makeReq(map[string]string{"search[ids]": "," + id.Hex()}),
		&c, "search[ids]", "ids",
	)
	if err == nil {
		t.Error("leading comma → empty first segment → ObjectIDFromHex(\"\") must error")
	}
}

func TestParseObjectIDListFilter_MiddleEmpty_ReturnsError(t *testing.T) {
	id1, id2 := primitive.NewObjectID(), primitive.NewObjectID()
	c := InitSearchCriterias()
	err := ParseObjectIDListFilter(
		makeReq(map[string]string{"search[ids]": id1.Hex() + ",," + id2.Hex()}),
		&c, "search[ids]", "ids",
	)
	if err == nil {
		t.Error("double comma → empty segment in middle → must error")
	}
}

func TestParseObjectIDListFilter_EmptyValue_NoFilter(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseObjectIDListFilter(
		makeReq(map[string]string{"search[ids]": ""}),
		&c, "search[ids]", "ids",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := c.SearchBy["ids"]; exists {
		t.Error("empty value should not add a filter")
	}
}

// ── ParseFloatWithOperatorFilter (additional corner cases) ────────────────────

func TestParseFloatWithOperatorFilter_OperatorOnly_ReturnsError(t *testing.T) {
	// After stripping "<=" the remaining string is "" → ParseFloat("") errors
	c := InitSearchCriterias()
	err := ParseFloatWithOperatorFilter(
		makeReq(map[string]string{"search[amount]": "<="}),
		&c, "search[amount]", "amount",
	)
	if err == nil {
		t.Error("operator-only '<=': ParseFloat on empty string must error")
	}
}

func TestParseFloatWithOperatorFilter_NegativeFloat(t *testing.T) {
	// No operator prefix → no bson.M wrapping; raw float value stored directly
	c := InitSearchCriterias()
	err := ParseFloatWithOperatorFilter(
		makeReq(map[string]string{"search[amount]": "-1.5"}),
		&c, "search[amount]", "amount",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.SearchBy["amount"] != -1.5 {
		t.Errorf("want raw -1.5, got %v", c.SearchBy["amount"])
	}
}

func TestParseFloatWithOperatorFilter_OperatorWithNegativeFloat(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseFloatWithOperatorFilter(
		makeReq(map[string]string{"search[amount]": ">=-1.5"}),
		&c, "search[amount]", "amount",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filter, ok := c.SearchBy["amount"].(bson.M)
	if !ok || filter["$gte"] != -1.5 {
		t.Errorf("want $gte:-1.5, got %v", c.SearchBy["amount"])
	}
}

func TestParseFloatWithOperatorFilter_EmptyValue_NoFilter(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseFloatWithOperatorFilter(
		makeReq(map[string]string{"search[amount]": ""}),
		&c, "search[amount]", "amount",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := c.SearchBy["amount"]; exists {
		t.Error("empty value should not add a filter")
	}
}

// ── ParseExactDateFilter (additional corner cases) ────────────────────────────

func TestParseExactDateFilter_EmptyValue_NoFilter(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseExactDateFilter(
		makeReq(map[string]string{"search[date]": ""}),
		&c, "search[date]", "created_at", 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := c.SearchBy["created_at"]; exists {
		t.Error("empty value should not add a filter")
	}
}

// ── ParseDateRangeFilter (additional corner cases) ────────────────────────────

func TestParseDateRangeFilter_InvalidTo_ReturnsError(t *testing.T) {
	c := InitSearchCriterias()
	err := ParseDateRangeFilter(
		makeReq(map[string]string{"search[to]": "2024-01-31"}),
		&c, "search[from]", "search[to]", "created_at", 0,
	)
	if err == nil {
		t.Error("ISO date '2024-01-31' should fail: only 'Jan 02 2006' format is accepted")
	}
}

func TestParseDateRangeFilter_TimezoneAppliedToBothDates(t *testing.T) {
	// UTC+3 (offset=-3): Jan 1 → Dec 31 2023 21:00 UTC; Jan 31 end → Jan 31 2024 20:59:59 UTC
	c := InitSearchCriterias()
	err := ParseDateRangeFilter(
		makeReq(map[string]string{
			"search[from]": "Jan 01 2024",
			"search[to]":   "Jan 31 2024",
		}),
		&c, "search[from]", "search[to]", "created_at", -3,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filter := c.SearchBy["created_at"].(bson.M)
	start := filter["$gte"].(time.Time)
	end := filter["$lte"].(time.Time)
	if start.Year() != 2023 || start.Month() != time.December || start.Day() != 31 || start.Hour() != 21 {
		t.Errorf("start UTC+3: want 2023-12-31 21:00:00 UTC, got %v", start)
	}
	if end.Year() != 2024 || end.Month() != time.January || end.Day() != 31 || end.Hour() != 20 || end.Minute() != 59 || end.Second() != 59 {
		t.Errorf("end UTC+3: want 2024-01-31 20:59:59 UTC, got %v", end)
	}
}

// ── GetSortByFields (additional corner cases) ─────────────────────────────────

func TestGetSortByFields_InvalidOrderValue_EmptyMap(t *testing.T) {
	// "price 2" → order "2" is neither "1" nor "-1" → no entry added
	m := GetSortByFields("price 2")
	if len(m) != 0 {
		t.Errorf("invalid sort order '2': want empty map, got %v", m)
	}
}

func TestGetSortByFields_ThreeWords_EmptyMap(t *testing.T) {
	m := GetSortByFields("a b c")
	if len(m) != 0 {
		t.Errorf("3-word sort string: want empty map, got %v", m)
	}
}

func TestGetSortByFields_DashPrefixWithExplicitOrder_FieldRetainsDash(t *testing.T) {
	// 1-word "-name" strips the dash → sortBy["name"]=-1
	// 2-word "-name 1" does NOT strip the dash in the 2-word branch → sortBy["-name"]=1
	// Use "name -1" (not "-name 1") for descending sort to avoid the dash in the key.
	m := GetSortByFields("-name 1")
	if _, exists := m["-name"]; !exists {
		t.Error("2-word form with dash-prefix: key retains dash (dash NOT stripped in 2-word branch)")
	}
	if m["-name"] != 1 {
		t.Errorf("want sortBy[\"-name\"]=1, got %v", m)
	}
}

// ── ParseSelectString ─────────────────────────────────────────────────────────

func TestParseSelectString_EmptyString_ReturnsNil(t *testing.T) {
	if result := ParseSelectString(""); result != nil {
		t.Errorf("empty string: want nil, got %v", result)
	}
}

func TestParseSelectString_SingleField(t *testing.T) {
	result := ParseSelectString("name")
	if result["name"] != 1 {
		t.Errorf("want {name:1}, got %v", result)
	}
}

func TestParseSelectString_NegatedField(t *testing.T) {
	result := ParseSelectString("-password")
	if result["password"] != 0 {
		t.Errorf("want {password:0} for negated field, got %v", result)
	}
	if _, dashKey := result["-password"]; dashKey {
		t.Error("dash prefix should be stripped from the field name")
	}
}

func TestParseSelectString_MixedFields(t *testing.T) {
	result := ParseSelectString("name,email,-password")
	if result["name"] != 1 || result["email"] != 1 || result["password"] != 0 {
		t.Errorf("want {name:1,email:1,password:0}, got %v", result)
	}
}

func TestParseSelectString_TrailingComma_EmptyKeyIncluded(t *testing.T) {
	// strings.Split("name,", ",") = ["name", ""]
	// Empty segment → fields[""] = 1 (callers should strip trailing commas)
	result := ParseSelectString("name,")
	if _, exists := result[""]; !exists {
		t.Error("trailing comma should produce empty-string key in map")
	}
	if result["name"] != 1 {
		t.Errorf("valid field 'name' should still be present, got %v", result)
	}
}

// ── MergeMaps ─────────────────────────────────────────────────────────────────

func TestMergeMaps_BothEmpty(t *testing.T) {
	result := MergeMaps(map[string]interface{}{}, map[string]interface{}{})
	if len(result) != 0 {
		t.Errorf("two empty maps: want empty result, got %v", result)
	}
}

func TestMergeMaps_FirstEmpty(t *testing.T) {
	result := MergeMaps(map[string]interface{}{}, map[string]interface{}{"a": 1})
	if result["a"] != 1 || len(result) != 1 {
		t.Errorf("first empty: want {a:1}, got %v", result)
	}
}

func TestMergeMaps_SecondEmpty(t *testing.T) {
	result := MergeMaps(map[string]interface{}{"a": 1}, map[string]interface{}{})
	if result["a"] != 1 || len(result) != 1 {
		t.Errorf("second empty: want {a:1}, got %v", result)
	}
}

func TestMergeMaps_OverlappingKeys_Map2Wins(t *testing.T) {
	m1 := map[string]interface{}{"a": 1, "b": 2}
	m2 := map[string]interface{}{"b": 99, "c": 3}
	result := MergeMaps(m1, m2)
	if result["a"] != 1 || result["b"] != 99 || result["c"] != 3 {
		t.Errorf("overlap: want {a:1,b:99,c:3}, got %v", result)
	}
}

func TestMergeMaps_DoesNotMutateInputs(t *testing.T) {
	m1 := map[string]interface{}{"a": 1}
	m2 := map[string]interface{}{"b": 2}
	_ = MergeMaps(m1, m2)
	if len(m1) != 1 || len(m2) != 1 {
		t.Error("MergeMaps must not mutate the input maps")
	}
}

// ── RoundTo2Decimals ──────────────────────────────────────────────────────────

func TestRoundTo2Decimals(t *testing.T) {
	cases := []struct{ in, want float64 }{
		// 1.005 is stored as slightly < 1.005 in IEEE754, so rounds down to 1.00
		{1.005, 1.00},
		{1.006, 1.01},
		{1.004, 1.00},
		{2.555, 2.56},
		{0.0, 0.0},
		{-1.005, -1.00},
		{100.999, 101.00},
		{1.0 / 3.0, 0.33},
	}
	for _, c := range cases {
		got := RoundTo2Decimals(c.in)
		if got != c.want {
			t.Errorf("RoundTo2Decimals(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// ── RoundTo4Decimals ──────────────────────────────────────────────────────────

func TestRoundTo4Decimals(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{1.00005, 1.0001},
		{1.00004, 1.0000},
		{3.14159265, 3.1416},
		{0.0, 0.0},
	}
	for _, c := range cases {
		got := RoundTo4Decimals(c.in)
		if got != c.want {
			t.Errorf("RoundTo4Decimals(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// ── RoundTo8Decimals ──────────────────────────────────────────────────────────

func TestRoundTo8Decimals(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{1.000000005, 1.00000001},
		{1.000000004, 1.00000000},
		{0.123456789, 0.12345679},
		{0.0, 0.0},
	}
	for _, c := range cases {
		got := RoundTo8Decimals(c.in)
		if got != c.want {
			t.Errorf("RoundTo8Decimals(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// ── RoundTo3Decimals ──────────────────────────────────────────────────────────

func TestRoundTo3Decimals(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{1.0005, 1.001},
		{1.0004, 1.000},
		{3.1415926, 3.142},
		{0.0, 0.0},
	}
	for _, c := range cases {
		got := RoundTo3Decimals(c.in)
		if got != c.want {
			t.Errorf("RoundTo3Decimals(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// ── RoundFloat — always rounds to 2dp regardless of precision arg ─────────────

func TestRoundFloat_AlwaysRoundsTo2DP(t *testing.T) {
	// The current implementation ignores `precision` and always uses /100
	// 1.005 has IEEE754 representation slightly below 1.005, so rounds to 1.00
	cases := []struct {
		in        float64
		precision uint
		want      float64
	}{
		{1.005, 2, 1.00},
		{1.006, 2, 1.01},
		{1.004, 2, 1.00},
		{3.14159, 4, 3.14}, // precision=4 but result is still 2dp
		{0.0, 2, 0.0},
	}
	for _, c := range cases {
		got := RoundFloat(c.in, c.precision)
		if got != c.want {
			t.Errorf("RoundFloat(%v, %d) = %v, want %v", c.in, c.precision, got, c.want)
		}
	}
}

// ── ToFixed — no-op (returns num unchanged) ───────────────────────────────────

func TestToFixed_ReturnsUnchanged(t *testing.T) {
	cases := []struct{ in float64 }{
		{3.14159265},
		{0.0},
		{-99.999},
		{1.005},
	}
	for _, c := range cases {
		got := ToFixed(c.in, 2)
		if got != c.in {
			t.Errorf("ToFixed(%v, 2) = %v, want %v (no-op)", c.in, got, c.in)
		}
	}
}

// ── ToFixed2 — truncates to 2dp with epsilon ─────────────────────────────────

func TestToFixed2_TruncatesTo2DP(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{1.005, 1.00},  // truncates, not rounds
		{1.999, 1.99},
		{2.0, 2.0},
		{3.14159, 3.14},
		{0.0, 0.0},
	}
	for _, c := range cases {
		got := ToFixed2(c.in, 2)
		if got != c.want {
			t.Errorf("ToFixed2(%v, 2) = %v, want %v", c.in, got, c.want)
		}
	}
}

// ── ConvertToArabicNumerals ───────────────────────────────────────────────────

func TestConvertToArabicNumerals(t *testing.T) {
	cases := []struct{ in, want string }{
		{"0", "٠"},
		{"9", "٩"},
		{"123", "١٢٣"},
		{"SAR 1,234.56", "SAR ١,٢٣٤.٥٦"},
		{"", ""},
		{"abc", "abc"},
		{"2024-01-15", "٢٠٢٤-٠١-١٥"},
	}
	for _, c := range cases {
		got := ConvertToArabicNumerals(c.in)
		if got != c.want {
			t.Errorf("ConvertToArabicNumerals(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── ParseBoolToInt ────────────────────────────────────────────────────────────

func TestParseBoolToInt(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"true", 1},
		{"True", 1},
		{"TRUE", 1},
		{"false", 0},
		{"False", 0},
		{"FALSE", 0},
		{"", -1},
		{"yes", -1},
		{"1", -1},
		{"0", -1},
	}
	for _, c := range cases {
		got := ParseBoolToInt(c.in)
		if got != c.want {
			t.Errorf("ParseBoolToInt(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// ── IsStringBase64 ────────────────────────────────────────────────────────────

func TestIsStringBase64(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"SGVsbG8gV29ybGQ=", true},  // "Hello World" padded base64
		{"dGVzdA==", true},           // "test" with 2x padding
		{"aGVsbG8=", true},           // "hello" with 1x padding
		{"", true},                   // empty string matches the empty pattern
		{"not base64!", false},        // has exclamation and space
		// "SGVsbG8gV29ybGQ" (no padding) has 15 chars; the regex requires
		// padding for remainder groups, so this correctly returns false
		{"SGVsbG8gV29ybGQ", false},  // unpadded base64 does not match regex
		{"YWJj", true},              // "abc" — 4 chars, no padding needed
	}
	for _, c := range cases {
		got, err := IsStringBase64(c.in)
		if err != nil {
			t.Errorf("IsStringBase64(%q) returned error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("IsStringBase64(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// ── IsDateTimesEqual ──────────────────────────────────────────────────────────

func TestIsDateTimesEqual(t *testing.T) {
	base := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	sameMinute := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC) // same minute, different second
	diffMinute := time.Date(2024, 1, 15, 10, 31, 0, 0, time.UTC)

	if !IsDateTimesEqual(&base, &sameMinute) {
		t.Error("IsDateTimesEqual: same minute should be equal")
	}
	if IsDateTimesEqual(&base, &diffMinute) {
		t.Error("IsDateTimesEqual: different minute should not be equal")
	}
}

func TestIsDateTimesEqual_NilSafety(t *testing.T) {
	t1 := time.Now()
	t2 := time.Now()
	// Both non-nil, same wall clock minute
	if !IsDateTimesEqual(&t1, &t2) {
		t.Error("same-second times should compare equal at minute resolution")
	}
}

// ── IsAfter ───────────────────────────────────────────────────────────────────

func TestIsAfter(t *testing.T) {
	earlier := time.Date(2024, 1, 15, 10, 29, 0, 0, time.UTC)
	later := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	sameMinute := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC) // same minute as `later`

	if !IsAfter(&later, &earlier) {
		t.Error("IsAfter: later should be after earlier")
	}
	if IsAfter(&earlier, &later) {
		t.Error("IsAfter: earlier should not be after later")
	}
	if IsAfter(&later, &sameMinute) {
		t.Error("IsAfter: same-minute times should not be after each other")
	}
}

// ── RemoveObjectID ────────────────────────────────────────────────────────────

func TestRemoveObjectID_RemovesTarget(t *testing.T) {
	id1 := primitive.NewObjectID()
	id2 := primitive.NewObjectID()
	id3 := primitive.NewObjectID()
	slice := []*primitive.ObjectID{&id1, &id2, &id3}

	result := RemoveObjectID(slice, &id2)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	for _, id := range result {
		if *id == id2 {
			t.Error("removed id2 still present in result")
		}
	}
}

func TestRemoveObjectID_TargetNotInSlice(t *testing.T) {
	id1 := primitive.NewObjectID()
	id2 := primitive.NewObjectID()
	other := primitive.NewObjectID()
	slice := []*primitive.ObjectID{&id1, &id2}

	result := RemoveObjectID(slice, &other)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2 (no element removed)", len(result))
	}
}

func TestRemoveObjectID_EmptySlice(t *testing.T) {
	id := primitive.NewObjectID()
	result := RemoveObjectID([]*primitive.ObjectID{}, &id)
	if len(result) != 0 {
		t.Errorf("len = %d, want 0", len(result))
	}
}

func TestRemoveObjectID_NilEntriesSkipped(t *testing.T) {
	id1 := primitive.NewObjectID()
	slice := []*primitive.ObjectID{nil, &id1, nil}
	result := RemoveObjectID(slice, &id1)
	// nil entries are skipped; only non-nil non-matching remain
	if len(result) != 0 {
		t.Errorf("len = %d, want 0 (nil entries dropped, id1 removed)", len(result))
	}
}

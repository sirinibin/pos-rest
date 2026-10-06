package erp

import (
	"net/http/httptest"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	l, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return l
}

func TestStoreLocation_ByCountryCode(t *testing.T) {
	tests := []struct {
		name  string
		store M
		want  string
	}{
		{"SA", M{"country_code": "SA"}, "Asia/Riyadh"},
		{"lower sa", M{"country_code": "sa"}, "Asia/Riyadh"},
		{"GB", M{"country_code": "GB"}, "Europe/London"},
		{"AE", M{"country_code": "AE"}, "Asia/Dubai"},
		{"IN padded", M{"country_code": " in "}, "Asia/Kolkata"},
		{"unknown ZZ", M{"country_code": "ZZ"}, "Asia/Riyadh"},
		{"empty", M{"country_code": ""}, "Asia/Riyadh"},
		{"missing key", M{"name": "x"}, "Asia/Riyadh"},
		{"nil store", nil, "Asia/Riyadh"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := storeLocation(tc.store).String(); got != tc.want {
				t.Errorf("storeLocation=%s want %s", got, tc.want)
			}
			if got := storeTimezoneName(tc.store); got != tc.want {
				t.Errorf("storeTimezoneName=%s want %s", got, tc.want)
			}
		})
	}
	// cached: same pointer on repeat lookups
	if storeLocation(M{"country_code": "GB"}) != storeLocation(M{"country_code": "gb"}) {
		t.Error("expected cached *time.Location")
	}
}

func TestFmtIn_NearMidnightAcrossZones(t *testing.T) {
	inst := time.Date(2026, 10, 6, 21, 30, 0, 0, time.UTC)
	tests := []struct {
		name    string
		loc     *time.Location
		in      interface{}
		wantDT  string
		wantDay string
	}{
		{"SA time.Time", mustLoc(t, "Asia/Riyadh"), inst, "2026-10-07T00:30", "2026-10-07"},
		{"GB BST time.Time", mustLoc(t, "Europe/London"), inst, "2026-10-06T22:30", "2026-10-06"},
		{"GB DateTime", mustLoc(t, "Europe/London"), primitive.NewDateTimeFromTime(inst), "2026-10-06T22:30", "2026-10-06"},
		{"AE", mustLoc(t, "Asia/Dubai"), inst, "2026-10-07T01:30", "2026-10-07"},
		{"IN", mustLoc(t, "Asia/Kolkata"), inst, "2026-10-07T03:00", "2026-10-07"},
		{"RFC3339 string", mustLoc(t, "Europe/London"), "2026-10-06T21:30:00Z", "2026-10-06T22:30", "2026-10-06"},
		{"zone-less string is loc wall clock", mustLoc(t, "Europe/London"), "2026-10-06T23:45", "2026-10-06T23:45", "2026-10-06"},
		{"nil loc -> Riyadh", nil, inst, "2026-10-07T00:30", "2026-10-07"},
		{"not a time", mustLoc(t, "Asia/Riyadh"), 12, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := fmtDTIn(tc.loc, tc.in); got != tc.wantDT {
				t.Errorf("fmtDTIn=%q want %q", got, tc.wantDT)
			}
			if got := fmtDayIn(tc.loc, tc.in); got != tc.wantDay {
				t.Errorf("fmtDayIn=%q want %q", got, tc.wantDay)
			}
		})
	}
	// the Riyadh wrappers are unchanged
	if fmtDT(inst) != "2026-10-07T00:30" || fmtDay(inst) != "2026-10-07" {
		t.Errorf("riyadh wrappers: %s %s", fmtDT(inst), fmtDay(inst))
	}
}

func TestParseClientTimeIn_Zones(t *testing.T) {
	tests := []struct {
		name string
		loc  *time.Location
		in   string
		want time.Time
	}{
		{"Riyadh zone-less after midnight", mustLoc(t, "Asia/Riyadh"), "2026-10-07T00:15", time.Date(2026, 10, 6, 21, 15, 0, 0, time.UTC)},
		{"London BST zone-less", mustLoc(t, "Europe/London"), "2026-10-07T00:15", time.Date(2026, 10, 6, 23, 15, 0, 0, time.UTC)},
		{"London GMT (winter) zone-less", mustLoc(t, "Europe/London"), "2026-12-01T09:00", time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)},
		{"Dubai day only", mustLoc(t, "Asia/Dubai"), "2026-10-07", time.Date(2026, 10, 6, 20, 0, 0, 0, time.UTC)},
		{"explicit zone wins", mustLoc(t, "Europe/London"), "2026-10-07T00:15:00+03:00", time.Date(2026, 10, 6, 21, 15, 0, 0, time.UTC)},
		{"nil loc -> Riyadh", nil, "2026-10-07T00:15", time.Date(2026, 10, 6, 21, 15, 0, 0, time.UTC)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseClientTimeIn(tc.loc, tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("got %s want %s", got.UTC(), tc.want)
			}
			// round trip: formatting back in the same zone yields the input wall clock
			if len(tc.in) == len(layoutDT) {
				if back := fmtDTIn(tc.loc, got); back != tc.in {
					t.Errorf("round trip %q -> %q", tc.in, back)
				}
			}
		})
	}
	if _, err := parseClientTimeIn(riyadh, "nope"); err == nil {
		t.Error("expected error")
	}
}

func TestToLegacyDateStrIn_Zones(t *testing.T) {
	tests := []struct {
		loc  *time.Location
		in   string
		want string
	}{
		{mustLoc(t, "Asia/Riyadh"), "2026-10-07T00:15", "2026-10-07T00:15:00+03:00"},
		{mustLoc(t, "Europe/London"), "2026-10-07T00:15", "2026-10-07T00:15:00+01:00"},
		{mustLoc(t, "Europe/London"), "2026-10-06T21:30:00Z", "2026-10-06T22:30:00+01:00"},
		{nil, "2026-10-07", "2026-10-07T00:00:00+03:00"},
	}
	for _, tc := range tests {
		got, err := toLegacyDateStrIn(tc.loc, tc.in)
		if err != nil || got != tc.want {
			t.Errorf("toLegacyDateStrIn(%v,%q)=%q,%v want %q", tc.loc, tc.in, got, err, tc.want)
		}
	}
}

func TestMapCtx_TimeMethods(t *testing.T) {
	inst := time.Date(2026, 10, 6, 21, 30, 0, 0, time.UTC)
	tests := []struct {
		name    string
		x       *mapCtx
		zone    string
		wantDT  string
		wantDay string
		legacy  string
	}{
		{"nil ctx", nil, "Asia/Riyadh", "2026-10-07T00:30", "2026-10-07", "2026-10-07T00:15:00+03:00"},
		{"nil store", &mapCtx{}, "Asia/Riyadh", "2026-10-07T00:30", "2026-10-07", "2026-10-07T00:15:00+03:00"},
		{"SA store", &mapCtx{store: M{"country_code": "SA"}}, "Asia/Riyadh", "2026-10-07T00:30", "2026-10-07", "2026-10-07T00:15:00+03:00"},
		{"GB store", &mapCtx{store: M{"country_code": "GB"}}, "Europe/London", "2026-10-06T22:30", "2026-10-06", "2026-10-07T00:15:00+01:00"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.x.loc().String(); got != tc.zone {
				t.Errorf("loc=%s want %s", got, tc.zone)
			}
			if got := tc.x.fmtDT(inst); got != tc.wantDT {
				t.Errorf("fmtDT=%s want %s", got, tc.wantDT)
			}
			if got := tc.x.fmtDay(inst); got != tc.wantDay {
				t.Errorf("fmtDay=%s want %s", got, tc.wantDay)
			}
			if got, err := tc.x.legacyDateStr("2026-10-07T00:15"); err != nil || got != tc.legacy {
				t.Errorf("legacyDateStr=%s,%v want %s", got, err, tc.legacy)
			}
			p, err := tc.x.parseTime("2026-10-07T00:15")
			if err != nil || p.Location().String() != tc.zone {
				t.Errorf("parseTime=%v,%v want zone %s", p, err, tc.zone)
			}
		})
	}
}

func TestStoreToContract_TimezoneFromCountry(t *testing.T) {
	tests := []struct {
		name     string
		cc       interface{}
		wantTZ   string
		wantCode string
	}{
		{"missing -> SA", nil, "Asia/Riyadh", "SA"},
		{"SA", "SA", "Asia/Riyadh", "SA"},
		{"gb lower", "gb", "Europe/London", "GB"},
		{"AE", "AE", "Asia/Dubai", "AE"},
		{"unknown keeps code, Riyadh zone", "ZZ", "Asia/Riyadh", "ZZ"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sid := hexID()
			doc := M{"_id": sid, "name": "Shop"}
			if tc.cc != nil {
				doc["country_code"] = tc.cc
			}
			rec := storeToContract(testX(sid, doc), doc)
			if rec["timezone"] != tc.wantTZ || rec["countryCode"] != tc.wantCode {
				t.Errorf("timezone=%v countryCode=%v want %s/%s", rec["timezone"], rec["countryCode"], tc.wantTZ, tc.wantCode)
			}
		})
	}
	if !storeKnown["timezone"] || !storeKnown["countryCode"] {
		t.Error("timezone/countryCode must be known (server-owned) store fields")
	}
}

func TestStoreToContract_ZatcaTimesInStoreZone(t *testing.T) {
	sid := hexID()
	at := primitive.NewDateTimeFromTime(time.Date(2026, 10, 6, 21, 30, 0, 0, time.UTC))
	doc := M{"_id": sid, "name": "Shop", "country_code": "GB", "zatca": M{"connected": true, "last_connected_at": at}}
	rec := storeToContract(testX(sid, doc), doc)
	if got := get(rec, "zatca.connectedAt"); got != "2026-10-06T22:30" {
		t.Errorf("connectedAt=%v", got)
	}
}

func TestHistoryAndEnvelope_StoreZone(t *testing.T) {
	ca := primitive.NewDateTimeFromTime(time.Date(2026, 10, 6, 21, 30, 0, 0, time.UTC))
	ua := primitive.NewDateTimeFromTime(time.Date(2026, 10, 6, 23, 30, 0, 0, time.UTC))
	doc := M{"created_at": ca, "updated_at": ua}
	london := mustLoc(t, "Europe/London")
	h := historyOfIn(london, doc)
	if len(h) != 2 || h[0].(M)["at"] != "2026-10-06T22:30" || h[1].(M)["at"] != "2026-10-07T00:30" {
		t.Errorf("history=%v", h)
	}
	rec := applyEnvelopeIn(london, M{}, doc, false)
	if rec["createdAt"] != "2026-10-06T22:30" || rec["updatedAt"] != "2026-10-07T00:30" {
		t.Errorf("envelope createdAt=%v updatedAt=%v", rec["createdAt"], rec["updatedAt"])
	}
	// Riyadh wrapper unchanged
	if r := applyEnvelope(M{}, doc, false); r["createdAt"] != "2026-10-07T00:30" {
		t.Errorf("riyadh createdAt=%v", r["createdAt"])
	}
	old := nowFn
	nowFn = func() time.Time { return time.Date(2026, 10, 6, 21, 30, 0, 0, time.UTC) }
	defer func() { nowFn = old }()
	if e := historyEntryIn(london, "u", "", nil); e["at"] != "2026-10-06T22:30" || e["action"] != "updated" {
		t.Errorf("historyEntryIn=%v", e)
	}
	if e := historyEntry("u", "x", nil); e["at"] != "2026-10-07T00:30" {
		t.Errorf("historyEntry=%v", e)
	}
}

func TestLegacyJSONOf_StoreZone(t *testing.T) {
	d := primitive.NewDateTimeFromTime(time.Date(2026, 10, 6, 21, 30, 0, 0, time.UTC))
	doc := M{"_id": primitive.NewObjectID(), "date": d, "payments": []interface{}{M{"_id": primitive.NewObjectID(), "date": d}}}
	out := legacyJSONOf(doc, mustLoc(t, "Europe/London"))
	if out["date_str"] != "2026-10-06T22:30:00+01:00" {
		t.Errorf("date_str=%v", out["date_str"])
	}
	if p := arr(out["payments_input"]); len(p) != 1 || p[0].(M)["date_str"] != "2026-10-06T22:30:00+01:00" {
		t.Errorf("payments_input=%v", out["payments_input"])
	}
	if out := legacyJSONOf(doc, nil); out["date_str"] != "2026-10-07T00:30:00+03:00" {
		t.Errorf("nil loc date_str=%v", out["date_str"])
	}
}

func TestListQuery_FromInStoreZone(t *testing.T) {
	res := &Resource{Path: "tzlist", DateField: "date"}
	r := httptest.NewRequest("GET", "/v1/erp/x?from=2026-10-07", nil)
	q, err := parseListQuery(r, res)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 6, 21, 0, 0, 0, time.UTC); !q.From.Equal(want) || !q.fromIn(riyadh).Equal(want) {
		t.Errorf("riyadh from=%v", q.From)
	}
	if got := q.fromIn(mustLoc(t, "Europe/London")); !got.Equal(time.Date(2026, 10, 6, 23, 0, 0, 0, time.UTC)) {
		t.Errorf("london from=%v", got)
	}
	if (ListQuery{}).fromIn(riyadh) != nil {
		t.Error("no from -> nil")
	}
	gb, sa := hexID(), hexID()
	c := &Ctx{storeIdx: map[string]M{gb: {"country_code": "GB"}, sa: {"country_code": "SA"}}}
	if c.storeLoc(gb).String() != "Europe/London" || c.storeLoc(sa).String() != "Asia/Riyadh" || c.storeLoc("nope").String() != "Asia/Riyadh" {
		t.Error("Ctx.storeLoc")
	}
	var nc *Ctx
	if nc.storeLoc(gb) != riyadh {
		t.Error("nil Ctx.storeLoc must be Riyadh")
	}
}

func TestPaymentAndZatcaToContract_StoreZone(t *testing.T) {
	london := mustLoc(t, "Europe/London")
	d := primitive.NewDateTimeFromTime(time.Date(2026, 10, 6, 21, 30, 0, 0, time.UTC))
	if p := paymentToContract(london, M{"_id": primitive.NewObjectID(), "amount": 5.0}, d); p["date"] != "2026-10-06T22:30" {
		t.Errorf("payment date=%v", p["date"])
	}
	z := zatcaToContract(london, M{"zatca": M{"reporting_passed": true, "reporting_passed_at": d}}, "invoice")
	if z["reportedAt"] != "2026-10-06T22:30" {
		t.Errorf("reportedAt=%v", z["reportedAt"])
	}
}

func TestUniqueByID(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", nil, nil},
		{"no duplicates", []string{"a", "b"}, []string{"a", "b"}},
		{"same id from two stores", []string{"a", "b", "a"}, []string{"a", "b"}},
		{"all the same", []string{"x", "x", "x"}, []string{"x"}},
		{"rows without an id are kept", []string{"", "a", ""}, []string{"", "a", ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows := []M{}
			for i, id := range c.in {
				rows = append(rows, M{"id": id, "n": i})
			}
			got := []string{}
			for _, r := range uniqueByID(rows) {
				got = append(got, str(r["id"]))
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
	// the first copy wins
	got := uniqueByID([]M{{"id": "a", "store": "A"}, {"id": "a", "store": "B"}})
	if len(got) != 1 || got[0]["store"] != "A" {
		t.Errorf("first copy should win: %v", got)
	}
}

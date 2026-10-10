package erp

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// DB-backed API + integration test of the registered-company list: the
// one-time seed, admin-only writes, one company per country, select, the
// public footer map and removing a company.
func TestAPI_CompanyRegistry(t *testing.T) {
	requireDB(t)
	admin := login(t, fx.AdminEmail)
	owner, sid := signupOwner(t, "companies")
	defer cleanupBilling(t, sid)

	// start from a clean slate and put the live list back afterwards
	ctx, cancel := dbctx()
	defer cancel()
	var saved []interface{}
	if cur, err := companyRegistry().Find(ctx, bson.M{}); err == nil {
		for cur.Next(ctx) {
			var raw bson.M
			_ = cur.Decode(&raw)
			saved = append(saved, raw)
		}
		cur.Close(ctx)
	}
	var savedSeed bson.M
	seedErr := billingSettings().FindOne(ctx, bson.M{"_id": companyRegistrySeedID}).Decode(&savedSeed)
	reset := func() {
		c2, cl := dbctx()
		defer cl()
		_, _ = companyRegistry().DeleteMany(c2, bson.M{})
		_, _ = billingSettings().DeleteOne(c2, bson.M{"_id": companyRegistrySeedID})
	}
	reset()
	defer func() {
		reset()
		c2, cl := dbctx()
		defer cl()
		if len(saved) > 0 {
			_, _ = companyRegistry().InsertMany(c2, saved)
		}
		if seedErr == nil {
			_, _ = billingSettings().InsertOne(c2, savedSeed)
		}
	}()

	// 1. the first read seeds Gulf Union Ozone for the six GCC countries
	pub := call(t, "GET", "/site/companies", "", nil)
	if pub.Code != 200 || !strings.Contains(pub.Header.Get("Cache-Control"), "max-age") {
		t.Fatalf("public map: %d %s", pub.Code, pub.Raw)
	}
	countries, _ := pub.Body["countries"].(M)
	for _, c := range []string{"SA", "AE", "OM", "QA", "BH", "KW"} {
		if get(countries, c+".nameEn") != "Gulf Union Ozone Co." || get(countries, c+".website") != "https://gulfunionozone.com" {
			t.Fatalf("%s must show the seeded company: %s", c, pub.Raw)
		}
	}
	if countries["IN"] != nil {
		t.Fatalf("India has no company: %s", pub.Raw)
	}
	l := call(t, "GET", "/admin/companies", admin, nil)
	if l.Code != 200 || intv(l.Body["total"]) != 1 {
		t.Fatalf("admin list after seed: %d %s", l.Code, l.Raw)
	}
	gulf := arr(l.Body["items"])[0].(M)
	gulfID := str(gulf["id"])
	if gulf["updatedBy"] != "system" || gulf["registeredIn"] != "SA" {
		t.Fatalf("seeded row: %v", gulf)
	}

	// 2. only platform admins see or change the list; the public map needs no sign-in
	if r := call(t, "GET", "/admin/companies", owner, nil); r.Code != 403 {
		t.Fatalf("owner list: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/admin/companies", owner, validCompany()); r.Code != 403 {
		t.Fatalf("owner create: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "PUT", "/admin/companies/"+gulfID, owner, validCompany()); r.Code != 403 {
		t.Fatalf("owner update: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "DELETE", "/admin/companies/"+gulfID, owner, nil); r.Code != 403 {
		t.Fatalf("owner delete: %d %s", r.Code, r.Raw)
	}

	// 3. validation
	if r := call(t, "POST", "/admin/companies", admin, M{"nameEn": "", "registeredIn": "US", "website": "x"}); r.Code != 400 ||
		r.errField("nameEn") == "" || r.errField("registeredIn") == "" || r.errField("website") == "" {
		t.Fatalf("validation: %d %s", r.Code, r.Raw)
	}

	// 4. a country belongs to one company: a second one asking for UAE is refused
	oman := M{"nameEn": "Oman Ozone LLC", "registeredIn": "OM", "countries": []string{"OM", "AE"}, "website": "oman.example.com"}
	if r := call(t, "POST", "/admin/companies", admin, oman); r.Code != 409 || r.errCode() != "country_taken" ||
		!strings.Contains(r.Raw, "AE (Gulf Union Ozone Co.)") {
		t.Fatalf("taken country: %d %s", r.Code, r.Raw)
	}
	// ... until the first company gives them up
	g2 := cloneM(gulf)
	g2["countries"] = []string{"SA", "QA", "BH", "KW"}
	g2["crNo"] = "4031012345"
	g2["vatNo"] = "300000000000003"
	if r := call(t, "PUT", "/admin/companies/"+gulfID, admin, g2); r.Code != 200 || r.Body["crNo"] != "4031012345" ||
		len(arr(r.Body["countries"])) != 4 || r.Body["updatedBy"] == "system" {
		t.Fatalf("update gulf: %d %s", r.Code, r.Raw)
	}
	cr := call(t, "POST", "/admin/companies", admin, oman)
	if cr.Code != 201 || cr.Body["website"] != "https://oman.example.com" || str(cr.Body["id"]) == "" {
		t.Fatalf("create oman: %d %s", cr.Code, cr.Raw)
	}
	omanID := str(cr.Body["id"])
	// editing a company with its own countries is fine
	oman["nameAr"] = "عمان أوزون"
	if r := call(t, "PUT", "/admin/companies/"+omanID, admin, oman); r.Code != 200 || r.Body["nameAr"] != "عمان أوزون" {
		t.Fatalf("update oman: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "PUT", "/admin/companies/0123456789abcdef01234567", admin, M{"nameEn": "X", "registeredIn": "SA"}); r.Code != 404 {
		t.Fatalf("update missing: %d %s", r.Code, r.Raw)
	}

	// 5. select returns only the fields asked for
	s := call(t, "GET", "/admin/companies?select=nameEn,countries", admin, nil)
	if s.Code != 200 || intv(s.Body["total"]) != 2 {
		t.Fatalf("select list: %d %s", s.Code, s.Raw)
	}
	for _, it := range arr(s.Body["items"]) {
		m := it.(M)
		if m["nameEn"] == nil || m["countries"] == nil || m["crNo"] != nil || m["website"] != nil {
			t.Fatalf("select row: %v", m)
		}
	}
	if r := call(t, "GET", "/admin/companies?select=bad%20name", admin, nil); r.Code != 400 {
		t.Fatalf("bad select: %d %s", r.Code, r.Raw)
	}

	// 6. the public map follows; contact details stay private
	pub = call(t, "GET", "/site/companies", "", nil)
	countries, _ = pub.Body["countries"].(M)
	if get(countries, "AE.nameEn") != "Oman Ozone LLC" || get(countries, "OM.nameAr") != "عمان أوزون" ||
		get(countries, "SA.crNo") != "4031012345" || get(countries, "SA.vatNo") != "300000000000003" {
		t.Fatalf("public map after edits: %s", pub.Raw)
	}
	if strings.Contains(pub.Raw, "updatedBy") || strings.Contains(pub.Raw, `"email"`) {
		t.Fatalf("public map leaks private fields: %s", pub.Raw)
	}

	// 7. removing a company leaves its countries without one; an emptied list is not re-seeded
	if r := call(t, "DELETE", "/admin/companies/"+omanID, admin, nil); r.Code != 200 || r.Body["deleted"] != true {
		t.Fatalf("delete oman: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "DELETE", "/admin/companies/"+omanID, admin, nil); r.Code != 404 {
		t.Fatalf("delete twice: %d %s", r.Code, r.Raw)
	}
	pub = call(t, "GET", "/site/companies", "", nil)
	countries, _ = pub.Body["countries"].(M)
	if countries["AE"] != nil || countries["OM"] != nil || get(countries, "SA.nameEn") != "Gulf Union Ozone Co." {
		t.Fatalf("map after delete: %s", pub.Raw)
	}
	if r := call(t, "DELETE", "/admin/companies/"+gulfID, admin, nil); r.Code != 200 {
		t.Fatalf("delete gulf: %d %s", r.Code, r.Raw)
	}
	pub = call(t, "GET", "/site/companies", "", nil)
	if pub.Code != 200 || len(pub.Body["countries"].(M)) != 0 {
		t.Fatalf("emptied list must stay empty: %s", pub.Raw)
	}
}

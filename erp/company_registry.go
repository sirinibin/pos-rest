package erp

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Company registrations: the legal entities StartERP is sold under.
//
// StartERP itself is not registered anywhere yet; it is launched as a product
// of a registered company (Gulf Union Ozone Co. in Saudi Arabia). Platform
// admins keep a list of such companies, each assigned to the countries it
// covers, and every country's marketing pages say "StartERP is a product of
// <company>" in their footer. A country belongs to at most one company; a
// country with none shows no such line.
//
//	GET    /admin/companies         platform admins: every company (select=)
//	POST   /admin/companies         platform admins: add one
//	PUT    /admin/companies/{id}    platform admins: replace one
//	DELETE /admin/companies/{id}    platform admins: remove one
//	GET    /site/companies          public: country -> company, for the footers
//
// The first read seeds Gulf Union Ozone Co. for the six GCC countries, once:
// a marker in the platform settings keeps a deliberately emptied list empty.

const (
	collCompanyRegistry    = "erp_company_registration"
	companyRegistrySeedID  = "company_registry_seeded"
	companyRegistryMax     = 50
	companyRegistryMaxText = 200
)

// companyFields are the text fields of a company (contract names).
var companyFields = []string{"nameEn", "nameAr", "legalNameEn", "legalNameAr", "crNo", "vatNo",
	"addressEn", "addressAr", "registeredIn", "website", "email", "phone"}

// companyPublicFields are what the public footer endpoint returns (no
// contact person, no audit fields).
var companyPublicFields = []string{"nameEn", "nameAr", "legalNameEn", "legalNameAr", "crNo", "vatNo",
	"addressEn", "addressAr", "registeredIn", "website"}

// DefaultCompany is the seed: the registered entity StartERP launches under.
var DefaultCompany = M{
	"nameEn":       "Gulf Union Ozone Co.",
	"nameAr":       "",
	"legalNameEn":  "",
	"legalNameAr":  "",
	"crNo":         "",
	"vatNo":        "",
	"addressEn":    "Al-Safeeri Street, Al Faisaliyah, Makkah 23442, Saudi Arabia",
	"addressAr":    "شارع الصفيري، الفيصلية، مكة المكرمة 23442، المملكة العربية السعودية",
	"registeredIn": "SA",
	"website":      "https://gulfunionozone.com",
	"email":        "",
	"phone":        "",
	"countries":    []string{"SA", "AE", "OM", "QA", "BH", "KW"},
}

var reCompanyEmail = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
var reCompanyPhone = regexp.MustCompile(`^\+?[0-9 ()-]{6,20}$`)

func companyRegistry() *mongo.Collection { return mainDB().Collection(collCompanyRegistry) }

// knownCountry reports whether code is a country StartERP supports.
func knownCountry(code string) bool { return models.CountryProfileFor(code) != nil && code != "" }

// NormalizeCompanyCountries upper-cases, de-duplicates and sorts in the
// order of models.Countries (SA first), so equal sets compare equal.
func NormalizeCompanyCountries(v interface{}) []string {
	seen := map[string]bool{}
	for _, c := range strs(v) {
		seen[normCountry(c)] = true
	}
	out := []string{}
	for _, p := range models.Countries {
		if seen[p.Code] {
			out = append(out, p.Code)
			delete(seen, p.Code)
		}
	}
	rest := []string{}
	for c := range seen {
		if c != "" {
			rest = append(rest, c)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// NormalizeWebsite trims and adds https:// to a bare domain.
func NormalizeWebsite(s string) string {
	s = strings.TrimSpace(s)
	if s != "" && !strings.Contains(s, "://") {
		s = "https://" + s
	}
	return s
}

func validWebsite(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && strings.Contains(u.Host, ".") &&
		!strings.ContainsAny(u.Host, " <>\"'")
}

// ValidateCompany checks a company body; field -> message.
func ValidateCompany(body M) map[string]string {
	e := map[string]string{}
	if strings.TrimSpace(str(body["nameEn"])) == "" {
		e["nameEn"] = "required"
	}
	reg := normCountry(str(body["registeredIn"]))
	if reg == "" {
		e["registeredIn"] = "required"
	} else if !knownCountry(reg) {
		e["registeredIn"] = "unknown country"
	}
	for _, k := range companyFields {
		max := companyRegistryMaxText
		if k == "addressEn" || k == "addressAr" {
			max = 400
		}
		if len([]rune(str(body[k]))) > max {
			e[k] = fmt.Sprintf("at most %d characters", max)
		}
	}
	if w := NormalizeWebsite(str(body["website"])); w != "" && !validWebsite(w) {
		e["website"] = "a web address like https://example.com"
	}
	if m := strings.TrimSpace(str(body["email"])); m != "" && !reCompanyEmail.MatchString(m) {
		e["email"] = "invalid email"
	}
	if p := strings.TrimSpace(str(body["phone"])); p != "" && !reCompanyPhone.MatchString(p) {
		e["phone"] = "invalid phone number"
	}
	if s := strings.TrimSpace(str(body["crNo"])); s != "" && !regexp.MustCompile(`^[A-Za-z0-9/ -]{1,30}$`).MatchString(s) {
		e["crNo"] = "letters, digits, / and - only"
	}
	if s := strings.TrimSpace(str(body["vatNo"])); s != "" {
		if p := models.CountryProfileFor(reg); p != nil && !p.ValidTaxID(s) {
			e["vatNo"] = taxIDErrorFor(p)
		}
	}
	if v, ok := body["countries"]; ok && v != nil {
		if _, isArr := v.([]interface{}); !isArr {
			if _, isStr := v.([]string); !isStr {
				e["countries"] = "a list of country codes"
			}
		}
	}
	if e["countries"] == "" {
		for _, c := range NormalizeCompanyCountries(body["countries"]) {
			if !knownCountry(c) {
				e["countries"] = "unknown country " + c
				break
			}
		}
	}
	return e
}

// companyDoc is the stored form of a validated body.
func companyDoc(body M) bson.M {
	d := bson.M{}
	for _, k := range companyFields {
		d[k] = strings.TrimSpace(str(body[k]))
	}
	d["registeredIn"] = normCountry(str(body["registeredIn"]))
	d["website"] = NormalizeWebsite(str(body["website"]))
	d["vatNo"] = strings.ReplaceAll(str(d["vatNo"]), " ", "")
	d["countries"] = NormalizeCompanyCountries(body["countries"])
	return d
}

// companyRow is a stored company in contract shape.
func companyRow(d M) M {
	row := M{"id": hexOf(d["_id"])}
	for _, k := range companyFields {
		row[k] = str(d[k])
	}
	row["countries"] = NormalizeCompanyCountries(d["countries"])
	row["createdAt"] = str(d["createdAt"])
	row["updatedAt"] = str(d["updatedAt"])
	row["updatedBy"] = str(d["updatedBy"])
	return row
}

// CountryConflicts lists the countries of want that another company (not
// selfID) already holds: country -> that company's name.
func CountryConflicts(want []string, others []M, selfID string) map[string]string {
	out := map[string]string{}
	for _, o := range others {
		if str(o["id"]) == selfID {
			continue
		}
		for _, c := range strs(o["countries"]) {
			for _, w := range want {
				if c == w {
					out[w] = str(o["nameEn"])
				}
			}
		}
	}
	return out
}

// conflictError is the 409 for countries that belong to another company.
func conflictError(conf map[string]string) error {
	codes := []string{}
	for c := range conf {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	parts := []string{}
	for _, c := range codes {
		parts = append(parts, c+" ("+conf[c]+")")
	}
	msg := "These countries already belong to another company: " + strings.Join(parts, ", ") +
		". Remove them from that company first."
	return errf(http.StatusConflict, "country_taken", msg, map[string]string{"countries": strings.Join(parts, ", ")})
}

// seedCompaniesOnce adds DefaultCompany the first time the list is read.
func seedCompaniesOnce() {
	ctx, cancel := dbctx()
	defer cancel()
	res, err := billingSettings().UpdateOne(ctx, bson.M{"_id": companyRegistrySeedID},
		bson.M{"$setOnInsert": bson.M{"seededAt": nowFn().In(riyadh).Format(layoutDT)}}, options.Update().SetUpsert(true))
	if err != nil || res.UpsertedCount == 0 {
		return
	}
	d := companyDoc(DefaultCompany)
	d["createdAt"] = nowFn().In(riyadh).Format(layoutDT)
	d["updatedAt"] = d["createdAt"]
	d["updatedBy"] = "system"
	_, _ = companyRegistry().InsertOne(ctx, d)
}

// loadCompanies returns every company, oldest first.
func loadCompanies() ([]M, error) {
	seedCompaniesOnce()
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := companyRegistry().Find(ctx, bson.M{}, options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(companyRegistryMax))
	if err != nil {
		return nil, errInternal("db: " + err.Error())
	}
	defer cur.Close(ctx)
	out := []M{}
	for cur.Next(ctx) {
		var raw bson.M
		if err := cur.Decode(&raw); err != nil {
			return nil, errInternal("db: " + err.Error())
		}
		out = append(out, companyRow(normDoc(raw)))
	}
	return out, nil
}

// SiteCompanies is the public map: country -> public fields of its company.
// A company listed for a country twice (should not happen) keeps the first.
func SiteCompanies(companies []M) M {
	countries := M{}
	for _, c := range companies {
		pub := M{}
		for _, k := range companyPublicFields {
			pub[k] = str(c[k])
		}
		for _, code := range strs(c["countries"]) {
			if _, taken := countries[code]; !taken {
				countries[code] = pub
			}
		}
	}
	return countries
}

// GET /site/companies — public (the marketing pages read it).
func handleSiteCompanies(w http.ResponseWriter, r *http.Request) {
	companies, err := loadCompanies()
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, M{"countries": SiteCompanies(companies)})
}

// GET /admin/companies?select=
func handleCompanyList(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	fs, err := parseSelect(r.URL.Query().Get("select"))
	if err != nil {
		return err
	}
	companies, err := loadCompanies()
	if err != nil {
		return err
	}
	items := []M{}
	for _, co := range companies {
		items = append(items, fs.apply(co))
	}
	writeJSON(w, http.StatusOK, M{"items": items, "total": len(items)})
	return nil
}

// saveCompany validates body and writes it (id "" = new).
func saveCompany(c *Ctx, w http.ResponseWriter, r *http.Request, id string) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	body, err := readBody(r)
	if err != nil {
		return err
	}
	if errs := ValidateCompany(body); len(errs) > 0 {
		return errBadRequest("", errs)
	}
	var oid primitive.ObjectID
	if id != "" {
		var ok bool
		if oid, ok = oidOf(id); !ok {
			return errNotFound()
		}
	}
	all, err := loadCompanies()
	if err != nil {
		return err
	}
	if id == "" && len(all) >= companyRegistryMax {
		return errBadRequest(fmt.Sprintf("At most %d companies.", companyRegistryMax), nil)
	}
	doc := companyDoc(body)
	if conf := CountryConflicts(doc["countries"].([]string), all, id); len(conf) > 0 {
		return conflictError(conf)
	}
	now := nowFn().In(riyadh).Format(layoutDT)
	doc["updatedAt"] = now
	doc["updatedBy"] = c.UserName
	ctx, cancel := dbctx()
	defer cancel()
	status := http.StatusOK
	if id == "" {
		doc["createdAt"] = now
		res, err := companyRegistry().InsertOne(ctx, doc)
		if err != nil {
			return errInternal("Unable to save the company.")
		}
		oid, _ = res.InsertedID.(primitive.ObjectID)
		status = http.StatusCreated
	} else {
		res, err := companyRegistry().UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": doc})
		if err != nil {
			return errInternal("Unable to save the company.")
		}
		if res.MatchedCount == 0 {
			return errNotFound()
		}
	}
	var raw bson.M
	if err := companyRegistry().FindOne(ctx, bson.M{"_id": oid}).Decode(&raw); err != nil {
		return errInternal("Unable to read the company back.")
	}
	writeJSON(w, status, companyRow(normDoc(raw)))
	return nil
}

func handleCompanyCreate(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	return saveCompany(c, w, r, "")
}

func handleCompanyUpdate(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	return saveCompany(c, w, r, mux.Vars(r)["id"])
}

// DELETE /admin/companies/{id} — its countries then show no company.
func handleCompanyDelete(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	oid, ok := oidOf(mux.Vars(r)["id"])
	if !ok {
		return errNotFound()
	}
	ctx, cancel := dbctx()
	defer cancel()
	res, err := companyRegistry().DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		return errInternal("Unable to remove the company.")
	}
	if res.DeletedCount == 0 {
		return errNotFound()
	}
	writeJSON(w, http.StatusOK, M{"id": oid.Hex(), "deleted": true})
	return nil
}

func registerCompanyRegistry(s *mux.Router) {
	s.HandleFunc("/site/companies", handleSiteCompanies).Methods("GET")
	s.HandleFunc("/admin/companies", authed(handleCompanyList)).Methods("GET")
	s.HandleFunc("/admin/companies", authed(handleCompanyCreate)).Methods("POST")
	s.HandleFunc("/admin/companies/{id}", authed(handleCompanyUpdate)).Methods("PUT")
	s.HandleFunc("/admin/companies/{id}", authed(handleCompanyDelete)).Methods("DELETE")
}

package erp

import (
	_ "embed"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
)

// Starter catalog: the product categories, brands, class / size / material
// options and products / services a store starts with, picked by its business
// category (POS terminal). Seeded when a store is created (sign-up or a
// platform admin adding one) and on request for an existing store
// (POST /stores/{id}/starter-catalog). Everything is created through the
// normal resources, so it is ordinary data the owner edits or deletes.
//
// Data: seeddata/starter_catalogs.json (Gulf market, English + Arabic). Prices
// are in SAR (VAT-inclusive for retail terminals, ex-VAT for B2B ones) and are
// converted to the store's currency; an item may be limited to some countries,
// carry a brand per country, or a local price. The web app keeps the same file
// for its mock API (starterp-frontend-v1 server/starter-catalogs.json).

//go:embed seeddata/starter_catalogs.json
var starterCatalogsJSON []byte

type starterCategory struct {
	Key    string `json:"key"`
	NameEn string `json:"nameEn"`
	NameAr string `json:"nameAr"`
}

type starterBrand struct {
	Name   string `json:"name"`
	NameAr string `json:"nameAr"`
}

type starterSpec struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	NameAr string `json:"nameAr"`
}

type starterItem struct {
	Key       string                `json:"key"`
	Section   string                `json:"section"`
	NameEn    string                `json:"nameEn"`
	NameAr    string                `json:"nameAr"`
	Price     float64               `json:"price"`
	Local     bool                  `json:"local"` // price already in the store's currency
	Unit      string                `json:"unit"`
	Service   bool                  `json:"service"`
	Brand     string                `json:"brand"`
	BrandBy   map[string]string     `json:"brandBy"` // country → brand (missing: none)
	Specs     map[string]string     `json:"specs"`   // kind → option name
	PartNo    string                `json:"partNo"`
	Countries []string              `json:"countries"` // empty: every country
	Variants  map[string]*[2]string `json:"variants"`  // country or "*" → [en, ar]; null keeps the name
	Prices    map[string]float64    `json:"prices"`
	Jewel     M                     `json:"jewel"`    // jewellery details (see jewellery.go)
	TaxRates  map[string]float64    `json:"taxRates"` // country → tax rate of the product
}

type starterTerminal struct {
	Category     string             `json:"category"`
	VatInclusive bool               `json:"vatInclusive"`
	Categories   []starterCategory  `json:"categories"`
	Brands       []starterBrand     `json:"brands"`
	Specs        []starterSpec      `json:"specs"`
	Items        []starterItem      `json:"items"`
	TaxRates     map[string]float64 `json:"taxRates"` // country → tax rate of the terminal's goods
}

type starterCatalogs struct {
	Version   int                         `json:"version"`
	Terminals map[string]*starterTerminal `json:"terminals"`
}

var (
	starterOnce sync.Once
	starterData *starterCatalogs
	starterErr  error
)

func loadStarterCatalogs() (*starterCatalogs, error) {
	starterOnce.Do(func() {
		var d starterCatalogs
		starterErr = json.Unmarshal(starterCatalogsJSON, &d)
		starterData = &d
	})
	return starterData, starterErr
}

// starterPriceFactor converts a SAR starter price to the store's currency
// (rounded for display; the owner sets real prices later). India is not the
// exchange rate (≈23) but what the same goods cost there.
var starterPriceFactor = map[string]float64{"SA": 1, "AE": 1, "QA": 1, "OM": 0.1, "BH": 0.1, "KW": 0.08, "IN": 15}

func starterPrice(sar float64, cc string, decimals int) float64 {
	f, ok := starterPriceFactor[cc]
	if !ok {
		f = 1
	}
	v := sar * f
	if cc == "IN" {
		return niceRupees(v)
	}
	if decimals >= 3 {
		// 3-decimal currencies: nearest 0.005
		return math.Round(v*200) / 200
	}
	return math.Round(v*100) / 100
}

// niceRupees rounds a rupee price the way shops price: whole rupees under
// ₹100, then the nearest ₹5, and the nearest ₹10 from ₹1,000.
func niceRupees(v float64) float64 {
	switch {
	case v < 100:
		return math.Max(1, math.Round(v))
	case v < 1000:
		return math.Round(v/5) * 5
	}
	return math.Round(v/10) * 10
}

// starterPlanItem is one product to create, resolved for a country.
type starterPlanItem struct {
	Key, Section, NameEn, NameAr, Unit, Brand, PartNo string
	Service                                           bool
	Price                                             float64 // in the store's currency, as sold (see VatInclusive)
	Specs                                             map[string]string
	Jewel                                             M        // money values in the store's currency
	Tax                                               *float64 // the product's own tax rate (nil: the store's)
}

// StarterPlan is what a store of one business category gets in one country.
type StarterPlan struct {
	Terminal     string
	Category     string
	VatInclusive bool
	Categories   []starterCategory
	Brands       []starterBrand
	Specs        []starterSpec
	Items        []starterPlanItem
}

func inCountries(list []string, cc string) bool {
	if len(list) == 0 {
		return true
	}
	for _, c := range list {
		if strings.EqualFold(c, cc) {
			return true
		}
	}
	return false
}

// planStarterCatalog resolves a terminal's starter data for a country: the
// items sold there, their names, brands and local prices, and only the
// brands / spec options those items use (plus the terminal's extra options).
func planStarterCatalog(terminal, cc string) (*StarterPlan, bool) {
	d, err := loadStarterCatalogs()
	if err != nil || d == nil {
		return nil, false
	}
	t := d.Terminals[terminal]
	if t == nil {
		return nil, false
	}
	cc = strings.ToUpper(strings.TrimSpace(cc))
	if cc == "" {
		cc = "SA"
	}
	cp := models.CountryProfileOrSaudi(cc)
	p := &StarterPlan{Terminal: terminal, Category: t.Category, VatInclusive: t.VatInclusive, Categories: t.Categories}
	usedBrand := map[string]bool{}
	for _, it := range t.Items {
		if !inCountries(it.Countries, cc) {
			continue
		}
		en, ar := it.NameEn, it.NameAr
		if v, ok := it.Variants[cc]; ok {
			if v != nil {
				en, ar = v[0], v[1]
			}
		} else if v, ok := it.Variants["*"]; ok && v != nil {
			en, ar = v[0], v[1]
		}
		brand := it.Brand
		if it.BrandBy != nil {
			brand = it.BrandBy[cc]
		}
		price := starterPrice(it.Price, cc, cp.Decimals)
		if it.Local {
			price = it.Price
		}
		if v, ok := it.Prices[cc]; ok {
			price = v
		}
		if brand != "" {
			usedBrand[brand] = true
		}
		pi := starterPlanItem{Key: it.Key, Section: it.Section, NameEn: en, NameAr: ar, Unit: it.Unit,
			Brand: brand, PartNo: it.PartNo, Service: it.Service, Price: price, Specs: it.Specs}
		if it.Jewel != nil {
			pi.Jewel = starterJewel(it.Jewel, cc, cp.Decimals, it.Local)
		}
		pi.Tax = starterTax(t, it, cc)
		p.Items = append(p.Items, pi)
	}
	for _, b := range t.Brands {
		if usedBrand[b.Name] {
			p.Brands = append(p.Brands, b)
		}
	}
	p.Specs = t.Specs
	return p, true
}

// starterJewel: an item's jewellery details for a country; the making charge
// (per gram or fixed) and the stones' value are money, converted like prices.
func starterJewel(j M, cc string, decimals int, local bool) M {
	out := cloneM(j)
	if local {
		return out
	}
	if mt := str(out["makingType"]); mt == "gram" || mt == "fixed" {
		out["making"] = starterPrice(num(out["making"]), cc, decimals)
	}
	if v, ok := out["stoneValue"]; ok {
		out["stoneValue"] = starterPrice(num(v), cc, decimals)
	}
	return out
}

// starterTax: the tax rate a product carries when it is not the store's
// standard rate: the item's or terminal's rate for the country (India sells
// gold jewellery at 3% GST and job work at 5%), and 0% for investment gold and
// silver (99%+ bars and coins) in the Gulf countries, where they are
// zero-rated.
func starterTax(t *starterTerminal, it starterItem, cc string) *float64 {
	if v, ok := it.TaxRates[cc]; ok {
		return &v
	}
	if it.Jewel != nil && boolv(it.Jewel["investment"]) && cc != "IN" {
		z := 0.0
		return &z
	}
	if v, ok := t.TaxRates[cc]; ok {
		return &v
	}
	return nil
}

// StarterCounts summarises a plan or a seeding run.
type StarterCounts struct {
	Categories int `json:"categories"`
	Brands     int `json:"brands"`
	Specs      int `json:"specs"`
	Products   int `json:"products"`
	Services   int `json:"services"`
}

func (p *StarterPlan) counts() StarterCounts {
	c := StarterCounts{Categories: len(p.Categories), Brands: len(p.Brands), Specs: len(p.Specs)}
	for _, it := range p.Items {
		if it.Service {
			c.Services++
		} else {
			c.Products++
		}
	}
	return c
}

// starterProductRecord builds the contract product for one plan item.
// vat: the store's VAT rate; refs: ids of the categories / brands / options.
func starterProductRecord(terminal string, vatInclusive bool, vat float64, it starterPlanItem, catID, brandID string, specIDs map[string]string) M {
	retail := it.Price
	if it.Tax != nil {
		vat = *it.Tax
	}
	if vatInclusive && vat > 0 {
		retail = roundN(it.Price/(1+vat/100), 4)
	}
	unit := it.Unit
	if unit == "" {
		unit = "Pcs"
		if it.Service {
			unit = "Service"
		}
	}
	rec := M{
		"nameEn": it.NameEn, "nameAr": it.NameAr, "unit": unit, "isService": it.Service, "vatPercent": vat,
		"categoryIds": []interface{}{}, "brandId": nil,
		"pricing": M{"purchase": 0.0, "retail": retail, "wholesale": 0.0, "retailMargin": 0.0, "wholesaleMargin": 0.0, "min": 0.0, "max": 0.0},
		"stock":   M{}, "components": []interface{}{},
		"posTerminal": terminal, "posSection": it.Section, "posKey": it.Key,
	}
	if catID != "" {
		rec["categoryIds"] = []interface{}{catID}
	}
	if brandID != "" {
		rec["brandId"] = brandID
	}
	if it.PartNo != "" {
		rec["partNo"] = it.PartNo
	}
	if it.Jewel != nil {
		rec["jewel"] = cloneM(it.Jewel)
	}
	if len(specIDs) > 0 {
		sp := M{}
		for k, v := range specIDs {
			sp[k] = v
		}
		rec["specs"] = sp
	}
	return rec
}

// scopedTo returns a copy of the caller limited to one store, so org-wide
// lookups (categories, brands) are read from and created in that store.
func (c *Ctx) scopedTo(storeHex string) *Ctx {
	cp := *c
	cp.User = cloneM(c.User)
	if oid, ok := oidOf(storeHex); ok {
		cp.User["store_id"] = oid
		cp.User["store_ids"] = []interface{}{oid}
	}
	cp.Stores = nil
	cp.storeIdx = map[string]M{}
	if s := c.store(storeHex); s != nil {
		cp.Stores = []M{s}
		cp.storeIdx[storeHex] = s
	}
	return &cp
}

const starterListLimit = 100000

func normKey(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// SeedStarterCatalog creates the starter catalog of the store's business
// category in that store. It is idempotent: categories, brands and options
// that already exist (same name) are reused, and products already added for
// the terminal (same posKey) are skipped, so it can run again safely.
func SeedStarterCatalog(c *Ctx, storeHex string) (StarterCounts, error) {
	var out StarterCounts
	st := c.store(storeHex)
	if st == nil {
		return out, errForbidden("You do not have access to this store.")
	}
	terminal := CategoryTerminal(str(st["business_category"]))
	if terminal == "" {
		return out, errf(http.StatusConflict, "no_business_category", "Choose the store's business category first.", map[string]string{"category": "required"})
	}
	plan, ok := planStarterCatalog(terminal, storeCountry(st))
	if !ok {
		return out, errf(http.StatusConflict, "no_starter_catalog", "There is no starter catalog for this business category.", nil)
	}
	sc := c.scopedTo(storeHex)
	vat := storeVatPercent(st)

	// categories (org resource, stored in the store's own DB)
	catRes := resourceByPath("categories")
	catByName := map[string]string{}
	if rows, _, err := catRes.Backend.List(sc, "", ListQuery{Limit: starterListLimit, Page: 1}); err == nil {
		for _, r := range rows {
			catByName[normKey(str(r["nameEn"]))] = str(r["id"])
		}
	}
	catID := map[string]string{}
	for _, k := range plan.Categories {
		if id := catByName[normKey(k.NameEn)]; id != "" {
			catID[k.Key] = id
			continue
		}
		rec, err := catRes.Backend.Create(sc, "", M{"nameEn": k.NameEn, "nameAr": k.NameAr}, WriteMeta{Action: "created"})
		if err != nil {
			return out, err
		}
		catID[k.Key] = str(rec["id"])
		out.Categories++
	}

	brandRes := resourceByPath("brands")
	brandByName := map[string]string{}
	codes := map[string]bool{}
	if rows, _, err := brandRes.Backend.List(sc, "", ListQuery{Limit: starterListLimit, Page: 1}); err == nil {
		for _, r := range rows {
			brandByName[normKey(str(r["name"]))] = str(r["id"])
			codes[strings.ToUpper(str(r["code"]))] = true
		}
	}
	for _, b := range plan.Brands {
		if brandByName[normKey(b.Name)] != "" {
			continue
		}
		body := M{"name": b.Name, "code": uniqueBrandCode(b.Name, codes)}
		if b.NameAr != "" {
			body["nameAr"] = b.NameAr
		}
		rec, err := brandRes.Backend.Create(sc, "", body, WriteMeta{Action: "created"})
		if err != nil {
			return out, err
		}
		brandByName[normKey(b.Name)] = str(rec["id"])
		out.Brands++
	}

	specRes := resourceByPath("product-specs")
	specByName := map[string]string{} // kind|name → id
	if rows, _, err := specRes.Backend.List(sc, storeHex, ListQuery{Limit: starterListLimit, Page: 1}); err == nil {
		for _, r := range rows {
			specByName[str(r["kind"])+"|"+normKey(str(r["name"]))] = str(r["id"])
		}
	}
	for _, s := range plan.Specs {
		k := s.Kind + "|" + normKey(s.Name)
		if specByName[k] != "" {
			continue
		}
		rec, err := specRes.Backend.Create(sc, storeHex, M{"kind": s.Kind, "name": s.Name, "nameAr": s.NameAr, "storeId": storeHex}, WriteMeta{Action: "created"})
		if err != nil {
			return out, err
		}
		specByName[k] = str(rec["id"])
		out.Specs++
	}

	// products already set up for this terminal
	have := starterKeysInStore(storeHex, terminal)
	prodRes := resourceByPath("products")
	for _, it := range plan.Items {
		if have[it.Key] {
			continue
		}
		ids := map[string]string{}
		for kind, name := range it.Specs {
			if id := specByName[kind+"|"+normKey(name)]; id != "" {
				ids[kind] = id
			}
		}
		rec := starterProductRecord(terminal, plan.VatInclusive, vat, it, catID[it.Section], brandByName[normKey(it.Brand)], ids)
		rec["storeId"] = storeHex
		if _, err := prodRes.Backend.Create(sc, storeHex, rec, WriteMeta{Action: "created"}); err != nil {
			return out, err
		}
		if it.Service {
			out.Services++
		} else {
			out.Products++
		}
	}
	dashboardTouched(prodRes, storeHex, "")
	return out, nil
}

// uniqueBrandCode: the legacy brand code (6 letters / digits of the name, as
// the brands resource derives it) made unique among the store's codes
// ("Arabian Oud" and "Arabian Pipes" would both be ARABIA).
func uniqueBrandCode(name string, used map[string]bool) string {
	base := strings.ToUpper(reNonAlnum.ReplaceAllString(name, ""))
	if len(base) > 6 {
		base = base[:6]
	}
	if base == "" {
		base = "BRAND"
	}
	code := base
	for n := 2; used[code]; n++ {
		suffix := str(float64(n))
		cut := base
		if len(cut)+len(suffix) > 8 {
			cut = cut[:8-len(suffix)]
		}
		code = cut + suffix
	}
	used[code] = true
	return code
}

// storeBrandCodes: the brand codes in use in a store (except prev's own).
func storeBrandCodes(storeHex string, prev M) map[string]bool {
	used := map[string]bool{}
	if storeHex == "" {
		return used
	}
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := storeDB(storeHex).Collection("product_brand").Find(ctx, M{"deleted": M{"$ne": true}})
	if err != nil {
		return used
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		d := bsonToM(cur.Current)
		if prev != nil && hexOf(d["_id"]) == hexOf(prev["_id"]) {
			continue
		}
		used[strings.ToUpper(str(d["code"]))] = true
	}
	return used
}

// starterKeysInStore: posKeys of the live products tagged for the terminal.
func starterKeysInStore(storeHex, terminal string) map[string]bool {
	have := map[string]bool{}
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := storeDB(storeHex).Collection("product").Find(ctx, M{"deleted": M{"$ne": true}, "erp.x.posTerminal": terminal})
	if err != nil {
		return have
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		d := bsonToM(cur.Current)
		if k := str(get(d, "erp.x.posKey")); k != "" {
			have[k] = true
		}
	}
	return have
}

// seedNewStore seeds a store that was just created; a failure never undoes
// the store (the owner can add the starter catalog later).
func seedNewStore(c *Ctx, storeHex string) {
	if c == nil || storeHex == "" {
		return
	}
	if _, err := SeedStarterCatalog(c, storeHex); err != nil {
		if ae, ok := err.(*APIError); !ok || ae.Status != http.StatusConflict {
			log.Printf("[starter-catalog] store %s: %v", storeHex, err)
		}
	}
}

// ---- endpoints ----

func registerStarterCatalog(s *mux.Router) {
	s.HandleFunc("/stores/{id}/starter-catalog", authed(handleStarterCatalogPreview)).Methods("GET")
	s.HandleFunc("/stores/{id}/starter-catalog", authed(handleStarterCatalogSeed)).Methods("POST")
}

// GET: what the starter catalog of the store's category adds, and how much of
// it the store already has (so the app offers it only when useful).
func handleStarterCatalogPreview(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	id := mux.Vars(r)["id"]
	st := c.store(id)
	if st == nil {
		return errForbidden("You do not have access to this store.")
	}
	terminal := CategoryTerminal(str(st["business_category"]))
	res := M{"terminal": terminal, "available": false}
	if terminal == "" {
		writeJSON(w, http.StatusOK, res)
		return nil
	}
	plan, ok := planStarterCatalog(terminal, storeCountry(st))
	if !ok {
		writeJSON(w, http.StatusOK, res)
		return nil
	}
	have := starterKeysInStore(id, terminal)
	missing := 0
	sections := []string{}
	seen := map[string]bool{}
	for _, it := range plan.Items {
		if !have[it.Key] {
			missing++
		}
		if !seen[it.Section] {
			seen[it.Section] = true
			sections = append(sections, it.Section)
		}
	}
	sort.Strings(sections)
	res["available"] = true
	res["category"] = plan.Category
	res["counts"] = plan.counts()
	res["added"] = len(plan.Items) - missing
	res["missing"] = missing
	writeJSON(w, http.StatusOK, res)
	return nil
}

func handleStarterCatalogSeed(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	id := mux.Vars(r)["id"]
	if c.store(id) == nil {
		return errForbidden("You do not have access to this store.")
	}
	if !c.can("inventory", "create") {
		return errForbidden("You do not have permission to add products.")
	}
	n, err := SeedStarterCatalog(c, id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, M{"created": n})
	return nil
}

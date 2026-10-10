package erp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Card terminals (card swiping machines): the POS sends the amount due to the
// store's card machine through the machine's payment provider, waits for the
// customer's card and records the payment with its approval code by itself.
// Recording a card payment by hand stays available as the fallback.
//
//	GET    /card-terminals/providers?storeId=          providers offered in the store's country
//	GET    /card-terminals?storeId=                    the store's card machines
//	POST   /card-terminals                             add one (settings edit)
//	PATCH  /card-terminals/{id}                        change it
//	DELETE /card-terminals/{id}?storeId=               remove it
//	POST   /card-terminals/{id}/check                  test the connection
//	POST   /card-terminals/{id}/payments               push an amount (sales create)
//	GET    /card-terminal-payments?storeId=            recent payments
//	GET    /card-terminal-payments/{id}?storeId=       a payment (refreshed while pending)
//	POST   /card-terminal-payments/{id}/cancel         cancel one still on the terminal
//	POST   /card-terminal-webhooks/{provider}/{store}/{payment}?sig=   provider call-back
//	GET    /admin/card-terminal-providers              StartERP admin: every provider
//	PUT    /admin/card-terminal-providers/{id}         StartERP admin: offer / credentials
//
// See card_terminal_providers.go for the provider catalog and adapters.

const (
	terminalProvidersColl = "erp_card_terminal_provider" // main DB
	terminalsColl         = "erp_card_terminal"          // store DB
	terminalPaymentsColl  = "erp_card_terminal_payment"  // store DB
	maxTerminalsPerStore  = 50
	maxTerminalAmount     = 10000000
)

var reTerminalRef = regexp.MustCompile(`^[A-Za-z0-9._:/#-]{1,60}$`)

var terminalModes = map[string]bool{"live": true, "test": true}

// ---------------------------------------------------------------- platform config

// providerConfig is the StartERP admin's setup of one provider.
type providerConfig struct {
	Enabled     bool
	Countries   []string
	LiveAllowed bool
	Partner     map[string]string // opened secrets / values
	UpdatedAt   string
	UpdatedBy   string
}

// defaultProviderConfig: real providers are offered in all their countries;
// the test terminal is off until an admin turns it on (always on when
// CARD_TERMINAL_SIMULATOR=1, e.g. CI and the e2e server).
func defaultProviderConfig(p *terminalProviderSpec) providerConfig {
	on := p.Model != "simulator" || os.Getenv("CARD_TERMINAL_SIMULATOR") == "1"
	return providerConfig{Enabled: on, Countries: append([]string{}, p.Countries...), LiveAllowed: true, Partner: map[string]string{}}
}

func terminalProviderConfigs() *mongo.Collection { return mainDB().Collection(terminalProvidersColl) }

func loadProviderConfig(p *terminalProviderSpec) providerConfig {
	cfg := defaultProviderConfig(p)
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	if err := terminalProviderConfigs().FindOne(ctx, bson.M{"_id": p.ID}).Decode(&raw); err != nil {
		return cfg
	}
	d := normDoc(raw)
	if v, ok := d["enabled"]; ok {
		cfg.Enabled = boolv(v)
	}
	if v, ok := d["countries"]; ok && v != nil {
		cfg.Countries = []string{}
		for _, c := range arr(v) {
			if p.servesCountry(str(c)) {
				cfg.Countries = append(cfg.Countries, str(c))
			}
		}
	}
	if v, ok := d["liveAllowed"]; ok {
		cfg.LiveAllowed = boolv(v)
	}
	for k, v := range sub(d, "partner") {
		if s, err := openSecret(str(v)); err == nil {
			cfg.Partner[k] = s
		}
	}
	cfg.UpdatedAt = str(d["updatedAt"])
	cfg.UpdatedBy = str(d["updatedBy"])
	return cfg
}

func (c providerConfig) offeredIn(cc string) bool {
	if !c.Enabled {
		return false
	}
	for _, x := range c.Countries {
		if x == cc {
			return true
		}
	}
	return false
}

// partnerReady: every required partner field is set.
func partnerReady(p *terminalProviderSpec, c providerConfig) bool {
	for _, f := range p.PartnerFields {
		if f.Required && strings.TrimSpace(c.Partner[f.Key]) == "" {
			return false
		}
	}
	return true
}

// fieldsOut shows values of plain fields and {set, hint} for secrets.
func fieldsOut(defs []terminalField, vals map[string]string) M {
	out := M{}
	for _, f := range defs {
		v := vals[f.Key]
		if f.Secret {
			out[f.Key] = M{"set": v != "", "hint": secretHint(v)}
		} else {
			out[f.Key] = v
		}
	}
	return out
}

// mergeFields applies a body's fields onto the current values: a plain field
// takes the new text; a secret keeps its value when the body leaves it out or
// sends "", and is cleared by null. Returns field errors.
func mergeFields(defs []terminalField, cur map[string]string, body M, prefix string, requireAll bool) (map[string]string, map[string]string) {
	out := map[string]string{}
	for k, v := range cur {
		out[k] = v
	}
	errs := map[string]string{}
	for _, f := range defs {
		max := f.Max
		if max == 0 {
			max = 200
		}
		raw, given := body[f.Key]
		if given {
			if raw == nil {
				out[f.Key] = ""
			} else if s, ok := raw.(string); ok {
				s = strings.TrimSpace(s)
				if len([]rune(s)) > max {
					errs[prefix+f.Key] = "at most " + strconv.Itoa(max) + " characters"
				} else if strings.ContainsAny(s, "\r\n\t") {
					errs[prefix+f.Key] = "one line only"
				} else if s != "" || !f.Secret {
					out[f.Key] = s
				}
			} else {
				errs[prefix+f.Key] = "must be text"
			}
		}
		if requireAll && f.Required && out[f.Key] == "" && errs[prefix+f.Key] == "" {
			errs[prefix+f.Key] = "required"
		}
	}
	return out, errs
}

func sealFields(defs []terminalField, vals map[string]string) (bson.M, error) {
	out := bson.M{}
	for _, f := range defs {
		v := vals[f.Key]
		if f.Secret {
			s, err := sealSecret(v)
			if err != nil {
				return nil, err
			}
			out[f.Key] = s
		} else {
			out[f.Key] = v
		}
	}
	return out, nil
}

func openFields(defs []terminalField, raw M) map[string]string {
	out := map[string]string{}
	for _, f := range defs {
		v := str(raw[f.Key])
		if f.Secret {
			if s, err := openSecret(v); err == nil {
				v = s
			} else {
				v = ""
			}
		}
		out[f.Key] = v
	}
	return out
}

func adminProviderRow(p *terminalProviderSpec, c providerConfig) M {
	row := p.catalogRow()
	row["enabled"] = c.Enabled
	row["enabledCountries"] = c.Countries
	row["liveAllowed"] = c.LiveAllowed
	row["partner"] = fieldsOut(p.PartnerFields, c.Partner)
	row["partnerReady"] = partnerReady(p, c)
	row["updatedAt"] = c.UpdatedAt
	row["updatedBy"] = c.UpdatedBy
	return row
}

// GET /admin/card-terminal-providers
func handleAdminTerminalProviders(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	items := []M{}
	for _, id := range terminalProviderIDs() {
		p := terminalProviders[id]
		items = append(items, adminProviderRow(p, loadProviderConfig(p)))
	}
	writeJSON(w, http.StatusOK, M{"items": items, "total": len(items)})
	return nil
}

// PUT /admin/card-terminal-providers/{id} {enabled, countries, liveAllowed, partner:{…}}
func handleAdminSaveTerminalProvider(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	p := terminalProviders[mux.Vars(r)["id"]]
	if p == nil {
		return errNotFound()
	}
	body, err := readBody(r)
	if err != nil {
		return err
	}
	cur := loadProviderConfig(p)
	errs := map[string]string{}
	if v, ok := body["enabled"]; ok {
		if b, isB := v.(bool); isB {
			cur.Enabled = b
		} else {
			errs["enabled"] = "true or false"
		}
	}
	if v, ok := body["liveAllowed"]; ok {
		if b, isB := v.(bool); isB {
			cur.LiveAllowed = b
		} else {
			errs["liveAllowed"] = "true or false"
		}
	}
	if v, ok := body["countries"]; ok {
		list, isL := v.([]interface{})
		if !isL {
			errs["countries"] = "a list of country codes"
		} else {
			seen := map[string]bool{}
			cur.Countries = []string{}
			for _, x := range list {
				cc := strings.ToUpper(strings.TrimSpace(str(x)))
				if !p.servesCountry(cc) {
					errs["countries"] = p.NameEn + " does not serve " + cc
					break
				}
				if !seen[cc] {
					seen[cc] = true
					cur.Countries = append(cur.Countries, cc)
				}
			}
			sort.Strings(cur.Countries)
		}
	}
	if v, ok := body["partner"]; ok && v != nil {
		pm, isM := v.(M)
		if !isM {
			errs["partner"] = "an object"
		} else {
			vals, fe := mergeFields(p.PartnerFields, cur.Partner, pm, "partner.", false)
			for k, e := range fe {
				errs[k] = e
			}
			cur.Partner = vals
		}
	}
	if len(errs) > 0 {
		return errBadRequest("", errs)
	}
	sealed, err := sealFields(p.PartnerFields, cur.Partner)
	if err != nil {
		return errInternal("Unable to protect the credentials.")
	}
	at := nowFn().In(riyadh).Format(layoutDT)
	ctx, cancel := dbctx()
	defer cancel()
	if _, err := terminalProviderConfigs().UpdateOne(ctx, bson.M{"_id": p.ID}, bson.M{"$set": bson.M{
		"enabled": cur.Enabled, "countries": cur.Countries, "liveAllowed": cur.LiveAllowed, "partner": sealed,
		"updatedAt": at, "updatedBy": c.UserName,
	}}, options.Update().SetUpsert(true)); err != nil {
		return errInternal("Unable to save the provider.")
	}
	writeJSON(w, http.StatusOK, adminProviderRow(p, loadProviderConfig(p)))
	return nil
}

// ---------------------------------------------------------------- store access

func terminalStore(c *Ctx, storeHex string) (M, error) {
	storeHex = strings.TrimSpace(storeHex)
	if storeHex == "" {
		return nil, errBadRequest("storeId is required.", map[string]string{"storeId": "required"})
	}
	s := c.store(storeHex)
	if s == nil {
		return nil, errForbidden("You do not have access to this store.")
	}
	return s, nil
}

func canManageTerminals(c *Ctx) bool { return c.Admin || c.can("settings", "edit") }
func canSeeTerminals(c *Ctx) bool {
	return c.Admin || c.can("settings", "view") || c.can("sales", "create")
}
func canChargeTerminals(c *Ctx) bool { return c.Admin || c.can("sales", "create") }

func storeIDParam(r *http.Request, body M) string {
	if v := strings.TrimSpace(r.URL.Query().Get("storeId")); v != "" {
		return v
	}
	return str(body["storeId"])
}

// GET /card-terminals/providers?storeId=
func handleTerminalProviders(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	s, err := terminalStore(c, r.URL.Query().Get("storeId"))
	if err != nil {
		return err
	}
	if !canSeeTerminals(c) {
		return errForbidden("")
	}
	cc := storeCountryOrSA(s)
	items := []M{}
	for _, id := range terminalProviderIDs() {
		p := terminalProviders[id]
		pc := loadProviderConfig(p)
		if !pc.offeredIn(cc) {
			continue
		}
		row := p.catalogRow()
		delete(row, "partnerFields")
		row["liveAllowed"] = pc.LiveAllowed
		row["partnerReady"] = partnerReady(p, pc)
		items = append(items, row)
	}
	writeJSON(w, http.StatusOK, M{"items": items, "total": len(items), "countryCode": cc})
	return nil
}

// ---------------------------------------------------------------- store terminals

func terminalsOf(storeHex string) *mongo.Collection {
	return storeDB(storeHex).Collection(terminalsColl)
}
func terminalPaymentsOf(storeHex string) *mongo.Collection {
	return storeDB(storeHex).Collection(terminalPaymentsColl)
}

// terminalOut is a card machine in the contract shape. Settings users see
// the fields (secrets as {set, hint}); cashiers only what the till needs.
func terminalOut(d M, full bool) M {
	p := terminalProviders[str(d["provider"])]
	out := M{
		"id": hexOf(d["_id"]), "storeId": str(d["storeId"]), "name": str(d["name"]), "provider": str(d["provider"]),
		"mode": str(d["mode"]), "terminalId": str(get(d, "fields.terminalId")), "active": boolv(d["active"]),
		"isDefault": boolv(d["isDefault"]), "connect": "api",
	}
	if str(d["bridgeId"]) != "" || str(d["driver"]) != "" {
		out["bridgeId"], out["driver"] = str(d["bridgeId"]), str(d["driver"])
	}
	if p != nil {
		out["providerName"] = M{"en": p.NameEn, "ar": p.NameAr}
		out["connect"] = p.connect()
	}
	if full {
		if p != nil {
			out["fields"] = fieldsOut(p.StoreFields, openFields(p.StoreFields, sub(d, "fields")))
		}
		out["lastCheck"] = sub(d, "lastCheck")
		out["createdAt"] = str(d["createdAt"])
		out["createdBy"] = str(d["createdBy"])
		out["updatedAt"] = str(d["updatedAt"])
	}
	return out
}

func loadTerminal(storeHex, id string) (M, error) {
	oid, ok := oidOf(id)
	if !ok {
		return nil, errNotFound()
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	if err := terminalsOf(storeHex).FindOne(ctx, bson.M{"_id": oid, "deleted": bson.M{"$ne": true}}).Decode(&raw); err != nil {
		return nil, errNotFound()
	}
	return normDoc(raw), nil
}

// GET /card-terminals?storeId=&select=
func handleListTerminals(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	storeHex := r.URL.Query().Get("storeId")
	if _, err := terminalStore(c, storeHex); err != nil {
		return err
	}
	if !canSeeTerminals(c) {
		return errForbidden("")
	}
	fs, err := parseSelect(r.URL.Query().Get("select"))
	if err != nil {
		return err
	}
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := terminalsOf(storeHex).Find(ctx, bson.M{"deleted": bson.M{"$ne": true}},
		options.Find().SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(maxTerminalsPerStore))
	if err != nil {
		return errInternal("db: " + err.Error())
	}
	defer cur.Close(ctx)
	full := c.Admin || c.can("settings", "view")
	items := []M{}
	for cur.Next(ctx) {
		var raw bson.M
		if err := cur.Decode(&raw); err != nil {
			return errInternal("db: " + err.Error())
		}
		items = append(items, fs.apply(terminalOut(normDoc(raw), full)))
	}
	writeJSON(w, http.StatusOK, M{"items": items, "total": len(items)})
	return nil
}

// validateTerminal checks a card machine's body against its provider; prev is
// the stored record on update (nil on create).
func validateTerminal(c *Ctx, store M, body M, prev M) (bson.M, map[string]string) {
	errs := map[string]string{}
	set := bson.M{}
	cc := storeCountryOrSA(store)
	provID := str(body["provider"])
	if prev != nil {
		if v, ok := body["provider"]; ok && str(v) != str(prev["provider"]) {
			errs["provider"] = "cannot change; add a new card machine instead"
		}
		provID = str(prev["provider"])
	}
	p := terminalProviders[provID]
	if p == nil {
		errs["provider"] = "choose a provider"
		return nil, errs
	}
	pc := loadProviderConfig(p)
	if prev == nil && !pc.offeredIn(cc) {
		errs["provider"] = p.NameEn + " is not offered in this store's country"
	}
	if v, ok := body["name"]; ok || prev == nil {
		n := strings.TrimSpace(str(v))
		if n == "" {
			errs["name"] = "required"
		} else if len([]rune(n)) > 60 {
			errs["name"] = "at most 60 characters"
		}
		set["name"] = n
	}
	if v, ok := body["mode"]; ok || prev == nil {
		m := str(v)
		if m == "" {
			m = "live"
		}
		if p.Model == "simulator" {
			m = "test"
		}
		if !terminalModes[m] {
			errs["mode"] = "live or test"
		} else if m == "live" && !pc.LiveAllowed {
			errs["mode"] = p.NameEn + " is not yet available for live payments; use test mode"
		}
		set["mode"] = m
	}
	for _, k := range []string{"active", "isDefault"} {
		if v, ok := body[k]; ok {
			b, isB := v.(bool)
			if !isB {
				errs[k] = "true or false"
			}
			set[k] = b
		} else if prev == nil {
			set[k] = k == "active"
		}
	}
	fm := sub(body, "fields")
	if v, ok := body["fields"]; ok && v != nil {
		if _, isM := v.(M); !isM {
			errs["fields"] = "an object"
		}
	}
	var curVals map[string]string
	if prev != nil {
		curVals = openFields(p.StoreFields, sub(prev, "fields"))
	} else {
		curVals = map[string]string{}
	}
	vals, fe := mergeFields(p.StoreFields, curVals, fm, "fields.", p.connect() != "manual")
	if p.connect() == "bridge" {
		bridgeTerminalFields(storeIDOf(store), p, body, prev, set, errs)
	}
	for k, e := range fe {
		errs[k] = e
	}
	if len(errs) > 0 {
		return nil, errs
	}
	sealed, err := sealFields(p.StoreFields, vals)
	if err != nil {
		errs["fields"] = "unable to protect the credentials"
		return nil, errs
	}
	set["fields"] = sealed
	set["provider"] = p.ID
	return set, nil
}

// POST /card-terminals {storeId, name, provider, mode, active, isDefault, fields}
func handleCreateTerminal(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	storeHex := storeIDParam(r, body)
	store, err := terminalStore(c, storeHex)
	if err != nil {
		return err
	}
	if !canManageTerminals(c) {
		return errForbidden("Only users who can edit the store's settings can add card machines.")
	}
	set, errs := validateTerminal(c, store, body, nil)
	if len(errs) > 0 {
		return errBadRequest("", errs)
	}
	ctx, cancel := dbctx()
	defer cancel()
	col := terminalsOf(storeHex)
	n, err := col.CountDocuments(ctx, bson.M{"deleted": bson.M{"$ne": true}})
	if err != nil {
		return errInternal("db: " + err.Error())
	}
	if n >= maxTerminalsPerStore {
		return errBadRequest("A store can have at most 50 card machines.", map[string]string{"name": "at most 50 card machines"})
	}
	if n == 0 {
		set["isDefault"] = true // the first machine is the store's default
	}
	at := nowFn().In(storeLocation(store)).Format(layoutDT)
	set["storeId"] = storeHex
	set["createdAt"], set["updatedAt"], set["createdBy"] = at, at, c.UserName
	oid := primitive.NewObjectID()
	set["_id"] = oid
	if boolv(set["isDefault"]) {
		_, _ = col.UpdateMany(ctx, bson.M{"isDefault": true}, bson.M{"$set": bson.M{"isDefault": false}})
	}
	if _, err := col.InsertOne(ctx, set); err != nil {
		return errInternal("Unable to save the card machine.")
	}
	d, err := loadTerminal(storeHex, oid.Hex())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, terminalOut(d, true))
	return nil
}

// PATCH /card-terminals/{id}
func handleUpdateTerminal(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	storeHex := storeIDParam(r, body)
	store, err := terminalStore(c, storeHex)
	if err != nil {
		return err
	}
	if !canManageTerminals(c) {
		return errForbidden("Only users who can edit the store's settings can change card machines.")
	}
	prev, err := loadTerminal(storeHex, mux.Vars(r)["id"])
	if err != nil {
		return err
	}
	set, errs := validateTerminal(c, store, body, prev)
	if len(errs) > 0 {
		return errBadRequest("", errs)
	}
	ctx, cancel := dbctx()
	defer cancel()
	col := terminalsOf(storeHex)
	if boolv(set["isDefault"]) {
		_, _ = col.UpdateMany(ctx, bson.M{"isDefault": true, "_id": bson.M{"$ne": prev["_id"]}}, bson.M{"$set": bson.M{"isDefault": false}})
	}
	set["updatedAt"] = nowFn().In(storeLocation(store)).Format(layoutDT)
	if _, err := col.UpdateOne(ctx, bson.M{"_id": prev["_id"]}, bson.M{"$set": set}); err != nil {
		return errInternal("Unable to save the card machine.")
	}
	d, err := loadTerminal(storeHex, hexOf(prev["_id"]))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, terminalOut(d, true))
	return nil
}

// DELETE /card-terminals/{id}?storeId=  (payments already taken stay)
func handleDeleteTerminal(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	storeHex := r.URL.Query().Get("storeId")
	if _, err := terminalStore(c, storeHex); err != nil {
		return err
	}
	if !canManageTerminals(c) {
		return errForbidden("Only users who can edit the store's settings can remove card machines.")
	}
	prev, err := loadTerminal(storeHex, mux.Vars(r)["id"])
	if err != nil {
		return err
	}
	ctx, cancel := dbctx()
	defer cancel()
	if n, _ := terminalPaymentsOf(storeHex).CountDocuments(ctx, bson.M{"terminalId": hexOf(prev["_id"]), "status": tpPending}); n > 0 {
		return errConflict("terminal_busy", "A payment is still waiting on this card machine. Cancel it first.")
	}
	if _, err := terminalsOf(storeHex).UpdateOne(ctx, bson.M{"_id": prev["_id"]}, bson.M{"$set": bson.M{
		"deleted": true, "isDefault": false, "fields": bson.M{}, "deletedAt": nowFn().UTC().Format(time.RFC3339), "deletedBy": c.UserName,
	}}); err != nil {
		return errInternal("Unable to remove the card machine.")
	}
	writeJSON(w, http.StatusOK, M{"id": hexOf(prev["_id"]), "deleted": true})
	return nil
}

// terminalConfig builds the adapter config of a stored card machine.
func terminalConfig(d M) (*terminalProviderSpec, terminalCfg, providerConfig, error) {
	p := terminalProviders[str(d["provider"])]
	if p == nil {
		return nil, terminalCfg{}, providerConfig{}, errConflict("provider_unavailable", "This card machine's provider is no longer available.")
	}
	pc := loadProviderConfig(p)
	vals := openFields(p.StoreFields, sub(d, "fields"))
	env := "live"
	if str(d["mode"]) == "test" {
		env = "sandbox"
	}
	base := p.BaseURL[env]
	if o := strings.TrimSpace(os.Getenv("CARD_TERMINAL_BASEURL_" + strings.ToUpper(strings.ReplaceAll(p.ID, "-", "_")))); o != "" {
		base = o // tests and private deployments
	}
	return p, terminalCfg{Env: env, BaseURL: base, Partner: pc.Partner, Store: vals, Terminal: vals["terminalId"], HTTP: terminalHTTP,
		StoreHex: str(d["storeId"]), TerminalID: hexOf(d["_id"]), BridgeID: str(d["bridgeId"]), Driver: terminalDriver(p, d)}, pc, nil
}

var terminalHTTP = &http.Client{Timeout: 25 * time.Second}

// POST /card-terminals/{id}/check {storeId}
func handleCheckTerminal(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	storeHex := storeIDParam(r, body)
	store, err := terminalStore(c, storeHex)
	if err != nil {
		return err
	}
	if !canManageTerminals(c) {
		return errForbidden("")
	}
	d, err := loadTerminal(storeHex, mux.Vars(r)["id"])
	if err != nil {
		return err
	}
	p, cfg, pc, err := terminalConfig(d)
	if err != nil {
		return err
	}
	res := M{"ok": true, "message": "Connected."}
	switch {
	case p.connect() == "manual":
		res = M{"ok": false, "message": p.NameEn + " has no public connection yet: take the payment on the machine and record it in the POS."}
	case !partnerReady(p, pc):
		res = M{"ok": false, "message": "StartERP has not finished its partner setup with " + p.NameEn + " yet. Please contact StartERP support."}
	default:
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if e := p.adapter().Check(ctx, cfg); e != nil {
			res = M{"ok": false, "message": e.Error()}
		}
	}
	res["at"] = nowFn().In(storeLocation(store)).Format(layoutDT)
	ctx, cancel := dbctx()
	defer cancel()
	_, _ = terminalsOf(storeHex).UpdateOne(ctx, bson.M{"_id": d["_id"]}, bson.M{"$set": bson.M{"lastCheck": res}})
	writeJSON(w, http.StatusOK, res)
	return nil
}

// ---------------------------------------------------------------- payments

func terminalPaymentOut(d M) M {
	return M{
		"id": hexOf(d["_id"]), "storeId": str(d["storeId"]), "terminalId": str(d["terminalId"]),
		"terminalName": str(d["terminalName"]), "provider": str(d["provider"]), "mode": str(d["mode"]),
		"test": boolv(d["test"]), "kind": str(d["kind"]),
		"amount": num(d["amount"]), "currency": str(d["currency"]), "reference": str(d["reference"]),
		"status": str(d["status"]), "providerRef": str(d["providerRef"]), "authCode": str(d["authCode"]),
		"rrn": str(d["rrn"]), "maskedPan": str(d["maskedPan"]), "scheme": str(d["scheme"]),
		"message": str(d["message"]), "createdAt": str(d["createdAt"]), "finishedAt": str(d["finishedAt"]),
		"createdBy": str(d["createdBy"]),
	}
}

func payReqOf(d M) terminalPayReq {
	return terminalPayReq{
		PaymentID: hexOf(d["_id"]), Reference: str(d["reference"]), Amount: num(d["amount"]),
		Currency: str(d["currency"]), Decimals: int(num(d["decimals"])), Kind: str(d["kind"]),
		RefundOf: str(d["refundOf"]),
	}
}

// validAmount: more than 0, at most 10,000,000 and no more decimals than the currency.
func validAmount(v interface{}, decimals int) (float64, string) {
	f, ok := v.(float64)
	if !ok {
		if i, isI := v.(int64); isI {
			f, ok = float64(i), true
		}
	}
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, "a number"
	}
	if f <= 0 {
		return 0, "more than 0"
	}
	if f > maxTerminalAmount {
		return 0, "at most 10,000,000"
	}
	scale := math.Pow(10, float64(decimals))
	if math.Abs(f*scale-math.Round(f*scale)) > 1e-6 {
		return 0, "at most " + strconv.Itoa(decimals) + " decimals"
	}
	return math.Round(f*scale) / scale, ""
}

// POST /card-terminals/{id}/payments {storeId, amount, reference}
// → 201 with the payment (usually "pending"). One payment at a time per
// machine: another one still waiting answers 409 terminal_busy with its id.
// The same reference while that payment is pending returns it again (200).
func handleStartTerminalPayment(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	storeHex := storeIDParam(r, body)
	store, err := terminalStore(c, storeHex)
	if err != nil {
		return err
	}
	if !canChargeTerminals(c) {
		return errForbidden("You cannot take payments in this store.")
	}
	d, err := loadTerminal(storeHex, mux.Vars(r)["id"])
	if err != nil {
		return err
	}
	if !boolv(d["active"]) {
		return errConflict("terminal_inactive", "This card machine is turned off in Settings.")
	}
	p, cfg, pc, err := terminalConfig(d)
	if err != nil {
		return err
	}
	if p.connect() == "manual" {
		return errConflict("terminal_manual", p.NameEn+" cannot receive amounts from StartERP yet. Take the payment on the machine and record it.")
	}
	if !pc.offeredIn(storeCountryOrSA(store)) {
		return errConflict("provider_unavailable", p.NameEn+" is not offered in this store's country any more.")
	}
	if str(d["mode"]) == "live" && !pc.LiveAllowed {
		return errConflict("provider_unavailable", p.NameEn+" is not available for live payments yet.")
	}
	if !partnerReady(p, pc) {
		return errConflict("provider_unavailable", "StartERP has not finished its partner setup with "+p.NameEn+" yet.")
	}
	prof := models.CountryProfileOrSaudi(storeCountryOrSA(store))
	errs := map[string]string{}
	amount, msg := validAmount(body["amount"], prof.Decimals)
	if msg != "" {
		errs["amount"] = msg
	}
	ref := strings.TrimSpace(str(body["reference"]))
	if ref != "" && !reTerminalRef.MatchString(ref) {
		errs["reference"] = "letters, digits and . _ : / # - only (max 60)"
	}
	if len(errs) > 0 {
		return errBadRequest("", errs)
	}
	tid := hexOf(d["_id"])
	ctx, cancel := dbctx()
	defer cancel()
	col := terminalPaymentsOf(storeHex)
	var busy bson.M
	if err := col.FindOne(ctx, bson.M{"terminalId": tid, "status": tpPending}).Decode(&busy); err == nil {
		b := normDoc(busy)
		if ref != "" && str(b["reference"]) == ref && num(b["amount"]) == amount {
			writeJSON(w, http.StatusOK, terminalPaymentOut(b))
			return nil
		}
		return errf(http.StatusConflict, "terminal_busy", "Another payment is still waiting on this card machine.", map[string]string{"paymentId": hexOf(b["_id"])})
	}
	oid := primitive.NewObjectID()
	loc := storeLocation(store)
	now := nowFn()
	doc := bson.M{
		"_id": oid, "storeId": storeHex, "terminalId": tid, "terminalName": str(d["name"]), "provider": p.ID,
		"mode": str(d["mode"]), "test": str(d["mode"]) == "test" || p.Model == "simulator", "kind": "sale",
		"amount": amount, "currency": prof.CurrencyCode, "decimals": prof.Decimals, "reference": ref,
		"status": tpPending, "createdAt": now.In(loc).Format(layoutDT), "sentAtMs": now.UnixMilli(),
		"createdBy": c.UserName, "createdById": c.UserID.Hex(),
	}
	if _, err := col.InsertOne(ctx, doc); err != nil {
		return errInternal("Unable to start the payment.")
	}
	req := payReqOf(normDoc(doc))
	req.WebhookURL = terminalWebhookURL(p.ID, storeHex, oid.Hex())
	var res terminalResult
	if run, ok := p.adapter().(terminalRunner); ok {
		res = terminalResult{Status: tpPending, ProviderRef: "run_" + oid.Hex(), Message: "Waiting for the card on the machine."}
		go runTerminalPayment(run, p, cfg, req, storeHex, loc, oid)
	} else {
		actx, acancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer acancel()
		var aerr error
		res, aerr = p.adapter().Start(actx, cfg, req)
		if aerr != nil {
			res = terminalResult{Status: tpFailed, Message: friendlyTerminalError(p, aerr)}
		}
	}
	out, err := applyTerminalResult(storeHex, loc, oid, res)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, terminalPaymentOut(out))
	return nil
}

// runTerminalPayment runs a blocking adapter call and stores its answer.
func runTerminalPayment(run terminalRunner, p *terminalProviderSpec, cfg terminalCfg, req terminalPayReq, storeHex string, loc *time.Location, oid primitive.ObjectID) {
	defer func() {
		if r := recover(); r != nil {
			_, _ = applyTerminalResult(storeHex, loc, oid, terminalResult{Status: tpFailed, Message: "The card machine connection stopped unexpectedly."})
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), terminalTimeout+30*time.Second)
	defer cancel()
	res, err := run.Run(ctx, cfg, req)
	if err != nil {
		res = terminalResult{Status: tpFailed, Message: friendlyTerminalError(p, err)}
	}
	if res.Status == "" || res.Status == tpPending {
		res.Status = tpTimeout
		if res.Message == "" {
			res.Message = "The card machine did not answer in time."
		}
	}
	_, _ = applyTerminalResult(storeHex, loc, oid, res)
}

func friendlyTerminalError(p *terminalProviderSpec, err error) string {
	m := err.Error()
	if len(m) > 300 {
		m = m[:300]
	}
	return "Could not reach " + p.NameEn + ": " + m
}

// applyTerminalResult writes an adapter's answer onto a pending payment (a
// payment that is already final never changes) and returns the record.
func applyTerminalResult(storeHex string, loc *time.Location, oid primitive.ObjectID, res terminalResult) (M, error) {
	ctx, cancel := dbctx()
	defer cancel()
	col := terminalPaymentsOf(storeHex)
	set := bson.M{"updatedAtMs": nowFn().UnixMilli()}
	if res.Status != "" {
		set["status"] = res.Status
	}
	if tpFinal[res.Status] {
		set["finishedAt"] = nowFn().In(loc).Format(layoutDT)
	}
	for k, v := range map[string]string{"providerRef": res.ProviderRef, "authCode": res.AuthCode, "rrn": res.RRN,
		"maskedPan": res.MaskedPan, "scheme": res.Scheme, "message": res.Message} {
		if v != "" {
			set[k] = v
		}
	}
	_, err := col.UpdateOne(ctx, bson.M{"_id": oid, "status": tpPending}, bson.M{"$set": set})
	if err != nil {
		return nil, errInternal("db: " + err.Error())
	}
	var raw bson.M
	if err := col.FindOne(ctx, bson.M{"_id": oid}).Decode(&raw); err != nil {
		return nil, errNotFound()
	}
	return normDoc(raw), nil
}

// refreshTerminalPayment asks the provider about a pending payment; past
// terminalTimeout it cancels it on the terminal and reports a time-out.
func refreshTerminalPayment(storeHex string, loc *time.Location, d M) (M, error) {
	if str(d["status"]) != tpPending {
		return d, nil
	}
	td, err := loadTerminalAny(storeHex, str(d["terminalId"]))
	if err != nil {
		return applyTerminalResult(storeHex, loc, d["_id"].(primitive.ObjectID), terminalResult{Status: tpFailed, Message: "The card machine was removed."})
	}
	p, cfg, _, err := terminalConfig(td)
	if err != nil {
		return d, nil
	}
	req := payReqOf(d)
	ref := str(d["providerRef"])
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	oid := d["_id"].(primitive.ObjectID)
	if ref == "" {
		// Start never answered: nothing to ask the provider about
		if nowFn().UnixMilli()-int64(num(d["sentAtMs"])) > terminalTimeout.Milliseconds() {
			return applyTerminalResult(storeHex, loc, oid, terminalResult{Status: tpTimeout, Message: "The card machine did not answer."})
		}
		return d, nil
	}
	if _, isRun := p.adapter().(terminalRunner); isRun {
		// the background call writes the answer; past the time-out (plus a
		// margin) the answer was lost, e.g. the server restarted mid-payment
		if nowFn().UnixMilli()-int64(num(d["sentAtMs"])) > (terminalTimeout + 90*time.Second).Milliseconds() {
			return applyTerminalResult(storeHex, loc, oid, terminalResult{Status: tpTimeout,
				Message: "The card machine's answer was lost. Check the machine's last receipt before taking the payment again."})
		}
		return d, nil
	}
	res, aerr := p.adapter().Status(ctx, cfg, req, ref)
	if aerr != nil {
		res = terminalResult{Status: tpPending, Message: friendlyTerminalError(p, aerr)}
	}
	if res.Status == tpPending && nowFn().UnixMilli()-int64(num(d["sentAtMs"])) > terminalTimeout.Milliseconds() {
		cres, cerr := p.adapter().Cancel(ctx, cfg, req, ref)
		if cerr == nil && cres.Status != "" && cres.Status != tpCancelled && cres.Status != tpPending {
			res = cres // e.g. approved at the very last moment
		} else {
			res = terminalResult{Status: tpTimeout, Message: "No card was presented in time. The payment was cancelled on the machine."}
		}
	}
	return applyTerminalResult(storeHex, loc, oid, res)
}

func loadTerminalAny(storeHex, id string) (M, error) {
	oid, ok := oidOf(id)
	if !ok {
		return nil, errNotFound()
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	if err := terminalsOf(storeHex).FindOne(ctx, bson.M{"_id": oid}).Decode(&raw); err != nil {
		return nil, errNotFound()
	}
	d := normDoc(raw)
	if boolv(d["deleted"]) {
		return nil, errNotFound()
	}
	return d, nil
}

func loadPayment(storeHex, id string) (M, error) {
	oid, ok := oidOf(id)
	if !ok {
		return nil, errNotFound()
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	if err := terminalPaymentsOf(storeHex).FindOne(ctx, bson.M{"_id": oid}).Decode(&raw); err != nil {
		return nil, errNotFound()
	}
	d := normDoc(raw)
	d["_id"] = oid
	return d, nil
}

// GET /card-terminal-payments/{id}?storeId=
func handleGetTerminalPayment(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	storeHex := r.URL.Query().Get("storeId")
	store, err := terminalStore(c, storeHex)
	if err != nil {
		return err
	}
	if !canChargeTerminals(c) && !c.can("settings", "view") {
		return errForbidden("")
	}
	d, err := loadPayment(storeHex, mux.Vars(r)["id"])
	if err != nil {
		return err
	}
	out, err := refreshTerminalPayment(storeHex, storeLocation(store), d)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, terminalPaymentOut(out))
	return nil
}

// POST /card-terminal-payments/{id}/cancel {storeId}
func handleCancelTerminalPayment(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	storeHex := storeIDParam(r, body)
	store, err := terminalStore(c, storeHex)
	if err != nil {
		return err
	}
	if !canChargeTerminals(c) {
		return errForbidden("")
	}
	d, err := loadPayment(storeHex, mux.Vars(r)["id"])
	if err != nil {
		return err
	}
	loc := storeLocation(store)
	if str(d["status"]) != tpPending {
		return errf(http.StatusConflict, "payment_final", "This payment is already "+str(d["status"])+".", map[string]string{"status": str(d["status"])})
	}
	res := terminalResult{Status: tpCancelled, Message: "Cancelled on the till."}
	if td, terr := loadTerminalAny(storeHex, str(d["terminalId"])); terr == nil && str(d["providerRef"]) != "" {
		if p, cfg, _, cerr := terminalConfig(td); cerr == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cres, aerr := p.adapter().Cancel(ctx, cfg, payReqOf(d), str(d["providerRef"]))
			if aerr != nil {
				// the terminal may still take the card: keep it pending and say so
				return errf(http.StatusBadGateway, "cancel_failed", "Could not cancel on the card machine: "+aerr.Error()+". Press Cancel on the machine itself.", nil)
			}
			if cres.Status != "" && cres.Status != tpPending {
				res = cres
			}
		}
	}
	out, err := applyTerminalResult(storeHex, loc, d["_id"].(primitive.ObjectID), res)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, terminalPaymentOut(out))
	return nil
}

// GET /card-terminal-payments?storeId=&terminalId=&status=&limit=  (newest first)
func handleListTerminalPayments(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	storeHex := q.Get("storeId")
	if _, err := terminalStore(c, storeHex); err != nil {
		return err
	}
	if !c.Admin && !c.can("settings", "view") && !c.can("sales", "view") {
		return errForbidden("")
	}
	fs, err := parseSelect(q.Get("select"))
	if err != nil {
		return err
	}
	limit := int64(20)
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			return errBadRequest("", map[string]string{"limit": "1 to 100"})
		}
		limit = int64(n)
	}
	f := bson.M{}
	if v := q.Get("terminalId"); v != "" {
		f["terminalId"] = v
	}
	if v := q.Get("status"); v != "" {
		if v != tpPending && !tpFinal[v] {
			return errBadRequest("", map[string]string{"status": "pending, approved, declined, cancelled, failed or timeout"})
		}
		f["status"] = v
	}
	ctx, cancel := dbctx()
	defer cancel()
	col := terminalPaymentsOf(storeHex)
	total, err := col.CountDocuments(ctx, f)
	if err != nil {
		return errInternal("db: " + err.Error())
	}
	cur, err := col.Find(ctx, f, options.Find().SetSort(bson.D{{Key: "sentAtMs", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit))
	if err != nil {
		return errInternal("db: " + err.Error())
	}
	defer cur.Close(ctx)
	items := []M{}
	for cur.Next(ctx) {
		var raw bson.M
		if err := cur.Decode(&raw); err != nil {
			return errInternal("db: " + err.Error())
		}
		items = append(items, fs.apply(terminalPaymentOut(normDoc(raw))))
	}
	writeJSON(w, http.StatusOK, M{"items": items, "total": total})
	return nil
}

// ---------------------------------------------------------------- webhooks

// webhookSig signs a payment's call-back URL so only the provider we gave it
// to can poke it. The body is never trusted: a call-back only makes us ask
// the provider for the payment's status.
func webhookSig(provider, storeHex, paymentID string) string {
	m := hmac.New(sha256.New, secretKeyFn())
	m.Write([]byte("webhook|" + provider + "|" + storeHex + "|" + paymentID))
	return hex.EncodeToString(m.Sum(nil))[:32]
}

// terminalWebhookURL is the call-back a provider gets for one payment ("" when
// the server's public URL is not configured: polling still works).
func terminalWebhookURL(provider, storeHex, paymentID string) string {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("API_PUBLIC_URL")), "/")
	if base == "" {
		return ""
	}
	return base + Prefix + "/card-terminal-webhooks/" + url.PathEscape(provider) + "/" + storeHex + "/" + paymentID +
		"?sig=" + webhookSig(provider, storeHex, paymentID)
}

// POST /card-terminal-webhooks/{provider}/{store}/{payment}?sig=  (no login)
func handleTerminalWebhook(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)
	prov, storeHex, pid := v["provider"], v["store"], v["payment"]
	if terminalProviders[prov] == nil || !hmac.Equal([]byte(r.URL.Query().Get("sig")), []byte(webhookSig(prov, storeHex, pid))) {
		writeErr(w, errNotFound())
		return
	}
	if _, ok := oidOf(storeHex); !ok {
		writeErr(w, errNotFound())
		return
	}
	d, err := loadPayment(storeHex, pid)
	if err != nil || str(d["provider"]) != prov {
		writeErr(w, errNotFound())
		return
	}
	loc := riyadh
	if s := loadStoreDoc(storeHex); s != nil {
		loc = storeLocation(s)
	}
	if _, err := refreshTerminalPayment(storeHex, loc, d); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
}

func loadStoreDoc(storeHex string) M {
	oid, ok := oidOf(storeHex)
	if !ok {
		return nil
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	if err := mainDB().Collection("store").FindOne(ctx, bson.M{"_id": oid},
		options.FindOne().SetProjection(bson.M{"country_code": 1, "timezone": 1, "address": 1})).Decode(&raw); err != nil {
		return nil
	}
	return normDoc(raw)
}

func registerCardTerminals(s *mux.Router) {
	s.HandleFunc("/admin/card-terminal-providers", authed(handleAdminTerminalProviders)).Methods("GET")
	s.HandleFunc("/admin/card-terminal-providers/{id}", authed(handleAdminSaveTerminalProvider)).Methods("PUT")
	s.HandleFunc("/card-terminals/providers", authed(handleTerminalProviders)).Methods("GET")
	s.HandleFunc("/card-terminals", authed(handleListTerminals)).Methods("GET")
	s.HandleFunc("/card-terminals", authed(handleCreateTerminal)).Methods("POST")
	s.HandleFunc("/card-terminals/{id}", authed(handleUpdateTerminal)).Methods("PATCH")
	s.HandleFunc("/card-terminals/{id}", authed(handleDeleteTerminal)).Methods("DELETE")
	s.HandleFunc("/card-terminals/{id}/check", authed(handleCheckTerminal)).Methods("POST")
	s.HandleFunc("/card-terminals/{id}/payments", authed(handleStartTerminalPayment)).Methods("POST")
	s.HandleFunc("/card-terminal-payments", authed(handleListTerminalPayments)).Methods("GET")
	s.HandleFunc("/card-terminal-payments/{id}", authed(handleGetTerminalPayment)).Methods("GET")
	s.HandleFunc("/card-terminal-payments/{id}/cancel", authed(handleCancelTerminalPayment)).Methods("POST")
	s.HandleFunc("/card-terminal-webhooks/{provider}/{store}/{payment}", handleTerminalWebhook).Methods("POST", "GET")
}

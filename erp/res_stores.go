package erp

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/controller"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// serialMap maps contract serial keys to legacy store.*_serial_number fields.
var serialMap = map[string]string{
	"sales": "sales_serial_number", "salesReturn": "sales_return_serial_number",
	"purchase": "purchase_serial_number", "purchaseReturn": "purchase_return_serial_number",
	"purchaseOrder": "purchase_order_serial_number", "purchaseRequest": "purchase_request_serial_number",
	"quotation": "quotation_serial_number", "quotationReturn": "quotation_sales_return_serial_number",
	"customer": "customer_serial_number", "vendor": "vendor_serial_number", "expense": "expense_serial_number",
	"deliveryNote": "delivery_note_serial_number", "deposit": "customer_deposit_serial_number",
	"withdrawal": "customer_withdrawal_serial_number", "capital": "capital_deposit_serial_number",
	"dividend": "divident_serial_number", "stockTransfer": "stock_transfer_serial_number",
	"nonvat": "non_vat_sales_serial_number", "nonvatReturn": "non_vat_sales_return_serial_number",
	"rfq": "rfq_received_serial_number",
}

// defaultSerialPrefix are the prototype defaults (fA, L12291) for keys with
// no legacy counter.
var defaultSerialPrefix = map[string]string{
	"proforma": "PI-", "repairJob": "JOB-", "capitalWithdrawal": "CWD-", "salary": "SAL-",
	"purchaseBill": "BILL-", "product": "P-", "employee": "EMP-", "vehicle": "VEH-",
}

// settingsFlag maps contract store.flags keys onto legacy settings keys.
var settingsFlag = map[string]string{
	"enable_warehouse_module":  "enable_warehouse_module",
	"enable_automobile_module": "enable_automobile_module",
	"enable_employee_module":   "enable_employee_module",
	"enable_ai_rfq_bot":        "enable_ai_rfq_bot",
	"enable_rbac_module":       "enable_rbac_module",
	"enable_purchase_bills":    "enable_purchase_bills_tracking",
	"enable_customer_po":       "enable_customer_po_no",
}

var reNonUpper = regexp.MustCompile(`[^A-Z]`)

// storeShort returns the contract store.short (2–5 capitals). Legacy stores
// have no such field: an explicitly saved erp.x.short wins, else it is derived
// from the legacy code / name (never written back to legacy).
func storeShort(st M) string {
	if st == nil {
		return "MAIN"
	}
	if s := str(get(st, "erp.x.short")); reShort.MatchString(s) {
		return s
	}
	code := strings.ToUpper(str(st["code"]))
	if reShort.MatchString(code) {
		return code
	}
	letters := reNonUpper.ReplaceAllString(code, "")
	if len(letters) >= 2 {
		if len(letters) > 5 {
			letters = letters[:5]
		}
		return letters
	}
	words := strings.Fields(strings.ToUpper(str(st["name"])))
	ini := ""
	for _, w := range words {
		w = reNonUpper.ReplaceAllString(w, "")
		if w != "" {
			ini += w[:1]
		}
	}
	if len(ini) >= 2 {
		if len(ini) > 5 {
			ini = ini[:5]
		}
		return ini
	}
	return "MAIN"
}

// serialCfg returns (prefix, start) for a serial key in contract terms.
func serialCfg(st M, key string) (string, int) {
	if lk, ok := serialMap[key]; ok && st != nil {
		sn := sub(st, lk)
		if p := str(sn["prefix"]); p != "" {
			start := int(intv(sn["start_from_count"]))
			if start < 1 {
				start = 1
			}
			return strings.TrimSuffix(p, "-") + "-", start
		}
	}
	if st != nil {
		if x := sub(st, "erp.x.serials."+key); len(x) > 0 && str(x["prefix"]) != "" {
			s := int(intv(x["start"]))
			if s < 1 {
				s = 1
			}
			return str(x["prefix"]), s
		}
	}
	if p, ok := defaultSerialPrefix[key]; ok {
		return p, 1
	}
	return strings.ToUpper(key) + "-", 1
}

// setting reads settings.x falling back to the legacy top-level x (§2.1c).
func setting(st M, key string) interface{} {
	if v, ok := sub(st, "settings")[key]; ok && v != nil {
		return v
	}
	return st[key]
}

const masked = "••••"

func mask(s string) string {
	if s == "" {
		return ""
	}
	return masked
}

// addressFromLegacy maps national_address (+country) to the contract address.
func addressFromLegacy(na M, countryName string) M {
	a := M{
		"buildingNo": str(na["building_no"]), "streetEn": str(na["street_name"]), "streetAr": str(na["street_name_arabic"]),
		"districtEn": str(na["district_name"]), "districtAr": str(na["district_name_arabic"]),
		"cityEn": str(na["city_name"]), "cityAr": str(na["city_name_arabic"]),
		"postalCode": str(na["zipcode"]), "additionalNo": str(na["additional_no"]),
		"countryEn": countryName, "countryAr": "",
	}
	if sc := str(na["short_code"]); sc != "" {
		a["shortAddress"] = sc
	}
	if u := str(na["unit_no"]); u != "" {
		a["unitNo"] = u
	}
	if countryName == "" || strings.EqualFold(countryName, "Saudi Arabia") {
		a["countryEn"] = "Saudi Arabia"
		a["countryAr"] = "المملكة العربية السعودية"
	}
	return a
}

// addressToLegacy maps a contract address onto a legacy national_address.
func addressToLegacy(a M) M {
	return M{
		"building_no": str(a["buildingNo"]), "building_no_arabic": toArabicDigits(str(a["buildingNo"])),
		"street_name": str(a["streetEn"]), "street_name_arabic": str(a["streetAr"]),
		"district_name": str(a["districtEn"]), "district_name_arabic": str(a["districtAr"]),
		"city_name": str(a["cityEn"]), "city_name_arabic": str(a["cityAr"]),
		"zipcode": str(a["postalCode"]), "zipcode_arabic": toArabicDigits(str(a["postalCode"])),
		"additional_no": str(a["additionalNo"]), "additional_no_arabic": toArabicDigits(str(a["additionalNo"])),
		"short_code": str(a["shortAddress"]), "unit_no": str(a["unitNo"]),
	}
}

// storeHasZatcaDocs reports whether the store issued any sales, sales
// returns, debit notes or credit notes (deleted ones included).
var storeHasZatcaDocs = func(d M) bool {
	oid, ok := oidOf(d["_id"])
	if !ok {
		oid, ok = oidOf(d["id"])
	}
	if !ok {
		return false
	}
	ctx, cancel := dbctx()
	defer cancel()
	one := int64(1)
	for _, c := range models.ZatcaDocumentCollections {
		n, err := storeDB(oid.Hex()).Collection(c).CountDocuments(ctx, bson.M{"store_id": oid}, &options.CountOptions{Limit: &one})
		if err == nil && n > 0 {
			return true
		}
	}
	return false
}

// zatcaEnv normalises a ZATCA environment name to the legacy value
// (NonProduction | Simulation | Production); "" when unknown.
func zatcaEnv(v string) string {
	switch strings.ToLower(strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.TrimSpace(v))) {
	case "nonproduction", "sandbox", "developerportal":
		return "NonProduction"
	case "simulation":
		return "Simulation"
	case "production":
		return "Production"
	}
	return ""
}

func toArabicDigits(s string) string {
	return strings.NewReplacer("0", "٠", "1", "١", "2", "٢", "3", "٣", "4", "٤", "5", "٥", "6", "٦", "7", "٧", "8", "٨", "9", "٩").Replace(s)
}

func storeToContract(x *mapCtx, d M) M {
	st := sub(d, "settings")
	loc := storeLocation(d)
	rec := M{
		"nameEn": str(d["name"]), "nameAr": str(d["name_in_arabic"]),
		"branchEn": str(d["branch_name"]),
		"short":    storeShort(d),
		"vatNo":    str(d["vat_no"]), "crNo": str(d["registration_number"]),
		"category": str(d["business_category"]),
		// the POS terminal this store's business category opens ("" = none)
		"posTerminal": CategoryTerminal(str(d["business_category"])),
		"address":     addressFromLegacy(sub(d, "national_address"), str(d["country_name"])),
		"phone":       str(d["phone"]), "email": str(d["email"]),
		"vatPercent": func() float64 {
			if v := num(d["vat_percent"]); v > 0 {
				return v
			}
			return 15
		}(),
		"currency": M{"code": "SAR", "nameEn": "Saudi Riyal", "nameAr": "ريال سعودي", "fractionEn": "Halala", "fractionAr": "هللة"},
		// derived from the legacy country_code; server-owned (not writable)
		"timezone":    storeTimezoneName(d),
		"countryCode": storeCountryOrSA(d),
		"bank": M{"name": str(get(d, "bank_account.bank_name")), "accountName": str(get(d, "bank_account.account_name")),
			"accountNo": str(get(d, "bank_account.account_no")), "iban": str(get(d, "bank_account.iban"))},
		"logo":      nilIfEmpty(str(d["logo"])),
		"invoiceBg": nilIfEmpty(str(d["invoice_background"])),
	}
	// titles (partially mapped)
	inv := sub(st, "invoice")
	titles := M{}
	if v := str(d["title"]); v != "" {
		titles["invoiceEn"] = v
	}
	if v := str(d["title_in_arabic"]); v != "" {
		titles["invoiceAr"] = v
	}
	if v := str(inv["quotation_title"]); v != "" {
		titles["quotationEn"] = v
	}
	if v := str(inv["delivery_note_title"]); v != "" {
		titles["deliveryEn"] = v
	}
	// Debit/Credit note default titles (§2.1a): legacy receivabale_title/payable_title
	rt := str(inv["receivabale_title"])
	if rt == "" {
		rt = "Debit Note"
	}
	pt := str(inv["payable_title"])
	if pt == "" {
		pt = "Credit Note"
	}
	titles["debitNoteEn"], titles["creditNoteEn"] = rt, pt
	rec["titles"] = titles
	// serials
	serials := M{}
	for k := range serialMap {
		p, s := serialCfg(d, k)
		serials[k] = M{"prefix": p, "start": s}
	}
	for k := range defaultSerialPrefix {
		p, s := serialCfg(d, k)
		serials[k] = M{"prefix": p, "start": s}
	}
	rec["serials"] = serials
	// flags
	flags := M{}
	for ck, lk := range settingsFlag {
		flags[ck] = boolv(setting(d, lk))
	}
	rec["flags"] = flags
	// zatca (secrets never exposed)
	z := sub(d, "zatca")
	phase := 1
	if str(z["phase"]) == "2" {
		phase = 2
	}
	zc := M{"phase": phase, "connected": boolv(z["connected"]), "connectedAt": fmtDTIn(loc, z["last_connected_at"]),
		"reconnectNeeded": boolv(z["zatca_reconnect_required"]), "env": str(z["env"])}
	if id := intv(z["production_request_id"]); id != 0 {
		zc["pcsid"] = str(id)
	} else if boolv(z["connected"]) {
		zc["pcsid"] = "legacy"
	} else {
		zc["pcsid"] = ""
	}
	if t := fmtDTIn(loc, z["last_disconnected_at"]); t != "" {
		zc["disconnectedAt"] = t
	}
	// once ZATCA documents exist the environment can't change (UI lock hint)
	zc["envLocked"] = storeHasZatcaDocs(d)
	rec["zatca"] = zc
	// evolution.status and waba have no legacy field: they live in erp.x and
	// are merged in by applyEnvelope, where mapped keys win. WhatsApp is
	// WABA-only, so the mode is always "waba", whatever an older client saved.
	rec["whatsapp"] = M{"mode": "waba", "evolution": M{
		"url": str(st["evolution_api_url"]), "instance": str(st["evolution_instance_name"]),
		"apiKey": mask(str(st["evolution_api_key"])),
	}}
	// an unset SMTP port is null, not 0: the client rejects port 0 (1..65535),
	// which blocked saving any settings tab
	var smtpPort interface{}
	if p := intv(st["outgoing_email_smtp_port"]); p > 0 {
		smtpPort = p
	}
	rec["emailSettings"] = M{"smtp": M{
		"host": str(st["outgoing_email_smtp_host"]), "port": smtpPort,
		"user": str(st["outgoing_email_smtp_username"]), "fromName": str(st["outgoing_email_from_name"]),
		"fromEmail": str(st["outgoing_email_from_address"]), "tls": boolv(st["outgoing_email_smtp_use_tls"]),
	}}
	rec["google"] = M{"mapsKey": mask(str(st["google_maps_api_key"]))}
	rec["rfq"] = M{"intro": str(st["rfq_intro"])}
	rec["ai"] = M{"provider": str(st["rfq_llm_provider"]), "model": str(st["rfq_llm_model"]), "apiKey": mask(str(st["rfq_llm_api_key"]))}
	rec["purchaseBills"] = M{"enabled": boolv(st["enable_purchase_bills_tracking"])}
	rec["openingBalances"] = M{"cash": num(st["cash_opening_balance"]), "bank": num(st["bank_opening_balance"]),
		"asOf": fmtDayIn(loc, st["cash_opening_balance_date"])}
	rec["legacyCode"] = str(d["code"])
	return rec
}

var storeKnown = knownSet("nameEn", "nameAr", "branchEn", "vatNo", "crNo", "category", "address", "phone", "email",
	"vatPercent", "bank", "logo", "invoiceBg", "titles", "serials", "flags", "zatca", "whatsapp", "emailSettings",
	"google", "rfq", "ai", "purchaseBills", "openingBalances", "legacyCode", "currency", "timezone", "countryCode", "posTerminal")

func storeValidate(x *mapCtx, rec M, prev M) map[string]string {
	e := map[string]string{}
	req := func(k string) {
		if strings.TrimSpace(str(rec[k])) == "" {
			e[k] = "required"
		}
	}
	req("nameEn")
	// business category: sent to ZATCA on Phase 2 onboarding and picks the POS
	// terminal, so a new or changed value must be one of BusinessCategories.
	// An unchanged legacy free-text value is still accepted.
	cat := strings.TrimSpace(str(rec["category"]))
	if prev == nil || cat != strings.TrimSpace(str(prev["business_category"])) {
		if cat == "" {
			e["category"] = "required"
		} else if _, ok := CanonicalCategory(cat); !ok {
			e["category"] = "choose a business category from the list"
		}
	}
	if s := str(rec["short"]); s != "" && !reShort.MatchString(s) {
		e["short"] = "2-5 capital letters"
	}
	if v := str(rec["vatNo"]); v != "" && !ValidVAT(v) {
		e["vatNo"] = "VAT No. must be 15 digits starting and ending with 3"
	}
	if v := str(rec["crNo"]); v != "" && !ValidCR(v) {
		e["crNo"] = "CR No. must be 10 digits"
	}
	if v := num(rec["vatPercent"]); v < 0 || v > 100 {
		e["vatPercent"] = "0..100"
	}
	if a, ok := rec["address"].(M); ok {
		if b := str(a["buildingNo"]); b != "" && !re4.MatchString(b) {
			e["address.buildingNo"] = "4 digits"
		}
		if p := str(a["postalCode"]); p != "" && !re5.MatchString(p) {
			e["address.postalCode"] = "5 digits"
		}
		if p := str(a["additionalNo"]); p != "" && !re4.MatchString(p) {
			e["address.additionalNo"] = "4 digits"
		}
	}
	if v := str(rec["email"]); v != "" && !validEmail(v) {
		e["email"] = "invalid email"
	}
	if v := str(get(rec, "zatca.env")); v != "" && zatcaEnv(v) == "" {
		e["zatca.env"] = "NonProduction, Simulation or Production"
	}
	if s, ok := rec["serials"].(M); ok {
		for k, v := range s {
			vm, _ := v.(M)
			if vm == nil {
				continue
			}
			if p := str(vm["prefix"]); !regexp.MustCompile(`^[A-Za-z0-9-]{0,16}$`).MatchString(p) {
				e["serials."+k+".prefix"] = "letters, digits and - only (max 16)"
			}
			if st, ok := vm["start"]; ok && (num(st) < 1 || num(st) != float64(intv(st))) {
				e["serials."+k+".start"] = "integer >= 1"
			}
		}
	}
	return e
}

func storeToLegacy(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
	p := M{}
	set := func(ck, lk string, v interface{}) {
		if ch[ck] {
			p[lk] = v
		}
	}
	set("nameEn", "name", str(rec["nameEn"]))
	set("nameAr", "name_in_arabic", str(rec["nameAr"]))
	set("branchEn", "branch_name", str(rec["branchEn"]))
	if ch["vatNo"] {
		p["vat_no"] = str(rec["vatNo"])
		p["vat_no_in_arabic"] = toArabicDigits(str(rec["vatNo"]))
	}
	if ch["crNo"] {
		p["registration_number"] = str(rec["crNo"])
		p["registration_number_in_arabic"] = toArabicDigits(str(rec["crNo"]))
	}
	if ch["category"] {
		cat := strings.TrimSpace(str(rec["category"]))
		if v, ok := CanonicalCategory(cat); ok {
			cat = v
		}
		p["business_category"] = cat
	}
	if ch["phone"] {
		p["phone"] = str(rec["phone"])
		p["phone_in_arabic"] = toArabicDigits(str(rec["phone"]))
	}
	set("email", "email", str(rec["email"]))
	if ch["vatPercent"] {
		p["vat_percent"] = num(rec["vatPercent"])
	}
	if ch["address"] {
		a := sub(rec, "address")
		p["national_address"] = addressToLegacy(a)
		if c := str(a["countryEn"]); c != "" {
			p["country_name"] = c
		}
	}
	if ch["bank"] {
		b := sub(rec, "bank")
		p["bank_account"] = M{"bank_name": str(b["name"]), "account_name": str(b["accountName"]),
			"account_no": str(b["accountNo"]), "iban": str(b["iban"]), "customer_no": str(get(prev, "bank_account.customer_no"))}
	}
	if ch["logo"] {
		l := str(rec["logo"])
		if l == "" {
			p["remove_logo"] = true
		} else if strings.HasPrefix(l, "data:") {
			p["logo_content"] = l
		}
	}
	if ch["invoiceBg"] {
		l := str(rec["invoiceBg"])
		if l == "" {
			p["remove_invoice_background"] = true
		} else if strings.HasPrefix(l, "data:") {
			p["invoice_background_content"] = l
		}
	}
	if ch["titles"] {
		t := sub(rec, "titles")
		p["title"] = str(t["invoiceEn"])
		p["title_in_arabic"] = str(t["invoiceAr"])
	}
	settings := M{}
	if ch["flags"] {
		f := sub(rec, "flags")
		for ck, lk := range settingsFlag {
			if v, ok := f[ck]; ok {
				settings[lk] = boolv(v)
			}
		}
	}
	if ch["whatsapp"] {
		ev := sub(rec, "whatsapp.evolution")
		settings["evolution_api_url"] = str(ev["url"])
		settings["evolution_instance_name"] = str(ev["instance"])
		if k := str(ev["apiKey"]); k != masked {
			settings["evolution_api_key"] = k
		}
	}
	if ch["emailSettings"] {
		sm := sub(rec, "emailSettings.smtp")
		settings["outgoing_email_smtp_host"] = str(sm["host"])
		settings["outgoing_email_smtp_port"] = intv(sm["port"])
		settings["outgoing_email_smtp_username"] = str(sm["user"])
		settings["outgoing_email_from_name"] = str(sm["fromName"])
		settings["outgoing_email_from_address"] = str(sm["fromEmail"])
		settings["outgoing_email_smtp_use_tls"] = boolv(sm["tls"])
	}
	if ch["google"] {
		if k := str(get(rec, "google.mapsKey")); k != masked {
			settings["google_maps_api_key"] = k
		}
	}
	if ch["rfq"] {
		settings["rfq_intro"] = str(get(rec, "rfq.intro"))
	}
	if ch["ai"] {
		ai := sub(rec, "ai")
		settings["rfq_llm_provider"] = str(ai["provider"])
		settings["rfq_llm_model"] = str(ai["model"])
		if k := str(ai["apiKey"]); k != masked {
			settings["rfq_llm_api_key"] = k
		}
	}
	if ch["purchaseBills"] {
		settings["enable_purchase_bills_tracking"] = boolv(get(rec, "purchaseBills.enabled"))
	}
	if ch["openingBalances"] {
		ob := sub(rec, "openingBalances")
		settings["cash_opening_balance"] = num(ob["cash"])
		settings["bank_opening_balance"] = num(ob["bank"])
	}
	if ch["titles"] {
		t := sub(rec, "titles")
		inv := sub(sub(prev, "settings"), "invoice")
		inv = cloneM(inv)
		if v, ok := t["quotationEn"]; ok {
			inv["quotation_title"] = str(v)
		}
		if v, ok := t["deliveryEn"]; ok {
			inv["delivery_note_title"] = str(v)
		}
		if v, ok := t["debitNoteEn"]; ok {
			inv["receivable_title"] = str(v) // json tag of bson receivabale_title
		}
		if v, ok := t["creditNoteEn"]; ok {
			inv["payable_title"] = str(v)
		}
		delete(inv, "receivabale_title")
		settings["invoice"] = inv
	}
	if len(settings) > 0 {
		p["settings"] = settings
	}
	if ch["serials"] {
		s := sub(rec, "serials")
		for ck, lk := range serialMap {
			v, ok := s[ck].(M)
			if !ok {
				continue
			}
			old := sub(prev, lk)
			prefix := strings.TrimSuffix(str(v["prefix"]), "-")
			if prefix == "" {
				prefix = str(old["prefix"])
			}
			start := intv(v["start"])
			if start < 1 {
				start = intv(old["start_from_count"])
			}
			pad := intv(old["padding_count"])
			if pad == 0 {
				pad = 4
			}
			if prefix != str(old["prefix"]) || start != intv(old["start_from_count"]) {
				p[lk] = M{"prefix": prefix, "start_from_count": start, "padding_count": pad}
			}
		}
	}
	// derived Arabic-digit copies the legacy validation requires on every
	// save: filled from the Latin value when missing (never overwritten)
	if len(p) > 0 || len(settings) > 0 {
		for _, k := range [][2]string{{"phone", "phone_in_arabic"}, {"vat_no", "vat_no_in_arabic"}, {"registration_number", "registration_number_in_arabic"}} {
			latin := str(p[k[0]])
			if latin == "" {
				latin = str(prev[k[0]])
			}
			legacyAr := str(prev[k[1]])
			if k[1] == "registration_number_in_arabic" {
				legacyAr = str(prev["registration_number_arabic"])
			}
			if _, set := p[k[1]]; !set && legacyAr == "" && latin != "" {
				p[k[1]] = toArabicDigits(latin)
			}
		}
	}
	if ch["zatca"] {
		// only phase and env are client-writable (rule 60); credentials and
		// connection state are server-owned. The legacy store update enforces
		// who may change env and when (controller.zatcaEnvChangeError).
		pz := sub(prev, "zatca")
		ph, env := str(pz["phase"]), str(pz["env"])
		if v := str(get(rec, "zatca.phase")); v == "1" || v == "2" {
			ph = v
		}
		if v := zatcaEnv(str(get(rec, "zatca.env"))); v != "" {
			env = v
		}
		if ph != str(pz["phase"]) || env != str(pz["env"]) {
			p["zatca"] = M{"phase": ph, "env": env}
		}
	}
	return p, nil
}

func newStoresResource() *Resource {
	b := &legacyBackend{
		coll: "store", inMain: true, mainOrg: true, deletedKey: "deleted",
		toC: storeToContract, toL: storeToLegacy, known: storeKnown, validate: storeValidate,
		hybrid: knownSet("currency", "titles", "serials", "flags", "whatsapp", "emailSettings", "rfq", "ai", "purchaseBills", "openingBalances", "bank", "address", "zatca"),
		access: func(c *Ctx) bson.M {
			ids := []primitive.ObjectID{}
			for _, s := range c.Stores {
				if id, ok := oidOf(s["_id"]); ok {
					ids = append(ids, id)
				}
			}
			return bson.M{"_id": bson.M{"$in": ids}}
		},
		v1: v1Ops{path: "/v1/store", update: controller.UpdateStore, noStoreParam: true},
		fieldErr: map[string]string{
			"name": "nameEn", "name_in_arabic": "nameAr", "branch_name": "branchEn", "vat_no": "vatNo",
			"registration_number": "crNo", "business_category": "category", "phone": "phone", "email": "email",
			"vat_percent": "vatPercent", "role": "role",
			"national_address_building_no": "address.buildingNo", "national_address_street_name": "address.streetEn",
			"national_address_district_name": "address.districtEn", "national_address_city_name": "address.cityEn",
			"national_address_zipcode": "address.postalCode", "national_address_street_name_arabic": "address.streetAr",
			"national_address_district_name_arabic": "address.districtAr", "national_address_city_name_arabic": "address.cityAr",
			"phone_in_arabic": "phone", "vat_no_in_arabic": "vatNo", "registration_number_in_arabic": "crNo", "country_code": "address.countryEn",
			"code": "legacyCode", "zatca_env": "zatca.env",
		},
		noDelete: "Stores cannot be deleted from StartERP.",
	}
	return &Resource{Name: "stores", Path: "stores", Scope: "org", Module: "settings", Legacy: "main DB `store`", Backend: &storesBackend{b}}
}

// storeBillingFields are kept from the stored record on every PATCH/PUT.
var storeBillingFields = []string{"plan", "trialEndsAt", "subscription"}

// storesBackend: stores are never created/deleted through the contract;
// PATCH keeps `short` (and other unmapped fields) in erp.x.
type storesBackend struct{ *legacyBackend }

// Create: a StartERP platform admin (legacy Admin) adds a store. Everyone
// else gets their store through sign-up. The store is built exactly like a
// sign-up's (serials, ZATCA phase 2, store DB and indexes).
func (s *storesBackend) Create(c *Ctx, storeHex string, body M, meta WriteMeta) (M, error) {
	if !c.Admin {
		return nil, errForbidden("Only StartERP admins can add stores. New businesses sign up.")
	}
	rec := stripServerOwned(body)
	x := newMapCtx(c, "")
	errs := storeValidate(x, rec, nil)
	for k, v := range storeCreateErrors(rec) {
		if _, dup := errs[k]; !dup {
			errs[k] = v
		}
	}
	if len(errs) > 0 {
		return nil, errBadRequest("", errs)
	}
	cat, _ := CanonicalCategory(str(rec["category"]))
	a := sub(rec, "address")
	var na models.NationalAddress
	if raw, err := json.Marshal(addressToLegacy(a)); err == nil {
		_ = json.Unmarshal(raw, &na)
	}
	now := time.Now()
	st := controller.NewRegistrationStore(controller.GuestRegisterRequest{
		Name: c.UserName, Email: strings.TrimSpace(str(rec["email"])),
		StoreName: strings.TrimSpace(str(rec["nameEn"])), StoreNameInArabic: strings.TrimSpace(str(rec["nameAr"])),
		BusinessCategory: cat, RegistrationNumber: strings.TrimSpace(str(rec["crNo"])), VATNo: strings.TrimSpace(str(rec["vatNo"])),
		Phone: cleanPhone(str(rec["phone"])), CountryCode: "SA", CountryName: "Saudi Arabia", ZatcaPhase: "2",
		NationalAddress: na,
	}, now)
	if b := strings.TrimSpace(str(rec["branchEn"])); b != "" {
		st.BranchName = b
	}
	if uid, ok := oidOf(c.User["_id"]); ok {
		st.CreatedBy, st.UpdatedBy = &uid, &uid
	}
	if err := st.Insert(); err != nil {
		return nil, errInternal("Unable to create the store: " + err.Error())
	}
	if _, err := st.CreateDB(); err != nil {
		return nil, errInternal("Unable to create the store database: " + err.Error())
	}
	if err := st.CreateAllIndexes(); err != nil {
		return nil, errInternal("Unable to create the store indexes: " + err.Error())
	}
	hex := st.ID.Hex()
	short := str(rec["short"])
	if !reShort.MatchString(short) {
		short = deriveShort(str(rec["nameEn"]))
	}
	branchAr := strings.TrimSpace(str(rec["branchAr"]))
	if branchAr == "" {
		branchAr = "الفرع الرئيسي"
	}
	loc := storeLocation(M{"country_code": st.CountryCode})
	env := M{"v": int64(1), "h": []interface{}{historyEntryIn(loc, c.UserName, "created", []interface{}{})}, "cb": c.UserName,
		"x": M{"short": short, "branchAr": branchAr, "plan": "professional",
			"trialEndsAt":  now.AddDate(0, 0, 14).In(loc).Format(layoutDay),
			"businessType": cat, "address": M{"countryAr": "المملكة العربية السعودية", "shortAddress": str(a["shortAddress"])}}}
	if err := s.setEnv("", hex, env); err != nil {
		return nil, err
	}
	// the admin's own store list is every store (legacy Admin), so the new
	// store is in c.Stores once reloaded
	if err := c.loadAccess(); err != nil {
		return nil, err
	}
	return s.Get(c, "", hex, true)
}

// storeCreateErrors: the fields a new store needs beyond storeValidate
// (the legacy store validation requires them; reported with contract keys).
func storeCreateErrors(rec M) map[string]string {
	e := map[string]string{}
	need := func(k, v, msg string) {
		if strings.TrimSpace(v) == "" {
			e[k] = msg
		}
	}
	need("nameAr", str(rec["nameAr"]), "required")
	if n := str(rec["nameAr"]); n != "" && !hasArabic(n) {
		e["nameAr"] = "must contain Arabic letters"
	}
	need("vatNo", str(rec["vatNo"]), "required")
	need("crNo", str(rec["crNo"]), "required")
	need("email", str(rec["email"]), "required")
	if p := str(rec["phone"]); p == "" {
		e["phone"] = "required"
	} else if !ValidSaudiPhone(p) {
		e["phone"] = "Saudi phone number"
	}
	a := sub(rec, "address")
	if !re4.MatchString(str(a["buildingNo"])) {
		e["address.buildingNo"] = "4 digits"
	}
	need("address.streetEn", str(a["streetEn"]), "required")
	if v := str(a["streetAr"]); v == "" || !hasArabic(v) {
		e["address.streetAr"] = "Arabic street name required"
	}
	need("address.districtEn", str(a["districtEn"]), "required")
	if v := str(a["districtAr"]); v == "" || !hasArabic(v) {
		e["address.districtAr"] = "Arabic district name required"
	}
	need("address.cityEn", str(a["cityEn"]), "required")
	if !re5.MatchString(str(a["postalCode"])) {
		e["address.postalCode"] = "5 digits"
	}
	return e
}

func (s *storesBackend) Delete(c *Ctx, storeHex, id string, meta WriteMeta) (M, error) {
	return nil, errForbidden("Stores cannot be deleted from StartERP.")
}

func (s *storesBackend) HardDelete(c *Ctx, storeHex, id string, meta WriteMeta) error {
	return errForbidden("Stores cannot be deleted from StartERP.")
}

// short, branchAr, plan, … are not legacy fields: they are kept in erp.x
// automatically (unknown keys) and `short` is read back by storeShort().

// Update keeps the server-owned zatca fields (rule 60): only zatca.phase is
// taken from the client; connected/pcsid/connectedAt/certExpires/snapshot are
// restored from the current record before anything is persisted.
func (s *storesBackend) Update(c *Ctx, storeHex, id string, prev, next M, changed []string, meta WriteMeta) (M, error) {
	// billing fields are server-owned: only an accepted bank-transfer
	// payment (billing.go) changes them
	for _, k := range storeBillingFields {
		if v, ok := prev[k]; ok {
			next[k] = v
		} else {
			delete(next, k)
		}
	}
	if nz, ok := next["zatca"].(M); ok {
		z := cloneM(sub(prev, "zatca"))
		if ph, ok := nz["phase"]; ok {
			z["phase"] = ph
		}
		if v := str(nz["env"]); v != "" {
			env := zatcaEnv(v)
			if env == "" {
				return nil, errBadRequest("", map[string]string{"zatca.env": "NonProduction, Simulation or Production"})
			}
			z["env"] = env
		}
		next["zatca"] = z
	}
	return s.legacyBackend.Update(c, storeHex, id, prev, next, changed, meta)
}

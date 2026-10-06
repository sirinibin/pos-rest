package erp

import (
	"math"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// linkF maps a contract link (id + optional code) to legacy fields.
type linkF struct {
	c, l         string // contract id key, legacy id key
	coll         string // legacy collection of the target (for erp.cid lookup)
	codeC, codeL string // optional code fields
	required     bool
}

// docCfg configures the generic document mapper (sales, returns, purchases…).
type docCfg struct {
	party           string // "customer" | "vendor" | ""
	priceKey        string // legacy line price key
	costKey         string // legacy line cost key ("" = none)
	retailWholesale bool   // purchase lines carry retail/wholesale prices
	selectedLines   bool   // return docs: lines carry selected:true
	noWarehouse     bool   // legacy lines have no warehouse fields
	payments        bool   // contract payments ↔ legacy payments / payments_input
	payColl         string // legacy payment collection (fallback read for old docs)
	payLink         string // payment → document link key
	forceVAT0       bool
	zatca           string // "" | "invoice" | "credit"
	links           []linkF
	cashDiscount    bool
	commission      bool
	rounding        bool
	shipping        bool
	discount        bool
	remarksKey      string // legacy key for remarks ("remarks" default, "notes" for PR)
	extra           []fld
	allowZeroPrice  bool
	codeRO          bool // code assigned by legacy counter (serverNumbers)
}

// fld is a simple scalar mapping.
type fld struct {
	c, l string
	k    kind
	ref  string // for kRef: legacy collection
	ro   bool   // read-only (never written)
}

type kind int

const (
	kStr kind = iota
	kNum
	kInt
	kBool
	kDT      // legacy time ↔ contract datetime; written as <l>_str (RFC3339)
	kDay     // legacy time ↔ contract date
	kRef     // legacy ObjectID ↔ contract id
	kRefs    // legacy []ObjectID ↔ contract []id
	kStrs    // []string
	kRaw     // as-is
	kDTPlain // legacy time ↔ contract datetime; written as RFC3339 into <l> itself
)

func readFld(x *mapCtx, d M, f fld) interface{} {
	v := d[f.l]
	switch f.k {
	case kStr:
		return str(v)
	case kNum:
		return num(v)
	case kInt:
		return intv(v)
	case kBool:
		return boolv(v)
	case kDT, kDTPlain:
		return fmtDT(v)
	case kDay:
		return fmtDay(v)
	case kRef:
		return idOrNil(v)
	case kRefs:
		return ids(v)
	case kStrs:
		return strs(v)
	}
	return v
}

func writeFld(x *mapCtx, p M, rec M, f fld) error {
	if f.ro {
		return nil
	}
	v := rec[f.c]
	switch f.k {
	case kStr:
		p[f.l] = str(v)
	case kNum:
		p[f.l] = num(v)
	case kInt:
		p[f.l] = intv(v)
	case kBool:
		p[f.l] = boolv(v)
	case kDT, kDay:
		if s := str(v); s != "" {
			ds, err := toLegacyDateStr(s)
			if err != nil {
				return errBadRequest("", map[string]string{f.c: "invalid date"})
			}
			p[f.l+"_str"] = ds
		} else {
			p[f.l] = nil
		}
	case kDTPlain:
		if s := str(v); s != "" {
			ds, err := toLegacyDateStr(s)
			if err != nil {
				return errBadRequest("", map[string]string{f.c: "invalid date"})
			}
			p[f.l] = ds
		} else {
			p[f.l] = nil
		}
	case kRef:
		h, err := x.ref(f.ref, v)
		if err != nil {
			return errBadRequest("", map[string]string{f.c: "unknown id"})
		}
		p[f.l] = h
	case kRefs:
		out := []interface{}{}
		for _, e := range arr(v) {
			h, err := x.ref(f.ref, e)
			if err != nil {
				return errBadRequest("", map[string]string{f.c: "unknown id " + str(e)})
			}
			if h != nil {
				out = append(out, h)
			}
		}
		p[f.l] = out
	case kStrs:
		p[f.l] = strs(v)
	default:
		p[f.l] = v
	}
	return nil
}

func docVAT(d M, def float64) float64 {
	if v, ok := d["vat_percent"]; ok && v != nil {
		return num(v)
	}
	return def
}

func (cfg *docCfg) lineToContract(x *mapCtx, l M, vat float64) M {
	it := M{
		"productId": idOrNil(l["product_id"]), "partNo": str(l["part_number"]),
		"nameEn": str(l["name"]), "nameAr": str(l["name_in_arabic"]), "unit": str(l["unit"]),
		"qty": num(l["quantity"]), "unitPrice": num(l[cfg.priceKey]), "unitDiscount": num(l["unit_discount"]),
		"vatPercent": vat,
	}
	if cfg.costKey != "" {
		it["purchasePrice"] = num(l[cfg.costKey])
	} else {
		it["purchasePrice"] = num(l[cfg.priceKey])
	}
	if !cfg.noWarehouse {
		it["warehouseId"] = x.whContractID(l["warehouse_id"], l["warehouse_code"])
	}
	if cfg.retailWholesale {
		it["retailPrice"] = num(l["retail_unit_price"])
		it["wholesalePrice"] = num(l["wholesale_unit_price"])
	}
	if p := str(l["prefix_part_number"]); p != "" {
		it["prefixPartNo"] = p
	}
	if boolv(l["is_service"]) {
		it["isService"] = true
	}
	if r := num(l["quantity_returned"]); r != 0 {
		it["qtyReturned"] = r
	}
	return it
}

func zatcaToContract(d M, kindOf string) M {
	z := sub(d, "zatca")
	status := "not_reported"
	if boolv(z["reporting_passed"]) {
		if boolv(z["is_simplified"]) {
			status = "reported"
		} else {
			status = "cleared"
		}
	} else if intv(z["reporting_failed_count"]) > 0 || intv(z["compliance_check_failed_count"]) > 0 {
		status = "failed"
	}
	simplified := boolv(z["is_simplified"])
	if !boolv(z["reporting_passed"]) {
		simplified = !ValidVAT(str(d["vat_no"]))
	}
	inv := "standard"
	if simplified {
		inv = "simplified"
	}
	if kindOf == "credit" {
		inv = "credit"
		if simplified {
			inv = "credit-simplified"
		}
	}
	out := M{"status": status, "invoiceType": inv, "uuid": str(d["uuid"]), "hash": str(d["hash"]),
		"pih": str(d["prev_hash"]), "icv": intv(d["invoice_count_value"]), "qr": str(z["qr_code"]),
		"signature": str(z["ecdsa_signature"]), "reportedAt": fmtDT(z["reporting_passed_at"]),
		"xmlUrl": str(z["cleared_xml_url"]), "error": ""}
	if errs := arr(z["reporting_errors"]); len(errs) > 0 {
		out["error"] = str(errs[len(errs)-1])
	} else if errs := arr(z["compliance_check_errors"]); len(errs) > 0 && status == "failed" {
		out["error"] = str(errs[len(errs)-1])
	}
	if t := fmtDT(z["reporting_last_failed_at"]); t != "" {
		out["attemptAt"] = t
	}
	return out
}

func paymentToContract(p M, defDate interface{}) M {
	d := p["date"]
	if d == nil {
		d = defDate
	}
	out := M{"id": hexOf(p["_id"]), "date": fmtDT(d), "amount": num(p["amount"]), "method": str(p["method"]),
		"description": str(p["description"])}
	if out["id"] == "" {
		out["id"] = hexOf(p["id"])
	}
	if ref := str(p["reference_type"]); ref != "" {
		out["referenceType"] = ref
		out["referenceId"] = idOrNil(p["reference_id"])
		out["referenceCode"] = str(p["reference_code"])
	}
	if br := str(p["bank_reference"]); br != "" {
		out["reference"] = br
	}
	return out
}

func (cfg *docCfg) paymentsToContract(x *mapCtx, d M) []interface{} {
	out := []interface{}{}
	ps := arr(d["payments"])
	if len(ps) == 0 && cfg.payColl != "" && num(d["total_payment_received"])+num(d["total_payment_paid"]) > 0 {
		// old documents created before payments[] was embedded
		ctx, cancel := dbctx()
		defer cancel()
		cur, err := storeDB(x.storeHex).Collection(cfg.payColl).Find(ctx, bson.M{cfg.payLink: d["_id"], "deleted": bson.M{"$ne": true}},
			options.Find().SetSort(bson.M{"date": 1}))
		if err == nil {
			for cur.Next(ctx) {
				ps = append(ps, bsonToM(cur.Current))
			}
			cur.Close(ctx)
		}
	}
	for _, p := range ps {
		pm, _ := p.(M)
		if pm == nil || boolv(pm["deleted"]) {
			continue
		}
		out = append(out, paymentToContract(pm, d["date"]))
	}
	return out
}

// docToContract renders a legacy document in contract shape.
func (cfg *docCfg) toContract(x *mapCtx, d M) M {
	vat := docVAT(d, x.vatPercent())
	if cfg.forceVAT0 {
		vat = 0
	}
	items := []interface{}{}
	for _, l := range arr(d["products"]) {
		if lm, ok := l.(M); ok {
			if cfg.selectedLines {
				if sel, has := lm["selected"]; has && !boolv(sel) {
					continue
				}
			}
			items = append(items, cfg.lineToContract(x, lm, vat))
		}
	}
	rec := M{"code": str(d["code"]), "date": fmtDT(d["date"]), "items": items, "vatPercent": vat}
	rk := cfg.remarksKey
	if rk == "" {
		rk = "remarks"
	}
	rec["remarks"] = str(d[rk])
	if cfg.discount {
		rec["discount"] = num(d["discount"])
	}
	if cfg.shipping {
		rec["shipping"] = num(d["shipping_handling_fees"])
	}
	switch cfg.party {
	case "customer":
		rec["customerId"] = idOrNil(d["customer_id"])
		rec["customerName"] = str(d["customer_name"])
		rec["customerNameAr"] = str(d["customer_name_arabic"])
		rec["phone"] = str(d["phone"])
		rec["vatNo"] = str(d["vat_no"])
		rec["address"] = str(d["address"])
	case "vendor":
		rec["vendorId"] = idOrNil(d["vendor_id"])
		rec["vendorName"] = str(d["vendor_name"])
		rec["vendorNameAr"] = str(d["vendor_name_arabic"])
		rec["vendorInvoiceNo"] = str(d["vendor_invoice_no"])
		rec["phone"] = str(d["phone"])
		rec["vatNo"] = str(d["vat_no"])
		rec["address"] = str(d["address"])
	}
	if cfg.payments {
		rec["payments"] = cfg.paymentsToContract(x, d)
	}
	if cfg.cashDiscount {
		rec["cashDiscount"] = num(d["cash_discount"])
	}
	if cfg.commission {
		rec["commission"] = num(d["commission"])
		rec["commissionMethod"] = str(d["commission_payment_method"])
	}
	if cfg.rounding {
		rec["roundingAuto"] = false
		rec["rounding"] = num(d["rounding_amount"])
	}
	for _, lk := range cfg.links {
		rec[lk.c] = idOrNil(d[lk.l])
		if lk.codeC != "" {
			rec[lk.codeC] = str(d[lk.codeL])
		}
	}
	for _, f := range cfg.extra {
		rec[f.c] = readFld(x, d, f)
	}
	if cfg.zatca != "" {
		rec["zatca"] = zatcaToContract(d, cfg.zatca)
	}
	// legacy-computed totals, exposed read-only for reconciliation
	rec["legacyTotals"] = M{"total": num(d["total"]), "vat": num(d["vat_price"]), "net": num(d["net_total"]),
		"paid": num(d["total_payment_received"]) + num(d["total_payment_paid"]), "balance": num(d["balance_amount"]),
		"paymentStatus": str(d["payment_status"]), "profit": num(d["profit"])}
	return rec
}

// ksTotals reproduces the client's ks() taxable/VAT/net for rounding.
func ksTotals(rec M, vat float64) (beforeRounding float64) {
	gross, itemDisc := 0.0, 0.0
	for _, it := range arr(rec["items"]) {
		im, _ := it.(M)
		q := num(im["qty"])
		gross += q * num(im["unitPrice"])
		itemDisc += q * num(im["unitDiscount"])
	}
	taxable := round2(gross - itemDisc - num(rec["discount"]) + num(rec["shipping"]))
	v := round2(taxable * vat / 100)
	return round2(taxable + v)
}

// matchPrevLine finds the legacy line a contract item corresponds to, so
// legacy-only line fields (quantity_returned, item_code, …) are preserved.
func matchPrevLine(prevLines []interface{}, used map[int]bool, pid string, wh interface{}, idx int) M {
	for i, l := range prevLines {
		lm, _ := l.(M)
		if lm == nil || used[i] {
			continue
		}
		if hexOf(lm["product_id"]) == pid && hexOf(lm["warehouse_id"]) == hexOf(wh) {
			used[i] = true
			return lm
		}
	}
	if idx < len(prevLines) {
		if lm, _ := prevLines[idx].(M); lm != nil && !used[idx] && hexOf(lm["product_id"]) == pid {
			used[idx] = true
			return lm
		}
	}
	return nil
}

func withVAT(v, vat float64) float64 { return roundN(v*(1+vat/100), 4) }

func (cfg *docCfg) linesToLegacy(x *mapCtx, rec M, prev M, vat float64) ([]interface{}, error) {
	items := arr(rec["items"])
	prevLines := arr(get(prev, "products"))
	used := map[int]bool{}
	out := []interface{}{}
	fields := map[string]string{}
	for i, it := range items {
		im, _ := it.(M)
		if im == nil {
			continue
		}
		pfx := "items." + itoa(i) + "."
		qty := num(im["qty"])
		up := num(im["unitPrice"])
		ud := num(im["unitDiscount"])
		if qty <= 0 {
			fields[pfx+"qty"] = "must be > 0"
		}
		if up < 0 {
			fields[pfx+"unitPrice"] = "must be >= 0"
		}
		if ud > up && up > 0 {
			fields[pfx+"unitDiscount"] = "cannot exceed unit price"
		}
		pidV, err := x.ref("product", im["productId"])
		if err != nil {
			fields[pfx+"productId"] = "unknown product"
			continue
		}
		var prod M
		pid := ""
		if pidV == nil {
			// free-text line: legacy requires a product → find-or-create a service product
			name := strings.TrimSpace(str(im["nameEn"]))
			if name == "" {
				fields[pfx+"nameEn"] = "required for lines without a product"
				continue
			}
			pid, err = x.freeTextProduct(name, str(im["nameAr"]), up, str(im["unit"]))
			if err != nil {
				return nil, err
			}
		} else {
			pid = pidV.(string)
		}
		prod = x.doc("product", pid)
		if prod == nil {
			fields[pfx+"productId"] = "unknown product"
			continue
		}
		name := strings.TrimSpace(str(im["nameEn"]))
		if name == "" {
			name = str(prod["name"])
		}
		nameAr := str(im["nameAr"])
		if nameAr == "" {
			nameAr = str(prod["name_in_arabic"])
		}
		partNo := str(im["partNo"])
		if partNo == "" {
			partNo = str(prod["part_number"])
		}
		unit := str(im["unit"])
		if unit == "" {
			unit = str(prod["unit"])
		}
		line := M{
			"product_id": pid, "name": name, "name_in_arabic": nameAr, "part_number": partNo,
			"item_code": str(prod["item_code"]), "prefix_part_number": str(prod["prefix_part_number"]),
			"quantity": qty, "unit": unit,
			cfg.priceKey: up, cfg.priceKey + "_with_vat": withVAT(up, vat),
			"unit_discount": ud, "unit_discount_with_vat": withVAT(ud, vat),
			"unit_discount_percent": 0.0, "unit_discount_percent_with_vat": 0.0,
			"is_service": boolv(prod["is_service"]),
		}
		if up > 0 && ud > 0 {
			line["unit_discount_percent"] = roundN(ud/up*100, 4)
			line["unit_discount_percent_with_vat"] = line["unit_discount_percent"]
		}
		if cfg.costKey != "" {
			cost := num(im["purchasePrice"])
			if _, ok := im["purchasePrice"]; !ok {
				cost = num(get(prod, "product_stores."+x.storeHex+".purchase_unit_price"))
			}
			line[cfg.costKey] = cost
			line[cfg.costKey+"_with_vat"] = withVAT(cost, vat)
		}
		if cfg.retailWholesale {
			line["retail_unit_price"] = num(im["retailPrice"])
			line["retail_unit_price_with_vat"] = withVAT(num(im["retailPrice"]), vat)
			line["wholesale_unit_price"] = num(im["wholesalePrice"])
			line["wholesale_unit_price_with_vat"] = withVAT(num(im["wholesalePrice"]), vat)
		}
		var whID interface{}
		if !cfg.noWarehouse {
			wid, code, err := x.legacyWarehouse(str(im["warehouseId"]))
			if err != nil {
				fields[pfx+"warehouseId"] = "unknown warehouse"
				continue
			}
			line["warehouse_id"], line["warehouse_code"] = wid, code
			whID = wid
		}
		if cfg.selectedLines {
			line["selected"] = true
		}
		if pl := matchPrevLine(prevLines, used, pid, whID, i); pl != nil {
			for _, k := range []string{"quantity_returned", "prefix_part_number", "item_code", "service_category_name", "rack"} {
				if v, ok := pl[k]; ok && v != nil {
					if _, set := line[k]; !set || str(line[k]) == "" || k == "quantity_returned" {
						line[k] = v
					}
				}
			}
		}
		if _, ok := line["quantity_returned"]; !ok && cfg.priceKey == "unit_price" && !cfg.selectedLines {
			line["quantity_returned"] = 0.0
		}
		out = append(out, line)
	}
	if len(items) == 0 {
		fields["items"] = "at least one line is required"
	}
	if len(fields) > 0 {
		return nil, errBadRequest("", fields)
	}
	return out, nil
}

func itoa(i int) string { return str(float64(i)) }

func (cfg *docCfg) paymentsToLegacy(rec M, prev M, defDate string) ([]interface{}, error) {
	prevIDs := map[string]bool{}
	for _, p := range arr(get(prev, "payments")) {
		if pm, ok := p.(M); ok {
			prevIDs[hexOf(pm["_id"])] = true
		}
	}
	out := []interface{}{}
	fields := map[string]string{}
	for i, p := range arr(rec["payments"]) {
		pm, _ := p.(M)
		if pm == nil {
			continue
		}
		pfx := "payments." + itoa(i) + "."
		amt := num(pm["amount"])
		if amt < 0 {
			fields[pfx+"amount"] = "must be >= 0"
		}
		if amt == 0 {
			continue // legacy rejects zero payments; a zero row carries no money
		}
		ds := str(pm["date"])
		if ds == "" {
			ds = defDate
		}
		lds, err := toLegacyDateStr(ds)
		if err != nil {
			fields[pfx+"date"] = "invalid date"
			continue
		}
		method := str(pm["method"])
		if method == "" {
			method = "cash"
		}
		if !validPaymentMethod(method) {
			// legacy would accept it and post a ledger line without an account
			fields[pfx+"method"] = "invalid payment method"
			continue
		}
		lp := M{"date_str": lds, "amount": amt, "method": method}
		if d := str(pm["description"]); d != "" {
			lp["description"] = d
		}
		if r := str(pm["reference"]); r != "" {
			lp["bank_reference"] = r
		}
		if rt := str(pm["referenceType"]); rt != "" {
			lp["reference_type"] = rt
			lp["reference_code"] = str(pm["referenceCode"])
			if h := hexOf(pm["referenceId"]); h != "" {
				lp["reference_id"] = h
			}
		}
		if id := str(pm["id"]); prevIDs[id] {
			lp["id"] = id
		}
		out = append(out, lp)
	}
	if len(fields) > 0 {
		return nil, errBadRequest("", fields)
	}
	return out, nil
}

// toLegacy builds the legacy JSON payload for create / update.
func (cfg *docCfg) toLegacy(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
	vat := x.vatPercent()
	if v, ok := rec["vatPercent"]; ok && v != nil {
		vat = num(v)
	}
	if cfg.forceVAT0 {
		vat = 0
	}
	p := M{"vat_percent": vat}
	fields := map[string]string{}
	date := str(rec["date"])
	if date == "" {
		if prev != nil {
			date = fmtDT(prev["date"])
		} else {
			fields["date"] = "required"
		}
	}
	if date != "" {
		ds, err := toLegacyDateStr(date)
		if err != nil {
			fields["date"] = "invalid date"
		} else {
			p["date_str"] = ds
		}
	}
	if len(fields) > 0 {
		return nil, errBadRequest("", fields)
	}
	if create || ch["items"] || ch["vatPercent"] {
		lines, err := cfg.linesToLegacy(x, rec, prev, vat)
		if err != nil {
			return nil, err
		}
		p["products"] = lines
	}
	rk := cfg.remarksKey
	if rk == "" {
		rk = "remarks"
	}
	if create || ch["remarks"] {
		p[rk] = str(rec["remarks"])
	}
	if cfg.discount && (create || ch["discount"] || ch["vatPercent"]) {
		d := num(rec["discount"])
		if d < 0 {
			return nil, errBadRequest("", map[string]string{"discount": "must be >= 0"})
		}
		p["discount"] = d
		p["discount_with_vat"] = round2(d * (1 + vat/100))
	}
	if cfg.shipping && (create || ch["shipping"]) {
		p["shipping_handling_fees"] = num(rec["shipping"])
	}
	switch cfg.party {
	case "customer":
		if create || ch["customerId"] || ch["customerName"] {
			cid, err := x.ref("customer", rec["customerId"])
			if err != nil {
				return nil, errBadRequest("", map[string]string{"customerId": "unknown customer"})
			}
			p["customer_id"] = cid
			if cid == nil {
				p["customer_name"] = str(rec["customerName"])
			}
		}
		if create || ch["phone"] {
			p["phone"] = cleanPhone(str(rec["phone"]))
		}
		if create || ch["vatNo"] {
			p["vat_no"] = strings.TrimSpace(str(rec["vatNo"]))
		}
		if create || ch["address"] {
			p["address"] = str(rec["address"])
		}
	case "vendor":
		if create || ch["vendorId"] {
			vid, err := x.ref("vendor", rec["vendorId"])
			if err != nil {
				return nil, errBadRequest("", map[string]string{"vendorId": "unknown vendor"})
			}
			p["vendor_id"] = vid
		}
		if create || ch["vendorInvoiceNo"] {
			p["vendor_invoice_no"] = str(rec["vendorInvoiceNo"])
		}
		if create || ch["phone"] {
			p["phone"] = cleanPhone(str(rec["phone"]))
		}
		if create || ch["vatNo"] {
			p["vat_no"] = strings.TrimSpace(str(rec["vatNo"]))
		}
		if create || ch["address"] {
			p["address"] = str(rec["address"])
		}
	}
	if cfg.cashDiscount && (create || ch["cashDiscount"]) {
		p["cash_discount"] = num(rec["cashDiscount"])
	}
	if cfg.commission && (create || ch["commission"] || ch["commissionMethod"]) {
		p["commission"] = num(rec["commission"])
		p["commission_payment_method"] = str(rec["commissionMethod"])
		if num(rec["commission"]) > 0 && str(rec["commissionMethod"]) == "" {
			p["commission_payment_method"] = "cash"
		}
	}
	if cfg.rounding && (create || ch["rounding"] || ch["roundingAuto"] || ch["items"] || ch["discount"] || ch["shipping"] || ch["vatPercent"]) {
		// The client's auto rounding (nearest 0.05) differs from legacy
		// auto_rounding_amount: send the exact amount instead.
		r := num(rec["rounding"])
		if boolv(rec["roundingAuto"]) {
			br := ksTotals(rec, vat)
			r = round2(math.Round(br*20)/20 - br)
		}
		p["auto_rounding_amount"] = false
		p["rounding_amount"] = r
	}
	if cfg.payments {
		pays, err := cfg.paymentsToLegacy(rec, prev, date)
		if err != nil {
			return nil, err
		}
		p["payments_input"] = pays
	}
	for _, lk := range cfg.links {
		if !(create || ch[lk.c]) {
			continue
		}
		v := rec[lk.c]
		if lk.required && str(v) == "" {
			return nil, errBadRequest("", map[string]string{lk.c: "required"})
		}
		h, err := x.ref(lk.coll, v)
		if err != nil {
			return nil, errBadRequest("", map[string]string{lk.c: "unknown id"})
		}
		if lk.required && h == nil {
			return nil, errBadRequest("", map[string]string{lk.c: "required"})
		}
		p[lk.l] = h
		if lk.codeL != "" && h != nil {
			if d := x.doc(lk.coll, h.(string)); d != nil {
				p[lk.codeL] = str(d["code"])
			}
		}
	}
	for _, f := range cfg.extra {
		if create || ch[f.c] {
			if err := writeFld(x, p, rec, f); err != nil {
				return nil, err
			}
		}
	}
	p["enable_report_to_zatca"] = false
	return p, nil
}

func (cfg *docCfg) known() map[string]bool {
	k := knownSet("code", "date", "items", "vatPercent", "remarks", "legacyTotals")
	if cfg.discount {
		k["discount"] = true
	}
	if cfg.shipping {
		k["shipping"] = true
	}
	switch cfg.party {
	case "customer":
		for _, s := range []string{"customerId", "customerName", "customerNameAr", "phone", "vatNo", "address"} {
			k[s] = true
		}
	case "vendor":
		for _, s := range []string{"vendorId", "vendorName", "vendorNameAr", "vendorInvoiceNo", "phone", "vatNo", "address"} {
			k[s] = true
		}
	}
	if cfg.payments {
		k["payments"] = true
	}
	if cfg.cashDiscount {
		k["cashDiscount"] = true
	}
	if cfg.commission {
		k["commission"], k["commissionMethod"] = true, true
	}
	if cfg.rounding {
		k["rounding"], k["roundingAuto"] = true, true
	}
	for _, l := range cfg.links {
		k[l.c] = true
		if l.codeC != "" {
			k[l.codeC] = true
		}
	}
	for _, f := range cfg.extra {
		k[f.c] = true
	}
	if cfg.zatca != "" {
		k["zatca"] = true
	}
	return k
}

// docValidate applies contract rules the legacy code does not check.
func (cfg *docCfg) validate(x *mapCtx, rec M, prev M) map[string]string {
	e := map[string]string{}
	if str(rec["date"]) == "" && prev == nil {
		e["date"] = "required"
	}
	if v := str(rec["vatNo"]); v != "" && cfg.party == "customer" && !ValidVAT(v) {
		e["vatNo"] = "VAT No. must be 15 digits starting and ending with 3"
	}
	if cfg.discount {
		if d := num(rec["discount"]); d < 0 {
			e["discount"] = "must be >= 0"
		}
	}
	paid := 0.0
	for i, p := range arr(rec["payments"]) {
		pm, _ := p.(M)
		if num(pm["amount"]) < 0 {
			e["payments."+itoa(i)+".amount"] = "must be >= 0"
		}
		paid += num(pm["amount"])
	}
	if cfg.payments && cfg.rounding {
		vat := x.vatPercent()
		if v, ok := rec["vatPercent"]; ok && v != nil {
			vat = num(v)
		}
		br := ksTotals(rec, vat)
		r := num(rec["rounding"])
		if boolv(rec["roundingAuto"]) {
			r = round2(math.Round(br*20)/20 - br)
		}
		net := round2(br + r)
		if paid+num(rec["cashDiscount"]) > net+0.009 {
			e["payments"] = "paid + cash discount exceeds the net total"
		}
	}
	return e
}

// paymentMethods is the contract enum (= the legacy method values; the legacy
// ledger maps cash → CASH, models.BANK_PAYMENT_METHODS → BANK).
var paymentMethods = map[string]bool{"cash": true, "debit_card": true, "credit_card": true, "bank_card": true,
	"bank_transfer": true, "bank_cheque": true, "purchase": true, "sales": true, "purchase_return": true,
	"sales_return": true, "customer_account": true, "vendor_account": true}

func validPaymentMethod(m string) bool { return paymentMethods[m] }

package erp

import (
	"regexp"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/controller"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ---- generic simple mapper built from fld lists ----

func simpleToC(fields []fld, extra func(x *mapCtx, d M, rec M)) toCFn {
	return func(x *mapCtx, d M) M {
		rec := M{}
		for _, f := range fields {
			rec[f.c] = readFld(x, d, f)
		}
		if extra != nil {
			extra(x, d, rec)
		}
		return rec
	}
}

func simpleToL(fields []fld, extra func(x *mapCtx, rec, prev M, ch map[string]bool, create bool, p M) error) toLFn {
	return func(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
		p := M{}
		for _, f := range fields {
			if create || ch[f.c] {
				if err := writeFld(x, p, rec, f); err != nil {
					return nil, err
				}
			}
		}
		if extra != nil {
			if err := extra(x, rec, prev, ch, create, p); err != nil {
				return nil, err
			}
		}
		return p, nil
	}
}

func fieldKeys(fields []fld, more ...string) map[string]bool {
	k := knownSet(more...)
	for _, f := range fields {
		k[f.c] = true
	}
	return k
}

func fieldErrMap(fields []fld, more map[string]string) map[string]string {
	m := map[string]string{}
	for _, f := range fields {
		m[f.l] = f.c
		m[f.l+"_str"] = f.c
	}
	for k, v := range more {
		m[k] = v
	}
	return m
}

// ---- warehouses (+ virtual main-store warehouse) ----

var warehouseFields = []fld{
	{c: "nameEn", l: "name"}, {c: "nameAr", l: "name_in_arabic"}, {c: "code", l: "code"},
	{c: "address", l: "address"}, {c: "phone", l: "phone"}, {c: "email", l: "email"},
}

func virtualWarehouse(storeHex string, st M) M {
	return M{"id": mainStoreWarehouseID(storeHex), "storeId": storeHex, "nameEn": "Main store", "nameAr": "المستودع الرئيسي",
		"code": "MAIN", "address": "", "manager": "", "virtual": true, "version": int64(1), "deleted": false,
		"history": []interface{}{}, "createdAt": fmtDT(st["created_at"]), "createdBy": "", "updatedAt": "", "updatedBy": ""}
}

// warehousesBackend prepends the virtual "main store" warehouse (legacy
// main_store bucket = stock without a warehouse) to the legacy warehouses.
type warehousesBackend struct{ *legacyBackend }

func (w *warehousesBackend) List(c *Ctx, storeHex string, q ListQuery) ([]M, int64, error) {
	rows, total, err := w.legacyBackend.List(c, storeHex, q)
	if err != nil {
		return nil, 0, err
	}
	if q.Page == 1 {
		rows = append([]M{virtualWarehouse(storeHex, c.store(storeHex))}, rows...)
	}
	return rows, total + 1, nil
}

func (w *warehousesBackend) Get(c *Ctx, storeHex, id string, inc bool) (M, error) {
	if isMainStoreWarehouse(id) {
		s := strings.TrimPrefix(id, "ms_")
		if c.store(s) == nil {
			return nil, nil
		}
		return virtualWarehouse(s, c.store(s)), nil
	}
	return w.legacyBackend.Get(c, storeHex, id, inc)
}

func (w *warehousesBackend) Locate(c *Ctx, id string) (string, bool) {
	if isMainStoreWarehouse(id) {
		s := strings.TrimPrefix(id, "ms_")
		return s, c.store(s) != nil
	}
	return w.legacyBackend.Locate(c, id)
}

func virtualWHGuard(id string) error {
	if isMainStoreWarehouse(id) {
		return errUnsupported("The main store warehouse is built in (legacy stock without a warehouse) and cannot be changed.")
	}
	return nil
}

func (w *warehousesBackend) Update(c *Ctx, storeHex, id string, prev, next M, changed []string, meta WriteMeta) (M, error) {
	if err := virtualWHGuard(id); err != nil {
		return nil, err
	}
	return w.legacyBackend.Update(c, storeHex, id, prev, next, changed, meta)
}

func (w *warehousesBackend) Delete(c *Ctx, storeHex, id string, meta WriteMeta) (M, error) {
	if err := virtualWHGuard(id); err != nil {
		return nil, err
	}
	return w.legacyBackend.Delete(c, storeHex, id, meta)
}

func (w *warehousesBackend) HardDelete(c *Ctx, storeHex, id string, meta WriteMeta) error {
	if err := virtualWHGuard(id); err != nil {
		return err
	}
	return w.legacyBackend.HardDelete(c, storeHex, id, meta)
}

func newWarehousesResource() *Resource {
	b := &legacyBackend{coll: "warehouse", deletedKey: "deleted", sortKey: "_id",
		toC: simpleToC(warehouseFields, nil), toL: simpleToL(warehouseFields, nil), known: fieldKeys(warehouseFields),
		validate: func(x *mapCtx, rec M, prev M) map[string]string {
			e := map[string]string{}
			if strings.TrimSpace(str(rec["nameEn"])) == "" {
				e["nameEn"] = "required"
			}
			if strings.TrimSpace(str(rec["code"])) == "" {
				e["code"] = "required"
			} else if strings.EqualFold(str(rec["code"]), "MAIN") || strings.EqualFold(str(rec["code"]), "main_store") {
				e["code"] = "reserved for the main store"
			}
			return e
		},
		v1:       v1Ops{path: "/v1/warehouse", create: controller.CreateWarehouse, update: controller.UpdateWarehouse, delete: controller.DeleteWarehouse},
		fieldErr: fieldErrMap(warehouseFields, nil),
	}
	return &Resource{Name: "warehouses", Path: "warehouses", Scope: "store", Module: "inventory", Legacy: "store DB `warehouse` + virtual main store", Backend: &warehousesBackend{b}}
}

// ---- products ----

func productToContract(x *mapCtx, d M) M {
	ps := sub(sub(d, "product_stores"), x.storeHex)
	rec := M{
		"code": str(d["item_code"]), "nameEn": str(d["name"]), "nameAr": str(d["name_in_arabic"]),
		"keywords": strs(d["additional_keywords"]), "partNo": str(d["part_number"]), "prefixPartNo": str(d["prefix_part_number"]),
		"unit": str(d["unit"]), "isService": boolv(d["is_service"]), "categoryIds": ids(d["category_id"]),
		"brandId": idOrNil(d["brand_id"]), "country": str(d["country_name"]), "barcode": str(d["bar_code"]),
		"rack": str(d["rack"]), "note": str(d["note"]), "images": strs(d["images"]), "isSet": boolv(d["is_set"]),
		"linkedIds": ids(d["linked_product_ids"]),
	}
	if rec["barcode"] == "" {
		rec["barcode"] = str(d["ean_12"])
	}
	pricing := M{
		"purchase": num(ps["purchase_unit_price"]), "retail": num(ps["retail_unit_price"]), "wholesale": num(ps["wholesale_unit_price"]),
		"retailMargin": num(ps["retail_margin_percent"]), "wholesaleMargin": num(ps["wholesale_margin_percent"]),
		"autoRetail": boolv(ps["auto_update_retail_price_from_last_purchase"]), "autoWholesale": boolv(ps["auto_update_wholesale_price_from_last_purchase"]),
	}
	rec["pricing"] = pricing
	// stock map keyed by contract warehouse id
	stock := M{}
	ws := sub(ps, "warehouse_stocks")
	racks := sub(ps, "warehouse_racks")
	if !boolv(d["is_service"]) {
		mainQty := num(ps["stock"])
		if v, ok := ws["main_store"]; ok {
			mainQty = num(v)
		} else {
			for _, w := range x.warehouses() {
				mainQty -= num(ws[str(w["code"])])
			}
		}
		mainRack := str(racks["main_store"])
		if mainRack == "" {
			mainRack = str(d["rack"])
		}
		// `min` has no legacy field: it is preserved in erp.x.stock (hybrid)
		entry := func(qty float64, rack string) M {
			e := M{"qty": roundN(qty, 4)}
			if rack != "" {
				e["rack"] = rack
			}
			return e
		}
		stock[mainStoreWarehouseID(x.storeHex)] = entry(mainQty, mainRack)
		for _, w := range x.warehouses() {
			if boolv(w["deleted"]) {
				continue
			}
			code := str(w["code"])
			stock[hexOf(w["_id"])] = entry(num(ws[code]), str(racks[code]))
		}
	}
	rec["stock"] = stock
	comps := []interface{}{}
	for _, sp := range arr(get(d, "set.products")) {
		if spm, ok := sp.(M); ok {
			pid := idOrNil(spm["produc_id"]) // sic: legacy bson key
			if pid == nil {
				pid = idOrNil(spm["product_id"])
			}
			comps = append(comps, M{"productId": pid, "qty": num(spm["quantity"])})
		}
	}
	rec["components"] = comps
	rec["legacyStockTotal"] = num(ps["stock"])
	return rec
}

var productKnown = knownSet("code", "nameEn", "nameAr", "keywords", "partNo", "prefixPartNo", "unit", "isService",
	"categoryIds", "brandId", "country", "barcode", "rack", "note", "images", "isSet", "linkedIds", "pricing", "stock",
	"components", "legacyStockTotal")

func productValidate(x *mapCtx, rec M, prev M) map[string]string {
	e := map[string]string{}
	if strings.TrimSpace(str(rec["nameEn"])) == "" {
		e["nameEn"] = "required"
	}
	if b := str(rec["barcode"]); b != "" && !regexpDigits(b, 8, 13) {
		e["barcode"] = "8-13 digits"
	}
	pr := sub(rec, "pricing")
	if num(pr["retail"]) < 0 {
		e["pricing.retail"] = "must be >= 0"
	}
	if mx := num(pr["max"]); mx > 0 && mx < num(pr["min"]) {
		e["pricing.max"] = "must be >= min"
	}
	validatePosFields(rec, e)
	if code := strings.TrimSpace(str(rec["code"])); code != "" && x.storeHex != "" {
		ctx, cancel := dbctx()
		defer cancel()
		f := bson.M{"item_code": bson.M{"$regex": "^" + quoteRe(code) + "$", "$options": "i"}, "deleted": bson.M{"$ne": true}}
		if prev != nil {
			f["_id"] = bson.M{"$ne": prev["_id"]}
		}
		if n, _ := storeDB(x.storeHex).Collection("product").CountDocuments(ctx, f); n > 0 {
			e["code"] = "already used by another product"
		}
	}
	return e
}

func regexpDigits(s string, min, max int) bool {
	if len(s) < min || len(s) > max {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// productStoreEntry returns the full legacy product_stores[store] object in
// JSON shape. The legacy decoder REPLACES map values, so partial objects
// would wipe stats: always send the whole entry.
func productStoreEntry(x *mapCtx, prev M) M {
	ps := cloneM(toJSONMap(sub(sub(prev, "product_stores"), x.storeHex)))
	if ps == nil {
		ps = M{}
	}
	ps["store_id"] = x.storeHex
	adj := []interface{}{}
	for _, a := range arr(get(prev, "product_stores."+x.storeHex+".stock_adjustments")) {
		am, _ := a.(M)
		if am == nil {
			continue
		}
		ja := toJSONMap(am)
		if t, ok := toTime(am["date"]); ok {
			ja["date_str"] = t.Format("2006-01-02T15:04:05Z07:00")
		}
		adj = append(adj, ja)
	}
	ps["stock_adjustments"] = adj
	return ps
}

func productToLegacy(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
	p := M{}
	s := func(ck, lk string, v interface{}) {
		if create || ch[ck] {
			p[lk] = v
		}
	}
	s("nameEn", "name", strings.TrimSpace(str(rec["nameEn"])))
	s("nameAr", "name_in_arabic", str(rec["nameAr"]))
	s("keywords", "additional_keywords", strs(rec["keywords"]))
	s("partNo", "part_number", str(rec["partNo"]))
	s("prefixPartNo", "prefix_part_number", str(rec["prefixPartNo"]))
	s("unit", "unit", str(rec["unit"]))
	s("isService", "is_service", boolv(rec["isService"]))
	s("country", "country_name", str(rec["country"]))
	s("rack", "rack", str(rec["rack"]))
	s("note", "note", str(rec["note"]))
	s("isSet", "is_set", boolv(rec["isSet"]))
	if create || ch["code"] {
		code := strings.TrimSpace(str(rec["code"]))
		if code == "" && create {
			code = nextProductCode(x)
		}
		p["item_code"] = code
	}
	if create || ch["barcode"] {
		p["bar_code"] = str(rec["barcode"])
	}
	if create || ch["categoryIds"] {
		out := []interface{}{}
		for _, c := range arr(rec["categoryIds"]) {
			h, err := x.ref("product_category", c)
			if err != nil {
				return nil, errBadRequest("", map[string]string{"categoryIds": "unknown category " + str(c)})
			}
			if h != nil {
				out = append(out, h)
			}
		}
		p["category_id"] = out
	}
	if create || ch["brandId"] {
		h, err := x.ref("product_brand", rec["brandId"])
		if err != nil {
			return nil, errBadRequest("", map[string]string{"brandId": "unknown brand"})
		}
		p["brand_id"] = h
	}
	if create || ch["linkedIds"] {
		out := []interface{}{}
		for _, c := range arr(rec["linkedIds"]) {
			if h, err := x.ref("product", c); err == nil && h != nil {
				out = append(out, h)
			}
		}
		p["linked_product_ids"] = out
	}
	if create || ch["images"] {
		urls, contents := []string{}, []string{}
		for _, im := range strs(rec["images"]) {
			if strings.HasPrefix(im, "data:") {
				contents = append(contents, im)
			} else {
				urls = append(urls, im)
			}
		}
		p["images"] = urls
		if len(contents) > 0 {
			p["images_content"] = contents
		}
	}
	if create || ch["components"] || ch["isSet"] {
		sps := []interface{}{}
		for _, cpt := range arr(rec["components"]) {
			cm, _ := cpt.(M)
			h, err := x.ref("product", cm["productId"])
			if err != nil || h == nil {
				return nil, errBadRequest("", map[string]string{"components": "unknown product"})
			}
			pd := x.doc("product", h.(string))
			sps = append(sps, M{"product_id": h, "quantity": num(cm["qty"]), "name": str(pd["name"]),
				"part_number": str(pd["part_number"]), "unit": str(pd["unit"])})
		}
		set := M{"products": sps}
		if prev != nil {
			if ps := sub(prev, "set"); len(ps) > 0 {
				set["name"] = str(ps["name"])
			}
		}
		p["set"] = set
	}
	// per-store pricing / stock
	if create || ch["pricing"] || ch["stock"] {
		ps := M{"store_id": x.storeHex}
		if prev != nil {
			ps = productStoreEntry(x, prev)
		} else {
			ps["stock_adjustments"] = []interface{}{}
		}
		vat := x.vatPercent()
		if create || ch["pricing"] {
			pr := sub(rec, "pricing")
			ps["purchase_unit_price"] = num(pr["purchase"])
			ps["purchase_unit_price_with_vat"] = withVAT(num(pr["purchase"]), vat)
			ps["retail_unit_price"] = num(pr["retail"])
			ps["retail_unit_price_with_vat"] = withVAT(num(pr["retail"]), vat)
			ps["wholesale_unit_price"] = num(pr["wholesale"])
			ps["wholesale_unit_price_with_vat"] = withVAT(num(pr["wholesale"]), vat)
			ps["retail_margin_percent"] = num(pr["retailMargin"])
			ps["wholesale_margin_percent"] = num(pr["wholesaleMargin"])
			ps["auto_update_retail_price_from_last_purchase"] = boolv(pr["autoRetail"])
			ps["auto_update_wholesale_price_from_last_purchase"] = boolv(pr["autoWholesale"])
		}
		if (create || ch["stock"]) && !boolv(rec["isService"]) {
			// contract stock is ABSOLUTE per warehouse: convert the difference
			// into legacy stock adjustments (adding/removing), which is how the
			// old app records manual stock changes.
			cur := M{}
			if prev != nil {
				cur = sub(productToContract(x, prev), "stock")
			}
			adjs := arr(ps["stock_adjustments"])
			now := nowFn()
			for whID, v := range sub(rec, "stock") {
				vm, _ := v.(M)
				want := roundN(num(vm["qty"]), 4)
				have := roundN(num(sub(cur, whID)["qty"]), 4)
				delta := roundN(want-have, 4)
				if delta == 0 {
					continue
				}
				wid, code, err := x.legacyWarehouse(whID)
				if err != nil {
					return nil, errBadRequest("", map[string]string{"stock": "unknown warehouse " + whID})
				}
				typ := "adding"
				if delta < 0 {
					typ = "removing"
					delta = -delta
				}
				now = now.Add(time1Minute)
				reason := "StartERP stock edit"
				if prev == nil {
					reason = "opening"
				}
				adjs = append(adjs, M{"date_str": now.Format("2006-01-02T15:04:05Z07:00"), "type": typ, "quantity": delta,
					"reason": reason, "warehouse_id": wid, "warehouse_code": code})
			}
			ps["stock_adjustments"] = adjs
			racks := sub(ps, "warehouse_racks")
			racks = cloneM(racks)
			for whID, v := range sub(rec, "stock") {
				vm, _ := v.(M)
				if _, ok := vm["rack"]; !ok {
					continue
				}
				key := "main_store"
				if !isMainStoreWarehouse(whID) {
					key = x.whCodeByID(whID)
				}
				if key != "" {
					racks[key] = str(vm["rack"])
				}
			}
			ps["warehouse_racks"] = racks
		}
		p["product_stores"] = M{x.storeHex: ps}
	}
	return p, nil
}

func newProductsResource() *Resource {
	b := &legacyBackend{coll: "product", deletedKey: "deleted", sortKey: "_id",
		searchKeys: []string{"name", "name_in_arabic", "item_code", "part_number", "bar_code"}, searchSort: "name",
		toC: productToContract, toL: productToLegacy, known: productKnown, validate: productValidate,
		hybrid: knownSet("pricing", "stock"),
		v1: v1Ops{path: "/v1/product", create: controller.CreateProduct, update: controller.UpdateProduct,
			delete: controller.DeleteProduct, restore: controller.RestoreProduct},
		fieldErr: map[string]string{"name": "nameEn", "name_in_arabic": "nameAr", "item_code": "code", "part_number": "partNo",
			"bar_code": "barcode", "unit": "unit", "brand_id": "brandId", "stock": "stock"},
		lineErr: map[string]string{"adjustment_date": "date", "adjustment_quantity": "qty", "adjustment_type": "type", "category_id": "id"},
		lineKey: "adjustments",
	}
	return &Resource{Name: "products", Path: "products", Scope: "store", Module: "inventory", Legacy: "store DB `product` (product_stores[store])", Backend: b}
}

// freeTextProduct finds or creates a SERVICE product for a document line
// without productId (legacy documents require a product on every line).
func (x *mapCtx) freeTextProduct(name, nameAr string, price float64, unit string) (string, error) {
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	err := storeDB(x.storeHex).Collection("product").FindOne(ctx, bson.M{"name": name, "is_service": true, "deleted": bson.M{"$ne": true}},
		options.FindOne().SetProjection(bson.M{"_id": 1})).Decode(&raw)
	if err == nil {
		return hexOf(raw["_id"]), nil
	}
	if len([]rune(name)) < 3 {
		name = name + " (service)"
	}
	payload := M{"name": name, "name_in_arabic": nameAr, "is_service": true, "unit": unit,
		"product_stores": M{x.storeHex: M{"store_id": x.storeHex, "retail_unit_price": price,
			"retail_unit_price_with_vat": withVAT(price, x.vatPercent()), "stock_adjustments": []interface{}{}}}}
	res, err := callV1(x.c, controller.CreateProduct, "POST", "/v1/product", nil, x.storeHex, payload)
	if err != nil {
		return "", err
	}
	if !res.ok() {
		return "", legacyErr(res, map[string]string{"name": "items.nameEn"}, nil, "")
	}
	return res.resultID(), nil
}

// ---- customers / vendors ----

// partySearchKeys: legacy keys ?q= matches for customers and vendors.
var partySearchKeys = []string{"name", "name_in_arabic", "code", "phone", "phone2", "vat_no", "email"}

var customerFields = []fld{
	{c: "code", l: "code", ro: true}, {c: "nameEn", l: "name"}, {c: "nameAr", l: "name_in_arabic"},
	{c: "titleEn", l: "title"}, {c: "titleAr", l: "title_in_arabic"}, {c: "vatNo", l: "vat_no"},
	{c: "crNo", l: "registration_number"}, {c: "creditLimit", l: "credit_limit", k: kNum},
	{c: "openingBalance", l: "opening_balance", k: kNum}, {c: "openingBalanceType", l: "opening_balance_type"},
	{c: "openingBalanceDate", l: "opening_balance_date", k: kDTPlain}, {c: "remarks", l: "remarks"},
	{c: "phone", l: "phone"}, {c: "phoneAr", l: "phone_in_arabic"}, {c: "phone2", l: "phone2"}, {c: "phone2Ar", l: "phone2_in_arabic"},
	{c: "email", l: "email"}, {c: "contactPerson", l: "contact_person"}, {c: "sponsor", l: "sponsor"},
	{c: "useRemarksInSales", l: "use_remarks_in_sales", k: kBool}, {c: "creditBalance", l: "credit_balance", k: kNum, ro: true},
}

func partyAddressToC(x *mapCtx, d M, rec M) {
	rec["address"] = addressFromLegacy(sub(d, "national_address"), str(d["country_name"]))
	if a := str(d["address"]); a != "" {
		rec["addressText"] = a
	}
	if imgs := strs(d["images"]); len(imgs) > 0 {
		rec["photo"] = imgs[0]
	}
}

func partyAddressToL(x *mapCtx, rec, prev M, ch map[string]bool, create bool, p M) error {
	if create || ch["address"] {
		a := sub(rec, "address")
		p["national_address"] = addressToLegacy(a)
		if c := str(a["countryEn"]); c != "" {
			p["country_name"] = c
			if strings.EqualFold(c, "Saudi Arabia") {
				p["country_code"] = "SA"
			}
		}
	}
	if create || ch["addressText"] {
		p["address"] = str(rec["addressText"])
	}
	if ch["vatNo"] || create {
		p["vat_no_in_arabic"] = toArabicDigits(str(rec["vatNo"]))
	}
	if (ch["phone"] || create) && str(rec["phoneAr"]) == "" {
		p["phone_in_arabic"] = toArabicDigits(str(rec["phone"]))
	}
	if ch["openingBalanceType"] || create {
		t := str(rec["openingBalanceType"])
		if t == "debit" {
			t = "receivable"
		} else if t == "credit" {
			t = "payable"
		}
		p["opening_balance_type"] = t
		if num(rec["openingBalance"]) == 0 && t == "" {
			delete(p, "opening_balance_type")
		}
	}
	return nil
}

func partyValidate(x *mapCtx, rec M, prev M) map[string]string {
	e := map[string]string{}
	if len([]rune(strings.TrimSpace(str(rec["nameEn"])))) < 2 {
		e["nameEn"] = "at least 2 characters"
	}
	if n := str(rec["nameAr"]); n != "" && !hasArabic(n) {
		e["nameAr"] = "must contain Arabic letters"
	}
	if v := str(rec["vatNo"]); v != "" && !ValidVAT(v) {
		e["vatNo"] = "VAT No. must be 15 digits starting and ending with 3"
	}
	if num(rec["creditLimit"]) < 0 {
		e["creditLimit"] = "must be >= 0"
	}
	if num(rec["openingBalance"]) < 0 {
		e["openingBalance"] = "must be >= 0"
	}
	if v := str(rec["email"]); v != "" && !validEmail(v) {
		e["email"] = "invalid email"
	}
	if v := str(rec["phone"]); v != "" && !ValidSaudiMobile(v) && !ValidSaudiPhone(v) {
		e["phone"] = "invalid phone"
	}
	a := sub(rec, "address")
	if v := str(a["postalCode"]); v != "" && !re5.MatchString(v) {
		e["address.postalCode"] = "5 digits"
	}
	if v := str(a["buildingNo"]); v != "" && !re4.MatchString(v) {
		e["address.buildingNo"] = "4 digits"
	}
	if v := str(a["additionalNo"]); v != "" && !re4.MatchString(v) {
		e["address.additionalNo"] = "4 digits"
	}
	return e
}

var partyErrExtra = map[string]string{"national_address_building_no": "address.buildingNo", "national_address_zipcode": "address.postalCode",
	"opening_balance_date": "openingBalanceDate", "mob": "phone"}

func newCustomersResource() *Resource {
	b := &legacyBackend{coll: "customer", deletedKey: "deleted", sortKey: "_id",
		searchKeys: partySearchKeys, searchSort: "name",
		toC: simpleToC(customerFields, partyAddressToC), toL: simpleToL(customerFields, partyAddressToL),
		known: fieldKeys(customerFields, "address", "addressText", "phoneAr", "phone2Ar"), validate: partyValidate,
		hybrid: knownSet("address"),
		v1: v1Ops{path: "/v1/customer", create: controller.CreateCustomer, update: controller.UpdateCustomer,
			delete: controller.DeleteCustomer, restore: controller.RestoreCustomer},
		fieldErr: fieldErrMap(customerFields, partyErrExtra),
	}
	return &Resource{Name: "customers", Path: "customers", Scope: "store", Module: "customers", Legacy: "store DB `customer`", Backend: b}
}

var vendorFields = []fld{
	{c: "code", l: "code", ro: true}, {c: "nameEn", l: "name"}, {c: "nameAr", l: "name_in_arabic"},
	{c: "titleEn", l: "title"}, {c: "titleAr", l: "title_in_arabic"}, {c: "vatNo", l: "vat_no"},
	{c: "crNo", l: "registration_number"}, {c: "creditLimit", l: "credit_limit", k: kNum},
	{c: "openingBalance", l: "opening_balance", k: kNum}, {c: "openingBalanceType", l: "opening_balance_type"},
	{c: "openingBalanceDate", l: "opening_balance_date", k: kDTPlain}, {c: "remarks", l: "remarks"},
	{c: "phone", l: "phone"}, {c: "phoneAr", l: "phone_in_arabic"}, {c: "email", l: "email"},
	{c: "contactPerson", l: "contact_person"}, {c: "sponsor", l: "sponsor"},
	{c: "useRemarksInPurchases", l: "use_remarks_in_purchases", k: kBool},
	{c: "productCategories", l: "product_categories", k: kStrs}, {c: "logo", l: "logo", ro: true},
	{c: "creditBalance", l: "credit_balance", k: kNum, ro: true},
}

func vendorExtraToC(x *mapCtx, d M, rec M) {
	partyAddressToC(x, d, rec)
	rec["category"] = strs(d["category_name"])
	if v := d["vat_percent"]; v != nil {
		rec["vatPercent"] = num(v)
	}
}

func vendorExtraToL(x *mapCtx, rec, prev M, ch map[string]bool, create bool, p M) error {
	if err := partyAddressToL(x, rec, prev, ch, create, p); err != nil {
		return err
	}
	if create || ch["category"] {
		// names → vendor_category ids (unknown names are kept in erp.x only)
		idsOut := []interface{}{}
		for _, n := range strs(rec["category"]) {
			ctx, cancel := dbctx()
			var raw bson.M
			if err := storeDB(x.storeHex).Collection("vendor_category").FindOne(ctx, bson.M{"name": n, "deleted": bson.M{"$ne": true}}).Decode(&raw); err == nil {
				idsOut = append(idsOut, hexOf(raw["_id"]))
			}
			cancel()
		}
		p["category_id"] = idsOut
	}
	if ch["vatPercent"] || (create && rec["vatPercent"] != nil) {
		p["vat_percent"] = num(rec["vatPercent"])
	}
	return nil
}

func newVendorsResource() *Resource {
	b := &legacyBackend{coll: "vendor", deletedKey: "deleted", sortKey: "_id",
		searchKeys: partySearchKeys, searchSort: "name",
		toC: simpleToC(vendorFields, vendorExtraToC), toL: simpleToL(vendorFields, vendorExtraToL),
		known: fieldKeys(vendorFields, "address", "addressText", "phoneAr", "category", "vatPercent"), validate: partyValidate,
		hybrid: knownSet("address", "category"),
		v1: v1Ops{path: "/v1/vendor", create: controller.CreateVendor, update: controller.UpdateVendor,
			delete: controller.DeleteVendor, restore: controller.RestoreVendor},
		fieldErr: fieldErrMap(vendorFields, partyErrExtra),
	}
	return &Resource{Name: "vendors", Path: "vendors", Scope: "store", Module: "vendors", Legacy: "store DB `vendor`", Backend: b}
}

// ---- lookups (org-scoped in the contract, per-store collections in legacy) ----

func newCategoriesResource() *Resource {
	fields := []fld{{c: "nameEn", l: "name"}, {c: "parentId", l: "parent_id", k: kRef, ref: "product_category"}}
	b := &legacyBackend{coll: "product_category", orgOverStores: true, deletedKey: "deleted", sortKey: "_id",
		searchKeys: []string{"name", "name_in_arabic"},
		toC:        simpleToC(fields, nil), toL: simpleToL(fields, nil), known: fieldKeys(fields),
		validate: func(x *mapCtx, rec M, prev M) map[string]string {
			e := map[string]string{}
			if strings.TrimSpace(str(rec["nameEn"])) == "" {
				e["nameEn"] = "required"
			}
			if prev != nil && str(rec["parentId"]) != "" && str(rec["parentId"]) == hexOf(prev["_id"]) {
				e["parentId"] = "cannot be itself"
			}
			return e
		},
		v1: v1Ops{path: "/v1/product-category", create: controller.CreateProductCategory, update: controller.UpdateProductCategory,
			delete: controller.DeleteProductCategory, restore: controller.RestoreProductCategory},
		fieldErr: fieldErrMap(fields, nil),
	}
	return &Resource{Name: "categories", Path: "categories", Scope: "org", Module: "inventory", Legacy: "store DBs `product_category` (union)", Backend: b}
}

func newBrandsResource() *Resource {
	fields := []fld{{c: "name", l: "name"}, {c: "code", l: "code"}}
	b := &legacyBackend{coll: "product_brand", orgOverStores: true, deletedKey: "deleted", sortKey: "_id",
		searchKeys: []string{"name", "name_in_arabic", "code"},
		toC:        simpleToC(fields, nil),
		toL: simpleToL(fields, func(x *mapCtx, rec, prev M, ch map[string]bool, create bool, p M) error {
			if str(p["code"]) == "" && (create || ch["name"]) {
				// legacy requires a brand code; the contract has none
				code := strings.ToUpper(reNonAlnum.ReplaceAllString(str(rec["name"]), ""))
				if len(code) > 6 {
					code = code[:6]
				}
				if code == "" {
					code = "BRAND"
				}
				p["code"] = code
			}
			return nil
		}),
		known: fieldKeys(fields),
		validate: func(x *mapCtx, rec M, prev M) map[string]string {
			if strings.TrimSpace(str(rec["name"])) == "" {
				return map[string]string{"name": "required"}
			}
			return nil
		},
		v1: v1Ops{path: "/v1/product-brand", create: controller.CreateProductBrand, update: controller.UpdateProductBrand,
			delete: controller.DeleteProductBrand, restore: controller.RestoreProductBrand},
		fieldErr: fieldErrMap(fields, nil),
	}
	return &Resource{Name: "brands", Path: "brands", Scope: "org", Module: "inventory", Legacy: "store DBs `product_brand` (union)", Backend: b}
}

func newExpenseCategoriesResource() *Resource {
	fields := []fld{{c: "nameEn", l: "name"}, {c: "parentId", l: "parent_id", k: kRef, ref: "expense_category"}}
	b := &legacyBackend{coll: "expense_category", orgOverStores: true, deletedKey: "deleted", sortKey: "_id",
		toC: simpleToC(fields, nil), toL: simpleToL(fields, nil), known: fieldKeys(fields),
		validate: func(x *mapCtx, rec M, prev M) map[string]string {
			if strings.TrimSpace(str(rec["nameEn"])) == "" {
				return map[string]string{"nameEn": "required"}
			}
			return nil
		},
		v1: v1Ops{path: "/v1/expense-category", create: controller.CreateExpenseCategory, update: controller.UpdateExpenseCategory,
			delete: controller.DeleteExpenseCategory},
		fieldErr: fieldErrMap(fields, nil),
	}
	return &Resource{Name: "expenseCategories", Path: "expense-categories", Scope: "org", Module: "finance", Legacy: "store DBs `expense_category` (union)", Backend: b}
}

func newVendorCategoriesResource() *Resource {
	fields := []fld{{c: "name", l: "name"}}
	b := &legacyBackend{coll: "vendor_category", orgOverStores: true, deletedKey: "deleted", sortKey: "_id",
		toC: simpleToC(fields, nil), toL: simpleToL(fields, nil), known: fieldKeys(fields),
		v1: v1Ops{path: "/v1/vendor-category", create: controller.CreateVendorCategory, update: controller.UpdateVendorCategory,
			delete: controller.DeleteVendorCategory},
		fieldErr: fieldErrMap(fields, nil),
	}
	return &Resource{Name: "vendorCategories", Path: "vendor-categories", Scope: "org", Module: "vendors", Legacy: "store DBs `vendor_category` (union)", Backend: b}
}

func newAccountsResource() *Resource {
	fields := []fld{{c: "code", l: "number"}, {c: "nameEn", l: "name"}, {c: "nameAr", l: "name_arabic"}, {c: "type", l: "type"},
		{c: "balance", l: "balance", k: kNum}, {c: "debitOrCredit", l: "debit_or_credit_balance"},
		{c: "referenceModel", l: "reference_model"}, {c: "referenceId", l: "reference_id", k: kRef}}
	b := &legacyBackend{coll: "account", orgOverStores: true, deletedKey: "deleted", sortKey: "number", readOnly: true,
		searchKeys: []string{"name", "number"},
		toC:        simpleToC(fields, func(x *mapCtx, d M, rec M) { rec["openingBalance"] = 0; rec["storeHint"] = x.storeHex }),
		known:      fieldKeys(fields)}
	return &Resource{Name: "accounts", Path: "accounts", Scope: "org", Module: "finance", Legacy: "store DBs `account` (read-only)", Backend: b,
		ReadOnly: "Accounts are maintained by the ledger of the existing system."}
}

// ---- workshop / hr / misc master data ----

var employeeFields = []fld{
	{c: "code", l: "code", ro: true}, {c: "nameEn", l: "name"}, {c: "nameAr", l: "name_in_arabic"},
	{c: "nationalId", l: "iqama_no"}, {c: "jobTitle", l: "position"}, {c: "basicSalary", l: "salary", k: kNum},
	{c: "salaryDay", l: "salary_day", k: kInt}, {c: "joinDate", l: "joining_date", k: kDay},
	{c: "openingBalance", l: "opening_balance", k: kNum}, {c: "openingBalanceType", l: "opening_balance_type"},
	{c: "openingBalanceDate", l: "opening_balance_date", k: kDTPlain}, {c: "addressText", l: "address"},
	{c: "phone", l: "mob1"}, {c: "phone2", l: "mob2"},
}

func newEmployeesResource() *Resource {
	b := &legacyBackend{coll: "employee", deletedKey: "deleted", sortKey: "_id",
		searchKeys: []string{"name", "name_in_arabic", "code", "mob1", "mob2", "iqama_no"}, searchSort: "name",
		toC: simpleToC(employeeFields, func(x *mapCtx, d M, rec M) {
			if boolv(d["is_active"]) || d["is_active"] == nil {
				rec["status"] = "active"
			} else {
				rec["status"] = "inactive"
			}
		}),
		toL: simpleToL(employeeFields, func(x *mapCtx, rec, prev M, ch map[string]bool, create bool, p M) error {
			if create || ch["status"] {
				p["is_active"] = str(rec["status"]) != "inactive"
			}
			if create && intv(rec["salaryDay"]) == 0 {
				p["salary_day"] = 1 // legacy requires 1..28; the contract has no pay day
			}
			if create && str(rec["joinDate"]) == "" {
				return errBadRequest("", map[string]string{"joinDate": "required"})
			}
			if create || ch["joinDate"] {
				if s := str(rec["joinDate"]); s != "" {
					ds, err := toLegacyDateStr(s)
					if err != nil {
						return errBadRequest("", map[string]string{"joinDate": "invalid date"})
					}
					delete(p, "joining_date_str")
					p["joining_date"] = ds
				}
			}
			return nil
		}),
		known: fieldKeys(employeeFields, "status"),
		validate: func(x *mapCtx, rec M, prev M) map[string]string {
			e := map[string]string{}
			if strings.TrimSpace(str(rec["nameEn"])) == "" {
				e["nameEn"] = "required"
			}
			if n := str(rec["nationalId"]); n != "" && !(len(n) == 10 && (n[0] == '1' || n[0] == '2') && regexpDigits(n, 10, 10)) {
				e["nationalId"] = "10 digits starting with 1 or 2"
			}
			if v, ok := rec["basicSalary"]; ok && num(v) < 0 {
				e["basicSalary"] = "must be >= 0"
			}
			return e
		},
		v1:       v1Ops{path: "/v1/employee", create: controller.CreateEmployee, update: controller.UpdateEmployee, delete: controller.DeleteEmployee},
		fieldErr: fieldErrMap(employeeFields, nil),
	}
	return &Resource{Name: "employees", Path: "employees", Scope: "store", Module: "hr", Legacy: "store DB `employee`", Backend: b}
}

var vehicleFields = []fld{
	{c: "plate", l: "vehicle_number"}, {c: "make", l: "brand"}, {c: "model", l: "model"}, {c: "variant", l: "variant"},
	{c: "year", l: "year", k: kInt}, {c: "color", l: "color"}, {c: "vin", l: "chassis_number"},
	{c: "engineNo", l: "engine_number"}, {c: "istimaraNo", l: "istimara_no"}, {c: "currentKm", l: "current_km", k: kNum},
	{c: "customerId", l: "customer_id", k: kRef, ref: "customer"}, {c: "customerName", l: "customer_name", ro: true},
	{c: "customerNameAr", l: "customer_name_arabic", ro: true}, {c: "notes", l: "remarks"},
}

func newVehiclesResource() *Resource {
	b := &legacyBackend{coll: "vehicle", deletedKey: "deleted", sortKey: "_id",
		searchKeys: []string{"vehicle_number", "brand", "model", "chassis_number", "customer_name", "customer_name_arabic"}, searchSort: "vehicle_number",
		toC: simpleToC(vehicleFields, nil),
		toL: simpleToL(vehicleFields, func(x *mapCtx, rec, prev M, ch map[string]bool, create bool, p M) error {
			if v, ok := p["vehicle_number"]; ok {
				p["vehicle_number"] = strings.Join(strings.Fields(strings.ToUpper(str(v))), " ")
			}
			return nil
		}),
		known: fieldKeys(vehicleFields),
		validate: func(x *mapCtx, rec M, prev M) map[string]string {
			e := map[string]string{}
			if strings.TrimSpace(str(rec["plate"])) == "" {
				e["plate"] = "required"
			}
			if strings.TrimSpace(str(rec["make"])) == "" {
				e["make"] = "required"
			}
			if str(rec["customerId"]) == "" {
				e["customerId"] = "required"
			}
			if y, ok := rec["year"]; ok && y != nil && num(y) != 0 && (num(y) < 1990 || num(y) > 2030) {
				e["year"] = "1990..2030"
			}
			return e
		},
		v1:       v1Ops{path: "/v1/vehicle", create: controller.CreateVehicle, update: controller.UpdateVehicle, delete: controller.DeleteVehicle},
		fieldErr: fieldErrMap(vehicleFields, nil),
	}
	return &Resource{Name: "vehicles", Path: "vehicles", Scope: "store", Module: "workshop", Legacy: "store DB `vehicle`", Backend: b}
}

func newSignaturesResource() *Resource {
	b := &legacyBackend{coll: "signature", deletedKey: "deleted", sortKey: "_id",
		toC: func(x *mapCtx, d M) M {
			img := str(d["signature"])
			if img != "" && !strings.Contains(img, "/") && !strings.HasPrefix(img, "data:") {
				img = "/images/" + x.storeHex + "/signatures/" + img // served by the legacy /images/ file server
			}
			return M{"name": str(d["name"]), "image": img}
		},
		toL: func(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
			p := M{}
			if create || ch["name"] {
				p["name"] = str(rec["name"])
			}
			img := str(rec["image"])
			if create || ch["image"] {
				if strings.HasPrefix(img, "data:") {
					p["signature_content"] = img
				}
			}
			return p, nil
		},
		known: knownSet("name", "image"),
		validate: func(x *mapCtx, rec M, prev M) map[string]string {
			e := map[string]string{}
			if strings.TrimSpace(str(rec["name"])) == "" {
				e["name"] = "required"
			}
			if str(rec["image"]) == "" {
				e["image"] = "required"
			}
			return e
		},
		v1:       v1Ops{path: "/v1/signature", create: controller.CreateSignature, update: controller.UpdateSignature, delete: controller.DeleteSignature},
		fieldErr: map[string]string{"name": "name", "signature_content": "image"},
	}
	return &Resource{Name: "signatures", Path: "signatures", Scope: "store", Module: "settings", Legacy: "store DB `signature`", Backend: b}
}

var packageFields = []fld{
	{c: "code", l: "code"}, {c: "customerId", l: "customer_id", k: kRef, ref: "customer"},
	{c: "customerName", l: "customer_name"}, {c: "customerNameAr", l: "customer_name_ar"},
	{c: "nameEn", l: "name"}, {c: "nameAr", l: "name_in_arabic"}, {c: "services", l: "services", k: kStrs},
	{c: "price", l: "price", k: kNum}, {c: "visits", l: "visits", k: kInt}, {c: "used", l: "used", k: kInt},
	{c: "validFrom", l: "valid_from"}, {c: "validDays", l: "valid_days", k: kInt}, {c: "status", l: "status"},
	{c: "notes", l: "notes"},
}

func newPackagesResource() *Resource {
	b := &legacyBackend{coll: "customer_package", inMain: true, mainStoreKey: "store_id", deletedKey: "deleted", sortKey: "_id",
		searchKeys: []string{"code", "name", "name_in_arabic", "customer_name", "customer_name_ar"},
		toC:        simpleToC(packageFields, nil),
		toL: simpleToL(packageFields, func(x *mapCtx, rec, prev M, ch map[string]bool, create bool, p M) error {
			p["store_id"] = x.storeHex
			return nil
		}),
		known: fieldKeys(packageFields),
		validate: func(x *mapCtx, rec M, prev M) map[string]string {
			e := map[string]string{}
			if str(rec["customerId"]) == "" {
				e["customerId"] = "required"
			}
			if strings.TrimSpace(str(rec["nameEn"])) == "" {
				e["nameEn"] = "required"
			}
			if num(rec["price"]) <= 0 {
				e["price"] = "must be > 0"
			}
			if num(rec["visits"]) <= 0 || num(rec["visits"]) != float64(intv(rec["visits"])) {
				e["visits"] = "integer > 0"
			}
			if u := num(rec["used"]); u < 0 || u > num(rec["visits"]) {
				e["used"] = "0..visits"
			}
			if num(rec["validDays"]) <= 0 {
				e["validDays"] = "must be > 0"
			}
			if str(rec["validFrom"]) == "" {
				e["validFrom"] = "required"
			}
			return e
		},
		v1: v1Ops{path: "/v1/customer-package", create: controller.CreateCustomerPackage, update: controller.UpdateCustomerPackage,
			delete: controller.DeleteCustomerPackage},
		fieldErr: fieldErrMap(packageFields, nil),
	}
	return &Resource{Name: "packages", Path: "packages", Scope: "store", Module: "sales", Legacy: "main DB `customer_package` (store_id)", Backend: b}
}

var rfqSupplierFields = []fld{
	{c: "name", l: "name"}, {c: "phone", l: "phone"}, {c: "email", l: "email"}, {c: "categories", l: "categories", k: kStrs},
	{c: "address", l: "address"}, {c: "rating", l: "rating", k: kNum}, {c: "lat", l: "latitude", k: kNum},
	{c: "lng", l: "longitude", k: kNum}, {c: "city", l: "purchase_market"}, {c: "website", l: "website"},
	{c: "code", l: "code", ro: true},
}

func newRFQSuppliersResource() *Resource {
	b := &legacyBackend{coll: "rfq_suppliers", inMain: true, mainStoreKey: "store_id", sortKey: "_id", adapterSoftDelete: true,
		searchKeys: []string{"name", "phone", "email", "code", "purchase_market"}, searchSort: "name",
		toC: simpleToC(rfqSupplierFields, func(x *mapCtx, d M, rec M) {
			rec["source"] = "manual"
			if str(d["google_place_id"]) != "" {
				rec["source"] = "google"
			}
			if d["is_active"] != nil && !boolv(d["is_active"]) {
				rec["status"] = "inactive"
			}
		}),
		toL: simpleToL(rfqSupplierFields, func(x *mapCtx, rec, prev M, ch map[string]bool, create bool, p M) error {
			if create {
				p["is_active"] = true
			}
			return nil
		}),
		known: fieldKeys(rfqSupplierFields, "source"),
		validate: func(x *mapCtx, rec M, prev M) map[string]string {
			if len(strs(rec["categories"])) == 0 {
				return map[string]string{"categories": "at least one"}
			}
			return nil
		},
		v1: v1Ops{path: "/v1/rfq-suppliers", create: controller.CreateRFQSupplierHandler, update: controller.UpdateRFQSupplierHandler,
			delete: controller.DeleteRFQSupplierHandler, storeQueryKey: "store_id"},
		fieldErr: fieldErrMap(rfqSupplierFields, nil),
	}
	return &Resource{Name: "rfqSuppliers", Path: "rfq-suppliers", Scope: "store", Module: "purchases", Legacy: "main DB `rfq_suppliers` (store_id)", Backend: b}
}

func quoteRe(s string) string {
	return strings.NewReplacer(`\`, `\\`, `.`, `\.`, `+`, `\+`, `*`, `\*`, `?`, `\?`, `(`, `\(`, `)`, `\)`, `[`, `\[`, `]`, `\]`,
		`{`, `\{`, `}`, `\}`, `^`, `\^`, `$`, `\$`, `|`, `\|`).Replace(s)
}

const time1Minute = time.Minute

var reNonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

// nextProductCode assigns "{short}-{prefix}{n}" (contract §5) for products
// created without a SKU; legacy has no product counter.
func nextProductCode(x *mapCtx) string {
	short := storeShort(x.store)
	prefix, start := serialCfg(x.store, "product")
	base := short + "-" + prefix
	max := start - 1
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := storeDB(x.storeHex).Collection("product").Find(ctx, bson.M{"item_code": bson.M{"$regex": "^" + quoteRe(base)}},
		options.Find().SetProjection(bson.M{"item_code": 1}))
	if err == nil {
		for cur.Next(ctx) {
			d := bsonToM(cur.Current)
			if n := int(num(strings.TrimPrefix(str(d["item_code"]), base))); n > max {
				max = n
			}
		}
		cur.Close(ctx)
	}
	return base + leftPad(max+1, 4)
}

func leftPad(n, w int) string {
	s := str(float64(n))
	for len(s) < w {
		s = "0" + s
	}
	return s
}

package erp

import (
	"strings"

	"github.com/sirinibin/startpos/backend/controller"
	"go.mongodb.org/mongo-driver/bson"
)

// ---- stock transfers ----
// Legacy transfers have no status: they move stock as soon as they exist.
// Contract "pending" transfers are therefore kept in a NEW collection
// (store DB `erp_stock_transfer_pending`) and only become a legacy
// `stocktransfer` (moving stock through the legacy code) when completed.

const collPendingTransfer = "erp_stock_transfer_pending"

func stockTransferToContract(x *mapCtx, d M) M {
	items := []interface{}{}
	for _, l := range arr(d["products"]) {
		lm, _ := l.(M)
		if lm == nil {
			continue
		}
		items = append(items, M{"productId": idOrNil(lm["product_id"]), "nameEn": str(lm["name"]), "nameAr": str(lm["name_in_arabic"]),
			"partNo": str(lm["part_number"]), "unit": str(lm["unit"]), "qty": num(lm["quantity"]), "unitPrice": num(lm["unit_price"])})
	}
	return M{"code": str(d["code"]), "date": x.fmtDT(d["date"]), "items": items, "remarks": str(d["remarks"]),
		"fromWarehouseId": x.whContractID(d["from_warehouse_id"], d["from_warehouse_code"]),
		"toWarehouseId":   x.whContractID(d["to_warehouse_id"], d["to_warehouse_code"]),
		"vatPercent":      docVAT(d, 0), "status": "completed", "completedAt": x.fmtDT(d["created_at"]),
		"completedBy": str(d["created_by_name"])}
}

func stockTransferToLegacy(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
	p := M{}
	ds, err := x.legacyDateStr(str(rec["date"]))
	if err != nil {
		return nil, errBadRequest("", map[string]string{"date": "required"})
	}
	p["date_str"] = ds
	fid, fcode, err := x.legacyWarehouse(str(rec["fromWarehouseId"]))
	if err != nil {
		return nil, errBadRequest("", map[string]string{"fromWarehouseId": "unknown warehouse"})
	}
	tid, tcode, err := x.legacyWarehouse(str(rec["toWarehouseId"]))
	if err != nil {
		return nil, errBadRequest("", map[string]string{"toWarehouseId": "unknown warehouse"})
	}
	p["from_warehouse_id"], p["from_warehouse_code"], p["to_warehouse_id"], p["to_warehouse_code"] = fid, fcode, tid, tcode
	vat := num(rec["vatPercent"])
	p["vat_percent"] = vat
	p["remarks"] = str(rec["remarks"])
	if create || ch["items"] {
		lines := []interface{}{}
		fields := map[string]string{}
		for i, it := range arr(rec["items"]) {
			im, _ := it.(M)
			h, err := x.ref("product", im["productId"])
			if err != nil || h == nil {
				fields["items."+itoa(i)+".productId"] = "required"
				continue
			}
			prod := x.doc("product", h.(string))
			name := str(im["nameEn"])
			if name == "" {
				name = str(prod["name"])
			}
			up := num(im["unitPrice"])
			lines = append(lines, M{"product_id": h, "name": name, "name_in_arabic": str(prod["name_in_arabic"]),
				"part_number": str(prod["part_number"]), "item_code": str(prod["item_code"]), "prefix_part_number": str(prod["prefix_part_number"]),
				"quantity": num(im["qty"]), "unit": str(prod["unit"]), "unit_price": up, "unit_price_with_vat": withVAT(up, vat),
				"purchase_unit_price": num(get(prod, "product_stores."+x.storeHex+".purchase_unit_price"))})
		}
		if len(fields) > 0 {
			return nil, errBadRequest("", fields)
		}
		p["products"] = lines
	}
	return p, nil
}

func stockTransferValidate(x *mapCtx, rec M, prev M) map[string]string {
	e := map[string]string{}
	if str(rec["date"]) == "" {
		e["date"] = "required"
	}
	if str(rec["fromWarehouseId"]) == "" {
		e["fromWarehouseId"] = "required"
	}
	if str(rec["toWarehouseId"]) == "" {
		e["toWarehouseId"] = "required"
	} else if rec["toWarehouseId"] == rec["fromWarehouseId"] {
		e["toWarehouseId"] = "must differ from source"
	}
	items := arr(rec["items"])
	if len(items) == 0 {
		e["items"] = "at least one line"
	}
	for i, it := range items {
		im, _ := it.(M)
		if num(im["qty"]) <= 0 {
			e["items."+itoa(i)+".qty"] = "must be > 0"
		}
	}
	return e
}

type stockTransfersBackend struct {
	legacy  *legacyBackend
	pending *nativeBackend
}

func (s *stockTransfersBackend) List(c *Ctx, storeHex string, q ListQuery) ([]M, int64, error) {
	rows, total, err := s.legacy.List(c, storeHex, q)
	if err != nil {
		return nil, 0, err
	}
	pend, ptotal, err := s.pending.List(c, storeHex, ListQuery{Limit: 5000, Page: 1, IncludeDeleted: q.IncludeDeleted, From: q.From, FromRaw: q.FromRaw})
	if err != nil {
		return nil, 0, err
	}
	if q.Page == 1 {
		rows = append(pend, rows...)
	}
	return rows, total + ptotal, nil
}

func (s *stockTransfersBackend) Get(c *Ctx, storeHex, id string, inc bool) (M, error) {
	if r, err := s.pending.Get(c, storeHex, id, inc); err != nil || r != nil {
		return r, err
	}
	return s.legacy.Get(c, storeHex, id, inc)
}

func (s *stockTransfersBackend) Locate(c *Ctx, id string) (string, bool) {
	if st, ok := s.pending.Locate(c, id); ok {
		return st, true
	}
	return s.legacy.Locate(c, id)
}

func (s *stockTransfersBackend) isPending(c *Ctx, storeHex, id string) bool {
	r, _ := s.pending.Get(c, storeHex, id, true)
	return r != nil
}

func (s *stockTransfersBackend) Create(c *Ctx, storeHex string, body M, meta WriteMeta) (M, error) {
	x := newMapCtx(c, storeHex)
	if errs := stockTransferValidate(x, body, nil); len(errs) > 0 {
		return nil, errBadRequest("", errs)
	}
	if str(body["status"]) == "pending" {
		return s.pending.Create(c, storeHex, body, meta)
	}
	return s.legacy.Create(c, storeHex, body, meta)
}

func (s *stockTransfersBackend) Update(c *Ctx, storeHex, id string, prev, next M, changed []string, meta WriteMeta) (M, error) {
	if s.isPending(c, storeHex, id) {
		x := newMapCtx(c, storeHex)
		if errs := stockTransferValidate(x, next, prev); len(errs) > 0 {
			return nil, errBadRequest("", errs)
		}
		if str(next["status"]) == "completed" {
			// completion: the legacy transfer is created now (moves stock via legacy code)
			body := cloneM(next)
			delete(body, "id")
			rec, err := s.legacy.Create(c, storeHex, body, WriteMeta{Action: "completed"})
			if err != nil {
				return nil, err
			}
			_ = s.pending.HardDelete(c, storeHex, id, meta)
			rec["replacedId"] = id
			return rec, nil
		}
		return s.pending.Update(c, storeHex, id, prev, next, changed, meta)
	}
	if str(next["status"]) == "pending" {
		return nil, errUnsupported("A completed transfer cannot be reopened in the existing system; create a reverse transfer instead.")
	}
	return s.legacy.Update(c, storeHex, id, prev, next, changed, meta)
}

func (s *stockTransfersBackend) Delete(c *Ctx, storeHex, id string, meta WriteMeta) (M, error) {
	if s.isPending(c, storeHex, id) {
		return s.pending.Delete(c, storeHex, id, meta)
	}
	return s.legacy.Delete(c, storeHex, id, meta)
}

func (s *stockTransfersBackend) Restore(c *Ctx, storeHex, id string, meta WriteMeta) (M, error) {
	if s.isPending(c, storeHex, id) {
		return s.pending.Restore(c, storeHex, id, meta)
	}
	return s.legacy.Restore(c, storeHex, id, meta)
}

func (s *stockTransfersBackend) HardDelete(c *Ctx, storeHex, id string, meta WriteMeta) error {
	if s.isPending(c, storeHex, id) {
		return s.pending.HardDelete(c, storeHex, id, meta)
	}
	return s.legacy.HardDelete(c, storeHex, id, meta)
}

func newStockTransfersResource() *Resource {
	lb := &legacyBackend{coll: "stocktransfer", dateKey: "date",
		toC: stockTransferToContract, toL: stockTransferToLegacy,
		known: knownSet("code", "date", "items", "remarks", "fromWarehouseId", "toWarehouseId", "vatPercent", "status", "completedAt", "completedBy"),
		v1:    v1Ops{path: "/v1/stock-transfer", create: controller.CreateStockTransfer, update: controller.UpdateStockTransfer},
		fieldErr: map[string]string{"date_str": "date", "from_warehouse_id": "fromWarehouseId", "to_warehouse_id": "toWarehouseId",
			"product_id": "items"},
		lineErr: docLineErr, lineKey: "items",
		noDelete:     "Stock transfers cannot be deleted in the existing system; create a reverse transfer instead.",
		affectsStock: true,
	}
	return &Resource{Name: "stockTransfers", Path: "stock-transfers", Scope: "store", Module: "inventory", DateField: "date",
		Legacy:  "store DB `stocktransfer` (completed) + `erp_stock_transfer_pending` (NEW, pending)",
		Backend: &stockTransfersBackend{legacy: lb, pending: &nativeBackend{coll: collPendingTransfer, dateField: "date", idPrefix: "sto", serialKey: "stockTransfer"}}}
}

// ---- repair jobs ----

func repairToContract(x *mapCtx, d M) M {
	parts := []interface{}{}
	for _, p := range arr(d["parts"]) {
		pm, _ := p.(M)
		if pm == nil {
			continue
		}
		parts = append(parts, M{"productId": idOrNil(pm["product_id"]), "nameEn": str(pm["name"]), "partNo": str(pm["part_number"]),
			"qty": num(pm["qty"]), "purchasePrice": num(pm["purchase_unit_price"]), "unitPrice": num(pm["unit_price"]),
			"unitDiscount": num(pm["unit_discount"]), "warehouseId": mainStoreWarehouseID(x.storeHex)})
	}
	return M{"code": str(d["job_number"]), "title": str(d["title"]), "date": x.fmtDT(d["date"]), "status": str(d["status"]),
		"vehicleId": idOrNil(d["vehicle_id"]), "plate": str(d["vehicle_number"]), "make": str(d["brand"]), "model": str(d["model"]),
		"customerId": idOrNil(d["customer_id"]), "customerName": str(d["customer_name"]), "odometer": num(d["km"]),
		"complaint": str(d["complaint"]), "inspection": str(d["inspection"]), "workDone": str(d["work_done"]),
		"technicianIds": ids(d["technician_ids"]), "parts": parts, "labour": num(d["labour_charge"]),
		"vatPercent": num(d["vat_percent"]), "saleId": idOrNil(d["order_id"]), "quotationId": idOrNil(d["quotation_id"]),
		"nonvatId": idOrNil(d["non_vat_sales_id"]), "estDelivery": x.fmtDay(d["estimated_delivery"]), "archived": boolv(d["archived"])}
}

func repairToLegacy(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
	p := M{}
	s := func(ck, lk string, v interface{}) {
		if create || ch[ck] {
			p[lk] = v
		}
	}
	if create || ch["date"] {
		ds, err := x.legacyDateStr(str(rec["date"]))
		if err != nil {
			return nil, errBadRequest("", map[string]string{"date": "required"})
		}
		p["date"] = ds
	}
	title := str(rec["title"])
	if title == "" {
		title = strings.TrimSpace(strings.SplitN(str(rec["complaint"]), "\n", 2)[0])
		if title == "" {
			title = "Repair job " + str(rec["plate"])
		}
	}
	if create || ch["title"] || ch["complaint"] {
		p["title"] = title
	}
	s("status", "status", str(rec["status"]))
	s("plate", "vehicle_number", str(rec["plate"]))
	s("make", "brand", str(rec["make"]))
	s("model", "model", str(rec["model"]))
	s("odometer", "km", num(rec["odometer"]))
	s("complaint", "complaint", str(rec["complaint"]))
	s("inspection", "inspection", str(rec["inspection"]))
	s("workDone", "work_done", str(rec["workDone"]))
	s("labour", "labour_charge", num(rec["labour"]))
	s("archived", "archived", boolv(rec["archived"]))
	if create || ch["vatPercent"] {
		v := x.vatPercent()
		if rv, ok := rec["vatPercent"]; ok && rv != nil {
			v = num(rv)
		}
		p["vat_percent"] = v
	}
	refs := []struct{ c, l, coll string }{{"vehicleId", "vehicle_id", "vehicle"}, {"customerId", "customer_id", "customer"},
		{"saleId", "order_id", "order"}, {"quotationId", "quotation_id", "quotation"}, {"nonvatId", "non_vat_sales_id", "non_vat_sales"}}
	for _, r := range refs {
		if create || ch[r.c] {
			h, err := x.ref(r.coll, rec[r.c])
			if err != nil {
				return nil, errBadRequest("", map[string]string{r.c: "unknown id"})
			}
			p[r.l] = h
		}
	}
	if create || ch["technicianIds"] {
		out := []interface{}{}
		for _, t := range arr(rec["technicianIds"]) {
			if h, err := x.ref("employee", t); err == nil && h != nil {
				out = append(out, h)
			}
		}
		p["technician_ids"] = out
	}
	if create || ch["estDelivery"] {
		if s := str(rec["estDelivery"]); s != "" {
			ds, err := x.legacyDateStr(s)
			if err != nil {
				return nil, errBadRequest("", map[string]string{"estDelivery": "invalid date"})
			}
			p["estimated_delivery"] = ds
		} else {
			p["estimated_delivery"] = nil
		}
	}
	if create || ch["parts"] {
		vat := num(p["vat_percent"])
		if _, ok := p["vat_percent"]; !ok {
			vat = num(rec["vatPercent"])
		}
		parts := []interface{}{}
		for i, pt := range arr(rec["parts"]) {
			pm, _ := pt.(M)
			h, err := x.ref("product", pm["productId"])
			if err != nil {
				return nil, errBadRequest("", map[string]string{"parts." + itoa(i) + ".productId": "unknown product"})
			}
			if num(pm["qty"]) <= 0 {
				return nil, errBadRequest("", map[string]string{"parts." + itoa(i) + ".qty": "must be > 0"})
			}
			up, ud, q := num(pm["unitPrice"]), num(pm["unitDiscount"]), num(pm["qty"])
			part := M{"product_id": h, "name": str(pm["nameEn"]), "part_number": str(pm["partNo"]), "qty": q,
				"purchase_unit_price": num(pm["purchasePrice"]), "unit_price": up, "unit_price_with_vat": withVAT(up, vat),
				"unit_discount": ud, "unit_discount_with_vat": withVAT(ud, vat), "total_price": round2(q * (up - ud)),
				"total_price_with_vat": round2(q * (up - ud) * (1 + vat/100))}
			if h != nil {
				if prod := x.doc("product", h.(string)); prod != nil && part["name"] == "" {
					part["name"] = str(prod["name"])
				}
			}
			parts = append(parts, part)
		}
		p["parts"] = parts
	}
	return p, nil
}

func newRepairJobsResource() *Resource {
	b := &legacyBackend{coll: "repair_job", dateKey: "date", deletedKey: "deleted",
		listWhere: map[string]whereKey{"vehicleId": {key: "vehicle_id", oid: true}, "customerId": {key: "customer_id", oid: true},
			"status": {fn: repairJobStatus}},
		// grand total with VAT, as the job keeps it (inventory/helpers.js wn().grand)
		listSumExpr: map[string]func(string) interface{}{"grand": func(string) interface{} {
			return bson.M{"$convert": bson.M{"input": "$total_with_vat", "to": "double", "onError": 0.0, "onNull": 0.0}}
		}},
		toC: repairToContract, toL: repairToLegacy,
		known: knownSet("code", "title", "date", "status", "vehicleId", "plate", "make", "model", "customerId", "customerName",
			"odometer", "complaint", "inspection", "workDone", "technicianIds", "parts", "labour", "vatPercent", "saleId",
			"quotationId", "nonvatId", "estDelivery", "archived"),
		validate: func(x *mapCtx, rec M, prev M) map[string]string {
			e := map[string]string{}
			for _, k := range []string{"date", "vehicleId", "customerId", "complaint"} {
				if str(rec[k]) == "" {
					e[k] = "required"
				}
			}
			if num(rec["odometer"]) < 0 {
				e["odometer"] = "must be >= 0"
			}
			if s := str(rec["status"]); s != "" && !map[string]bool{"open": true, "in_progress": true, "completed": true,
				"delivered": true, "closed": true, "cancelled": true}[s] {
				e["status"] = "invalid status"
			}
			return e
		},
		v1:       v1Ops{path: "/v1/repair-job", create: controller.CreateRepairJob, update: controller.UpdateRepairJob, delete: controller.DeleteRepairJob},
		fieldErr: map[string]string{"title": "complaint", "vehicle_id": "vehicleId", "customer_id": "customerId"},
	}
	return &Resource{Name: "repairJobs", Path: "repair-jobs", Scope: "store", Module: "workshop", DateField: "date", Legacy: "store DB `repair_job`", Backend: b}
}

// ---- RFQs (rfq_received, main DB) ----

func rfqToContract(x *mapCtx, d M) M {
	items := []interface{}{}
	for i, p := range arr(d["products"]) {
		pm, _ := p.(M)
		if pm == nil {
			continue
		}
		items = append(items, M{"id": "it" + itoa(i), "name": str(pm["name"]), "qty": num(pm["quantity"]), "unit": str(pm["unit"]),
			"notes": str(pm["notes"]), "productId": idOrNil(pm["product_id"]), "partNo": str(pm["part_no"])})
	}
	status := str(d["status"])
	if xs := str(get(d, "erp.x.status")); xs != "" {
		status = xs // status changes made in StartERP (legacy status is owned by the RFQ pipeline)
	}
	return M{"code": str(d["code"]), "customerName": str(d["customer_name"]), "customerId": idOrNil(d["customer_id"]),
		"source": str(d["source"]), "receivedAt": x.fmtDT(d["received_at"]), "status": status,
		"message": str(d["text_content"]), "items": items, "supplierIds": ids(d["matched_supplier_ids"]),
		"quotationIds": ids(d["quotation_ids"])}
}

func newRFQsResource() *Resource {
	b := &legacyBackend{coll: "rfq_received", inMain: true, mainStoreKey: "store_id", dateKey: "received_at",
		toC: rfqToContract,
		toL: func(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
			// the legacy update handler overwrites every customer field from the
			// body, so unchanged legacy values are always sent back.
			p := M{"customer_name": str(rec["customerName"]), "text_content": str(rec["message"])}
			if prev != nil {
				for _, k := range []string{"customer_phone", "customer_email", "customer_company", "customer_rfq_id", "general_instructions", "customer_city"} {
					p[k] = str(prev[k])
				}
			}
			if h, err := x.ref("customer", rec["customerId"]); err == nil && h != nil {
				p["customer_id"] = h
			} else {
				p["customer_id"] = ""
			}
			prods := []interface{}{}
			for _, it := range arr(rec["items"]) {
				im, _ := it.(M)
				pr := M{"name": str(im["name"]), "quantity": num(im["qty"]), "unit": str(im["unit"]), "notes": str(im["notes"]),
					"part_no": str(im["partNo"])}
				if h, err := x.ref("product", im["productId"]); err == nil && h != nil {
					pr["product_id"] = h
				}
				prods = append(prods, pr)
			}
			p["products"] = prods
			return p, nil
		},
		known:             knownSet("code", "customerName", "customerId", "source", "receivedAt", "message", "items", "supplierIds", "quotationIds"),
		adapterSoftDelete: true,
		v1: v1Ops{path: "/v1/rfq-received", create: controller.CreateRFQReceivedHandler, update: controller.UpdateRFQReceivedHandler,
			delete: controller.DeleteRFQReceivedHandler, storeQueryKey: "store_id"},
	}
	return &Resource{Name: "rfqs", Path: "rfqs", Scope: "store", Module: "purchases", DateField: "receivedAt", Legacy: "main DB `rfq_received` (store_id)", Backend: b}
}

// ---- resources with NO legacy equivalent: new additive collections ----

func newNativeResource(name, path, module, scope, coll, dateField, serialKey, prefix string, validate func(x *mapCtx, rec M, prev M) map[string]string) *Resource {
	return &Resource{Name: name, Path: path, Scope: scope, Module: module, DateField: dateField,
		Legacy:  "NEW collection `" + coll + "`",
		Backend: &nativeBackend{coll: coll, org: scope == "org", dateField: dateField, serialKey: serialKey, idPrefix: prefix, validate: validate}}
}

func purchaseBillValidate(x *mapCtx, rec M, prev M) map[string]string {
	e := map[string]string{}
	if str(rec["vendorId"]) == "" {
		e["vendorId"] = "required"
	} else if _, err := x.ref("vendor", rec["vendorId"]); err != nil {
		e["vendorId"] = "unknown vendor"
	}
	if num(rec["amount"]) <= 0 {
		e["amount"] = "must be > 0"
	}
	if str(rec["receivedAt"]) == "" {
		e["receivedAt"] = "required"
	}
	if str(rec["fileName"]) == "" {
		e["fileName"] = "required"
	}
	return e
}

func proformaValidate(x *mapCtx, rec M, prev M) map[string]string {
	e := map[string]string{}
	if str(rec["date"]) == "" {
		e["date"] = "required"
	}
	if len(arr(rec["items"])) == 0 {
		e["items"] = "at least one line"
	}
	for i, it := range arr(rec["items"]) {
		im, _ := it.(M)
		if num(im["qty"]) <= 0 {
			e["items."+itoa(i)+".qty"] = "must be > 0"
		}
		if num(im["unitPrice"]) < 0 {
			e["items."+itoa(i)+".unitPrice"] = "must be >= 0"
		}
	}
	if v, ok := rec["validityDays"]; ok && v != nil && num(v) <= 0 {
		e["validityDays"] = "must be > 0"
	}
	if v, ok := rec["deliveryDays"]; ok && v != nil && num(v) <= 0 {
		e["deliveryDays"] = "must be > 0"
	}
	return e
}

// allResources lists every contract resource (contract §11, 44 paths).
func allResources() []*Resource {
	return []*Resource{
		newStoresResource(), newUsersResource(), newRolesResource(), newCategoriesResource(), newBrandsResource(),
		newNativeResource("customerCategories", "customer-categories", "customers", "org", "erp_customer_category", "", "", "cct", nil),
		newVendorCategoriesResource(), newExpenseCategoriesResource(), newAccountsResource(), newWarehousesResource(),
		newProductsResource(), newCustomersResource(), newVendorsResource(), newEmployeesResource(), newVehiclesResource(),
		newSignaturesResource(), newPackagesResource(), newRFQSuppliersResource(), newSalesResource(), newQuotationsResource(),
		newNativeResource("proformas", "proformas", "sales", "store", "erp_proforma", "date", "proforma", "pro", proformaValidate),
		newSalesReturnsResource(), newDeliveryNotesResource(), newNonVATSalesResource(), newNonVATReturnsResource(),
		newQuotationReturnsResource(), newPurchasesResource(), newPurchaseOrdersResource(), newPurchaseRequestsResource(),
		newPurchaseReturnsResource(),
		newNativeResource("purchaseBills", "purchase-bills", "purchases", "store", "erp_purchase_bill", "receivedAt", "purchaseBill", "pbl", purchaseBillValidate),
		newStockTransfersResource(), newExpensesResource(), newDepositsResource(), newWithdrawalsResource(),
		newCapitalsResource(), newCapitalWithdrawalsResource(), newDividendsResource(), newSalariesResource(),
		newRepairJobsResource(), newRFQsResource(),
		newNativeResource("threads", "threads", "purchases", "store", "erp_thread", "", "", "thr", nil),
		newNativeResource("notifications", "notifications", "", "store", "erp_notification", "", "", "not", nil),
		newNativeResource("productSpecs", "product-specs", "inventory", "store", "erp_product_spec", "", "", "psp", productSpecValidate),
		newPosRecordsResource(),
	}
}

var _ = bson.M{}

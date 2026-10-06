package erp

import (
	"strings"

	"github.com/sirinibin/startpos/backend/controller"
	"go.mongodb.org/mongo-driver/bson"
)

var docFieldErr = map[string]string{
	"date_str": "date", "customer_id": "customerId", "vendor_id": "vendorId", "discount": "discount",
	"discount_with_vat": "discount", "cash_discount": "cashDiscount", "shipping_handling_fees": "shipping",
	"total_payment": "payments", "products": "items", "product_id": "items", "vat_percent": "vatPercent",
	"phone": "phone", "vat_no": "vatNo", "order_id": "orderId", "purchase_id": "purchaseId", "quotation_id": "quotationId",
	"commission_payment_method": "commissionMethod", "customer_credit_limit": "customerId", "delivered_by": "salesman",
	"blocked": "customerId", "zatca_reconnect": "zatca", "code": "code", "non_vat_sales_id": "orderId",
	"from_warehouse_id": "fromWarehouseId", "to_warehouse_id": "toWarehouseId", "expected_date_str": "expectedDate",
	"validity_days": "validityDays", "delivery_days": "deliveryDays",
}

var docLineErr = map[string]string{
	"product_id": "productId", "quantity": "qty", "name": "nameEn", "unit_price": "unitPrice",
	"unit_discount": "unitDiscount", "purchase_unit_price": "unitPrice", "purchasereturn_unit_price": "unitPrice",
	"unit_price_with_vat": "unitPrice", "warehouse_id": "warehouseId",
}

// docResource wires a docCfg onto a legacy collection and its v1 handlers.
func docResource(name, path, module, coll string, cfg *docCfg, ops v1Ops, desc string, mod func(b *legacyBackend)) *Resource {
	hy := knownSet()
	if cfg.rounding {
		hy["roundingAuto"] = true
	}
	b := &legacyBackend{coll: coll, dateKey: "date", deletedKey: "deleted",
		toC: func(x *mapCtx, d M) M {
			rec := cfg.toContract(x, d)
			if cfg.rounding {
				rec["roundingAuto"] = boolv(get(d, "erp.x.roundingAuto"))
			}
			return rec
		},
		toL: cfg.toLegacy, known: cfg.known(), validate: cfg.validate, v1: ops,
		fieldErr: docFieldErr, lineErr: docLineErr, lineKey: "items", hybrid: hy}
	b.settleLedger = map[string]bool{"sales": true, "salesReturns": true, "purchases": true, "purchaseReturns": true,
		"nonvatSales": true, "nonvatReturns": true, "quotationReturns": true}[name]
	b.affectsStock = map[string]bool{"sales": true, "salesReturns": true, "purchases": true, "purchaseReturns": true,
		"nonvatSales": true, "nonvatReturns": true, "quotations": true, "quotationReturns": true}[name]
	if mod != nil {
		mod(b)
	}
	return &Resource{Name: name, Path: path, Scope: "store", Module: module, DateField: "date", Legacy: desc, Backend: b}
}

func newSalesResource() *Resource {
	cfg := &docCfg{party: "customer", priceKey: "unit_price", costKey: "purchase_unit_price", payments: true,
		payColl: "sales_payment", payLink: "order_id", zatca: "invoice", cashDiscount: true, commission: true,
		rounding: true, shipping: true, discount: true,
		links: []linkF{{c: "quotationId", l: "quotation_id", coll: "quotation", codeC: "quotationCode", codeL: "quotation_code"},
			{c: "repairJobId", l: "repair_job_id", coll: "repair_job"}, {c: "vehicleId", l: "vehicle_id", coll: "vehicle"}},
		extra: []fld{{c: "poNo", l: "customer_po_no"}, {c: "km", l: "km_driven", k: kNum}},
	}
	return docResource("sales", "sales", "sales", "order", cfg,
		v1Ops{path: "/v1/order", create: controller.CreateOrder, update: controller.UpdateOrder},
		"store DB `order` (+ `sales_payment`)", func(b *legacyBackend) {
			b.deletedKey = ""
			b.noDelete = "Sales invoices cannot be deleted in the existing system; issue a sales return (credit note) instead."
		})
}

func newSalesReturnsResource() *Resource {
	cfg := &docCfg{party: "customer", priceKey: "unit_price", costKey: "purchase_unit_price", selectedLines: true,
		payments: true, payColl: "sales_return_payment", payLink: "sales_return_id", zatca: "credit", cashDiscount: true,
		commission: true, rounding: true, shipping: true, discount: true,
		links: []linkF{{c: "orderId", l: "order_id", coll: "order", codeC: "orderCode", codeL: "order_code", required: true}},
	}
	return docResource("salesReturns", "sales-returns", "sales", "salesreturn", cfg,
		v1Ops{path: "/v1/sales-return", create: controller.CreateSalesReturn, update: controller.UpdateSalesReturn,
			delete: controller.DeleteSalesReturn, restore: controller.UndeleteSalesReturn},
		"store DB `salesreturn` (+ `sales_return_payment`)", nil)
}

func newQuotationsResource() *Resource {
	cfg := &docCfg{party: "customer", priceKey: "unit_price", costKey: "purchase_unit_price", payments: true,
		payColl: "quotation_payment", payLink: "quotation_id", cashDiscount: true, commission: true, rounding: true,
		shipping: true, discount: true,
		links: []linkF{{c: "rfqId", l: "rfq_received_id", coll: "rfq_received", codeC: "rfqCode", codeL: "rfq_received_code"},
			{c: "vehicleId", l: "vehicle_id", coll: "vehicle"}, {c: "repairJobId", l: "repair_job_id", coll: "repair_job"}},
		extra: []fld{{c: "type", l: "type"}, {c: "status", l: "status"}, {c: "validityDays", l: "validity_days", k: kInt},
			{c: "deliveryDays", l: "delivery_days", k: kInt}, {c: "deliveryFrom", l: "delivery_from"},
			{c: "orderIds", l: "order_ids", k: kRefs, ref: "order"}, {c: "orderCodes", l: "order_codes", k: kStrs, ro: true}},
	}
	return docResource("quotations", "quotations", "sales", "quotation", cfg,
		v1Ops{path: "/v1/quotation", create: controller.CreateQuotation, update: controller.UpdateQuotation, delete: controller.DeleteQuotation},
		"store DB `quotation` (type quotation|invoice)", func(b *legacyBackend) {
			inner := b.validate
			b.validate = func(x *mapCtx, rec M, prev M) map[string]string {
				e := inner(x, rec, prev)
				if t := str(rec["type"]); t != "" && t != "quotation" && t != "invoice" {
					e["type"] = "quotation or invoice"
				}
				if v, ok := rec["validityDays"]; ok && v != nil && num(v) <= 0 {
					e["validityDays"] = "must be > 0"
				}
				if v, ok := rec["deliveryDays"]; ok && v != nil && num(v) <= 0 {
					e["deliveryDays"] = "must be > 0"
				}
				return e
			}
			innerL := b.toL
			b.toL = func(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
				p, err := innerL(x, rec, prev, ch, create)
				if err != nil {
					return nil, err
				}
				if create && str(p["type"]) == "" {
					p["type"] = "quotation"
				}
				if str(rec["type"]) != "invoice" && (prev == nil || str(prev["type"]) != "invoice") {
					delete(p, "payments_input") // plain quotations carry no payments
				}
				return p, nil
			}
		})
}

func newQuotationReturnsResource() *Resource {
	cfg := &docCfg{party: "customer", priceKey: "unit_price", costKey: "purchase_unit_price", selectedLines: true,
		payments: true, payColl: "quotation_sales_return_payment", payLink: "quotation_sales_return_id",
		cashDiscount: true, commission: true, rounding: true, shipping: true, discount: true,
		links: []linkF{{c: "quotationId", l: "quotation_id", coll: "quotation", codeC: "orderCode", codeL: "quotation_code", required: true}},
	}
	return docResource("quotationReturns", "quotation-returns", "sales", "quotation_sales_return", cfg,
		v1Ops{path: "/v1/quotation-sales-return", create: controller.CreateQuotationSalesReturn, update: controller.UpdateQuotationSalesReturn},
		"store DB `quotation_sales_return`", func(b *legacyBackend) {
			b.noDelete = "Quotation sales returns cannot be deleted in the existing system."
		})
}

func newDeliveryNotesResource() *Resource {
	cfg := &docCfg{party: "customer", priceKey: "unit_price", costKey: "purchase_unit_price", noWarehouse: true,
		shipping: true, discount: true, rounding: true, allowZeroPrice: true,
		links: []linkF{{c: "orderId", l: "order_id", coll: "order", codeC: "orderCode", codeL: "order_code"}},
		extra: []fld{{c: "deliveredBy", l: "delivered_by", k: kRef, ref: "user"}},
	}
	return docResource("deliveryNotes", "delivery-notes", "sales", "delivery_note", cfg,
		v1Ops{path: "/v1/delivery-note", create: controller.CreateDeliveryNote, update: controller.UpdateDeliveryNote},
		"store DB `delivery_note`", func(b *legacyBackend) {
			b.noDelete = "Delivery notes cannot be deleted in the existing system."
			inner := b.validate
			b.validate = func(x *mapCtx, rec M, prev M) map[string]string {
				e := inner(x, rec, prev)
				if str(rec["estDelivery"]) == "" && prev == nil {
					e["estDelivery"] = "required"
				}
				return e
			}
		})
}

func newNonVATSalesResource() *Resource {
	cfg := &docCfg{party: "customer", priceKey: "unit_price", costKey: "purchase_unit_price", forceVAT0: true, payments: true,
		payColl: "quotation_payment", payLink: "non_vat_sales_id", cashDiscount: true, rounding: true, shipping: true, discount: true,
		links: []linkF{{c: "repairJobId", l: "repair_job_id", coll: "repair_job"}, {c: "vehicleId", l: "vehicle_id", coll: "vehicle"}},
	}
	return docResource("nonvatSales", "nonvat-sales", "sales", "non_vat_sales", cfg,
		v1Ops{path: "/v1/non-vat-sales", create: controller.CreateNonVATSales, update: controller.UpdateNonVATSales, delete: controller.DeleteNonVATSales},
		"store DB `non_vat_sales`", func(b *legacyBackend) { b.replaceUpdate = true })
}

func newNonVATReturnsResource() *Resource {
	cfg := &docCfg{party: "customer", priceKey: "unit_price", costKey: "purchase_unit_price", forceVAT0: true, selectedLines: true,
		payments: true, cashDiscount: true, rounding: true, shipping: true, discount: true,
		links: []linkF{{c: "orderId", l: "non_vat_sales_id", coll: "non_vat_sales", codeC: "orderCode", codeL: "non_vat_sales_code", required: true}},
	}
	return docResource("nonvatReturns", "nonvat-returns", "sales", "non_vat_sales_return", cfg,
		v1Ops{path: "/v1/non-vat-sales-return", create: controller.CreateNonVATSalesReturn, update: controller.UpdateNonVATSalesReturn, delete: controller.DeleteNonVATSalesReturn},
		"store DB `non_vat_sales_return`", func(b *legacyBackend) { b.replaceUpdate = true })
}

func newPurchasesResource() *Resource {
	cfg := &docCfg{party: "vendor", priceKey: "purchase_unit_price", retailWholesale: true, payments: true,
		payColl: "purchase_payment", payLink: "purchase_id", cashDiscount: true, commission: true, rounding: true,
		shipping: true, discount: true,
	}
	return docResource("purchases", "purchases", "purchases", "purchase", cfg,
		v1Ops{path: "/v1/purchase", create: controller.CreatePurchase, update: controller.UpdatePurchase, delete: controller.DeletePurchase},
		"store DB `purchase` (+ `purchase_payment`)", func(b *legacyBackend) {
			inner := b.validate
			b.validate = func(x *mapCtx, rec M, prev M) map[string]string {
				e := inner(x, rec, prev)
				if str(rec["vendorId"]) == "" {
					e["vendorId"] = "required"
				}
				return e
			}
		})
}

func newPurchaseReturnsResource() *Resource {
	cfg := &docCfg{party: "vendor", priceKey: "purchasereturn_unit_price", selectedLines: true, payments: true,
		payColl: "purchase_return_payment", payLink: "purchase_return_id", cashDiscount: true, commission: true,
		rounding: true, shipping: true, discount: true,
		links: []linkF{{c: "purchaseId", l: "purchase_id", coll: "purchase", codeC: "purchaseCode", codeL: "purchase_code", required: true}},
	}
	return docResource("purchaseReturns", "purchase-returns", "purchases", "purchasereturn", cfg,
		v1Ops{path: "/v1/purchase-return", create: controller.CreatePurchaseReturn, update: controller.UpdatePurchaseReturn, delete: controller.DeletePurchaseReturn},
		"store DB `purchasereturn` (+ `purchase_return_payment`)", func(b *legacyBackend) {
			b.noRestore = "Purchase returns are deleted permanently by the existing system and cannot be restored."
			// legacy requires purchase_returned_by (the old app sends the signed-in user)
			inner := b.toL
			b.toL = func(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
				p, err := inner(x, rec, prev, ch, create)
				if err == nil && p["purchase_returned_by"] == nil {
					if prev != nil && prev["purchase_returned_by"] != nil {
						p["purchase_returned_by"] = hexOf(prev["purchase_returned_by"])
					} else {
						p["purchase_returned_by"] = x.c.UserID.Hex()
					}
				}
				return p, err
			}
		})
}

func newPurchaseOrdersResource() *Resource {
	cfg := &docCfg{party: "vendor", priceKey: "purchase_unit_price", shipping: true, discount: true, rounding: true,
		links: []linkF{{c: "prId", l: "purchase_request_id", coll: "purchase_request", codeC: "prCode", codeL: "purchase_request_code"},
			{c: "purchaseId", l: "purchase_id", coll: "purchase", codeC: "purchaseCode", codeL: "purchase_code"}},
		extra: []fld{{c: "status", l: "status"}, {c: "expectedDate", l: "expected_date", k: kDay}},
	}
	return docResource("purchaseOrders", "purchase-orders", "purchases", "purchase_order", cfg,
		v1Ops{path: "/v1/purchase-order", create: controller.CreatePurchaseOrder, update: controller.UpdatePurchaseOrder, delete: controller.DeletePurchaseOrder},
		"store DB `purchase_order`", func(b *legacyBackend) {
			b.replaceUpdate = true
			b.adapterSoftDelete = true // legacy DeletePurchaseOrder is a hard DeleteOne
			inner := b.validate
			b.validate = func(x *mapCtx, rec M, prev M) map[string]string {
				e := inner(x, rec, prev)
				if str(rec["vendorId"]) == "" && str(rec["status"]) != "draft" {
					e["vendorId"] = "required unless draft"
				}
				return e
			}
		})
}

func newPurchaseRequestsResource() *Resource {
	cfg := &docCfg{priceKey: "purchase_unit_price", noWarehouse: true, remarksKey: "notes", allowZeroPrice: true,
		links: []linkF{{c: "poId", l: "purchase_order_id", coll: "purchase_order", codeC: "poCode", codeL: "purchase_order_code"}},
		extra: []fld{{c: "status", l: "status"}, {c: "assignedTo", l: "assigned_to", k: kRef, ref: "user"}},
	}
	return docResource("purchaseRequests", "purchase-requests", "purchases", "purchase_request", cfg,
		v1Ops{path: "/v1/purchase-request", create: controller.CreatePurchaseRequest, update: controller.UpdatePurchaseRequest, delete: controller.DeletePurchaseRequest},
		"store DB `purchase_request`", func(b *legacyBackend) {
			b.replaceUpdate = true
			b.adapterSoftDelete = true // legacy DeletePurchaseRequest is a hard DeleteOne
			innerC := b.toC
			b.toC = func(x *mapCtx, d M) M {
				rec := innerC(x, d)
				rec["notes"] = rec["remarks"]
				return rec
			}
			innerL := b.toL
			b.toL = func(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
				r := cloneM(rec)
				if _, ok := r["remarks"]; !ok {
					r["remarks"] = r["notes"]
				}
				if ch["notes"] {
					ch["remarks"] = true
				}
				if create && str(r["assignedTo"]) == "" {
					// legacy requires an assignee: default to the requester
					r["assignedTo"] = x.c.UserID.Hex()
					ch["assignedTo"] = true
				}
				return innerL(x, r, prev, ch, create)
			}
			b.known["notes"] = true
		})
}

func newExpensesResource() *Resource {
	b := &legacyBackend{coll: "expense", dateKey: "date", deletedKey: "deleted",
		toC: func(x *mapCtx, d M) M {
			cats := ids(d["category_id"])
			cat := interface{}(nil)
			if len(cats) > 0 {
				cat = cats[0]
			}
			gross := num(d["amount"])
			vat := num(d["vat_price"])
			return M{"code": str(d["code"]), "date": x.fmtDT(d["date"]), "categoryId": cat, "description": str(d["description"]),
				"amount": round2(gross - vat), "vatAmount": vat, "method": str(d["payment_method"]),
				"vendorId": idOrNil(d["vendor_id"]), "reference": str(d["vendor_invoice_no"]), "payee": str(d["vendor_name"])}
		},
		toL: func(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
			p := M{}
			date := str(rec["date"])
			ds, err := x.legacyDateStr(date)
			if err != nil {
				return nil, errBadRequest("", map[string]string{"date": "required"})
			}
			p["date_str"] = ds
			if create || ch["amount"] || ch["vatAmount"] {
				p["amount"] = round2(num(rec["amount"]) + num(rec["vatAmount"])) // legacy amount is VAT-inclusive
			}
			if create || ch["method"] {
				p["payment_method"] = str(rec["method"])
			}
			if create || ch["categoryId"] || ch["description"] {
				h, err := x.ref("expense_category", rec["categoryId"])
				if err != nil {
					return nil, errBadRequest("", map[string]string{"categoryId": "unknown category"})
				}
				if h != nil {
					p["category_id"] = []interface{}{h}
				} else {
					p["category_id"] = []interface{}{}
				}
				desc := strings.TrimSpace(str(rec["description"]))
				if desc == "" {
					// legacy requires a description: fall back to notes / category name
					desc = strings.TrimSpace(str(rec["notes"]))
					if desc == "" && h != nil {
						desc = str(x.doc("expense_category", h.(string))["name"])
					}
				}
				p["description"] = desc
			}
			if create || ch["vendorId"] {
				h, err := x.ref("vendor", rec["vendorId"])
				if err != nil {
					return nil, errBadRequest("", map[string]string{"vendorId": "unknown vendor"})
				}
				p["vendor_id"] = h
			}
			if create || ch["reference"] {
				p["vendor_invoice_no"] = str(rec["reference"])
			}
			return p, nil
		},
		known: knownSet("code", "date", "categoryId", "description", "amount", "vatAmount", "method", "vendorId", "reference", "payee"),
		validate: func(x *mapCtx, rec M, prev M) map[string]string {
			e := map[string]string{}
			if str(rec["categoryId"]) == "" {
				e["categoryId"] = "required"
			}
			if str(rec["date"]) == "" {
				e["date"] = "required"
			}
			if num(rec["amount"]) <= 0 {
				e["amount"] = "must be > 0"
			}
			if str(rec["method"]) == "" {
				e["method"] = "required"
			} else if !validPaymentMethod(str(rec["method"])) {
				e["method"] = "invalid payment method"
			}
			if num(rec["vatAmount"]) < 0 {
				e["vatAmount"] = "must be >= 0"
			}
			return e
		},
		v1: v1Ops{path: "/v1/expense", create: controller.CreateExpense, update: controller.UpdateExpense, delete: controller.DeleteExpense},
		fieldErr: map[string]string{"date_str": "date", "amount": "amount", "payment_method": "method", "description": "description",
			"category_id": "categoryId", "vendor_id": "vendorId"},
		lineErr: map[string]string{"category_id": "categoryId"},
	}
	return &Resource{Name: "expenses", Path: "expenses", Scope: "store", Module: "finance", DateField: "date", Legacy: "store DB `expense`", Backend: b}
}

// ---- deposits (Debit Note = customerdeposit) / withdrawals (Credit Note = customerwithdrawal) ----

func moneyNoteResource(name, path, coll, payPrefix, kind string, ops v1Ops) *Resource {
	b := &legacyBackend{coll: coll, dateKey: "date", deletedKey: "deleted", settleLedger: true,
		toC: func(x *mapCtx, d M) M {
			pays := arr(d["payments"])
			var p0 M
			if len(pays) > 0 {
				p0, _ = pays[0].(M)
			}
			rec := M{"code": str(d["code"]), "date": x.fmtDT(d["date"]), "customerId": idOrNil(d["customer_id"]),
				"customerName": str(d["customer_name"]), "customerNameAr": str(d["customer_name_arabic"]),
				"amount": num(d["net_total"]), "method": str(d["payment_method"]), "reference": str(d["bank_reference_no"]),
				"notes": str(d["remarks"]), "description": str(d["description"]), "partyType": str(d["type"]),
				"orderId": nil, "orderCode": "", "paymentsCount": len(pays)}
			if rec["amount"] == 0.0 {
				rec["amount"] = num(d["amount"])
			}
			if p0 != nil {
				if rec["method"] == "" {
					rec["method"] = str(p0["method"])
				}
				if str(p0["invoice_type"]) == "sales" || str(p0["invoice_type"]) == "" {
					rec["orderId"] = idOrNil(p0["invoice_id"])
					rec["orderCode"] = str(p0["invoice_code"])
				}
			}
			if str(d["type"]) == "vendor" {
				rec["vendorId"] = idOrNil(d["vendor_id"])
				rec["vendorName"] = str(d["vendor_name"])
			}
			lz := zatcaToContract(x.loc(), d, kind)
			if boolv(get(d, "zatca.reporting_passed")) {
				rec["zatca"] = lz
			} else if xz := sub(d, "erp.x.zatca"); len(xz) > 0 {
				rec["zatca"] = xz // client-recorded report (rule 62); legacy chain untouched
			} else {
				rec["zatca"] = lz
			}
			return rec
		},
		toL: func(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
			p := M{"type": "customer"}
			if prev != nil && str(prev["type"]) != "" {
				p["type"] = str(prev["type"])
			}
			ds, err := x.legacyDateStr(str(rec["date"]))
			if err != nil {
				return nil, errBadRequest("", map[string]string{"date": "required"})
			}
			p["date_str"] = ds
			if create || ch["customerId"] {
				h, err := x.ref("customer", rec["customerId"])
				if err != nil || h == nil {
					return nil, errBadRequest("", map[string]string{"customerId": "unknown customer"})
				}
				p["customer_id"] = h
			}
			if create || ch["notes"] {
				p["remarks"] = str(rec["notes"])
			}
			if create || ch["description"] || ch["notes"] {
				desc := str(rec["description"])
				if desc == "" {
					desc = str(rec["notes"])
				}
				p["description"] = desc
			}
			if create || ch["reference"] {
				p["bank_reference_no"] = str(rec["reference"])
			}
			payChanged := create || ch["amount"] || ch["method"] || ch["orderId"] || ch["date"] || ch["reference"]
			if prev != nil && len(arr(prev["payments"])) > 1 && (ch["amount"] || ch["orderId"]) {
				return nil, errUnsupported("This note settles several invoices in the existing system; edit it in the existing app.")
			}
			pay := M{"date_str": ds, "amount": num(rec["amount"]), "discount": 0, "method": str(rec["method"])}
			if r := str(rec["reference"]); r != "" {
				pay["bank_reference"] = r
			}
			if h, err := x.ref("order", rec["orderId"]); err == nil && h != nil {
				pay["invoice_id"] = h
				pay["invoice_type"] = "sales"
				if od := x.doc("order", h.(string)); od != nil {
					pay["invoice_code"] = str(od["code"])
				}
			}
			if prev != nil {
				if ps := arr(prev["payments"]); len(ps) == 1 {
					if pm, ok := ps[0].(M); ok {
						pay["id"] = hexOf(pm["_id"])
					}
				}
			}
			if payChanged || prev != nil {
				// legacy validation needs payments[].date_str on every save
				p["payments"] = []interface{}{pay}
			}
			p["payment_method"] = str(rec["method"])
			p["enable_report_to_zatca"] = false
			return p, nil
		},
		known: knownSet("code", "date", "customerId", "customerName", "customerNameAr", "amount", "method", "reference",
			"notes", "description", "partyType", "orderId", "orderCode", "paymentsCount", "vendorId", "vendorName"),
		hybrid: knownSet("zatca"),
		validate: func(x *mapCtx, rec M, prev M) map[string]string {
			e := map[string]string{}
			if str(rec["customerId"]) == "" && (prev == nil || str(prev["type"]) == "customer" || str(prev["type"]) == "") {
				e["customerId"] = "required"
			}
			if str(rec["date"]) == "" {
				e["date"] = "required"
			}
			if num(rec["amount"]) <= 0 {
				e["amount"] = "must be > 0"
			}
			if str(rec["method"]) == "" {
				e["method"] = "required"
			} else if !validPaymentMethod(str(rec["method"])) {
				e["method"] = "invalid payment method"
			}
			return e
		},
		v1:       ops,
		fieldErr: map[string]string{"date_str": "date", "customer_id": "customerId", "payment_method": "method", "duplicate": "amount"},
	}
	desc := "store DB `" + coll + "`"
	return &Resource{Name: name, Path: path, Scope: "store", Module: "finance", DateField: "date", Legacy: desc, Backend: b}
}

func newDepositsResource() *Resource {
	return moneyNoteResource("deposits", "deposits", "customerdeposit", "customer_receivable_payment_", "invoice",
		v1Ops{path: "/v1/customer-deposit", create: controller.CreateCustomerDeposit, update: controller.UpdateCustomerDeposit, delete: controller.DeleteCustomerDeposit})
}

func newWithdrawalsResource() *Resource {
	r := moneyNoteResource("withdrawals", "withdrawals", "customerwithdrawal", "customer_payable_payment_", "credit",
		v1Ops{path: "/v1/customer-withdrawal", create: controller.CreateCustomerWithdrawal, update: controller.UpdateCustomerWithdrawal, delete: controller.DeleteCustomerWithdrawal})
	b := r.Backend.(*legacyBackend)
	inner := b.toC
	b.toC = func(x *mapCtx, d M) M {
		rec := inner(x, d)
		if t := str(get(d, "erp.x.type")); t != "" {
			rec["type"] = t
		} else {
			rec["type"] = "refund"
		}
		return rec
	}
	return r
}

// ---- capital / capital withdrawal / dividend (investor = legacy user) ----

func findOrgUserByName(c *Ctx, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	ctx, cancel := dbctx()
	defer cancel()
	f := bson.M{"name": bson.M{"$regex": "^" + quoteRe(name) + "$", "$options": "i"}, "deleted": bson.M{"$ne": true}}
	cur, err := mainDB().Collection("user").Find(ctx, f)
	if err != nil {
		return ""
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		u := bsonToM(cur.Current)
		if c.Admin {
			return hexOf(u["_id"])
		}
		for _, s := range arr(u["store_ids"]) {
			if c.store(hexOf(s)) != nil {
				return hexOf(u["_id"])
			}
		}
	}
	return ""
}

func capitalLikeResource(name, path, coll, personC, userKey, nameKey string, ops v1Ops) *Resource {
	b := &legacyBackend{coll: coll, dateKey: "date", deletedKey: "deleted",
		toC: func(x *mapCtx, d M) M {
			return M{"code": str(d["code"]), "date": x.fmtDT(d["date"]), personC: str(d[nameKey]),
				personC + "UserId": idOrNil(d[userKey]), "amount": num(d["amount"]), "method": str(d["payment_method"]),
				"notes": str(d["description"])}
		},
		toL: func(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
			p := M{}
			ds, err := x.legacyDateStr(str(rec["date"]))
			if err != nil {
				return nil, errBadRequest("", map[string]string{"date": "required"})
			}
			p["date_str"] = ds
			if create || ch[personC] || ch[personC+"UserId"] {
				uid := hexOf(rec[personC+"UserId"])
				if uid == "" || (prev != nil && ch[personC] && !ch[personC+"UserId"]) {
					uid = findOrgUserByName(x.c, str(rec[personC]))
				}
				if uid == "" {
					return nil, errBadRequest("The existing system records "+personC+"s as users: enter the name of an existing user.",
						map[string]string{personC: "must be the name of an existing user"})
				}
				p[userKey] = uid
			}
			if create || ch["amount"] {
				p["amount"] = num(rec["amount"])
			}
			if create || ch["method"] {
				p["payment_method"] = str(rec["method"])
			}
			if create || ch["notes"] {
				n := str(rec["notes"])
				if n == "" {
					n = name
				}
				p["description"] = n
			}
			return p, nil
		},
		known: knownSet("code", "date", personC, personC+"UserId", "amount", "method", "notes"),
		validate: func(x *mapCtx, rec M, prev M) map[string]string {
			e := map[string]string{}
			if str(rec[personC]) == "" && str(rec[personC+"UserId"]) == "" {
				e[personC] = "required"
			}
			if str(rec["date"]) == "" {
				e["date"] = "required"
			}
			if num(rec["amount"]) <= 0 {
				e["amount"] = "must be > 0"
			}
			if str(rec["method"]) == "" {
				e["method"] = "required"
			} else if !validPaymentMethod(str(rec["method"])) {
				e["method"] = "invalid payment method"
			}
			return e
		},
		v1:       ops,
		fieldErr: map[string]string{"date_str": "date", userKey: personC, "amount": "amount", "payment_method": "method", "description": "notes"},
	}
	return &Resource{Name: name, Path: path, Scope: "store", Module: "finance", DateField: "date", Legacy: "store DB `" + coll + "`", Backend: b}
}

func newCapitalsResource() *Resource {
	return capitalLikeResource("capitals", "capitals", "capital", "investor", "invested_by_user_id", "invested_by_user_name",
		v1Ops{path: "/v1/capital", create: controller.CreateCapital, update: controller.UpdateCapital, delete: controller.DeleteCapital})
}

func newCapitalWithdrawalsResource() *Resource {
	r := capitalLikeResource("capitalWithdrawals", "capital-withdrawals", "capitalwithdrawal", "investor", "withdrawn_by_user_id", "withdrawn_by_user_name",
		v1Ops{path: "/v1/capital-withdrawal", create: controller.CreateCapitalWithdrawal, update: controller.UpdateCapitalWithdrawal, delete: controller.DeleteCapitalWithdrawal})
	// legacy UpdateCapitalWithdrawal re-reads the stored document and ignores
	// the request body (and then fails its own date validation): the old app
	// cannot edit capital withdrawals either.
	b := r.Backend.(*legacyBackend)
	b.v1.update = nil
	b.noUpdate = "Capital withdrawals cannot be edited in the existing system (its update ignores changes); delete and re-create it."
	return r
}

func newDividendsResource() *Resource {
	return capitalLikeResource("dividends", "dividends", "divident", "recipient", "withdrawn_by_user_id", "withdrawn_by_user_name",
		v1Ops{path: "/v1/divident", create: controller.CreateDivident, update: controller.UpdateDivident, delete: controller.DeleteDivident})
}

// ---- salaries (employee_salary_payment) ----

func newSalariesResource() *Resource {
	b := &legacyBackend{coll: "employee_salary_payment", dateKey: "date", deletedKey: "deleted",
		toC: func(x *mapCtx, d M) M {
			period := ""
			if y, m := intv(d["year"]), intv(d["month"]); y > 0 && m > 0 {
				period = str(float64(y)) + "-" + pad2(m)
			}
			emp := x.doc("employee", hexOf(d["employee_id"]))
			rec := M{"code": str(d["code"]), "employeeId": idOrNil(d["employee_id"]), "employeeName": str(d["employee_name"]),
				"period": period, "netSalary": num(d["amount"]), "paymentDate": x.fmtDay(d["date"]), "method": str(d["payment_method"]),
				"notes": str(d["description"]), "status": "paid"}
			if emp != nil {
				rec["employeeNameAr"] = str(emp["name_in_arabic"])
				if _, ok := get(d, "erp.x.basicSalary").(float64); !ok {
					rec["basicSalary"] = num(emp["salary"])
				}
			}
			return rec
		},
		toL: func(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
			p := M{}
			pd := str(rec["paymentDate"])
			if pd == "" {
				pd = str(rec["period"]) + "-01"
			}
			ds, err := x.legacyDateStr(pd)
			if err != nil {
				return nil, errBadRequest("", map[string]string{"paymentDate": "invalid date"})
			}
			p["date_str"] = ds
			p["date"] = ds // legacy UpdateEmployeeSalaryPayment copies payload.Date (not date_str)
			if create || ch["employeeId"] {
				h, err := x.ref("employee", rec["employeeId"])
				if err != nil || h == nil {
					return nil, errBadRequest("", map[string]string{"employeeId": "unknown employee"})
				}
				p["employee_id"] = h
			}
			if create || ch["period"] {
				per := str(rec["period"])
				if len(per) != 7 || per[4] != '-' {
					return nil, errBadRequest("", map[string]string{"period": "YYYY-MM"})
				}
				p["year"] = intv(per[:4])
				p["month"] = intv(per[5:])
			}
			if create || ch["netSalary"] {
				p["amount"] = num(rec["netSalary"])
			}
			if create || ch["method"] {
				p["payment_method"] = str(rec["method"])
			}
			if create || ch["notes"] {
				p["description"] = str(rec["notes"])
			}
			return p, nil
		},
		known: knownSet("code", "employeeId", "employeeName", "period", "netSalary", "paymentDate", "method", "notes"),
		validate: func(x *mapCtx, rec M, prev M) map[string]string {
			e := map[string]string{}
			if str(rec["employeeId"]) == "" {
				e["employeeId"] = "required"
			}
			per := str(rec["period"])
			if len(per) != 7 || per[4] != '-' {
				e["period"] = "YYYY-MM"
			}
			if num(rec["netSalary"]) < 0 {
				e["netSalary"] = "must be >= 0"
			}
			for _, k := range []string{"housing", "transport", "otherAllowances", "deductions", "advanceDeducted"} {
				if num(rec[k]) < 0 {
					e[k] = "must be >= 0"
				}
			}
			if len(e) == 0 && x.storeHex != "" {
				// one salary per (employeeId, period)
				if h, err := x.ref("employee", rec["employeeId"]); err == nil && h != nil {
					oid, _ := oidOf(h)
					f := bson.M{"employee_id": oid, "year": intv(per[:4]), "month": intv(per[5:]), "deleted": bson.M{"$ne": true}}
					if prev != nil {
						f["_id"] = bson.M{"$ne": prev["_id"]}
					}
					ctx, cancel := dbctx()
					n, _ := storeDB(x.storeHex).Collection("employee_salary_payment").CountDocuments(ctx, f)
					cancel()
					if n > 0 {
						e["period"] = "a salary for this employee and period already exists"
					}
				}
			}
			return e
		},
		v1: v1Ops{path: "/v1/employee-salary-payment", create: controller.CreateEmployeeSalaryPayment,
			update: controller.UpdateEmployeeSalaryPayment, delete: controller.DeleteEmployeeSalaryPayment},
		fieldErr: map[string]string{"date_str": "paymentDate", "employee_id": "employeeId", "amount": "netSalary",
			"payment_method": "method", "month": "period", "year": "period"},
		replaceUpdate: true, // legacy copies only the payload's fields
	}
	return &Resource{Name: "salaries", Path: "salaries", Scope: "store", Module: "hr", DateField: "paymentDate", Legacy: "store DB `employee_salary_payment`", Backend: b}
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + str(float64(n))
	}
	return str(float64(n))
}

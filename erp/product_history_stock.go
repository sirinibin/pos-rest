package erp

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// The product's stock after each history row, the way the old app's product
// history modal shows it (reactjs-pos src/product/product_history.js, Stock
// column): the legacy code keeps a product_history record per product line of
// every sale, return, purchase, quotation sale, delivery note, transfer and
// stock adjustment, holding the stock after it ("stock") and the stock per
// warehouse ("warehouse_stocks", by warehouse code).  A row whose document has
// no record (a plain quotation, a non-VAT sale) gets the stock at its date: the
// latest record at or before it.
//
//	row["stock"]           the product's total stock after the row
//	row["warehouseStocks"] {contract warehouse id: qty} after the row

const historyColl = "product_history"

// historyWarehouseStocks maps a legacy warehouse_stocks map (by code,
// "main_store" for the main store) to contract warehouse ids.
func historyWarehouseStocks(x *mapCtx, ws M) M {
	out := M{}
	for code, v := range ws {
		id := mainStoreWarehouseID(x.storeHex)
		if code != "main_store" {
			id = x.whContractID(nil, code)
			if isMainStoreWarehouse(id) {
				continue // a deleted / unknown warehouse
			}
		}
		out[id] = roundN(num(out[id])+num(v), 4)
	}
	return out
}

// historyStockRec: what a product_history record says about the stock.
type historyStockRec struct {
	stock float64
	wh    M
}

func historyStockOf(x *mapCtx, d M) historyStockRec {
	return historyStockRec{stock: roundN(num(d["stock"]), 4), wh: historyWarehouseStocks(x, sub(d, "warehouse_stocks"))}
}

// setRowStock writes a record's stock onto a row.
func setRowStock(row M, s historyStockRec) {
	row["stock"] = s.stock
	row["warehouseStocks"] = s.wh
}

// matchHistoryStock gives each row the stock of its own product_history
// record: the k-th row of a document gets the k-th record of that document
// (the legacy code dates a product's second line one second later), the last
// one when the document has fewer records.  It returns the rows left without.
func matchHistoryStock(rows []M, byDoc map[string][]historyStockRec) []M {
	seen := map[string]int{}
	left := []M{}
	for _, row := range rows {
		doc := str(row["docId"])
		recs := byDoc[doc]
		if len(recs) == 0 {
			left = append(left, row)
			continue
		}
		k := seen[doc]
		seen[doc] = k + 1
		if k >= len(recs) {
			k = len(recs) - 1
		}
		setRowStock(row, recs[k])
	}
	return left
}

// attachHistoryStock adds stock / warehouseStocks to the rows of one page.
// rawDates: document id → its legacy date (for rows without a record).
func attachHistoryStock(x *mapCtx, pid primitive.ObjectID, rows []M, rawDates map[string]time.Time) {
	if len(rows) == 0 {
		return
	}
	col := storeDB(x.storeHex).Collection(historyColl)
	oids := bson.A{}
	for doc := range rawDates {
		if oid, ok := oidOf(doc); ok {
			oids = append(oids, oid)
		}
	}
	byDoc := map[string][]historyStockRec{}
	ctx, cancel := dbctx()
	defer cancel()
	if len(oids) > 0 {
		cur, err := col.Find(ctx, bson.M{"product_id": pid, "reference_id": bson.M{"$in": oids}},
			options.Find().SetSort(bson.D{{Key: "date", Value: 1}, {Key: "_id", Value: 1}}).
				SetProjection(bson.M{"reference_id": 1, "stock": 1, "warehouse_stocks": 1}))
		if err == nil {
			for cur.Next(ctx) {
				d := bsonToM(cur.Current)
				ref := hexOf(d["reference_id"])
				byDoc[ref] = append(byDoc[ref], historyStockOf(x, d))
			}
			cur.Close(ctx)
		}
	}
	at := map[string]historyStockRec{}
	for _, row := range matchHistoryStock(rows, byDoc) {
		doc := str(row["docId"])
		if s, ok := at[doc]; ok {
			setRowStock(row, s)
			continue
		}
		s := historyStockRec{wh: M{}}
		if t, ok := rawDates[doc]; ok {
			var d bson.M
			err := col.FindOne(ctx, bson.M{"product_id": pid, "date": bson.M{"$lte": t}},
				options.FindOne().SetSort(bson.D{{Key: "date", Value: -1}, {Key: "_id", Value: -1}}).
					SetProjection(bson.M{"stock": 1, "warehouse_stocks": 1})).Decode(&d)
			if err == nil {
				s = historyStockOf(x, normDoc(d))
			}
		}
		at[doc] = s
		setRowStock(row, s)
	}
}

// ---- kind=all: every record of the product, like the old product history modal ----

// historyRefKinds: legacy reference_type → the history kind (and so the
// permission and the document screen); "adjustment" has no document.
var historyRefKinds = map[string]string{
	"sales": "sales", "sales_return": "salesReturns", "purchase": "purchases", "purchase_return": "purchaseReturns",
	"delivery_note": "deliveryNotes", "quotation": "quotations", "quotation_invoice": "quotations",
	"quotation_sales_return": "quotationReturns", "stock_transfer": "stockTransfers",
	"stock_adjustment_by_adding": "adjustment", "stock_adjustment_by_removing": "adjustment",
}

// historyStockSign: how a record moves the stock (+1 in, -1 out, 0 none), as
// models.ComputeStockAfterEvent does; quotation invoices and their returns
// move it only when the store setting update_product_stock_on_quotation_sales
// is on.
var historyStockSign = map[string]int{
	"purchase": 1, "sales_return": 1, "stock_adjustment_by_adding": 1, "quotation_sales_return": 1,
	"sales": -1, "purchase_return": -1, "stock_adjustment_by_removing": -1, "quotation_invoice": -1,
}

func historySign(ref string, quotationStock bool) int {
	if (ref == "quotation_invoice" || ref == "quotation_sales_return") && !quotationStock {
		return 0
	}
	return historyStockSign[ref]
}

// quotationMovesStock: the store's update_product_stock_on_quotation_sales.
func quotationMovesStock(x *mapCtx) bool {
	return boolv(get(x.store, "settings.update_product_stock_on_quotation_sales"))
}

// historyAllTypes: the reference types the user may read (each kind's view
// permission; adjustments come with the product itself).
func historyAllTypes(c *Ctx) []string {
	out := []string{}
	ok := map[string]bool{"adjustment": true}
	for ref, kind := range historyRefKinds {
		if _, done := ok[kind]; !done {
			res := historyResource(kind)
			ok[kind] = res != nil && res.checkPerm(c, "view") == nil
		}
		if ok[kind] {
			out = append(out, ref)
		}
	}
	return out
}

var reHistoryDay = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

// historyAllFilter: the product_history filter for kind=all.
func historyAllFilter(x *mapCtx, pid primitive.ObjectID, types []string, q, from, to string) (bson.M, error) {
	f := bson.M{"product_id": pid, "reference_type": bson.M{"$in": types}}
	if s := strings.TrimSpace(q); s != "" {
		rx := primitive.Regex{Pattern: regexp.QuoteMeta(s), Options: "i"}
		f["$or"] = bson.A{bson.M{"reference_code": rx}, bson.M{"customer_name": rx}, bson.M{"customer_name_arabic": rx},
			bson.M{"vendor_name": rx}, bson.M{"vendor_name_arabic": rx}, bson.M{"reason": rx}}
	}
	rng := bson.M{}
	for k, v := range map[string]string{"from": from, "to": to} {
		if v == "" {
			continue
		}
		if !reHistoryDay.MatchString(v) {
			return nil, errBadRequest("Invalid "+k+".", map[string]string{k: "expected YYYY-MM-DD"})
		}
		day, err := time.ParseInLocation("2006-01-02", v[:10], x.loc())
		if err != nil {
			return nil, errBadRequest("Invalid "+k+".", map[string]string{k: "expected YYYY-MM-DD"})
		}
		if k == "from" {
			rng["$gte"] = day
		} else {
			rng["$lt"] = day.AddDate(0, 0, 1)
		}
	}
	if len(rng) > 0 {
		f["date"] = rng
	}
	return f, nil
}

// historyAllRow: one product_history record as a row.
func historyAllRow(x *mapCtx, d M, quotationStock bool) M {
	ref := str(d["reference_type"])
	qty := num(d["quantity"])
	party, partyAr, partyID := str(d["customer_name"]), str(d["customer_name_arabic"]), d["customer_id"]
	if party == "" && d["vendor_id"] != nil {
		party, partyAr, partyID = str(d["vendor_name"]), str(d["vendor_name_arabic"]), d["vendor_id"]
	}
	row := M{
		"id": hexOf(d["_id"]), "kind": historyRefKinds[ref], "refType": ref, "date": x.fmtDT(d["date"]),
		"docId": nilIfEmpty(hexOf(d["reference_id"])), "code": str(d["reference_code"]),
		"partyId": nilIfEmpty(hexOf(partyID)), "partyName": party, "partyNameAr": partyAr,
		"qty": qty, "change": float64(historySign(ref, quotationStock)) * qty, "unit": str(d["unit"]),
		"unitPrice": num(d["unit_price"]), "unitDiscount": num(d["unit_discount"]),
		"price": round2(num(d["unit_price"]) - num(d["unit_discount"])), "total": round2(num(d["net_price"])),
		"profit": round2(num(d["profit"]) - num(d["loss"])), "purchasePrice": num(d["purchase_unit_price"]),
		"warehouseId": x.whContractID(d["warehouse_id"], d["warehouse_code"]),
	}
	if strings.HasPrefix(ref, "purchase") {
		row["price"] = num(d["purchase_unit_price"])
		row["unitPrice"] = num(d["purchase_unit_price"])
	}
	if historyRefKinds[ref] == "adjustment" {
		// the legacy record points at the product itself
		row["docId"], row["code"] = nil, ""
	}
	if ref == "stock_transfer" {
		row["fromWarehouseId"] = x.whContractID(d["from_warehouse_id"], d["from_warehouse_code"])
		row["toWarehouseId"] = x.whContractID(d["to_warehouse_id"], d["to_warehouse_code"])
	}
	if r := str(d["reason"]); r != "" {
		row["reason"] = r
	}
	setRowStock(row, historyStockOf(x, d))
	return row
}

func handleProductHistoryAll(w http.ResponseWriter, r *http.Request, c *Ctx, products *Resource, pid primitive.ObjectID) {
	qv := r.URL.Query()
	page, limit := 1, historyDefaultLimit
	for k, p := range map[string]*int{"page": &page, "limit": &limit} {
		if v := qv.Get(k); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				writeErr(w, errBadRequest("Invalid "+k+".", map[string]string{k: "must be a positive integer"}))
				return
			}
			*p = n
		}
	}
	if limit > historyMaxLimit {
		limit = historyMaxLimit
	}
	storeHex, err := resolveStore(c, products, "", nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	if storeHex == "" {
		writeErr(w, errBadRequest("storeId is required.", map[string]string{"storeId": "required"}))
		return
	}
	x := newMapCtx(c, storeHex)
	f, err := historyAllFilter(x, pid, historyAllTypes(c), qv.Get("q"), qv.Get("from"), qv.Get("to"))
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx, cancel := dbctx()
	defer cancel()
	col := storeDB(storeHex).Collection(historyColl)
	total, err := col.CountDocuments(ctx, f)
	if err != nil {
		writeErr(w, errInternal("db: "+err.Error()))
		return
	}
	rows := []M{}
	sums := M{"qty": 0.0, "in": 0.0, "out": 0.0, "lines": total}
	if total > 0 {
		cur, err := col.Find(ctx, f, options.Find().SetSort(bson.D{{Key: "date", Value: -1}, {Key: "_id", Value: -1}}).
			SetSkip(int64((page-1)*limit)).SetLimit(int64(limit)))
		if err != nil {
			writeErr(w, errInternal("db: "+err.Error()))
			return
		}
		qs := quotationMovesStock(x)
		for cur.Next(ctx) {
			rows = append(rows, historyAllRow(x, bsonToM(cur.Current), qs))
		}
		cur.Close(ctx)
		agg, err := col.Aggregate(ctx, bson.A{
			bson.M{"$match": f},
			bson.M{"$group": bson.M{"_id": "$reference_type", "qty": bson.M{"$sum": "$quantity"}}},
		})
		if err == nil {
			var docs []bson.M
			if agg.All(ctx, &docs) == nil {
				var in, out float64
				for _, d := range docs {
					switch historySign(str(d["_id"]), qs) {
					case 1:
						in += num(d["qty"])
					case -1:
						out += num(d["qty"])
					}
				}
				sums["in"], sums["out"], sums["qty"] = roundN(in, 4), roundN(out, 4), roundN(in-out, 4)
			}
		}
	}
	writeJSON(w, http.StatusOK, M{"productId": pid.Hex(), "kind": "all", "data": rows, "total": total,
		"page": page, "limit": limit, "sums": sums})
}

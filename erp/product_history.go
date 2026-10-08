package erp

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// A product's sales, quotation, purchase … history, a page at a time, read
// from the documents themselves (the POS terminals and the product Info menu
// never download whole document lists for it):
//
//	GET /products/{id}/history?kind=sales&storeId=…&page=1&limit=25&q=…&from=…&to=…&where.customerId=…
//	  → {"productId", "kind", "data": [line rows, newest first], "total": documents, "page", "limit",
//	     "sums": {"qty", "value", "cost", "profit", "lines"}}
//
// q, from, to and where.<field> filter the documents the way the document list
// does (q searches the code and the party). A product on two lines of one
// document gives two rows; total counts documents (the pages).

// productHistoryKinds: kind → document resource.
var productHistoryKinds = map[string]string{
	"sales": "sales", "salesReturns": "salesReturns", "quotations": "quotations",
	"quotationReturns": "quotationReturns", "deliveryNotes": "deliveryNotes",
	"nonvatSales": "nonvatSales", "nonvatReturns": "nonvatReturns",
	"purchases": "purchases", "purchaseReturns": "purchaseReturns",
	"quotationSales": "sales", "stockTransfers": "stockTransfers",
}

// historyKindFilter narrows a kind's documents: quotation sales are the sales
// made from a quotation.
var historyKindFilter = map[string]bson.M{
	"quotationSales": {"quotation_id": bson.M{"$exists": true, "$ne": nil}},
}

// historyBackend: the legacy store collection of a document list (completed
// stock transfers for stockTransfers).
func historyBackend(res *Resource) *legacyBackend {
	switch b := res.Backend.(type) {
	case *legacyBackend:
		return b
	case *stockTransfersBackend:
		return b.legacy
	}
	return nil
}

// historyPriceKey: the legacy line field holding the kind's unit price
// (unit_price unless listed) and the cost price ("" = none).
var historyPriceKey = map[string][2]string{
	"purchases":       {"purchase_unit_price", ""},
	"purchaseReturns": {"purchasereturn_unit_price", "purchase_unit_price"},
}

func historyKeys(kind string) (price, cost string) {
	if k, ok := historyPriceKey[kind]; ok {
		return k[0], k[1]
	}
	return "unit_price", "purchase_unit_price"
}

const (
	historyDefaultLimit = 25
	historyMaxLimit     = 100
)

func historyResource(kind string) *Resource {
	name, ok := productHistoryKinds[kind]
	if !ok {
		return nil
	}
	for _, r := range Resources() {
		if r.Name == name {
			return r
		}
	}
	return nil
}

func historyKindList() string {
	return "sales, salesReturns, quotations, quotationSales, quotationReturns, deliveryNotes, nonvatSales, nonvatReturns, purchases, purchaseReturns or stockTransfers"
}

// historyRows: the lines of one rendered document that carry product pid.
func historyRows(rec M, pid string) []M {
	out := []M{}
	party := "customer"
	if _, ok := rec["vendorId"]; ok {
		party = "vendor"
	}
	for i, it := range arr(rec["items"]) {
		l, _ := it.(M)
		if l == nil || str(l["productId"]) != pid {
			continue
		}
		qty := num(l["qty"])
		price := round2(num(l["unitPrice"]) - num(l["unitDiscount"]))
		cost := num(l["purchasePrice"])
		row := M{
			"id": str(rec["id"]) + ":" + strconv.Itoa(i), "docId": rec["id"], "code": rec["code"], "date": rec["date"],
			"partyId": rec[party+"Id"], "partyName": str(rec[party+"Name"]), "partyNameAr": str(rec[party+"NameAr"]),
			"qty": qty, "unit": str(l["unit"]), "unitPrice": num(l["unitPrice"]), "unitDiscount": num(l["unitDiscount"]),
			"price": price, "total": round2(qty * price), "purchasePrice": cost, "cost": round2(qty * cost),
			"profit": round2(qty * (price - cost)), "vatPercent": num(l["vatPercent"]),
			"warehouseId": l["warehouseId"],
		}
		for _, k := range []string{"status", "type", "paymentStatus", "vendorInvoiceNo", "fromWarehouseId", "toWarehouseId", "quotationId", "quotationCode"} {
			if v, ok := rec[k]; ok && v != nil && v != "" {
				row[k] = v
			}
		}
		if r := num(l["qtyReturned"]); r != 0 {
			row["qtyReturned"] = r
		}
		out = append(out, row)
	}
	return out
}

// historySumsPipeline adds up the product's lines of the documents matching f.
func historySumsPipeline(f bson.M, pid primitive.ObjectID, kind string) bson.A {
	priceKey, costKey := historyKeys(kind)
	qty := bson.M{"$ifNull": bson.A{"$products.quantity", 0}}
	price := bson.M{"$subtract": bson.A{
		bson.M{"$ifNull": bson.A{"$products." + priceKey, 0}},
		bson.M{"$ifNull": bson.A{"$products.unit_discount", 0}},
	}}
	var cost interface{} = bson.M{"$ifNull": bson.A{"$products." + priceKey, 0}}
	if costKey != "" {
		cost = bson.M{"$ifNull": bson.A{"$products." + costKey, 0}}
	}
	return bson.A{
		bson.M{"$match": f},
		bson.M{"$project": bson.M{"products": 1}},
		bson.M{"$unwind": "$products"},
		bson.M{"$match": bson.M{"products.product_id": pid}},
		bson.M{"$group": bson.M{
			"_id":   nil,
			"qty":   bson.M{"$sum": qty},
			"value": bson.M{"$sum": bson.M{"$multiply": bson.A{qty, price}}},
			"cost":  bson.M{"$sum": bson.M{"$multiply": bson.A{qty, cost}}},
			"lines": bson.M{"$sum": 1},
		}},
	}
}

func historySums(doc bson.M) M {
	value, cost := round2(num(doc["value"])), round2(num(doc["cost"]))
	return M{"qty": round2(num(doc["qty"])), "value": value, "cost": cost, "profit": round2(value - cost),
		"lines": int64(num(doc["lines"]))}
}

func handleProductHistory(w http.ResponseWriter, r *http.Request, products *Resource) {
	c, err := authenticate(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := products.checkPerm(c, "view"); err != nil {
		writeErr(w, err)
		return
	}
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	res := historyResource(kind)
	if res == nil {
		writeErr(w, errBadRequest("Unknown history kind.", map[string]string{"kind": "one of " + historyKindList()}))
		return
	}
	if err := res.checkPerm(c, "view"); err != nil {
		writeErr(w, err)
		return
	}
	pid, perr := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if perr != nil {
		writeErr(w, errNotFound())
		return
	}
	q, err := parseListQuery(r, res)
	if err != nil {
		writeErr(w, err)
		return
	}
	if r.URL.Query().Get("limit") == "" {
		q.Limit = historyDefaultLimit
	}
	if q.Limit > historyMaxLimit {
		q.Limit = historyMaxLimit
	}
	q.Sort, q.Desc, q.Sum, q.IDs = "", false, nil, nil
	storeHex, err := resolveStore(c, res, "", nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	if storeHex == "" {
		writeErr(w, errBadRequest("storeId is required.", map[string]string{"storeId": "required"}))
		return
	}
	b := historyBackend(res)
	if b == nil {
		writeErr(w, errNotFound())
		return
	}
	f, _, err := b.listFilter(c, storeHex, q)
	if err != nil {
		writeErr(w, err)
		return
	}
	f = andFilter(f, historyKindFilter[kind], bson.M{"products.product_id": pid})
	ctx, cancel := dbctx()
	defer cancel()
	col := b.col(storeHex)
	total, err := col.CountDocuments(ctx, f)
	if err != nil {
		writeErr(w, errInternal("db: "+err.Error()))
		return
	}
	rows := []M{}
	if total > 0 {
		opts := options.Find().SetSort(bson.D{{Key: b.dateKey, Value: -1}, {Key: "_id", Value: -1}}).
			SetSkip(int64((q.Page - 1) * q.Limit)).SetLimit(int64(q.Limit)).
			SetProjection(bson.M{envKey + ".h": 0})
		cur, err := col.Find(ctx, f, opts)
		if err != nil {
			writeErr(w, errInternal("db: "+err.Error()))
			return
		}
		x := newMapCtx(c, storeHex)
		for cur.Next(ctx) {
			rec := b.render(x, bsonToM(cur.Current), b.storeScoped())
			rows = append(rows, historyRows(rec, pid.Hex())...)
		}
		cur.Close(ctx)
	}
	sums := historySums(bson.M{})
	if total > 0 {
		cur, err := col.Aggregate(ctx, historySumsPipeline(f, pid, kind))
		if err != nil {
			writeErr(w, errInternal("db: "+err.Error()))
			return
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			writeErr(w, errInternal("db: "+err.Error()))
			return
		}
		if len(docs) > 0 {
			sums = historySums(docs[0])
		}
	}
	writeJSON(w, http.StatusOK, M{"productId": pid.Hex(), "kind": kind, "data": rows, "total": total,
		"page": q.Page, "limit": q.Limit, "sums": sums})
}

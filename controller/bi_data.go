package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ── helpers ────────────────────────────────────────────────────────────────────

func parseDateParam(r *http.Request, key string) (t time.Time, ok bool) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, v); err == nil {
			return parsed, true
		}
	}
	return
}

func oidStr(id *primitive.ObjectID) string {
	if id == nil {
		return ""
	}
	return id.Hex()
}

func biAuthAndStore(w http.ResponseWriter, r *http.Request) (*models.Store, bool) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false
		resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return nil, false
	}
	store, err := ParseStore(r)
	if err != nil {
		resp.Status = false
		resp.Errors["store_id"] = "Invalid store_id: " + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return nil, false
	}
	return store, true
}

// ── BIProductSalesHistory  GET /v1/bi/product-sales-history ───────────────────

type BISalesHistoryRow struct {
	ProductID         string  `json:"product_id"`
	Date              string  `json:"date"`
	Quantity          float64 `json:"quantity"`
	UnitPrice         float64 `json:"unit_price"`
	Price             float64 `json:"price"`
	NetPrice          float64 `json:"net_price"`
	Profit            float64 `json:"profit"`
	Loss              float64 `json:"loss"`
	PurchaseUnitPrice float64 `json:"purchase_unit_price"`
	CustomerID        string  `json:"customer_id"`
	CustomerName      string  `json:"customer_name"`
	OrderID           string  `json:"order_id"`
}

func BIProductSalesHistory(w http.ResponseWriter, r *http.Request) {
	store, ok := biAuthAndStore(w, r)
	if !ok {
		return
	}

	filter := bson.M{}
	if fromDate, ok := parseDateParam(r, "from_date"); ok {
		filter["date"] = bson.M{"$gte": fromDate}
	}
	if toDate, ok := parseDateParam(r, "to_date"); ok {
		if existing, has := filter["date"]; has {
			filter["date"] = bson.M{"$gte": existing.(bson.M)["$gte"], "$lte": toDate}
		} else {
			filter["date"] = bson.M{"$lte": toDate}
		}
	}

	proj := bson.M{
		"product_id": 1, "date": 1, "quantity": 1,
		"unit_price": 1, "price": 1, "net_price": 1,
		"profit": 1, "loss": 1, "purchase_unit_price": 1,
		"customer_id": 1, "customer_name": 1, "order_id": 1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	coll := db.GetDB("store_" + store.ID.Hex()).Collection("product_sales_history")
	cursor, err := coll.Find(ctx, filter, options.Find().SetProjection(proj))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": false, "error": err.Error()})
		return
	}
	defer cursor.Close(ctx)

	type rawRow struct {
		ProductID         primitive.ObjectID  `bson:"product_id"`
		Date              *time.Time          `bson:"date"`
		Quantity          float64             `bson:"quantity"`
		UnitPrice         float64             `bson:"unit_price"`
		Price             float64             `bson:"price"`
		NetPrice          float64             `bson:"net_price"`
		Profit            float64             `bson:"profit"`
		Loss              float64             `bson:"loss"`
		PurchaseUnitPrice float64             `bson:"purchase_unit_price"`
		CustomerID        *primitive.ObjectID `bson:"customer_id"`
		CustomerName      string              `bson:"customer_name"`
		OrderID           *primitive.ObjectID `bson:"order_id"`
	}

	var rows []BISalesHistoryRow
	for cursor.Next(ctx) {
		var r rawRow
		if err := cursor.Decode(&r); err != nil {
			continue
		}
		row := BISalesHistoryRow{
			ProductID:         r.ProductID.Hex(),
			Quantity:          r.Quantity,
			UnitPrice:         r.UnitPrice,
			Price:             r.Price,
			NetPrice:          r.NetPrice,
			Profit:            r.Profit,
			Loss:              r.Loss,
			PurchaseUnitPrice: r.PurchaseUnitPrice,
			CustomerName:      r.CustomerName,
			CustomerID:        oidStr(r.CustomerID),
			OrderID:           oidStr(r.OrderID),
		}
		if r.Date != nil {
			row.Date = r.Date.Format(time.RFC3339)
		}
		rows = append(rows, row)
	}
	if rows == nil {
		rows = []BISalesHistoryRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"status": true, "result": rows})
}

// ── BIProducts  GET /v1/bi/products ──────────────────────────────────────────

type BIProductRow struct {
	ID                string  `json:"product_id"`
	Name              string  `json:"name"`
	PartNumber        string  `json:"part_number"`
	ItemCode          string  `json:"item_code"`
	Stock             float64 `json:"stock"`
	PurchaseUnitPrice float64 `json:"purchase_unit_price"`
}

func BIProducts(w http.ResponseWriter, r *http.Request) {
	store, ok := biAuthAndStore(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pipeline := bson.A{
		bson.M{"$match": bson.M{"stores.store_id": store.ID}},
		bson.M{"$addFields": bson.M{
			"_store": bson.M{"$first": bson.M{
				"$filter": bson.M{
					"input": "$stores",
					"as":    "s",
					"cond":  bson.M{"$eq": bson.A{"$$s.store_id", store.ID}},
				},
			}},
		}},
		bson.M{"$project": bson.M{
			"name":                1,
			"part_number":         1,
			"item_code":           1,
			"stock":               "$_store.stock",
			"purchase_unit_price": "$_store.purchase_unit_price",
		}},
	}

	coll := db.GetDB("store_" + store.ID.Hex()).Collection("product")
	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": false, "error": err.Error()})
		return
	}
	defer cursor.Close(ctx)

	type rawProd struct {
		ID                primitive.ObjectID `bson:"_id"`
		Name              string             `bson:"name"`
		PartNumber        string             `bson:"part_number"`
		ItemCode          string             `bson:"item_code"`
		Stock             float64            `bson:"stock"`
		PurchaseUnitPrice float64            `bson:"purchase_unit_price"`
	}

	var rows []BIProductRow
	for cursor.Next(ctx) {
		var p rawProd
		if err := cursor.Decode(&p); err != nil {
			continue
		}
		rows = append(rows, BIProductRow{
			ID:                p.ID.Hex(),
			Name:              p.Name,
			PartNumber:        p.PartNumber,
			ItemCode:          p.ItemCode,
			Stock:             p.Stock,
			PurchaseUnitPrice: p.PurchaseUnitPrice,
		})
	}
	if rows == nil {
		rows = []BIProductRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"status": true, "result": rows})
}

// ── BICustomers  GET /v1/bi/customers ────────────────────────────────────────

type BICustomerRow struct {
	ID                    string  `json:"customer_id"`
	Name                  string  `json:"customer_name"`
	Phone                 string  `json:"phone"`
	CreditBalance         float64 `json:"credit_balance"`
	SalesBalanceAmount    float64 `json:"sales_balance_amount"`
	CreatedAt             string  `json:"created_at"`
}

func BICustomers(w http.ResponseWriter, r *http.Request) {
	store, ok := biAuthAndStore(w, r)
	if !ok {
		return
	}

	storeHex := store.ID.Hex()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	proj := bson.M{
		"name": 1, "phone": 1, "credit_balance": 1,
		"created_at": 1,
		"sales_balance": bson.M{"$ifNull": bson.A{
			"$stores." + storeHex + ".sales_balance_amount", 0,
		}},
	}

	coll := db.GetDB("store_" + storeHex).Collection("customer")
	cursor, err := coll.Find(ctx, bson.M{}, options.Find().SetProjection(proj))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": false, "error": err.Error()})
		return
	}
	defer cursor.Close(ctx)

	type rawCust struct {
		ID            primitive.ObjectID `bson:"_id"`
		Name          string             `bson:"name"`
		Phone         string             `bson:"phone"`
		CreditBalance float64            `bson:"credit_balance"`
		SalesBalance  float64            `bson:"sales_balance"`
		CreatedAt     *time.Time         `bson:"created_at"`
	}

	var rows []BICustomerRow
	for cursor.Next(ctx) {
		var c rawCust
		if err := cursor.Decode(&c); err != nil {
			continue
		}
		row := BICustomerRow{
			ID:                 c.ID.Hex(),
			Name:               c.Name,
			Phone:              c.Phone,
			CreditBalance:      c.CreditBalance,
			SalesBalanceAmount: c.SalesBalance,
		}
		if c.CreatedAt != nil {
			row.CreatedAt = c.CreatedAt.Format(time.RFC3339)
		}
		rows = append(rows, row)
	}
	if rows == nil {
		rows = []BICustomerRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"status": true, "result": rows})
}

// ── BIOrders  GET /v1/bi/orders ──────────────────────────────────────────────

type BIOrderRow struct {
	ID             string  `json:"order_id"`
	Code           string  `json:"code"`
	Date           string  `json:"date"`
	CustomerID     string  `json:"customer_id"`
	CustomerName   string  `json:"customer_name"`
	NetTotal       float64 `json:"net_total"`
	BalanceAmount  float64 `json:"balance_amount"`
	PaymentStatus  string  `json:"payment_status"`
}

func BIOrders(w http.ResponseWriter, r *http.Request) {
	store, ok := biAuthAndStore(w, r)
	if !ok {
		return
	}

	filter := bson.M{}
	dateFilter := bson.M{}
	if fromDate, ok := parseDateParam(r, "from_date"); ok {
		dateFilter["$gte"] = fromDate
	}
	if toDate, ok := parseDateParam(r, "to_date"); ok {
		dateFilter["$lte"] = toDate
	}
	if len(dateFilter) > 0 {
		filter["date"] = dateFilter
	}

	proj := bson.M{
		"code": 1, "date": 1,
		"customer_id": 1, "customer_name": 1,
		"net_total": 1, "balance_amount": 1, "payment_status": 1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	coll := db.GetDB("store_" + store.ID.Hex()).Collection("order")
	cursor, err := coll.Find(ctx, filter, options.Find().SetProjection(proj))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": false, "error": err.Error()})
		return
	}
	defer cursor.Close(ctx)

	type rawOrder struct {
		ID            primitive.ObjectID  `bson:"_id"`
		Code          string              `bson:"code"`
		Date          *time.Time          `bson:"date"`
		CustomerID    *primitive.ObjectID `bson:"customer_id"`
		CustomerName  string              `bson:"customer_name"`
		NetTotal      float64             `bson:"net_total"`
		BalanceAmount float64             `bson:"balance_amount"`
		PaymentStatus string              `bson:"payment_status"`
	}

	var rows []BIOrderRow
	for cursor.Next(ctx) {
		var o rawOrder
		if err := cursor.Decode(&o); err != nil {
			continue
		}
		row := BIOrderRow{
			ID:            o.ID.Hex(),
			Code:          o.Code,
			CustomerID:    oidStr(o.CustomerID),
			CustomerName:  o.CustomerName,
			NetTotal:      o.NetTotal,
			BalanceAmount: o.BalanceAmount,
			PaymentStatus: o.PaymentStatus,
		}
		if o.Date != nil {
			row.Date = o.Date.Format(time.RFC3339)
		}
		rows = append(rows, row)
	}
	if rows == nil {
		rows = []BIOrderRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"status": true, "result": rows})
}

// ── BILedger  GET /v1/bi/ledger ──────────────────────────────────────────────

type BILedgerRow struct {
	Date           string  `json:"date"`
	ReferenceModel string  `json:"reference_model"`
	Debit          float64 `json:"debit"`
	Credit         float64 `json:"credit"`
}

func BILedger(w http.ResponseWriter, r *http.Request) {
	store, ok := biAuthAndStore(w, r)
	if !ok {
		return
	}

	filter := bson.M{}
	dateFilter := bson.M{}
	if fromDate, ok := parseDateParam(r, "from_date"); ok {
		dateFilter["$gte"] = fromDate
	}
	if toDate, ok := parseDateParam(r, "to_date"); ok {
		dateFilter["$lte"] = toDate
	}

	var pipeline bson.A
	if len(dateFilter) > 0 {
		pipeline = bson.A{
			bson.M{"$unwind": "$journals"},
			bson.M{"$match": bson.M{"journals.date": dateFilter}},
		}
	} else {
		pipeline = bson.A{
			bson.M{"$match": filter},
			bson.M{"$unwind": "$journals"},
		}
	}
	pipeline = append(pipeline, bson.M{"$project": bson.M{
		"date":            "$journals.date",
		"reference_model": "$reference_model",
		"debit":           "$journals.debit",
		"credit":          "$journals.credit",
	}})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	coll := db.GetDB("store_" + store.ID.Hex()).Collection("ledger")
	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": false, "error": err.Error()})
		return
	}
	defer cursor.Close(ctx)

	type rawLedger struct {
		Date           *time.Time `bson:"date"`
		ReferenceModel string     `bson:"reference_model"`
		Debit          float64    `bson:"debit"`
		Credit         float64    `bson:"credit"`
	}

	var rows []BILedgerRow
	for cursor.Next(ctx) {
		var l rawLedger
		if err := cursor.Decode(&l); err != nil {
			continue
		}
		row := BILedgerRow{
			ReferenceModel: l.ReferenceModel,
			Debit:          l.Debit,
			Credit:         l.Credit,
		}
		if l.Date != nil {
			row.Date = l.Date.Format(time.RFC3339)
		}
		rows = append(rows, row)
	}
	if rows == nil {
		rows = []BILedgerRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"status": true, "result": rows})
}

// ── BIStoreSettings  GET /v1/bi/store-settings ───────────────────────────────

type BIStoreSettingsResult struct {
	QuotationInvoiceAccounting bool    `json:"quotation_invoice_accounting"`
	DisablePurchasesOnAccounts bool    `json:"disable_purchases_on_accounts"`
	VatPercent                 float64 `json:"vat_percent"`
}

func BIStoreSettings(w http.ResponseWriter, r *http.Request) {
	store, ok := biAuthAndStore(w, r)
	if !ok {
		return
	}
	result := BIStoreSettingsResult{
		QuotationInvoiceAccounting: store.Settings.QuotationInvoiceAccounting,
		DisablePurchasesOnAccounts: store.Settings.DisablePurchasesOnAccounts,
		VatPercent:                 store.VatPercent,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": true, "result": result})
}

// ── BISalesReturns  GET /v1/bi/sales-returns ─────────────────────────────────

type BISalesReturnRow struct {
	ProductID    string  `json:"product_id"`
	ProductName  string  `json:"product_name"`
	Date         string  `json:"date"`
	Quantity     float64 `json:"quantity"`
	Price        float64 `json:"price"`
	Profit       float64 `json:"profit"`
	Loss         float64 `json:"loss"`
	CustomerID   string  `json:"customer_id"`
	CustomerName string  `json:"customer_name"`
	OrderID      string  `json:"order_id"`
}

func BISalesReturns(w http.ResponseWriter, r *http.Request) {
	store, ok := biAuthAndStore(w, r)
	if !ok {
		return
	}

	filter := bson.M{}
	dateFilter := bson.M{}
	if fromDate, ok := parseDateParam(r, "from_date"); ok {
		dateFilter["$gte"] = fromDate
	}
	if toDate, ok := parseDateParam(r, "to_date"); ok {
		dateFilter["$lte"] = toDate
	}
	if len(dateFilter) > 0 {
		filter["date"] = dateFilter
	}

	proj := bson.M{
		"product_id": 1, "product_name": 1, "date": 1,
		"quantity": 1, "price": 1, "profit": 1, "loss": 1,
		"customer_id": 1, "customer_name": 1, "order_id": 1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	coll := db.GetDB("store_" + store.ID.Hex()).Collection("product_sales_return_history")
	cursor, err := coll.Find(ctx, filter, options.Find().SetProjection(proj))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": false, "error": err.Error()})
		return
	}
	defer cursor.Close(ctx)

	type rawReturn struct {
		ProductID    primitive.ObjectID  `bson:"product_id"`
		ProductName  string              `bson:"product_name"`
		Date         *time.Time          `bson:"date"`
		Quantity     float64             `bson:"quantity"`
		Price        float64             `bson:"price"`
		Profit       float64             `bson:"profit"`
		Loss         float64             `bson:"loss"`
		CustomerID   *primitive.ObjectID `bson:"customer_id"`
		CustomerName string              `bson:"customer_name"`
		OrderID      *primitive.ObjectID `bson:"order_id"`
	}

	var rows []BISalesReturnRow
	for cursor.Next(ctx) {
		var ret rawReturn
		if err := cursor.Decode(&ret); err != nil {
			continue
		}
		row := BISalesReturnRow{
			ProductID:    ret.ProductID.Hex(),
			ProductName:  ret.ProductName,
			Quantity:     ret.Quantity,
			Price:        ret.Price,
			Profit:       ret.Profit,
			Loss:         ret.Loss,
			CustomerName: ret.CustomerName,
			CustomerID:   oidStr(ret.CustomerID),
			OrderID:      oidStr(ret.OrderID),
		}
		if ret.Date != nil {
			row.Date = ret.Date.Format(time.RFC3339)
		}
		rows = append(rows, row)
	}
	if rows == nil {
		rows = []BISalesReturnRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"status": true, "result": rows})
}

package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ─── Response types (returned by entity aggregation endpoints) ────────────────

type DashboardProductSummary struct {
	ProductName  string  `bson:"product_name"  json:"product_name"`
	SalesRevenue float64 `bson:"sales_revenue" json:"sales_revenue"`
	QtnRevenue   float64 `bson:"qtn_revenue"   json:"qtn_revenue"`
	TotalRevenue float64 `bson:"total_revenue" json:"total_revenue"`
}

type DashboardCustomerSummary struct {
	CustomerName string  `bson:"customer_name" json:"customer_name"`
	SalesAmount  float64 `bson:"sales_amount"  json:"sales_amount"`
	QtnAmount    float64 `bson:"qtn_amount"    json:"qtn_amount"`
	TotalAmount  float64 `bson:"total_amount"  json:"total_amount"`
	Outstanding  float64 `bson:"outstanding"   json:"outstanding"`
}

type DashboardCategorySummary struct {
	CategoryName string  `bson:"category_name" json:"category_name"`
	Sales        float64 `bson:"sales"         json:"sales"`
	Profit       float64 `bson:"profit"        json:"profit"`
}

type DashboardVendorSummary struct {
	VendorName     string  `bson:"vendor_name"     json:"vendor_name"`
	PurchaseAmount float64 `bson:"purchase_amount" json:"purchase_amount"`
}

type DashboardAccountSummary struct {
	AccountType string  `bson:"account_type" json:"account_type"`
	Balance     float64 `bson:"balance"      json:"balance"`
}

type DashboardStockSummary struct {
	OutOfStock   int `json:"out_of_stock"`
	LowStock     int `json:"low_stock"`
	HealthyStock int `json:"healthy_stock"`
	Total        int `json:"total"`
}

// ─── dateRange helper ────────────────────────────────────────────────────────

// dateRangeFilter builds a MongoDB date filter from local YYYY-MM-DD strings and tzOffset.
// Returns nil if both strings are empty (no filter).
func dateRangeFilter(fromDateStr, toDateStr string, tzOffset float64) bson.M {
	if fromDateStr == "" && toDateStr == "" {
		return nil
	}
	dur := time.Duration(float64(time.Hour) * tzOffset)
	df := bson.M{}
	if fromDateStr != "" {
		t, err := time.Parse("2006-01-02", fromDateStr)
		if err == nil {
			df["$gte"] = t.Add(dur)
		}
	}
	if toDateStr != "" {
		t, err := time.Parse("2006-01-02", toDateStr)
		if err == nil {
			df["$lt"] = t.AddDate(0, 0, 1).Add(dur) // exclusive end = next day start
		}
	}
	if len(df) == 0 {
		return nil
	}
	return bson.M{"date": df}
}

// ─── Top Products ─────────────────────────────────────────────────────────────

// GetDashboardTopProducts aggregates order line-items and quotation-invoice line-items
// by product name, returning the top `limit` products by total revenue.
func GetDashboardTopProducts(storeID primitive.ObjectID, fromDateStr, toDateStr string, tzOffset float64, limit int) ([]DashboardProductSummary, error) {
	if limit <= 0 {
		limit = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sdb := db.GetDB("store_" + storeID.Hex())

	dateF := dateRangeFilter(fromDateStr, toDateStr, tzOffset)
	orderMatch := bson.M{}
	if dateF != nil {
		for k, v := range dateF {
			orderMatch[k] = v
		}
	}

	// Aggregate from orders (unit_price × quantity per product line-item)
	salesMap, err := aggregateProductRevenue(ctx, sdb.Collection("order"), orderMatch)
	if err != nil {
		return nil, err
	}

	// Aggregate from quotation invoices
	qtnMatch := bson.M{"type": "invoice"}
	if dateF != nil {
		for k, v := range dateF {
			qtnMatch[k] = v
		}
	}
	qtnMap, _ := aggregateProductRevenue(ctx, sdb.Collection("quotation"), qtnMatch)

	// Merge
	combined := map[string]*DashboardProductSummary{}
	for name, rev := range salesMap {
		combined[name] = &DashboardProductSummary{ProductName: name, SalesRevenue: rev}
	}
	for name, rev := range qtnMap {
		if e, ok := combined[name]; ok {
			e.QtnRevenue = rev
		} else {
			combined[name] = &DashboardProductSummary{ProductName: name, QtnRevenue: rev}
		}
	}

	results := make([]DashboardProductSummary, 0, len(combined))
	for _, s := range combined {
		s.TotalRevenue = s.SalesRevenue + s.QtnRevenue
		if s.TotalRevenue > 0 {
			results = append(results, *s)
		}
	}
	// Sort descending by total revenue and take top N
	sortDashboardProducts(results)
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func aggregateProductRevenue(ctx context.Context, coll *mongo.Collection, match bson.M) (map[string]float64, error) {
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$unwind", Value: "$products"}},
		{{Key: "$group", Value: bson.M{
			"_id": "$products.name",
			"revenue": bson.M{"$sum": bson.M{
				"$multiply": []interface{}{"$products.unit_price", "$products.quantity"},
			}},
		}}},
	}
	cur, err := coll.Aggregate(ctx, pipe)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := map[string]float64{}
	for cur.Next(ctx) {
		var r struct {
			Name    string  `bson:"_id"`
			Revenue float64 `bson:"revenue"`
		}
		if cur.Decode(&r) == nil && r.Name != "" {
			out[r.Name] += r.Revenue
		}
	}
	return out, nil
}

func sortDashboardProducts(s []DashboardProductSummary) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].TotalRevenue > s[j-1].TotalRevenue; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ─── Top Customers ────────────────────────────────────────────────────────────

// GetDashboardTopCustomers returns top customers by revenue, merging sales orders
// and quotation invoices. Outstanding comes from customer.credit_balance.
func GetDashboardTopCustomers(storeID primitive.ObjectID, fromDateStr, toDateStr string, tzOffset float64, limit int) ([]DashboardCustomerSummary, error) {
	if limit <= 0 {
		limit = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sdb := db.GetDB("store_" + storeID.Hex())

	dateF := dateRangeFilter(fromDateStr, toDateStr, tzOffset)

	// Orders grouped by customer_name
	orderMatch := bson.M{}
	if dateF != nil {
		for k, v := range dateF {
			orderMatch[k] = v
		}
	}
	salesMap := aggregateByCustomerName(ctx, sdb.Collection("order"), orderMatch, "net_total")

	// Quotation invoices grouped by customer_name
	qtnMatch := bson.M{"type": "invoice"}
	if dateF != nil {
		for k, v := range dateF {
			qtnMatch[k] = v
		}
	}
	qtnMap := aggregateByCustomerName(ctx, sdb.Collection("quotation"), qtnMatch, "net_total")

	// Outstanding from customer collection (not date-filtered — always current balance)
	outstandingMap := map[string]float64{}
	custCur, err := db.GetDB("store_"+storeID.Hex()).Collection("customer").
		Find(ctx, bson.M{"credit_balance": bson.M{"$gt": 0}, "deleted": bson.M{"$ne": true}},
			options.Find().SetProjection(bson.M{"name": 1, "credit_balance": 1}))
	if err == nil {
		defer custCur.Close(ctx)
		for custCur.Next(ctx) {
			var c struct {
				Name          string  `bson:"name"`
				CreditBalance float64 `bson:"credit_balance"`
			}
			if custCur.Decode(&c) == nil {
				outstandingMap[c.Name] = c.CreditBalance
			}
		}
	}

	// Merge
	allNames := map[string]struct{}{}
	for n := range salesMap {
		allNames[n] = struct{}{}
	}
	for n := range qtnMap {
		allNames[n] = struct{}{}
	}

	results := make([]DashboardCustomerSummary, 0, len(allNames))
	for name := range allNames {
		if name == "" || name == "UNKNOWN" {
			continue
		}
		s := salesMap[name]
		q := qtnMap[name]
		total := s + q
		if total <= 0 {
			continue
		}
		results = append(results, DashboardCustomerSummary{
			CustomerName: name,
			SalesAmount:  s,
			QtnAmount:    q,
			TotalAmount:  total,
			Outstanding:  outstandingMap[name],
		})
	}
	sortDashboardCustomers(results)
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func aggregateByCustomerName(ctx context.Context, coll *mongo.Collection, match bson.M, sumField string) map[string]float64 {
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$customer_name",
			"total": bson.M{"$sum": "$" + sumField},
		}}},
	}
	cur, err := coll.Aggregate(ctx, pipe)
	if err != nil {
		return nil
	}
	defer cur.Close(ctx)
	out := map[string]float64{}
	for cur.Next(ctx) {
		var r struct {
			Name  string  `bson:"_id"`
			Total float64 `bson:"total"`
		}
		if cur.Decode(&r) == nil {
			out[r.Name] += r.Total
		}
	}
	return out
}

func sortDashboardCustomers(s []DashboardCustomerSummary) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].TotalAmount > s[j-1].TotalAmount; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ─── Outstanding receivables ──────────────────────────────────────────────────

func GetDashboardOutstanding(storeID primitive.ObjectID, limit int) ([]DashboardCustomerSummary, error) {
	if limit <= 0 {
		limit = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cur, err := db.GetDB("store_"+storeID.Hex()).Collection("customer").
		Find(ctx,
			bson.M{"credit_balance": bson.M{"$gt": 0}, "deleted": bson.M{"$ne": true}},
			options.Find().
				SetProjection(bson.M{"name": 1, "credit_balance": 1}).
				SetSort(bson.D{{Key: "credit_balance", Value: -1}}).
				SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []DashboardCustomerSummary
	for cur.Next(ctx) {
		var c struct {
			Name          string  `bson:"name"`
			CreditBalance float64 `bson:"credit_balance"`
		}
		if cur.Decode(&c) == nil {
			out = append(out, DashboardCustomerSummary{CustomerName: c.Name, Outstanding: c.CreditBalance})
		}
	}
	return out, nil
}

// ─── Categories ───────────────────────────────────────────────────────────────

// GetDashboardCategories aggregates product-store sales and profit by category.
func GetDashboardCategories(storeID primitive.ObjectID) ([]DashboardCategorySummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	storeIDStr := storeID.Hex()

	pipe := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"deleted": bson.M{"$ne": true},
			"product_stores." + storeIDStr + ".sales": bson.M{"$gt": 0},
		}}},
		{{Key: "$unwind", Value: "$category_name"}},
		{{Key: "$group", Value: bson.M{
			"_id":    "$category_name",
			"sales":  bson.M{"$sum": "$product_stores." + storeIDStr + ".sales"},
			"profit": bson.M{"$sum": "$product_stores." + storeIDStr + ".sales_profit"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "sales", Value: -1}}}},
	}
	cur, err := db.GetDB("store_"+storeIDStr).Collection("product").Aggregate(ctx, pipe)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []DashboardCategorySummary
	for cur.Next(ctx) {
		var r struct {
			Cat    string  `bson:"_id"`
			Sales  float64 `bson:"sales"`
			Profit float64 `bson:"profit"`
		}
		if cur.Decode(&r) == nil && r.Cat != "" {
			out = append(out, DashboardCategorySummary{CategoryName: r.Cat, Sales: r.Sales, Profit: r.Profit})
		}
	}
	return out, nil
}

// ─── Vendors ──────────────────────────────────────────────────────────────────

func GetDashboardVendors(storeID primitive.ObjectID, fromDateStr, toDateStr string, tzOffset float64) ([]DashboardVendorSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	match := bson.M{}
	if df := dateRangeFilter(fromDateStr, toDateStr, tzOffset); df != nil {
		for k, v := range df {
			match[k] = v
		}
	}
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$vendor_name",
			"total": bson.M{"$sum": "$net_total"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "total", Value: -1}}}},
	}
	cur, err := db.GetDB("store_"+storeID.Hex()).Collection("purchase").Aggregate(ctx, pipe)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []DashboardVendorSummary
	for cur.Next(ctx) {
		var r struct {
			Name  string  `bson:"_id"`
			Total float64 `bson:"total"`
		}
		if cur.Decode(&r) == nil && r.Name != "" && r.Total > 0 {
			out = append(out, DashboardVendorSummary{VendorName: r.Name, PurchaseAmount: r.Total})
		}
	}
	return out, nil
}

// ─── Accounts ─────────────────────────────────────────────────────────────────

func GetDashboardAccounts(storeID primitive.ObjectID) ([]DashboardAccountSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pipe := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"deleted": bson.M{"$ne": true}}}},
		{{Key: "$group", Value: bson.M{
			"_id":     "$type",
			"balance": bson.M{"$sum": bson.M{"$abs": "$balance"}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "balance", Value: -1}}}},
	}
	cur, err := db.GetDB("store_"+storeID.Hex()).Collection("account").Aggregate(ctx, pipe)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []DashboardAccountSummary
	for cur.Next(ctx) {
		var r struct {
			Type    string  `bson:"_id"`
			Balance float64 `bson:"balance"`
		}
		if cur.Decode(&r) == nil && r.Balance > 0 {
			out = append(out, DashboardAccountSummary{AccountType: r.Type, Balance: r.Balance})
		}
	}
	return out, nil
}

// ─── Stock Health ─────────────────────────────────────────────────────────────

func GetDashboardStock(storeID primitive.ObjectID) (DashboardStockSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	storeIDStr := storeID.Hex()

	cur, err := db.GetDB("store_"+storeIDStr).Collection("product").
		Find(ctx, bson.M{"deleted": bson.M{"$ne": true}, "is_service": bson.M{"$ne": true}},
			options.Find().SetProjection(bson.M{"product_stores." + storeIDStr + ".stock": 1}))
	if err != nil {
		return DashboardStockSummary{}, err
	}
	defer cur.Close(ctx)

	var out DashboardStockSummary
	for cur.Next(ctx) {
		var p struct {
			ProductStores map[string]struct {
				Stock float64 `bson:"stock"`
			} `bson:"product_stores"`
		}
		if cur.Decode(&p) != nil {
			continue
		}
		s := p.ProductStores[storeIDStr].Stock
		out.Total++
		switch {
		case s <= 0:
			out.OutOfStock++
		case s < 5:
			out.LowStock++
		default:
			out.HealthyStock++
		}
	}
	return out, nil
}

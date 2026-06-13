package models

import (
	"context"
	"fmt"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DashboardMonthly holds precomputed aggregates for one store for one calendar month
// (in the store's local timezone). Stored in the per-store collection "dashboard_monthly".
// MonthStr format: "2025-01" (YYYY-MM).
type DashboardMonthly struct {
	ID       primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	StoreID  primitive.ObjectID `bson:"store_id"      json:"store_id"`
	MonthStr string             `bson:"month_str"     json:"month_str"` // "2025-01"

	// Sales
	SalesAmount float64 `bson:"sales_amount" json:"sales_amount"`
	SalesCount  int64   `bson:"sales_count"  json:"sales_count"`

	// Sales order payment-status breakdown (net_total bucketed by status)
	PaidAmount    float64 `bson:"paid_amount"    json:"paid_amount"`
	UnpaidAmount  float64 `bson:"unpaid_amount"  json:"unpaid_amount"`
	PartialAmount float64 `bson:"partial_amount" json:"partial_amount"`

	// Sales Returns
	SalesReturnAmount float64 `bson:"sales_return_amount" json:"sales_return_amount"`
	SalesReturnCount  int64   `bson:"sales_return_count"  json:"sales_return_count"`

	// Sales payments by method
	PaymentCash       float64 `bson:"payment_cash"             json:"payment_cash"`
	PaymentDebitCard  float64 `bson:"payment_debit_card"       json:"payment_debit_card"`
	PaymentBankCard   float64 `bson:"payment_bank_card"        json:"payment_bank_card"`
	PaymentCreditCard float64 `bson:"payment_credit_card"      json:"payment_credit_card"`
	PaymentBankXfer   float64 `bson:"payment_bank_transfer"    json:"payment_bank_transfer"`
	PaymentBankCheque float64 `bson:"payment_bank_cheque"      json:"payment_bank_cheque"`
	PaymentCustAcct   float64 `bson:"payment_customer_account" json:"payment_customer_account"`

	// Quotation-invoice totals
	QtnInvoiceAmount       float64 `bson:"qtn_invoice_amount"        json:"qtn_invoice_amount"`
	QtnInvoiceReturnAmount float64 `bson:"qtn_invoice_return_amount" json:"qtn_invoice_return_amount"`

	// Quotation-invoice embedded payments by method
	QtnPaymentCash       float64 `bson:"qtn_payment_cash"             json:"qtn_payment_cash"`
	QtnPaymentDebitCard  float64 `bson:"qtn_payment_debit_card"       json:"qtn_payment_debit_card"`
	QtnPaymentBankCard   float64 `bson:"qtn_payment_bank_card"        json:"qtn_payment_bank_card"`
	QtnPaymentCreditCard float64 `bson:"qtn_payment_credit_card"      json:"qtn_payment_credit_card"`
	QtnPaymentBankXfer   float64 `bson:"qtn_payment_bank_transfer"    json:"qtn_payment_bank_transfer"`
	QtnPaymentBankCheque float64 `bson:"qtn_payment_bank_cheque"      json:"qtn_payment_bank_cheque"`
	QtnPaymentCustAcct   float64 `bson:"qtn_payment_customer_account" json:"qtn_payment_customer_account"`

	// Purchases
	PurchaseAmount       float64 `bson:"purchase_amount"        json:"purchase_amount"`
	PurchaseReturnAmount float64 `bson:"purchase_return_amount" json:"purchase_return_amount"`

	// Accounted purchases (enable_on_accounts = true)
	AcctPurchaseAmount       float64 `bson:"accounted_purchase_amount"        json:"accounted_purchase_amount"`
	AcctPurchaseReturnAmount float64 `bson:"accounted_purchase_return_amount" json:"accounted_purchase_return_amount"`

	// Cash discounts (for P&L expense adjustment)
	SalesCashDiscount                  float64 `bson:"sales_cash_discount"                    json:"sales_cash_discount"`
	SalesReturnCashDiscount            float64 `bson:"sales_return_cash_discount"             json:"sales_return_cash_discount"`
	PurchaseCashDiscount               float64 `bson:"purchase_cash_discount"                 json:"purchase_cash_discount"`
	PurchaseReturnCashDiscount         float64 `bson:"purchase_return_cash_discount"          json:"purchase_return_cash_discount"`
	AcctPurchaseCashDiscount           float64 `bson:"accounted_purchase_cash_discount"       json:"accounted_purchase_cash_discount"`
	AcctPurchaseReturnCashDiscount     float64 `bson:"accounted_purchase_return_cash_discount" json:"accounted_purchase_return_cash_discount"`
	QtnSalesCashDiscount               float64 `bson:"qtn_sales_cash_discount"                json:"qtn_sales_cash_discount"`
	QtnSalesReturnCashDiscount         float64 `bson:"qtn_sales_return_cash_discount"         json:"qtn_sales_return_cash_discount"`

	// Expenses
	ExpenseAmount float64 `bson:"expense_amount" json:"expense_amount"`
	ExpenseCount  int64   `bson:"expense_count"  json:"expense_count"`

	// Customer-deposit purchase-fund (used when disable_purchases_on_accounts = true)
	DepositPurchaseFund float64 `bson:"deposit_purchase_fund" json:"deposit_purchase_fund"`

	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

// ─── Compute & upsert ────────────────────────────────────────────────────────

// ComputeAndUpsertDashboardMonthly aggregates all data for one (store, local month) pair
// and upserts the result into the "dashboard_monthly" collection.
// monthStr: "2025-01", tzOffset: from CountryTimezoneOffset (e.g. -3 for UTC+3).
func ComputeAndUpsertDashboardMonthly(storeID primitive.ObjectID, monthStr string, tzOffset float64) error {
	t, err := time.Parse("2006-01", monthStr)
	if err != nil {
		return fmt.Errorf("invalid monthStr %q: %w", monthStr, err)
	}

	// Convert local month boundaries to UTC for MongoDB date filters.
	// tzOffset is negative for UTC+ zones (e.g. -3 for UTC+3 / Saudi Arabia).
	dur := time.Duration(float64(time.Hour) * tzOffset)
	startUTC := t.Add(dur)
	// First day of next month
	nextMonth := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	endUTC := nextMonth.Add(dur)

	// Include store_id to match the stats-page query exactly.
	dateFilter := bson.M{
		"store_id": storeID,
		"date":     bson.M{"$gte": startUTC, "$lt": endUTC},
	}
	dateFilterWithDelete := dmMerge(dateFilter, bson.M{"deleted": bson.M{"$ne": true}})

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	sdb := db.GetDB("store_" + storeID.Hex())
	d := DashboardMonthly{StoreID: storeID, MonthStr: monthStr, UpdatedAt: time.Now()}

	// Orders
	d.SalesAmount, d.SalesCount = dmSumCount(ctx, sdb.Collection("order"), dateFilterWithDelete, "net_total")
	d.PaidAmount, _ = dmSum(ctx, sdb.Collection("order"), dmMerge(dateFilterWithDelete, bson.M{"payment_status": "paid"}), "net_total")
	d.UnpaidAmount, _ = dmSum(ctx, sdb.Collection("order"), dmMerge(dateFilterWithDelete, bson.M{"payment_status": "not_paid"}), "net_total")
	d.PartialAmount, _ = dmSum(ctx, sdb.Collection("order"), dmMerge(dateFilterWithDelete, bson.M{"payment_status": "paid_partially"}), "net_total")

	// Sales returns
	d.SalesReturnAmount, d.SalesReturnCount = dmSumCount(ctx, sdb.Collection("salesreturn"), dateFilterWithDelete, "net_total")

	// Sales payments by method
	payMap := map[string]*float64{
		"cash": &d.PaymentCash, "debit_card": &d.PaymentDebitCard,
		"bank_card": &d.PaymentBankCard, "credit_card": &d.PaymentCreditCard,
		"bank_transfer": &d.PaymentBankXfer, "bank_cheque": &d.PaymentBankCheque,
		"customer_account": &d.PaymentCustAcct,
	}
	for method, ptr := range payMap {
		*ptr, _ = dmSum(ctx, sdb.Collection("sales_payment"), dmMerge(dateFilterWithDelete, bson.M{"method": method}), "amount")
	}

	// Quotation invoices
	qtnBase := dmMerge(dateFilterWithDelete, bson.M{"type": "invoice"})
	d.QtnInvoiceAmount, _ = dmSum(ctx, sdb.Collection("quotation"), qtnBase, "net_total")
	qtnPayMap := map[string]*float64{
		"cash": &d.QtnPaymentCash, "debit_card": &d.QtnPaymentDebitCard,
		"bank_card": &d.QtnPaymentBankCard, "credit_card": &d.QtnPaymentCreditCard,
		"bank_transfer": &d.QtnPaymentBankXfer, "bank_cheque": &d.QtnPaymentBankCheque,
		"customer_account": &d.QtnPaymentCustAcct,
	}
	for method, ptr := range qtnPayMap {
		*ptr = dmSumEmbeddedPayments(ctx, sdb.Collection("quotation"), dmMerge(dateFilterWithDelete, bson.M{"type": "invoice"}), method)
	}

	// Quotation sales returns
	d.QtnInvoiceReturnAmount, _ = dmSum(ctx, sdb.Collection("quotation_sales_return"), dateFilterWithDelete, "net_total")

	// Purchases
	d.PurchaseAmount, _ = dmSum(ctx, sdb.Collection("purchase"), dateFilterWithDelete, "net_total")
	d.PurchaseReturnAmount, _ = dmSum(ctx, sdb.Collection("purchasereturn"), dateFilterWithDelete, "net_total")

	// Accounted purchases
	acctF := dmMerge(dateFilterWithDelete, bson.M{"enable_on_accounts": true})
	d.AcctPurchaseAmount, _ = dmSum(ctx, sdb.Collection("purchase"), acctF, "net_total")
	d.AcctPurchaseReturnAmount, _ = dmSum(ctx, sdb.Collection("purchasereturn"), acctF, "net_total")

	// Cash discounts
	d.SalesCashDiscount, _ = dmSum(ctx, sdb.Collection("order"), dateFilterWithDelete, "cash_discount")
	d.SalesReturnCashDiscount, _ = dmSum(ctx, sdb.Collection("salesreturn"), dateFilterWithDelete, "cash_discount")
	d.PurchaseCashDiscount, _ = dmSum(ctx, sdb.Collection("purchase"), dateFilterWithDelete, "cash_discount")
	d.PurchaseReturnCashDiscount, _ = dmSum(ctx, sdb.Collection("purchasereturn"), dateFilterWithDelete, "cash_discount")
	d.AcctPurchaseCashDiscount, _ = dmSum(ctx, sdb.Collection("purchase"), acctF, "cash_discount")
	d.AcctPurchaseReturnCashDiscount, _ = dmSum(ctx, sdb.Collection("purchasereturn"), acctF, "cash_discount")
	d.QtnSalesCashDiscount, _ = dmSum(ctx, sdb.Collection("quotation"), dmMerge(dateFilterWithDelete, bson.M{"type": "invoice"}), "cash_discount")
	d.QtnSalesReturnCashDiscount, _ = dmSum(ctx, sdb.Collection("quotation_sales_return"), dateFilterWithDelete, "cash_discount")

	// Expenses
	d.ExpenseAmount, d.ExpenseCount = dmSumCount(ctx, sdb.Collection("expense"), dateFilterWithDelete, "amount")

	// Customer-deposit purchase fund
	d.DepositPurchaseFund = dmSumEmbeddedPayments(ctx, sdb.Collection("customerdeposit"), dateFilterWithDelete, "purchase_fund")

	// Skip months with no activity — nothing to store, nothing to show.
	if d.SalesAmount+d.SalesReturnAmount+d.PurchaseAmount+d.ExpenseAmount+d.QtnInvoiceAmount == 0 {
		// Remove any stale record for this month (e.g. left over from bad data that was corrected).
		_, _ = sdb.Collection("dashboard_monthly").DeleteOne(
			ctx,
			bson.M{"store_id": storeID, "month_str": monthStr},
		)
		return nil
	}

	// Upsert
	_, err = sdb.Collection("dashboard_monthly").UpdateOne(
		ctx,
		bson.M{"store_id": storeID, "month_str": monthStr},
		bson.M{"$set": d},
		options.Update().SetUpsert(true),
	)
	return err
}

// ─── Query ───────────────────────────────────────────────────────────────────

// GetDashboardMonthly returns precomputed monthly records for a store in ascending order.
// fromMonthStr / toMonthStr may be empty (no filter on that side). Format: "YYYY-MM".
func GetDashboardMonthly(storeID primitive.ObjectID, fromMonthStr, toMonthStr string) ([]DashboardMonthly, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	filter := bson.M{"store_id": storeID}
	if fromMonthStr != "" && toMonthStr != "" {
		filter["month_str"] = bson.M{"$gte": fromMonthStr, "$lte": toMonthStr}
	} else if fromMonthStr != "" {
		filter["month_str"] = bson.M{"$gte": fromMonthStr}
	} else if toMonthStr != "" {
		filter["month_str"] = bson.M{"$lte": toMonthStr}
	}

	cur, err := db.GetDB("store_"+storeID.Hex()).Collection("dashboard_monthly").
		Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "month_str", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []DashboardMonthly
	return out, cur.All(ctx, &out)
}

// EnsureDashboardMonthlyIndexes creates the unique compound index on (store_id, month_str).
func EnsureDashboardMonthlyIndexes(storeID primitive.ObjectID) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	coll := db.GetDB("store_" + storeID.Hex()).Collection("dashboard_monthly")
	_, _ = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "store_id", Value: 1}, {Key: "month_str", Value: 1}},
		Options: options.Index().SetUnique(true).SetBackground(true),
	})
}

// ClearDashboardMonthlyForStore deletes all dashboard_monthly records for a single store.
func ClearDashboardMonthlyForStore(storeID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	coll := db.GetDB("store_" + storeID.Hex()).Collection("dashboard_monthly")
	_, err := coll.DeleteMany(ctx, bson.M{})
	return err
}

// ClearDashboardMonthlyForAllStores deletes all dashboard_monthly records for every active store.
func ClearDashboardMonthlyForAllStores() {
	stores, err := GetAllStores()
	if err != nil {
		return
	}
	for _, s := range stores {
		if !s.Deleted {
			_ = ClearDashboardMonthlyForStore(s.ID)
		}
	}
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func dmMerge(base, extra bson.M) bson.M {
	out := bson.M{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func dmSumCount(ctx context.Context, coll *mongo.Collection, filter bson.M, field string) (float64, int64) {
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$group", Value: bson.M{"_id": nil,
			"total": bson.M{"$sum": "$" + field},
			"cnt":   bson.M{"$sum": 1},
		}}},
	}
	cur, err := coll.Aggregate(ctx, pipe)
	if err != nil {
		return 0, 0
	}
	defer cur.Close(ctx)
	var r struct {
		Total float64 `bson:"total"`
		Cnt   int64   `bson:"cnt"`
	}
	if cur.Next(ctx) {
		_ = cur.Decode(&r)
	}
	return r.Total, r.Cnt
}

func dmSum(ctx context.Context, coll *mongo.Collection, filter bson.M, field string) (float64, error) {
	v, _ := dmSumCount(ctx, coll, filter, field)
	return v, nil
}

func dmSumEmbeddedPayments(ctx context.Context, coll *mongo.Collection, baseFilter bson.M, method string) float64 {
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: baseFilter}},
		{{Key: "$unwind", Value: "$payments"}},
		{{Key: "$match", Value: bson.M{"payments.method": method}}},
		{{Key: "$group", Value: bson.M{"_id": nil, "total": bson.M{"$sum": "$payments.amount"}}}},
	}
	cur, err := coll.Aggregate(ctx, pipe)
	if err != nil {
		return 0
	}
	defer cur.Close(ctx)
	var r struct {
		Total float64 `bson:"total"`
	}
	if cur.Next(ctx) {
		_ = cur.Decode(&r)
	}
	return r.Total
}

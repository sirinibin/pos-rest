package models

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ── ABC-XYZ ───────────────────────────────────────────────────────────────────

type AbcXyzScore struct {
	ProductID        string  `json:"product_id"`
	Class            string  `json:"class"`
	AbcTier          string  `json:"abc_tier"`
	XyzTier          string  `json:"xyz_tier"`
	ClassReason      string  `json:"class_reason"`
	CV               float64 `json:"cv"`
	CVValid          bool    `json:"cv_valid"`
	ActiveMonths     int     `json:"active_months"`
	StockingStrategy string  `json:"stocking_strategy"`
	ProductName      string  `json:"product_name"`
	ProductNameArabic string `json:"product_name_arabic"`
	TotalRevenue     float64 `json:"total_revenue"`
	AvgMonthlyQty    float64 `json:"avg_monthly_qty"`
}

type AbcXyzScoreRequest struct {
	StoreID string        `json:"store_id"`
	Date    string        `json:"date"` // "YYYY-MM"
	Scores  []AbcXyzScore `json:"scores"`
}

func (store *Store) BulkUpsertAbcXyzScores(req AbcXyzScoreRequest) error {
	storeDB := db.GetDB("store_" + store.ID.Hex())
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	dateMonth := parseDateMonth(req.Date)
	now := time.Now().UTC()

	var prodOps, histOps []mongo.WriteModel
	for _, s := range req.Scores {
		pid, err := primitive.ObjectIDFromHex(s.ProductID)
		if err != nil {
			continue
		}
		setDoc := bson.M{
			"class":             s.Class,
			"class_reason":      s.ClassReason,
			"abc_tier":          s.AbcTier,
			"xyz_tier":          s.XyzTier,
			"stocking_strategy": s.StockingStrategy,
			"active_months":     s.ActiveMonths,
		}
		if s.CVValid {
			setDoc["cv"] = s.CV
		}
		prodOps = append(prodOps, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": pid}).
			SetUpdate(bson.M{"$set": setDoc}))

		histOps = append(histOps, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"date": dateMonth, "store_id": store.ID, "product_id": pid}).
			SetUpdate(bson.M{
				"$set": bson.M{
					"date": dateMonth, "store_id": store.ID, "product_id": pid,
					"product_name": s.ProductName, "product_name_in_arabic": s.ProductNameArabic,
					"class": s.Class, "class_reason": s.ClassReason,
					"abc_tier": s.AbcTier, "xyz_tier": s.XyzTier,
					"stocking_strategy": s.StockingStrategy, "active_months": s.ActiveMonths,
					"updated_at": now,
				},
				"$setOnInsert": bson.M{"created_at": now},
			}).SetUpsert(true))
	}
	return bulkWritePair(ctx, storeDB, "product", prodOps, "product_abc_xyz_classification_history", histOps)
}

// ── Sales Velocity ────────────────────────────────────────────────────────────

type VelocityScore struct {
	ProductID         string  `json:"product_id"`
	Trend             string  `json:"trend"`
	Reason            string  `json:"reason"`
	SlopePctPM        float64 `json:"slope_pct_pm"`
	Momentum3M        float64 `json:"momentum_3m"`
	AvgMonthlyQty     float64 `json:"avg_monthly_qty"`
	Recent3MQty       float64 `json:"recent_3m_qty"`
	TotalRevenue      float64 `json:"total_revenue"`
	TotalProfit       float64 `json:"total_profit"`
	ProductName       string  `json:"product_name"`
	ProductNameArabic string  `json:"product_name_arabic"`
}

type VelocityScoreRequest struct {
	StoreID string          `json:"store_id"`
	Date    string          `json:"date"`
	Scores  []VelocityScore `json:"scores"`
}

func (store *Store) BulkUpsertVelocityScores(req VelocityScoreRequest) error {
	storeDB := db.GetDB("store_" + store.ID.Hex())
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	dateMonth := parseDateMonth(req.Date)
	now := time.Now().UTC()

	var prodOps, histOps []mongo.WriteModel
	for _, s := range req.Scores {
		pid, err := primitive.ObjectIDFromHex(s.ProductID)
		if err != nil {
			continue
		}
		prodOps = append(prodOps, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": pid}).
			SetUpdate(bson.M{"$set": bson.M{
				"sales_velocity_trend":        s.Trend,
				"sales_velocity_trend_reason": s.Reason,
				"slop_percent_per_month":       s.SlopePctPM,
				"momentum_percent_per_3month":  s.Momentum3M,
				"avg_monthly_qty":              s.AvgMonthlyQty,
				"recent_3month_qty":            s.Recent3MQty,
				"revenue":                      s.TotalRevenue,
			}}))

		histOps = append(histOps, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"date": dateMonth, "store_id": store.ID, "product_id": pid}).
			SetUpdate(bson.M{
				"$set": bson.M{
					"date": dateMonth, "store_id": store.ID, "product_id": pid,
					"product_name": s.ProductName, "product_name_in_arabic": s.ProductNameArabic,
					"sales_velocity_trend": s.Trend, "sales_velocity_trend_reason": s.Reason,
					"slop_percent_per_month": s.SlopePctPM, "momentum_percent_per_3month": s.Momentum3M,
					"avg_monthly_qty": s.AvgMonthlyQty, "recent_3month_qty": s.Recent3MQty,
					"revenue": s.TotalRevenue, "profit": s.TotalProfit,
					"updated_at": now,
				},
				"$setOnInsert": bson.M{"created_at": now},
			}).SetUpsert(true))
	}
	return bulkWritePair(ctx, storeDB, "product", prodOps, "product_sales_trend_history", histOps)
}

// ── Customer Lifetime Value ───────────────────────────────────────────────────

type CLVScore struct {
	CustomerID           string  `json:"customer_id"`
	CLVSegment           string  `json:"clv_segment"`
	PredictedCLV12M      float64 `json:"predicted_clv_12m"`
	PredictedPurchases12M float64 `json:"predicted_purchases_12m"`
	PredictedAvgOrder    float64 `json:"predicted_avg_order_value"`
	Frequency            float64 `json:"frequency"`
	Recency              float64 `json:"recency"`
	T                    float64 `json:"T"`
	Model                string  `json:"model"`
	Reason               string  `json:"reason"`
	CustomerName         string  `json:"customer_name"`
	CustomerNameArabic   string  `json:"customer_name_arabic"`
}

type CLVScoreRequest struct {
	StoreID string     `json:"store_id"`
	Date    string     `json:"date"`
	Scores  []CLVScore `json:"scores"`
}

func (store *Store) BulkUpsertCLVScores(req CLVScoreRequest) error {
	storeDB := db.GetDB("store_" + store.ID.Hex())
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	dateMonth := parseDateMonth(req.Date)
	now := time.Now().UTC()

	var custOps, histOps []mongo.WriteModel
	for _, s := range req.Scores {
		cid, err := primitive.ObjectIDFromHex(s.CustomerID)
		if err != nil {
			continue
		}
		custOps = append(custOps, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": cid}).
			SetUpdate(bson.M{"$set": bson.M{
				"predicted_clv_amount_12months":      s.PredictedCLV12M,
				"predicted_purchases_count_forecast": s.PredictedPurchases12M,
				"predicted_avg_order_amount":         s.PredictedAvgOrder,
				"lifetime_value_segment_for_12months": s.CLVSegment,
				"lifetime_value_model":               s.Model,
				"lifetime_value_reason":              s.Reason,
			}}))

		histOps = append(histOps, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"date": dateMonth, "store_id": store.ID, "customer_id": cid}).
			SetUpdate(bson.M{
				"$set": bson.M{
					"date": dateMonth, "store_id": store.ID, "customer_id": cid,
					"customer_name": s.CustomerName, "customer_name_in_arabic": s.CustomerNameArabic,
					"predicted_clv_amount_12months":       s.PredictedCLV12M,
					"predicted_purchases_count_forecast":  s.PredictedPurchases12M,
					"predicted_avg_order_amount":          s.PredictedAvgOrder,
					"lifetime_value_segment_for_12months": s.CLVSegment,
					"lifetime_value_model":                s.Model,
					"lifetime_value_reason":               s.Reason,
					"updated_at":                          now,
				},
				"$setOnInsert": bson.M{"created_at": now},
			}).SetUpsert(true))
	}
	return bulkWritePair(ctx, storeDB, "customer", custOps, "customer_predicted_12month_clv_history", histOps)
}

// ── Cohort Retention ──────────────────────────────────────────────────────────

type CohortCustomerScore struct {
	CustomerID        string `json:"customer_id"`
	FirstPurchaseAt   string `json:"first_purchase_at"` // RFC3339
	LastPurchaseAt    string `json:"last_purchase_at"`
	TenureDays        int    `json:"tenure_days"`
	Retention1Month   string `json:"retention_1month"`
	Retention3Month   string `json:"retention_3month"`
	Retention6Month   string `json:"retention_6month"`
	Retention12Month  string `json:"retention_12month"`
	Retention24Month  string `json:"retention_24month"`
}

type CohortReportRow struct {
	CohortMonth         string  `json:"cohort_month"`
	TotalCustomers      int     `json:"total_customers"`
	TotalSpend          float64 `json:"total_spend"`
	AvgSpendPerCustomer float64 `json:"avg_spend_per_customer"`
	ActiveAt1M          int     `json:"active_at_1m"`
	ActiveAt3M          int     `json:"active_at_3m"`
	ActiveAt6M          int     `json:"active_at_6m"`
	ActiveAt12M         int     `json:"active_at_12m"`
	ActiveAt24M         int     `json:"active_at_24m"`
	Retention1M         float64 `json:"retention_1m"`
	Retention3M         float64 `json:"retention_3m"`
	Retention6M         float64 `json:"retention_6m"`
	Retention12M        float64 `json:"retention_12m"`
	Retention24M        float64 `json:"retention_24m"`
	RetentionTrend      string  `json:"retention_trend"`
}

type CohortScoreRequest struct {
	StoreID        string                `json:"store_id"`
	Date           string                `json:"date"`
	CustomerScores []CohortCustomerScore `json:"customer_scores"`
	CohortReports  []CohortReportRow     `json:"cohort_reports"`
}

func (store *Store) BulkUpsertCohortScores(req CohortScoreRequest) error {
	storeDB := db.GetDB("store_" + store.ID.Hex())
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	now := time.Now().UTC()

	var custOps []mongo.WriteModel
	for _, s := range req.CustomerScores {
		cid, err := primitive.ObjectIDFromHex(s.CustomerID)
		if err != nil {
			continue
		}
		setDoc := bson.M{
			"retention_1month":  s.Retention1Month,
			"retention_3month":  s.Retention3Month,
			"retention_6month":  s.Retention6Month,
			"retention_12month": s.Retention12Month,
			"retention_24month": s.Retention24Month,
		}
		if t, err2 := time.Parse(time.RFC3339, s.FirstPurchaseAt); err2 == nil {
			setDoc["first_purchase_at"] = t
		}
		if t, err2 := time.Parse(time.RFC3339, s.LastPurchaseAt); err2 == nil {
			setDoc["last_purchase_at"] = t
		}
		custOps = append(custOps, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": cid}).
			SetUpdate(bson.M{"$set": setDoc}))
	}
	if len(custOps) > 0 {
		if _, err := storeDB.Collection("customer").BulkWrite(ctx, custOps); err != nil {
			return fmt.Errorf("cohort customer bulk write: %w", err)
		}
	}

	var cohortOps []mongo.WriteModel
	for _, r := range req.CohortReports {
		doc := bson.M{
			"store_id": store.ID, "cohort_first_buy_month": r.CohortMonth,
			"total_customers": r.TotalCustomers,
			"total_spend": r.TotalSpend, "avg_spend_per_customer": r.AvgSpendPerCustomer,
			"active_customers_at_1month": r.ActiveAt1M, "active_customers_at_3month": r.ActiveAt3M,
			"active_customers_at_6month": r.ActiveAt6M, "active_customers_at_12month": r.ActiveAt12M,
			"active_customers_at_24month": r.ActiveAt24M,
			"retention_1month": r.Retention1M, "retention_3month": r.Retention3M,
			"retention_6month": r.Retention6M, "retention_12month": r.Retention12M,
			"retention_24month": r.Retention24M, "retention_trend": r.RetentionTrend,
			"updated_at": now,
		}
		cohortOps = append(cohortOps, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"store_id": store.ID, "cohort_first_buy_month": r.CohortMonth}).
			SetUpdate(bson.M{"$set": doc, "$setOnInsert": bson.M{"created_at": now}}).
			SetUpsert(true))
	}
	if len(cohortOps) > 0 {
		if _, err := storeDB.Collection("customer_cohort_retention_report").BulkWrite(ctx, cohortOps); err != nil {
			return fmt.Errorf("cohort report bulk write: %w", err)
		}
	}
	return nil
}

// ── Customer Churn ────────────────────────────────────────────────────────────

type ChurnScore struct {
	CustomerID         string  `json:"customer_id"`
	ChurnProb          float64 `json:"churn_prob"`
	RiskTier           string  `json:"risk_tier"`
	Reason             string  `json:"reason"`
	TotalSpend         float64 `json:"total_spend"`
	TotalOrders        int     `json:"total_orders"`
	RecencyDays        int     `json:"recency_days"`
	CustomerName       string  `json:"customer_name"`
	CustomerNameArabic string  `json:"customer_name_arabic"`
}

type ChurnScoreRequest struct {
	StoreID string       `json:"store_id"`
	Date    string       `json:"date"`
	Scores  []ChurnScore `json:"scores"`
}

func (store *Store) BulkUpsertChurnScores(req ChurnScoreRequest) error {
	storeDB := db.GetDB("store_" + store.ID.Hex())
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	dateMonth := parseDateMonth(req.Date)
	now := time.Now().UTC()

	var custOps, histOps []mongo.WriteModel
	for _, s := range req.Scores {
		cid, err := primitive.ObjectIDFromHex(s.CustomerID)
		if err != nil {
			continue
		}
		custOps = append(custOps, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": cid}).
			SetUpdate(bson.M{"$set": bson.M{
				"churn_risk_tier":        s.RiskTier,
				"churn_risk_tier_reason": s.Reason,
				"churn_percent":          s.ChurnProb * 100,
				"total_spend":            s.TotalSpend,
				"days_since_last_buy":    s.RecencyDays,
			}}))

		histOps = append(histOps, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"date": dateMonth, "store_id": store.ID, "customer_id": cid}).
			SetUpdate(bson.M{
				"$set": bson.M{
					"date": dateMonth, "store_id": store.ID, "customer_id": cid,
					"customer_name": s.CustomerName, "customer_name_arabic": s.CustomerNameArabic,
					"risk_tier": s.RiskTier, "risk_tier_reason": s.Reason,
					"churn_percent": s.ChurnProb * 100, "total_spend": s.TotalSpend,
					"days_since_last_buy": s.RecencyDays, "updated_at": now,
				},
				"$setOnInsert": bson.M{"created_at": now},
			}).SetUpsert(true))
	}
	return bulkWritePair(ctx, storeDB, "customer", custOps, "customer_churn_risk_tier_history", histOps)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func bulkWritePair(ctx context.Context, storeDB *mongo.Database,
	col1 string, ops1 []mongo.WriteModel,
	col2 string, ops2 []mongo.WriteModel,
) error {
	if len(ops1) > 0 {
		if _, err := storeDB.Collection(col1).BulkWrite(ctx, ops1); err != nil {
			return fmt.Errorf("%s bulk write: %w", col1, err)
		}
	}
	if len(ops2) > 0 {
		if _, err := storeDB.Collection(col2).BulkWrite(ctx, ops2); err != nil {
			return fmt.Errorf("%s bulk write: %w", col2, err)
		}
	}
	return nil
}

func parseDateMonth(s string) time.Time {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) < 2 {
		return time.Now().UTC()
	}
	y, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	if y == 0 || m == 0 {
		return time.Now().UTC()
	}
	return time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
}

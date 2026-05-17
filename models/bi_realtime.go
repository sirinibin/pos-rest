package models

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UpdateCustomerBIOnOrderChange recomputes rule-based BI fields (churn tier,
// CLV segment) for a customer after an order is created or updated.
// It inserts history records into the BI history collections only when the
// tier or segment changes.  Intended to be called in a goroutine.
func UpdateCustomerBIOnOrderChange(storeID *primitive.ObjectID, customerID *primitive.ObjectID) {
	if storeID == nil || customerID == nil || customerID.IsZero() {
		return
	}

	storeDBName := "store_" + storeID.Hex()
	now := time.Now()

	// 1. Load all orders for this customer in this store
	type orderRow struct {
		Date     time.Time
		NetTotal float64
	}

	orderCol := db.GetDB(storeDBName).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := orderCol.Find(ctx, bson.M{
		"customer_id": customerID,
		"deleted":     bson.M{"$ne": true},
	}, options.Find().SetProjection(bson.M{"date": 1, "net_total": 1}))
	if err != nil {
		log.Printf("bi_realtime UpdateCustomerBI: %v", err)
		return
	}
	defer cursor.Close(ctx)

	var orders []orderRow
	for cursor.Next(ctx) {
		var row struct {
			Date     *time.Time `bson:"date"`
			NetTotal float64    `bson:"net_total"`
		}
		if err := cursor.Decode(&row); err != nil {
			continue
		}
		if row.Date != nil {
			orders = append(orders, orderRow{Date: *row.Date, NetTotal: row.NetTotal})
		}
	}

	if len(orders) == 0 {
		return
	}

	// 2. Compute metrics
	sort.Slice(orders, func(i, j int) bool { return orders[i].Date.Before(orders[j].Date) })
	firstDate := orders[0].Date
	lastDate := orders[len(orders)-1].Date
	tenureDays := int(now.Sub(firstDate).Hours() / 24)
	daysSinceLastBuy := int(now.Sub(lastDate).Hours() / 24)

	var totalSpend float64
	for _, o := range orders {
		totalSpend += o.NetTotal
	}
	orderCount := len(orders)

	// Churn tier (rule-based on recency)
	var churnTier, churnReason string
	var churnPercent float64
	switch {
	case daysSinceLastBuy <= 30:
		churnTier, churnPercent, churnReason = "Low", 5.0, "Purchased within last 30 days"
	case daysSinceLastBuy <= 60:
		churnTier, churnPercent, churnReason = "Medium", 30.0, "No purchase in 31-60 days"
	case daysSinceLastBuy <= 90:
		churnTier, churnPercent, churnReason = "High", 60.0, "No purchase in 61-90 days"
	case daysSinceLastBuy <= 180:
		churnTier, churnPercent, churnReason = "Critical", 85.0, "No purchase in 91-180 days"
	default:
		churnTier, churnPercent, churnReason = "Critical", 95.0, "No purchase in over 180 days"
	}

	// CLV: avg annual spend × 2-year horizon
	yearsActive := math.Max(float64(tenureDays)/365.0, 1.0/12.0)
	predictedCLV := (totalSpend / yearsActive) * 2.0
	avgOrderAmount := totalSpend / float64(orderCount)

	var clvSegment, clvReason string
	switch {
	case predictedCLV >= 10000:
		clvSegment = "High Value"
		clvReason = fmt.Sprintf("Predicted CLV %.0f ≥ 10000", predictedCLV)
	case predictedCLV >= 2000:
		clvSegment = "Mid Value"
		clvReason = fmt.Sprintf("Predicted CLV %.0f in 2000-9999", predictedCLV)
	default:
		clvSegment = "Low Value"
		clvReason = fmt.Sprintf("Predicted CLV %.0f < 2000", predictedCLV)
	}

	// 3. Fetch current customer BI values for change detection
	custCol := db.GetDB(storeDBName).Collection("customer")
	var existing struct {
		Name                            string `bson:"name"`
		NameInArabic                    string `bson:"name_in_arabic"`
		ChurnRiskTier                   string `bson:"churn_risk_tier"`
		LifetimeValueSegmentFor12Months string `bson:"lifetime_value_segment_for_12months"`
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	custCol.FindOne(ctx2, bson.M{"_id": customerID},
		options.FindOne().SetProjection(bson.M{
			"name": 1, "name_in_arabic": 1,
			"churn_risk_tier": 1, "lifetime_value_segment_for_12months": 1,
		})).Decode(&existing)

	dateMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// 4. Insert churn history on tier change
	if existing.ChurnRiskTier != churnTier {
		histCol := db.GetDB(storeDBName).Collection("customer_churn_risk_tier_history")
		ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel3()
		_, err := histCol.InsertOne(ctx3, &CustomerChurnRiskTierHistory{
			ID:                 primitive.NewObjectID(),
			Date:               &dateMonth,
			StoreID:            storeID,
			CustomerID:         *customerID,
			CustomerName:       existing.Name,
			CustomerNameArabic: existing.NameInArabic,
			RiskTier:           churnTier,
			ChurnPercent:       churnPercent,
			TotalSpend:         totalSpend,
			DaysSinceLastBuy:   daysSinceLastBuy,
			CreatedAt:          &now,
			UpdatedAt:          &now,
		})
		if err != nil {
			log.Printf("bi_realtime: insert churn history: %v", err)
		}
	}

	// 5. Insert CLV history on segment change
	if existing.LifetimeValueSegmentFor12Months != clvSegment {
		clvHistCol := db.GetDB(storeDBName).Collection("customer_predicted_12month_clv_history")
		ctx4, cancel4 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel4()
		_, err := clvHistCol.InsertOne(ctx4, &CustomerPredicted12MonthCLVHistory{
			ID:                                    primitive.NewObjectID(),
			Date:                                  &dateMonth,
			StoreID:                               storeID,
			CustomerID:                            *customerID,
			CustomerName:                          existing.Name,
			CustomerNameArabic:                    existing.NameInArabic,
			LifetimeValueSegmentFor12Months:       clvSegment,
			LifetimeValueSegmentReasonFor12Months: clvReason,
			PredictedCLVAmount12Months:            predictedCLV,
			PredictedAvgOrderAmount:               avgOrderAmount,
			HistoryOrdersCount:                    orderCount,
			HistorySpendAmount:                    totalSpend,
			TenureDays:                            tenureDays,
			CreatedAt:                             &now,
			UpdatedAt:                             &now,
		})
		if err != nil {
			log.Printf("bi_realtime: insert CLV history: %v", err)
		}
	}

	// 6. Update customer document (targeted $set — avoids full struct overwrite)
	ctx5, cancel5 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel5()
	_, err = custCol.UpdateOne(ctx5, bson.M{"_id": customerID}, bson.M{"$set": bson.M{
		"churn_risk_tier":                            churnTier,
		"churn_risk_tier_reason":                     churnReason,
		"churn_percent":                              churnPercent,
		"days_since_last_buy":                        daysSinceLastBuy,
		"total_spend":                                totalSpend,
		"tenure_days":                                tenureDays,
		"first_purchase_at":                          firstDate,
		"last_purchase_at":                           lastDate,
		"lifetime_value_segment_for_12months":        clvSegment,
		"lifetime_value_segment_reason_for_12months": clvReason,
		"predicted_clv_amount_12months":              predictedCLV,
		"predicted_avg_order_amount":                 avgOrderAmount,
		"history_orders_count":                       orderCount,
		"history_spend_amount":                       totalSpend,
	}})
	if err != nil {
		log.Printf("bi_realtime: update customer BI fields: %v", err)
	}
}

// UpdateProductBIOnOrderChange recomputes velocity trend and XYZ classification
// for a single product after an order is created or updated.
// ABC tier is left to the Python cron (requires cross-product revenue ranking).
// History records are inserted only when the trend or tier changes.
// Intended to be called in a goroutine.
func UpdateProductBIOnOrderChange(storeID *primitive.ObjectID, productID *primitive.ObjectID) {
	if storeID == nil || productID == nil || productID.IsZero() {
		return
	}

	storeDBName := "store_" + storeID.Hex()
	now := time.Now()
	cutoff := now.AddDate(-2, 0, 0) // 24 months of history

	// 1. Aggregate monthly sales qty + revenue from product_history
	histCol := db.GetDB(storeDBName).Collection("product_history")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pipeline := bson.A{
		bson.M{"$match": bson.M{
			"product_id":     productID,
			"reference_type": "sales",
			"date":           bson.M{"$gte": cutoff},
		}},
		bson.M{"$group": bson.M{
			"_id": bson.M{
				"year":  bson.M{"$year": "$date"},
				"month": bson.M{"$month": "$date"},
			},
			"qty":     bson.M{"$sum": "$quantity"},
			"revenue": bson.M{"$sum": "$net_price"},
		}},
		bson.M{"$sort": bson.M{"_id.year": 1, "_id.month": 1}},
	}

	cursor, err := histCol.Aggregate(ctx, pipeline)
	if err != nil {
		log.Printf("bi_realtime UpdateProductBI: %v", err)
		return
	}
	defer cursor.Close(ctx)

	type monthBucket struct {
		Year    int
		Month   int
		Qty     float64
		Revenue float64
	}
	var buckets []monthBucket
	for cursor.Next(ctx) {
		var row struct {
			ID struct {
				Year  int `bson:"year"`
				Month int `bson:"month"`
			} `bson:"_id"`
			Qty     float64 `bson:"qty"`
			Revenue float64 `bson:"revenue"`
		}
		if err := cursor.Decode(&row); err != nil {
			continue
		}
		buckets = append(buckets, monthBucket{
			Year: row.ID.Year, Month: row.ID.Month,
			Qty: row.Qty, Revenue: row.Revenue,
		})
	}

	if len(buckets) < 2 {
		return // not enough data for trend analysis
	}

	// 2. Build dense monthly array over the 24-month window
	type slot struct{ qty, revenue float64 }
	monthMap := map[[2]int]slot{}
	for _, b := range buckets {
		monthMap[[2]int{b.Year, b.Month}] = slot{b.Qty, b.Revenue}
	}

	var qtyArr []float64
	var totalRevenue float64
	activeMonths := 0
	cur := time.Date(cutoff.Year(), cutoff.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	for !cur.After(end) {
		s := monthMap[[2]int{cur.Year(), int(cur.Month())}]
		qtyArr = append(qtyArr, s.qty)
		totalRevenue += s.revenue
		if s.qty > 0 {
			activeMonths++
		}
		cur = cur.AddDate(0, 1, 0)
	}

	n := len(qtyArr)
	if n < 2 {
		return
	}

	// Recent 3 months qty
	recent3 := 0.0
	for i := n - 3; i < n; i++ {
		if i >= 0 {
			recent3 += qtyArr[i]
		}
	}

	// Average monthly qty (active months only)
	var sumQty float64
	for _, q := range qtyArr {
		sumQty += q
	}
	avgMonthly := 0.0
	if activeMonths > 0 {
		avgMonthly = sumQty / float64(activeMonths)
	}

	// Linear regression slope
	xMean := float64(n-1) / 2.0
	var yMean, ssXX, ssXY float64
	for _, q := range qtyArr {
		yMean += q
	}
	yMean /= float64(n)
	for i, q := range qtyArr {
		xi := float64(i) - xMean
		ssXX += xi * xi
		ssXY += xi * (q - yMean)
	}
	slopePct := 0.0
	if ssXX > 0 && math.Abs(yMean) > 0 {
		slope := ssXY / ssXX
		slopePct = (slope / math.Abs(yMean)) * 100.0
	}

	// Momentum: recent 3m vs prior 3m
	prior3 := 0.0
	for i := n - 6; i < n-3; i++ {
		if i >= 0 {
			prior3 += qtyArr[i]
		}
	}
	momentumPct := 0.0
	if math.Abs(prior3) > 0 {
		momentumPct = ((recent3 - prior3) / math.Abs(prior3)) * 100.0
	}

	// Velocity trend label
	var velocityTrend, velocityReason string
	switch {
	case slopePct > 10:
		velocityTrend = "Trending Up"
		velocityReason = fmt.Sprintf("Slope +%.1f%%/month", slopePct)
	case slopePct < -10:
		velocityTrend = "Trending Down"
		velocityReason = fmt.Sprintf("Slope %.1f%%/month", slopePct)
	default:
		velocityTrend = "Stable"
		velocityReason = fmt.Sprintf("Slope %.1f%%/month (stable range)", slopePct)
	}

	// CV for XYZ tier (coefficient of variation of monthly qty)
	xyzTier := ""
	cv := 0.0
	if activeMonths > 1 && avgMonthly > 0 {
		var sumSq float64
		for _, q := range qtyArr {
			diff := q - avgMonthly
			sumSq += diff * diff
		}
		cv = math.Sqrt(sumSq/float64(n)) / avgMonthly * 100.0
		switch {
		case cv <= 50:
			xyzTier = "X"
		case cv <= 100:
			xyzTier = "Y"
		default:
			xyzTier = "Z"
		}
	}

	// 3. Fetch current product BI values for change detection
	prodCol := db.GetDB(storeDBName).Collection("product")
	var existingProd struct {
		Name               string `bson:"name"`
		NameInArabic       string `bson:"name_in_arabic"`
		SalesVelocityTrend string `bson:"sales_velocity_trend"`
		XyzTier            string `bson:"xyz_tier"`
		AbcTier            string `bson:"abc_tier"`
		Class              string `bson:"class"`
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	prodCol.FindOne(ctx2, bson.M{"_id": productID},
		options.FindOne().SetProjection(bson.M{
			"name": 1, "name_in_arabic": 1,
			"sales_velocity_trend": 1, "xyz_tier": 1, "abc_tier": 1, "class": 1,
		})).Decode(&existingProd)

	dateMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// 4. Insert velocity trend history on change
	if existingProd.SalesVelocityTrend != velocityTrend {
		trendHistCol := db.GetDB(storeDBName).Collection("product_sales_trend_history")
		ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel3()
		_, err := trendHistCol.InsertOne(ctx3, &ProductSalesTrendHistory{
			ID:                       primitive.NewObjectID(),
			Date:                     &dateMonth,
			StoreID:                  storeID,
			ProductID:                *productID,
			ProductName:              existingProd.Name,
			ProductNameInArabic:      existingProd.NameInArabic,
			SalesVelocityTrend:       velocityTrend,
			SalesVelocityTrendReason: velocityReason,
			SlopPercentPerMonth:      slopePct,
			MomentumPercentPer3Month: momentumPct,
			AvgMonthlyQty:            avgMonthly,
			Recent3MonthQty:          recent3,
			Revenue:                  totalRevenue,
			CreatedAt:                &now,
			UpdatedAt:                &now,
		})
		if err != nil {
			log.Printf("bi_realtime: insert product trend history: %v", err)
		}
	}

	// 5. Insert ABC-XYZ history on xyz_tier or class change
	if xyzTier != "" {
		newClass := ""
		if existingProd.AbcTier != "" {
			newClass = existingProd.AbcTier + xyzTier
		}
		if existingProd.XyzTier != xyzTier || (newClass != "" && existingProd.Class != newClass) {
			abcHistCol := db.GetDB(storeDBName).Collection("product_abc_xyz_classification_history")
			ctx4, cancel4 := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel4()
			stocking := deriveStockingStrategy(existingProd.AbcTier, xyzTier)
			_, err := abcHistCol.InsertOne(ctx4, &ProductAbcXyzClassificationHistory{
				ID:                  primitive.NewObjectID(),
				Date:                &dateMonth,
				StoreID:             storeID,
				ProductID:           *productID,
				ProductName:         existingProd.Name,
				ProductNameInArabic: existingProd.NameInArabic,
				Class:               newClass,
				AbcTier:             existingProd.AbcTier,
				XyzTier:             xyzTier,
				CV:                  cv,
				ActiveMonths:        activeMonths,
				StockingStrategy:    stocking,
				CreatedAt:           &now,
				UpdatedAt:           &now,
			})
			if err != nil {
				log.Printf("bi_realtime: insert product abc-xyz history: %v", err)
			}
		}
	}

	// 6. Update product document (targeted $set)
	ctx5, cancel5 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel5()
	update := bson.M{
		"sales_velocity_trend":        velocityTrend,
		"sales_velocity_trend_reason": velocityReason,
		"slop_percent_per_month":      slopePct,
		"momentum_percent_per_3month": momentumPct,
		"avg_monthly_qty":             avgMonthly,
		"recent_3month_qty":           recent3,
		"revenue":                     totalRevenue,
		"active_months":               activeMonths,
	}
	if xyzTier != "" {
		update["xyz_tier"] = xyzTier
		update["cv"] = cv
		if existingProd.AbcTier != "" {
			newClass := existingProd.AbcTier + xyzTier
			update["class"] = newClass
			update["stocking_strategy"] = deriveStockingStrategy(existingProd.AbcTier, xyzTier)
		}
	}
	_, err = prodCol.UpdateOne(ctx5, bson.M{"_id": productID}, bson.M{"$set": update})
	if err != nil {
		log.Printf("bi_realtime: update product BI fields: %v", err)
	}
}

// deriveStockingStrategy returns a human-readable stocking recommendation
// based on ABC tier (revenue importance) and XYZ tier (demand variability).
func deriveStockingStrategy(abcTier, xyzTier string) string {
	switch abcTier + xyzTier {
	case "AX":
		return "Continuous replenishment"
	case "AY":
		return "Safety stock with regular review"
	case "AZ":
		return "Frequent monitoring, flexible ordering"
	case "BX":
		return "Periodic replenishment"
	case "BY":
		return "Safety stock with periodic review"
	case "BZ":
		return "Flexible ordering strategy"
	case "CX":
		return "Lean stock, infrequent replenishment"
	case "CY":
		return "Minimal safety stock"
	case "CZ":
		return "On-demand or consignment"
	default:
		return ""
	}
}

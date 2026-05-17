package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CustomerCohortRetentionReport holds cohort-level retention statistics.
// One document per cohort month per store; upserted by {store_id, cohort_first_buy_month}.
// Written by the BI cron job every 3 hours.
type CustomerCohortRetentionReport struct {
	ID                               primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID                          *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CohortFirstBuyMonth              string              `bson:"cohort_first_buy_month,omitempty" json:"cohort_first_buy_month,omitempty"`
	CohortSize                       int                 `bson:"cohort_size" json:"cohort_size"`
	TotalSpend                       float64             `bson:"total_spend" json:"total_spend"`
	AvgSpendPerCustomer              float64             `bson:"avg_spend_per_customer" json:"avg_spend_per_customer"`
	RetainedAt1Month                 int                 `bson:"retained_at_1month" json:"retained_at_1month"`
	RetainedAt1MonthPercentOfCohort  float64             `bson:"retained_at_1month_percent_of_cohort" json:"retained_at_1month_percent_of_cohort"`
	RetainedAt3Month                 int                 `bson:"retained_at_3month" json:"retained_at_3month"`
	RetainedAt3MonthPercentOfCohort  float64             `bson:"retained_at_3month_percent_of_cohort" json:"retained_at_3month_percent_of_cohort"`
	RetainedAt6Month                 int                 `bson:"retained_at_6month" json:"retained_at_6month"`
	RetainedAt6MonthPercentOfCohort  float64             `bson:"retained_at_6month_percent_of_cohort" json:"retained_at_6month_percent_of_cohort"`
	RetainedAt12Month                int                 `bson:"retained_at_12month" json:"retained_at_12month"`
	RetainedAt12MonthPercentOfCohort float64             `bson:"retained_at_12month_percent_of_cohort" json:"retained_at_12month_percent_of_cohort"`
	RetainedAt24Month                int                 `bson:"retained_at_24month" json:"retained_at_24month"`
	RetainedAt24MonthPercentOfCohort float64             `bson:"retained_at_24month_percent_of_cohort" json:"retained_at_24month_percent_of_cohort"`
	BestRetentionPeriod              string              `bson:"best_retention_period,omitempty" json:"best_retention_period,omitempty"`
	BestRetentionPeriodPercent       float64             `bson:"best_retention_period_percent" json:"best_retention_period_percent"`
	RetentionTrend                   string              `bson:"retention_trend,omitempty" json:"retention_trend,omitempty"`
	CreatedAt                        *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt                        *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

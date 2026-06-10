package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BIBatchCost accumulates AWS Batch cost for one report_key across all runs and stores.
// One document per report_key, stored in the global DB (not per-store).
type BIBatchCost struct {
	ReportKey        string    `bson:"report_key" json:"report_key"`
	TotalCostUSD     float64   `bson:"total_cost_usd" json:"total_cost_usd"`
	TotalDurationSec int64     `bson:"total_duration_sec" json:"total_duration_sec"`
	RunCount         int       `bson:"run_count" json:"run_count"`
	UpdatedAt        time.Time `bson:"updated_at" json:"updated_at"`
}

func AddBIBatchCost(reportKey string, costUSD float64, durationSec int64) error {
	col := db.GetDB("").Collection("bi_batch_cost")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Update().SetUpsert(true)
	_, err := col.UpdateOne(ctx,
		bson.M{"report_key": reportKey},
		bson.M{
			"$inc": bson.M{
				"total_cost_usd":     costUSD,
				"total_duration_sec": durationSec,
				"run_count":          1,
			},
			"$set": bson.M{"updated_at": time.Now().UTC()},
		},
		opts,
	)
	return err
}

func GetAllBIBatchCosts() ([]BIBatchCost, error) {
	col := db.GetDB("").Collection("bi_batch_cost")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	findOpts := options.Find().SetSort(bson.M{"total_cost_usd": -1})
	cur, err := col.Find(ctx, bson.M{}, findOpts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var results []BIBatchCost
	if err := cur.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

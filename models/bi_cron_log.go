package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BICronReportLog records the outcome of one report within a cron run.
type BICronReportLog struct {
	ReportKey  string    `bson:"report_key" json:"report_key"`
	ReportName string    `bson:"report_name" json:"report_name"`
	Status     string    `bson:"status" json:"status"` // "running" | "done" | "error" | "skipped"
	Log        string    `bson:"log" json:"log"`
	StartedAt  time.Time `bson:"started_at" json:"started_at"`
	FinishedAt time.Time `bson:"finished_at,omitempty" json:"finished_at,omitempty"`
	ElapsedSec float64   `bson:"elapsed_sec,omitempty" json:"elapsed_sec,omitempty"`
}

// BICronLog records the last full cron run for a store.
// Only one document is kept per store — replaced on every run.
type BICronLog struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	StoreID      primitive.ObjectID `bson:"store_id" json:"store_id"`
	RunID        string             `bson:"run_id" json:"run_id"`
	StartedAt    time.Time          `bson:"started_at" json:"started_at"`
	FinishedAt   time.Time          `bson:"finished_at,omitempty" json:"finished_at,omitempty"`
	Status       string             `bson:"status" json:"status"` // "running" | "done" | "error"
	TotalReports int                `bson:"total_reports" json:"total_reports"`
	DoneReports  int                `bson:"done_reports" json:"done_reports"`
	Reports      []BICronReportLog  `bson:"reports" json:"reports"`
}

func (store *Store) GetBICronLog() (*BICronLog, error) {
	col := db.GetDB("store_" + store.ID.Hex()).Collection("bi_cron_log")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var result BICronLog
	opts := options.FindOne().SetSort(bson.D{{Key: "started_at", Value: -1}})
	if err := col.FindOne(ctx, bson.M{}, opts).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (store *Store) DeleteBICronLog() error {
	col := db.GetDB("store_" + store.ID.Hex()).Collection("bi_cron_log")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := col.DeleteMany(ctx, bson.M{"store_id": store.ID})
	return err
}

func (store *Store) UpsertBICronLog(cronLog *BICronLog) error {
	col := db.GetDB("store_" + store.ID.Hex()).Collection("bi_cron_log")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cronLog.StoreID = store.ID
	opts := options.Replace().SetUpsert(true)
	_, err := col.ReplaceOne(ctx, bson.M{"store_id": store.ID}, cronLog, opts)
	return err
}

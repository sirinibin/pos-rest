package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BIAwsBatchSettings stores the AWS Batch configuration used by the BI cron runner.
// A single document is kept in the global DB — upserted on every save.
type BIAwsBatchSettings struct {
	Region        string    `bson:"region" json:"region"`
	JobQueue      string    `bson:"job_queue" json:"job_queue"`
	JobDefinition string    `bson:"job_definition" json:"job_definition"`
	LogGroup      string    `bson:"log_group" json:"log_group"`
	UpdatedAt     time.Time `bson:"updated_at" json:"updated_at"`
}

func GetBIAwsBatchSettings() (*BIAwsBatchSettings, error) {
	col := db.GetDB("").Collection("bi_aws_batch_settings")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var s BIAwsBatchSettings
	if err := col.FindOne(ctx, bson.M{}).Decode(&s); err != nil {
		return &BIAwsBatchSettings{LogGroup: "/aws/batch/job"}, nil
	}
	return &s, nil
}

func UpsertBIAwsBatchSettings(s *BIAwsBatchSettings) error {
	col := db.GetDB("").Collection("bi_aws_batch_settings")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s.UpdatedAt = time.Now().UTC()
	if s.LogGroup == "" {
		s.LogGroup = "/aws/batch/job"
	}

	opts := options.Replace().SetUpsert(true)
	_, err := col.ReplaceOne(ctx, bson.M{}, s, opts)
	return err
}

package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BICronStoreSettings persists the Store Settings section of the Cron Jobs UI.
// One document per POS instance URL, stored in the global DB.
type BICronStoreSettings struct {
	BaseURL         string            `bson:"base_url" json:"base_url"`
	EnabledStores   []string          `bson:"enabled_stores" json:"enabled_stores"`
	ReportSchedules map[string]string `bson:"report_schedules" json:"report_schedules"`
	ReportPlatforms map[string]string `bson:"report_platforms" json:"report_platforms"`
	UpdatedAt       time.Time         `bson:"updated_at" json:"updated_at"`
}

func GetBICronStoreSettings(baseURL string) (*BICronStoreSettings, error) {
	col := db.GetDB("").Collection("bi_cron_settings")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{"base_url": baseURL}
	var s BICronStoreSettings
	if err := col.FindOne(ctx, filter).Decode(&s); err != nil {
		return &BICronStoreSettings{
			BaseURL:         baseURL,
			EnabledStores:   []string{},
			ReportSchedules: map[string]string{},
			ReportPlatforms: map[string]string{},
		}, nil
	}
	return &s, nil
}

func UpsertBICronStoreSettings(s *BICronStoreSettings) error {
	col := db.GetDB("").Collection("bi_cron_settings")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s.UpdatedAt = time.Now().UTC()
	if s.EnabledStores == nil {
		s.EnabledStores = []string{}
	}
	if s.ReportSchedules == nil {
		s.ReportSchedules = map[string]string{}
	}
	if s.ReportPlatforms == nil {
		s.ReportPlatforms = map[string]string{}
	}

	opts := options.Replace().SetUpsert(true)
	_, err := col.ReplaceOne(ctx, bson.M{"base_url": s.BaseURL}, s, opts)
	return err
}

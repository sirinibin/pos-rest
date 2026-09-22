package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AdminSettings holds global configuration that applies across all stores.
// A single document is kept in the admin_settings collection (singleton).
type AdminSettings struct {
	// AWS S3 file storage (shared by all stores)
	S3Enabled       bool      `bson:"s3_enabled" json:"s3_enabled"`
	S3BucketName    string    `bson:"s3_bucket_name,omitempty" json:"s3_bucket_name,omitempty"`
	S3Region        string    `bson:"s3_region,omitempty" json:"s3_region,omitempty"`
	S3AccessKeyID   string    `bson:"s3_access_key_id,omitempty" json:"s3_access_key_id,omitempty"`
	S3SecretKey     string    `bson:"s3_secret_key,omitempty" json:"s3_secret_key,omitempty"`
	S3Endpoint      string    `bson:"s3_endpoint,omitempty" json:"s3_endpoint,omitempty"`
	S3PublicBaseURL string    `bson:"s3_public_base_url,omitempty" json:"s3_public_base_url,omitempty"`
	UpdatedAt       time.Time `bson:"updated_at" json:"updated_at"`
}

func GetAdminSettings() (*AdminSettings, error) {
	col := db.GetDB("").Collection("admin_settings")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var s AdminSettings
	if err := col.FindOne(ctx, bson.M{}).Decode(&s); err != nil {
		return &AdminSettings{}, nil
	}
	return &s, nil
}

func UpsertAdminSettings(s *AdminSettings) error {
	col := db.GetDB("").Collection("admin_settings")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s.UpdatedAt = time.Now().UTC()
	opts := options.Replace().SetUpsert(true)
	_, err := col.ReplaceOne(ctx, bson.M{}, s, opts)
	return err
}

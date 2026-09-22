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

const googlePlacesCacheTTL = 30 * 24 * time.Hour

type GooglePlacesCacheEntry struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	Category  string             `bson:"category"`
	Market    string             `bson:"market"`
	Suppliers []CachedSupplier   `bson:"suppliers"`
	CachedAt  time.Time          `bson:"cached_at"`
	ExpiresAt time.Time          `bson:"expires_at"`
}

type CachedSupplier struct {
	Name    string   `bson:"name"`
	Phone   string   `bson:"phone"`
	Address string   `bson:"address"`
	Lat     float64  `bson:"lat"`
	Lng     float64  `bson:"lng"`
	Rating  float64  `bson:"rating"`
	PlaceID string   `bson:"place_id"`
	Website string   `bson:"website"`
	Types   []string `bson:"types,omitempty"`
}

func googlePlacesCacheCollection() string { return "google_places_cache" }

func EnsureGooglePlacesCacheIndexes() error {
	col := db.Client("").Database(db.GetPosDB()).Collection(googlePlacesCacheCollection())
	ctx := context.Background()

	// TTL index — MongoDB auto-deletes expired documents
	_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0),
		},
		{
			Keys:    bson.D{{Key: "category", Value: 1}, {Key: "market", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	})
	return err
}

// GetGooglePlacesCache returns cached suppliers for the given category+market pair.
// Returns nil, nil when no valid cache entry exists.
func GetGooglePlacesCache(category, market string) ([]CachedSupplier, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection(googlePlacesCacheCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var entry GooglePlacesCacheEntry
	err := col.FindOne(ctx, bson.M{
		"category":   category,
		"market":     market,
		"expires_at": bson.M{"$gt": time.Now()},
	}).Decode(&entry)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return entry.Suppliers, nil
}

// SetGooglePlacesCache stores suppliers for the given category+market pair.
func SetGooglePlacesCache(category, market string, suppliers []CachedSupplier) error {
	col := db.Client("").Database(db.GetPosDB()).Collection(googlePlacesCacheCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	_, err := col.UpdateOne(
		ctx,
		bson.M{"category": category, "market": market},
		bson.M{"$set": bson.M{
			"suppliers":  suppliers,
			"cached_at":  now,
			"expires_at": now.Add(googlePlacesCacheTTL),
		}},
		options.Update().SetUpsert(true),
	)
	return err
}

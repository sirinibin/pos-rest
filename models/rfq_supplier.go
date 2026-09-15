package models

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// RFQSupplier stores vendor/supplier records discovered via Google Maps or added manually.
type RFQSupplier struct {
	ID            primitive.ObjectID  `bson:"_id,omitempty" json:"id,omitempty"`
	StoreID       primitive.ObjectID  `bson:"store_id" json:"store_id"`
	Name          string              `bson:"name" json:"name"`
	Phone         string              `bson:"phone" json:"phone"` // WhatsApp number (international, no +)
	Address       string              `bson:"address,omitempty" json:"address,omitempty"`
	Latitude      float64             `bson:"latitude,omitempty" json:"latitude,omitempty"`
	Longitude     float64             `bson:"longitude,omitempty" json:"longitude,omitempty"`
	Categories    []string            `bson:"categories,omitempty" json:"categories,omitempty"`
	Rating        float64             `bson:"rating" json:"rating"`
	GooglePlaceID   string `bson:"google_place_id,omitempty" json:"google_place_id,omitempty"`
	GoogleMapsURL   string `bson:"google_maps_url,omitempty" json:"google_maps_url,omitempty"`
	PurchaseMarket  string `bson:"purchase_market,omitempty" json:"purchase_market,omitempty"`
	Website         string `bson:"website,omitempty" json:"website,omitempty"`
	IsActive      bool                `bson:"is_active" json:"is_active"`
	AddedAt       time.Time           `bson:"added_at" json:"added_at"`
	CreatedBy     *primitive.ObjectID `bson:"created_by,omitempty" json:"created_by,omitempty"`
	// MatchedCategory is set transiently when a supplier is selected for a specific RFQ category.
	// Not persisted to the database.
	MatchedCategory string `bson:"-" json:"-"`
}

func rfqSupplierCollection() string {
	return "rfq_suppliers"
}

// EnsureRFQSupplierIndexes creates text and supporting indexes on the rfq_suppliers collection.
func EnsureRFQSupplierIndexes() {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "name", Value: "text"},
				{Key: "phone", Value: "text"},
				{Key: "address", Value: "text"},
				{Key: "categories", Value: "text"},
			},
			Options: options.Index().SetName("rfq_suppliers_text_idx"),
		},
		{Keys: bson.D{{Key: "store_id", Value: 1}, {Key: "purchase_market", Value: 1}}},
		{Keys: bson.D{{Key: "store_id", Value: 1}, {Key: "google_place_id", Value: 1}}},
		{Keys: bson.D{{Key: "store_id", Value: 1}, {Key: "is_active", Value: 1}, {Key: "categories", Value: 1}}},
	})
}

func UpsertRFQSupplierByPlaceID(supplier *RFQSupplier) error {
	if supplier.ID.IsZero() {
		supplier.ID = primitive.NewObjectID()
	}
	if supplier.AddedAt.IsZero() {
		supplier.AddedAt = time.Now()
	}
	supplier.IsActive = true

	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"store_id":        supplier.StoreID,
		"google_place_id": supplier.GooglePlaceID,
	}
	if supplier.GooglePlaceID == "" {
		filter = bson.M{"store_id": supplier.StoreID, "phone": supplier.Phone}
	}

	// $set all scalar fields; $addToSet merges categories so existing ones are not lost.
	setFields := bson.M{
		"store_id":        supplier.StoreID,
		"name":            supplier.Name,
		"phone":           supplier.Phone,
		"address":         supplier.Address,
		"latitude":        supplier.Latitude,
		"longitude":       supplier.Longitude,
		"rating":          supplier.Rating,
		"google_place_id": supplier.GooglePlaceID,
		"google_maps_url": supplier.GoogleMapsURL,
		"purchase_market": supplier.PurchaseMarket,
		"website":         supplier.Website,
		"is_active":       supplier.IsActive,
		"added_at":        supplier.AddedAt,
	}
	update := bson.M{"$set": setFields}
	if len(supplier.Categories) > 0 {
		update["$addToSet"] = bson.M{"categories": bson.M{"$each": supplier.Categories}}
	}

	// Use FindOneAndUpdate so we get back the real _id (whether newly inserted or existing).
	fopts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	var result RFQSupplier
	if err := col.FindOneAndUpdate(ctx, filter, update, fopts).Decode(&result); err == nil {
		supplier.ID = result.ID
	}
	return nil
}

func CreateRFQSupplier(supplier *RFQSupplier) error {
	if supplier.ID.IsZero() {
		supplier.ID = primitive.NewObjectID()
	}
	supplier.AddedAt = time.Now()
	supplier.IsActive = true

	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.InsertOne(ctx, supplier)
	return err
}

func UpdateRFQSupplier(supplier *RFQSupplier) error {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.ReplaceOne(ctx, bson.M{"_id": supplier.ID, "store_id": supplier.StoreID}, supplier)
	return err
}

func DeleteRFQSupplier(id, storeID primitive.ObjectID) error {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.DeleteOne(ctx, bson.M{"_id": id, "store_id": storeID})
	return err
}

type RFQSupplierListResult struct {
	Items      []RFQSupplier `json:"items"`
	TotalCount int64         `json:"total_count"`
}

func ListRFQSuppliers(storeID primitive.ObjectID, page, limit int64, search string, categories []string) (*RFQSupplierListResult, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{"store_id": storeID}
	if search != "" {
		filter["$or"] = bson.A{
			bson.M{"name": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"phone": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"address": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"categories": bson.M{"$regex": search, "$options": "i"}},
		}
	}
	if len(categories) > 0 {
		filter["categories"] = bson.M{"$in": categories}
	}

	total, _ := col.CountDocuments(ctx, filter)

	skip := (page - 1) * limit
	opts := options.Find().
		SetSort(bson.D{{Key: "added_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(limit)

	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var items []RFQSupplier
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []RFQSupplier{}
	}
	return &RFQSupplierListResult{Items: items, TotalCount: total}, nil
}

func FindRFQSupplierByID(id, storeID primitive.ObjectID) (*RFQSupplier, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var s RFQSupplier
	err := col.FindOne(ctx, bson.M{"_id": id, "store_id": storeID}).Decode(&s)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// FindRFQSuppliersByCategories returns active suppliers that match any of the given categories.
// flexCategoryRegex returns a MongoDB regex pattern that matches common plural/singular
// variants of a category name (e.g. "System" ↔ "Systems", "Supply" ↔ "Supplies").
// Use with "$options": "i" for case-insensitive matching.
func flexCategoryRegex(cat string) string {
	trimmed := strings.TrimSpace(cat)
	if trimmed == "" {
		return "^$"
	}
	lower := strings.ToLower(trimmed)
	escaped := regexp.QuoteMeta(trimmed)

	switch {
	case strings.HasSuffix(lower, "ies") && len(lower) > 3:
		// Supplies → also match Supply
		base := regexp.QuoteMeta(trimmed[:len(trimmed)-3])
		return "^(" + escaped + "|" + base + "y)$"
	case strings.HasSuffix(lower, "y") && len(lower) > 1:
		// Supply → also match Supplies
		base := regexp.QuoteMeta(trimmed[:len(trimmed)-1])
		return "^(" + escaped + "|" + base + "ies)$"
	case strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ss") && len(lower) > 1:
		// Systems → also match System
		base := regexp.QuoteMeta(trimmed[:len(trimmed)-1])
		return "^(" + escaped + "|" + base + ")$"
	default:
		// System → also match Systems
		return "^(" + escaped + "|" + escaped + "s)$"
	}
}

func FindRFQSuppliersByCategories(storeID primitive.ObjectID, categories []string, limit int64) ([]RFQSupplier, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var catOr bson.A
	for _, cat := range categories {
		catOr = append(catOr, bson.M{"categories": bson.M{"$regex": flexCategoryRegex(cat), "$options": "i"}})
	}
	filter := bson.M{
		"store_id":  storeID,
		"is_active": true,
		"phone":     bson.M{"$ne": ""},
	}
	if len(catOr) > 0 {
		filter["$or"] = catOr
	}
	opts := options.Find().SetLimit(limit).SetSort(bson.D{{Key: "rating", Value: -1}})
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []RFQSupplier
	cur.All(ctx, &items)
	return items, nil
}

// FindRFQSuppliersByIDs fetches suppliers by their ObjectID slice, preserving order.
func FindRFQSuppliersByIDs(ids []primitive.ObjectID) ([]RFQSupplier, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cur, err := col.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []RFQSupplier
	cur.All(ctx, &items)
	// reorder to match input ids
	byID := map[primitive.ObjectID]RFQSupplier{}
	for _, s := range items {
		byID[s.ID] = s
	}
	ordered := make([]RFQSupplier, 0, len(ids))
	for _, id := range ids {
		if s, ok := byID[id]; ok {
			ordered = append(ordered, s)
		}
	}
	return ordered, nil
}

// FindRFQSuppliersByMarketAndCategories returns active suppliers for a specific purchase market.
// When market == "" it matches any market. When categories is empty, all active suppliers qualify.
// For a non-empty market it also includes suppliers with no market set (can serve any market).
func FindRFQSuppliersByMarketAndCategories(storeID primitive.ObjectID, categories []string, market string, limit int64) ([]RFQSupplier, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{
		"store_id":  storeID,
		"is_active": true,
		"phone":     bson.M{"$ne": ""},
	}

	// Case-insensitive category matching: build a regex OR for each category.
	// If market is also set we combine both conditions under $and so they don't overwrite each other.
	var categoryOr bson.A
	if len(categories) > 0 {
		for _, cat := range categories {
			categoryOr = append(categoryOr, bson.M{"categories": bson.M{"$regex": flexCategoryRegex(cat), "$options": "i"}})
		}
	}

	var marketOr bson.A
	if market != "" {
		marketOr = bson.A{
			bson.M{"purchase_market": market},
			bson.M{"purchase_market": bson.M{"$in": bson.A{"", nil}}},
			bson.M{"purchase_market": bson.M{"$exists": false}},
		}
	}

	switch {
	case len(categoryOr) > 0 && len(marketOr) > 0:
		filter["$and"] = bson.A{
			bson.M{"$or": categoryOr},
			bson.M{"$or": marketOr},
		}
	case len(categoryOr) > 0:
		filter["$or"] = categoryOr
	case len(marketOr) > 0:
		filter["$or"] = marketOr
	}

	opts := options.Find().SetLimit(limit).SetSort(bson.D{{Key: "rating", Value: -1}})
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []RFQSupplier
	cur.All(ctx, &items)
	return items, nil
}

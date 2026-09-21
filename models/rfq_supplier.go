package models

import (
	"context"
	"fmt"
	"log"
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
	Code          string              `bson:"code,omitempty" json:"code,omitempty"` // auto-generated serial ID, e.g. SUP-000001
	Name          string              `bson:"name" json:"name"`
	Phone         string              `bson:"phone" json:"phone"`  // primary WhatsApp number (international, no +)
	Phone2        string              `bson:"phone2,omitempty" json:"phone2,omitempty"` // secondary/alternate WhatsApp number
	Address       string              `bson:"address,omitempty" json:"address,omitempty"`
	Latitude      float64             `bson:"latitude,omitempty" json:"latitude,omitempty"`
	Longitude     float64             `bson:"longitude,omitempty" json:"longitude,omitempty"`
	Categories    []string            `bson:"categories,omitempty" json:"categories,omitempty"`
	Rating        float64             `bson:"rating" json:"rating"`
	GooglePlaceID   string `bson:"google_place_id,omitempty" json:"google_place_id,omitempty"`
	GoogleMapsURL   string `bson:"google_maps_url,omitempty" json:"google_maps_url,omitempty"`
	PurchaseMarket  string `bson:"purchase_market,omitempty" json:"purchase_market,omitempty"`
	Website         string `bson:"website,omitempty" json:"website,omitempty"`
	Email           string `bson:"email,omitempty" json:"email,omitempty"`
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

// MakeRFQSupplierCode generates a sequential serial ID for a new supplier, e.g. "SUP-000001".
func MakeRFQSupplierCode(storeID primitive.ObjectID) string {
	redisKey := storeID.Hex() + "_rfq_supplier_counter"
	n, err := db.RedisClient.Incr(redisKey).Result()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("SUP-%06d", n)
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
		// Phone is the primary uniqueness key — no two suppliers in the same store may share a phone number.
		{
			Keys:    bson.D{{Key: "store_id", Value: 1}, {Key: "phone", Value: 1}},
			Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"phone": bson.M{"$gt": ""}}).SetName("rfq_suppliers_store_phone_uniq"),
		},
	})
}

// UpsertRFQSupplierByPlaceID saves or updates a supplier.
// Phone is the primary uniqueness key (per user requirement).
// Match priority: 1) phone  2) google_place_id  3) insert new.
func UpsertRFQSupplierByPlaceID(supplier *RFQSupplier) error {
	if supplier.AddedAt.IsZero() {
		supplier.AddedAt = time.Now()
	}
	supplier.IsActive = true

	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// $set all scalar fields; $addToSet merges categories so existing ones are not lost.
	setFields := bson.M{
		"store_id":        supplier.StoreID,
		"name":            supplier.Name,
		"phone":           supplier.Phone,
		"phone2":          supplier.Phone2,
		"address":         supplier.Address,
		"latitude":        supplier.Latitude,
		"longitude":       supplier.Longitude,
		"rating":          supplier.Rating,
		"google_place_id": supplier.GooglePlaceID,
		"google_maps_url": supplier.GoogleMapsURL,
		"purchase_market": supplier.PurchaseMarket,
		"website":         supplier.Website,
		"email":           supplier.Email,
		"is_active":       true,
		"added_at":        supplier.AddedAt,
	}
	update := bson.M{"$set": setFields}
	if len(supplier.Categories) > 0 {
		update["$addToSet"] = bson.M{"categories": bson.M{"$each": supplier.Categories}}
	}
	fopts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)

	// 1. Phone is the uniqueness key — always try phone first.
	if supplier.Phone != "" {
		var result RFQSupplier
		filter := bson.M{"store_id": supplier.StoreID, "phone": supplier.Phone}
		if err := col.FindOneAndUpdate(ctx, filter, update, fopts).Decode(&result); err == nil {
			supplier.ID = result.ID
			return nil
		}
	}

	// 2. Fallback: match by google_place_id (covers suppliers imported before phone was required).
	if supplier.GooglePlaceID != "" {
		var result RFQSupplier
		filter := bson.M{"store_id": supplier.StoreID, "google_place_id": supplier.GooglePlaceID}
		if err := col.FindOneAndUpdate(ctx, filter, update, fopts).Decode(&result); err == nil {
			supplier.ID = result.ID
			return nil
		}
	}

	// 3. Nothing matched — insert new.
	if supplier.ID.IsZero() {
		supplier.ID = primitive.NewObjectID()
	}
	if supplier.Code == "" {
		supplier.Code = MakeRFQSupplierCode(supplier.StoreID)
	}
	_, err := col.InsertOne(ctx, supplier)
	return err
}

// CreateRFQSupplier upserts a supplier by phone (primary uniqueness key).
// If a supplier with the same phone already exists it is updated, not duplicated.
func CreateRFQSupplier(supplier *RFQSupplier) error {
	return UpsertRFQSupplierByPlaceID(supplier)
}

func UpdateRFQSupplier(supplier *RFQSupplier) error {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.ReplaceOne(ctx, bson.M{"_id": supplier.ID, "store_id": supplier.StoreID}, supplier)
	return err
}

// SetRFQSupplierEmail saves the email field for a supplier.
func SetRFQSupplierEmail(supplierID primitive.ObjectID, email string) error {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.UpdateOne(ctx,
		bson.M{"_id": supplierID},
		bson.M{"$set": bson.M{"email": email}},
	)
	return err
}

// ListSuppliersWithWebsiteNoEmail returns suppliers that have a website but no email set.
func ListSuppliersWithWebsiteNoEmail(storeID primitive.ObjectID) ([]RFQSupplier, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cur, err := col.Find(ctx, bson.M{
		"store_id": storeID,
		"website":  bson.M{"$ne": "", "$exists": true},
		"$or":      bson.A{bson.M{"email": ""}, bson.M{"email": bson.M{"$exists": false}}},
	})
	if err != nil {
		return nil, err
	}
	var suppliers []RFQSupplier
	_ = cur.All(ctx, &suppliers)
	return suppliers, nil
}

// SetRFQSupplierCategories replaces the categories field for a specific supplier.
func SetRFQSupplierCategories(supplierID, storeID primitive.ObjectID, categories []string) error {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.UpdateOne(ctx,
		bson.M{"_id": supplierID, "store_id": storeID},
		bson.M{"$set": bson.M{"categories": categories}},
	)
	return err
}

// DeduplicateRFQSuppliersAllStores runs DeduplicateRFQSuppliers for every active store.
// Called once at startup to clean up any pre-existing duplicates.
func DeduplicateRFQSuppliersAllStores() {
	stores, err := GetAllStores()
	if err != nil {
		return
	}
	for _, s := range stores {
		if s.Deleted {
			continue
		}
		n, _ := DeduplicateRFQSuppliers(s.ID)
		if n > 0 {
			log.Printf("rfq_suppliers: removed %d duplicate(s) for store %s", n, s.ID.Hex())
		}
	}
}

// DeduplicateRFQSuppliers removes duplicate supplier records for the given store.
// It groups by phone number, keeps the record with the most data (categories, google_place_id),
// and deletes the rest. Returns the number of duplicates removed.
func DeduplicateRFQSuppliers(storeID primitive.ObjectID) (int, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Aggregate: group by phone, collect all _ids, keep the one with the best data.
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"store_id": storeID, "phone": bson.M{"$gt": ""}}}},
		{{Key: "$sort", Value: bson.D{
			{Key: "google_place_id", Value: -1}, // prefer records with a place ID
			{Key: "added_at", Value: 1},          // among equals, keep the oldest (first added)
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":     "$phone",
			"keep_id": bson.M{"$first": "$$ROOT._id"},
			"all_ids": bson.M{"$push": "$$ROOT._id"},
			"count":   bson.M{"$sum": 1},
		}}},
		{{Key: "$match", Value: bson.M{"count": bson.M{"$gt": 1}}}},
	}
	cursor, err := col.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	removed := 0
	for cursor.Next(ctx) {
		var row struct {
			KeepID primitive.ObjectID   `bson:"keep_id"`
			AllIDs []primitive.ObjectID `bson:"all_ids"`
		}
		if err := cursor.Decode(&row); err != nil {
			continue
		}
		var deleteIDs []primitive.ObjectID
		for _, id := range row.AllIDs {
			if id != row.KeepID {
				deleteIDs = append(deleteIDs, id)
			}
		}
		if len(deleteIDs) == 0 {
			continue
		}
		res, err := col.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": deleteIDs}})
		if err == nil {
			removed += int(res.DeletedCount)
		}
	}
	return removed, cursor.Err()
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
			bson.M{"phone2": bson.M{"$regex": search, "$options": "i"}},
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

func FindRFQSupplierByPhone(storeID primitive.ObjectID, phone string) (*RFQSupplier, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqSupplierCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Check phone2 first — if a supplier explicitly configured this as their secondary number,
	// that match is more specific and should take priority over another supplier's phone1.
	var s RFQSupplier
	if err := col.FindOne(ctx, bson.M{"store_id": storeID, "phone2": phone}).Decode(&s); err == nil {
		return &s, nil
	}
	if err := col.FindOne(ctx, bson.M{"store_id": storeID, "phone": phone}).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
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
			bson.M{"purchase_market": bson.M{"$regex": "^" + regexp.QuoteMeta(market) + "$", "$options": "i"}},
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

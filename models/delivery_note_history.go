package models

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ProductDeliveryNoteHistory struct {
	ID               primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date             *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	StoreID          *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName        string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	ProductID        primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	CustomerID       *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	CustomerName     string              `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	DeliveryNoteID   *primitive.ObjectID `json:"delivery_note_id,omitempty" bson:"delivery_note_id,omitempty"`
	DeliveryNoteCode string              `json:"delivery_note_code,omitempty" bson:"delivery_note_code,omitempty"`
	Quantity         float64             `json:"quantity,omitempty" bson:"quantity,omitempty"`
	Unit             string              `bson:"unit,omitempty" json:"unit,omitempty"`
	CreatedAt        *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt        *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

type DeliveryNoteHistoryStats struct {
	ID            *primitive.ObjectID `json:"id" bson:"_id"`
	TotalQuantity float64             `json:"total_quantity" bson:"total_quantity"`
}

func (store *Store) GetDeliveryNoteHistoryStats(filter map[string]interface{}) (stats DeliveryNoteHistoryStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_delivery_note_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":            nil,
				"total_quantity": bson.M{"$sum": "$quantity"},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return stats, err
	}
	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return stats, err
		}
		stats.TotalQuantity = RoundFloat(stats.TotalQuantity, 2)
	}

	return stats, nil
}

func (store *Store) SearchDeliveryNoteHistory(w http.ResponseWriter, r *http.Request) (models []ProductDeliveryNoteHistory, criterias SearchCriterias, err error) {

	criterias = InitSearchCriterias()
	criterias.SortBy = map[string]interface{}{"created_at": -1}

	timeZoneOffset := CountryTimezoneOffset(store.CountryCode)
	var keys []string
	var ok bool

	if err = ParseExactDateFilter(r, &criterias, "search[date_str]", "date", timeZoneOffset); err != nil {
		return models, criterias, err
	}

	if err = ParseDateRangeFilter(r, &criterias, "search[from_date]", "search[to_date]", "date", timeZoneOffset); err != nil {
		return models, criterias, err
	}

	if err = ParseExactDateFilter(r, &criterias, "search[created_at]", "created_at", timeZoneOffset); err != nil {
		return models, criterias, err
	}

	if err = ParseDateRangeFilter(r, &criterias, "search[created_at_from]", "search[created_at_to]", "created_at", timeZoneOffset); err != nil {
		return models, criterias, err
	}

	ParseTextSearch(r, &criterias, "search[store_name]", "store_name")

	ParseTextSearch(r, &criterias, "search[customer_name]", "customer_name")

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {

		customerIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range customerIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return models, criterias, err
			}
			objecIds = append(objecIds, customerID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["customer_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[quantity]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["quantity"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["quantity"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[discount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["discount"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["discount"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[discount_percent]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["discount_percent"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["discount_percent"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {

		customerIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range customerIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return models, criterias, err
			}
			objecIds = append(objecIds, customerID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["customer_id"] = bson.M{"$in": objecIds}
		}
	}

	if err = ParseObjectIDFilter(r, &criterias, "search[store_id]", "store_id"); err != nil {
		return models, criterias, err
	}

	if err = ParseObjectIDFilter(r, &criterias, "search[product_id]", "product_id"); err != nil {
		return models, criterias, err
	}

	if err = ParseObjectIDFilter(r, &criterias, "search[delivery_note_id]", "delivery_note_id"); err != nil {
		return models, criterias, err
	}

	keys, ok = r.URL.Query()["search[delivery_note_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["delivery_note_code"] = keys[0]
	}

	ParsePaginationAndSort(r, &criterias)

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_delivery_note_history")

	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		//Relational Select Fields
	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return models, criterias, errors.New("Error fetching product deliverynote history:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		model := ProductDeliveryNoteHistory{}
		err = cur.Decode(&model)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		models = append(models, model)
	} //end for loop

	return models, criterias, nil
}

func (deliverynote *DeliveryNote) CreateProductsDeliveryNoteHistory() error {
	store, err := FindStoreByID(deliverynote.StoreID, bson.M{})
	if err != nil {
		return err
	}

	exists, err := store.IsDeliveryNoteHistoryExistsByDeliveryNoteID(&deliverynote.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.GetDB("store_" + deliverynote.StoreID.Hex()).Collection("product_delivery_note_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, deliverynoteProduct := range deliverynote.Products {

		history := ProductDeliveryNoteHistory{
			Date:             deliverynote.Date,
			StoreID:          deliverynote.StoreID,
			StoreName:        deliverynote.StoreName,
			ProductID:        deliverynoteProduct.ProductID,
			CustomerID:       deliverynote.CustomerID,
			CustomerName:     deliverynote.CustomerName,
			DeliveryNoteID:   &deliverynote.ID,
			DeliveryNoteCode: deliverynote.Code,
			Quantity:         deliverynoteProduct.Quantity,
			Unit:             deliverynoteProduct.Unit,
			CreatedAt:        deliverynote.CreatedAt,
			UpdatedAt:        deliverynote.UpdatedAt,
		}

		history.ID = primitive.NewObjectID()
		_, err := collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}

		product, err := store.FindProductByID(&deliverynoteProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				history := ProductDeliveryNoteHistory{
					Date:             deliverynote.Date,
					StoreID:          deliverynote.StoreID,
					StoreName:        deliverynote.StoreName,
					ProductID:        *setProduct.ProductID,
					CustomerID:       deliverynote.CustomerID,
					CustomerName:     deliverynote.CustomerName,
					DeliveryNoteID:   &deliverynote.ID,
					DeliveryNoteCode: deliverynote.Code,
					Quantity:         (deliverynoteProduct.Quantity * setProduct.Quantity),
					Unit:             deliverynoteProduct.Unit,
					CreatedAt:        deliverynote.CreatedAt,
					UpdatedAt:        deliverynote.UpdatedAt,
				}

				history.ID = primitive.NewObjectID()
				_, err := collection.InsertOne(ctx, &history)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (store *Store) IsDeliveryNoteHistoryExistsByDeliveryNoteID(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_delivery_note_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"delivery_note_id": ID,
	})

	return (count > 0), err
}

func (deliverynote *DeliveryNote) ClearProductsDeliveryNoteHistory() error {
	//log.Printf("Clearing Sales history of order id:%s", order.Code)
	collection := db.GetDB("store_" + deliverynote.StoreID.Hex()).Collection("product_delivery_note_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"delivery_note_id": deliverynote.ID})
	if err != nil {
		return err
	}
	return nil
}

func (store *Store) ProcessDeliveryNoteHistory() error {
	log.Print("Processing delivery note history")
	totalCount, err := store.GetTotalCount(bson.M{}, "product_delivery_note_history")
	if err != nil {
		return err
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_delivery_note_history")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return errors.New("Error fetching product delivery note history:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	bar := progressbar.Default(totalCount)
	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		model := ProductDeliveryNoteHistory{}
		err = cur.Decode(&model)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		deliveryNote, err := store.FindDeliveryNoteByID(model.DeliveryNoteID, map[string]interface{}{})
		if err != nil {
			return errors.New("Error finding delivery note:" + err.Error())
		}
		model.Date = deliveryNote.Date
		err = model.Update()
		if err != nil {
			return errors.New("Error updating delivery note history:" + err.Error())
		}
		bar.Add(1)
	}

	log.Print("Delivery note history DONE!")
	return nil
}

func (model *ProductDeliveryNoteHistory) Update() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_delivery_note_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": model.ID},
		bson.M{"$set": model},
		updateOptions,
	)
	if err != nil {
		return err
	}

	if updateResult.MatchedCount > 0 {
		return nil
	}

	return nil
}

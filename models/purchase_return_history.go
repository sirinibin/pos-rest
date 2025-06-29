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
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ProductPurchaseReturnHistory struct {
	ID                 primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date               *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	StoreID            *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName          string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	ProductID          primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	VendorID           *primitive.ObjectID `json:"vendor_id,omitempty" bson:"vendor_id,omitempty"`
	VendorName         string              `json:"vendor_name,omitempty" bson:"vendor_name,omitempty"`
	PurchaseReturnID   *primitive.ObjectID `json:"purchase_return_id,omitempty" bson:"purchase_return_id,omitempty"`
	PurchaseReturnCode string              `json:"purchase_return_code,omitempty" bson:"purchase_return_code,omitempty"`
	PurchaseID         *primitive.ObjectID `json:"purchase_id,omitempty" bson:"purchase_id,omitempty"`
	PurchaseCode       string              `json:"purchase_code,omitempty" bson:"purchase_code,omitempty"`
	Quantity           float64             `json:"quantity,omitempty" bson:"quantity,omitempty"`
	UnitPrice          float64             `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	UnitDiscount       float64             `bson:"unit_discount" json:"unit_discount"`
	Discount           float64             `bson:"discount" json:"discount"`
	DiscountPercent    float64             `bson:"discount_percent" json:"discount_percent"`
	Price              float64             `bson:"price,omitempty" json:"price,omitempty"`
	NetPrice           float64             `bson:"net_price,omitempty" json:"net_price,omitempty"`
	VatPercent         float64             `bson:"vat_percent,omitempty" json:"vat_percent,omitempty"`
	VatPrice           float64             `bson:"vat_price,omitempty" json:"vat_price,omitempty"`
	Unit               string              `bson:"unit,omitempty" json:"unit,omitempty"`
	Store              *Store              `json:"store,omitempty"`
	Vendor             *Vendor             `json:"vendor,omitempty"`
	CreatedAt          *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt          *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

type PurchaseReturnHistoryStats struct {
	ID                  *primitive.ObjectID `json:"id" bson:"_id"`
	TotalPurchaseReturn float64             `json:"total_purchase_return" bson:"total_purchase_return"`
	TotalVatReturn      float64             `json:"total_vat_return" bson:"total_vat_return"`
}

func (store *Store) GetPurchaseReturnHistoryStats(filter map[string]interface{}) (stats PurchaseReturnHistoryStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_purchase_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                   nil,
				"total_purchase_return": bson.M{"$sum": "$net_price"},
				"total_vat_return":      bson.M{"$sum": "$vat_price"},
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
		stats.TotalPurchaseReturn = RoundFloat(stats.TotalPurchaseReturn, 2)
		stats.TotalVatReturn = RoundFloat(stats.TotalVatReturn, 2)
	}

	return stats, nil
}

func (store *Store) SearchPurchaseReturnHistory(w http.ResponseWriter, r *http.Request) (models []ProductPurchaseReturnHistory, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SortBy = map[string]interface{}{
		"created_at": -1,
	}

	criterias.SearchBy = make(map[string]interface{})

	timeZoneOffset := 0.0
	keys, ok := r.URL.Query()["search[timezone_offset]"]
	if ok && len(keys[0]) >= 1 {
		if s, err := strconv.ParseFloat(keys[0], 64); err == nil {
			timeZoneOffset = s
		}

	}

	keys, ok = r.URL.Query()["search[date_str]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
		}

		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}

		endDate := startDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
		criterias.SearchBy["date"] = bson.M{"$gte": startDate, "$lte": endDate}
	}

	var startDate time.Time
	var endDate time.Time

	keys, ok = r.URL.Query()["search[from_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
		}

		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}
	}

	keys, ok = r.URL.Query()["search[to_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		endDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
		}

		if timeZoneOffset != 0 {
			endDate = ConvertTimeZoneToUTC(timeZoneOffset, endDate)
		}

		endDate = endDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
	}

	if !startDate.IsZero() && !endDate.IsZero() {
		criterias.SearchBy["date"] = bson.M{"$gte": startDate, "$lte": endDate}
	} else if !startDate.IsZero() {
		criterias.SearchBy["date"] = bson.M{"$gte": startDate}
	} else if !endDate.IsZero() {
		criterias.SearchBy["date"] = bson.M{"$lte": endDate}
	}

	var createdAtStartDate time.Time
	var createdAtEndDate time.Time

	keys, ok = r.URL.Query()["search[created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
		}

		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}

		endDate := startDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
		criterias.SearchBy["created_at"] = bson.M{"$gte": startDate, "$lte": endDate}
	}

	keys, ok = r.URL.Query()["search[created_at_from]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtStartDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
		}

		if timeZoneOffset != 0 {
			createdAtStartDate = ConvertTimeZoneToUTC(timeZoneOffset, createdAtStartDate)
		}
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
		}

		if timeZoneOffset != 0 {
			createdAtEndDate = ConvertTimeZoneToUTC(timeZoneOffset, createdAtEndDate)
		}

		createdAtEndDate = createdAtEndDate.Add(time.Hour * time.Duration(24))
		createdAtEndDate = createdAtEndDate.Add(-time.Second * time.Duration(1))
	}

	if !createdAtStartDate.IsZero() && !createdAtEndDate.IsZero() {
		criterias.SearchBy["created_at"] = bson.M{"$gte": createdAtStartDate, "$lte": createdAtEndDate}
	} else if !createdAtStartDate.IsZero() {
		criterias.SearchBy["created_at"] = bson.M{"$gte": createdAtStartDate}
	} else if !createdAtEndDate.IsZero() {
		criterias.SearchBy["created_at"] = bson.M{"$lte": createdAtEndDate}
	}

	keys, ok = r.URL.Query()["search[store_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["store_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[vendor_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["vendor_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[vendor_id]"]
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
			criterias.SearchBy["vendor_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[price]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["price"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["price"] = float64(value)
		}

	}

	keys, ok = r.URL.Query()["search[unit_price]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["unit_price"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["unit_price"] = float64(value)
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

	keys, ok = r.URL.Query()["search[vendor_id]"]
	if ok && len(keys[0]) >= 1 {

		vendorIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range vendorIds {
			vendorID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return models, criterias, err
			}
			objecIds = append(objecIds, vendorID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["vendor_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[product_id]"]
	if ok && len(keys[0]) >= 1 {
		productID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["product_id"] = productID
	}

	keys, ok = r.URL.Query()["search[purchase_id]"]
	if ok && len(keys[0]) >= 1 {
		orderID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["purchase_id"] = orderID
	}

	keys, ok = r.URL.Query()["search[purchase_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["purchase_code"] = keys[0]
	}

	keys, ok = r.URL.Query()["search[purchase_return_id]"]
	if ok && len(keys[0]) >= 1 {
		orderID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["purchase_return_id"] = orderID
	}

	keys, ok = r.URL.Query()["search[purchase_return_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["purchase_return_code"] = keys[0]
	}

	keys, ok = r.URL.Query()["limit"]
	if ok && len(keys[0]) >= 1 {
		criterias.Size, _ = strconv.Atoi(keys[0])
	}
	keys, ok = r.URL.Query()["page"]
	if ok && len(keys[0]) >= 1 {
		criterias.Page, _ = strconv.Atoi(keys[0])
	}
	keys, ok = r.URL.Query()["sort"]
	if ok && len(keys[0]) >= 1 {
		criterias.SortBy = GetSortByFields(keys[0])
	}

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_purchase_return_history")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	storeSelectFields := map[string]interface{}{}
	vendorSelectFields := map[string]interface{}{}

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		//Relational Select Fields
		if _, ok := criterias.Select["store.id"]; ok {
			storeSelectFields = ParseRelationalSelectString(keys[0], "store")
		}

		if _, ok := criterias.Select["vendor.id"]; ok {
			vendorSelectFields = ParseRelationalSelectString(keys[0], "vendor")
		}
	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return models, criterias, errors.New("Error fetching product purchase return history:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		model := ProductPurchaseReturnHistory{}
		err = cur.Decode(&model)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["store.id"]; ok {
			model.Store, _ = FindStoreByID(model.StoreID, storeSelectFields)
		}
		if _, ok := criterias.Select["vendor.id"]; ok {
			model.Vendor, _ = store.FindVendorByID(model.VendorID, vendorSelectFields)
		}

		models = append(models, model)
	} //end for loop

	return models, criterias, nil
}

func (purchaseReturn *PurchaseReturn) CreateProductsPurchaseReturnHistory() error {
	store, err := FindStoreByID(purchaseReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	exists, err := store.IsPurchaseReturnHistoryExistsByPurchaseReturnID(&purchaseReturn.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.GetDB("store_" + purchaseReturn.StoreID.Hex()).Collection("product_purchase_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, purchaseReturnProduct := range purchaseReturn.Products {
		if !purchaseReturnProduct.Selected {
			continue
		}

		history := ProductPurchaseReturnHistory{
			Date:               purchaseReturn.Date,
			StoreID:            purchaseReturn.StoreID,
			StoreName:          purchaseReturn.StoreName,
			ProductID:          purchaseReturnProduct.ProductID,
			VendorID:           purchaseReturn.VendorID,
			VendorName:         purchaseReturn.VendorName,
			PurchaseReturnID:   &purchaseReturn.ID,
			PurchaseReturnCode: purchaseReturn.Code,
			PurchaseID:         purchaseReturn.PurchaseID,
			PurchaseCode:       purchaseReturn.PurchaseCode,
			Quantity:           purchaseReturnProduct.Quantity,
			UnitPrice:          purchaseReturnProduct.PurchaseReturnUnitPrice,
			Unit:               purchaseReturnProduct.Unit,
			Discount:           purchaseReturnProduct.UnitDiscount,
			DiscountPercent:    purchaseReturnProduct.UnitDiscountPercent,
			CreatedAt:          purchaseReturn.CreatedAt,
			UpdatedAt:          purchaseReturn.UpdatedAt,
		}

		history.UnitPrice = RoundFloat(purchaseReturnProduct.PurchaseReturnUnitPrice, 2)
		history.Price = RoundFloat(((purchaseReturnProduct.PurchaseReturnUnitPrice - purchaseReturnProduct.UnitDiscount) * purchaseReturnProduct.Quantity), 2)
		history.VatPercent = RoundFloat(*purchaseReturn.VatPercent, 2)
		history.VatPrice = RoundFloat((history.Price * (history.VatPercent / 100)), 2)
		history.NetPrice = RoundFloat((history.Price + history.VatPrice), 2)

		history.ID = primitive.NewObjectID()

		_, err := collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}

		product, err := store.FindProductByID(&purchaseReturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}

				history := ProductPurchaseReturnHistory{
					Date:               purchaseReturn.Date,
					StoreID:            purchaseReturn.StoreID,
					StoreName:          purchaseReturn.StoreName,
					ProductID:          *setProduct.ProductID,
					VendorID:           purchaseReturn.VendorID,
					VendorName:         purchaseReturn.VendorName,
					PurchaseReturnID:   &purchaseReturn.ID,
					PurchaseReturnCode: purchaseReturn.Code,
					PurchaseID:         purchaseReturn.PurchaseID,
					PurchaseCode:       purchaseReturn.PurchaseCode,
					Quantity:           RoundTo8Decimals(purchaseReturnProduct.Quantity * *setProduct.Quantity),
					UnitPrice:          RoundTo4Decimals(purchaseReturnProduct.PurchaseReturnUnitPrice * (*setProduct.PurchasePricePercent / 100)),
					Unit:               setProductObj.Unit,
					UnitDiscount:       RoundTo8Decimals(purchaseReturnProduct.UnitDiscount * (*setProduct.PurchasePricePercent / 100)),
					Discount:           RoundTo8Decimals((purchaseReturnProduct.UnitDiscount * (*setProduct.PurchasePricePercent / 100)) * RoundTo8Decimals(purchaseReturnProduct.Quantity**setProduct.Quantity)),
					DiscountPercent:    purchaseReturnProduct.UnitDiscountPercent,
					CreatedAt:          purchaseReturn.CreatedAt,
					UpdatedAt:          purchaseReturn.UpdatedAt,
				}

				history.UnitPrice = RoundTo4Decimals(purchaseReturnProduct.PurchaseReturnUnitPrice * (*setProduct.PurchasePricePercent / 100))
				history.Price = RoundTo2Decimals((history.UnitPrice - history.UnitDiscount) * history.Quantity)

				history.VatPercent = RoundTo2Decimals(*purchaseReturn.VatPercent)
				history.VatPrice = RoundTo2Decimals(history.Price * (history.VatPercent / 100))
				history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))
				history.ID = primitive.NewObjectID()

				_, err = collection.InsertOne(ctx, &history)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (store *Store) IsPurchaseReturnHistoryExistsByPurchaseReturnID(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_purchase_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"purchase_return_id": ID,
	})

	return (count > 0), err
}

func (purchaseReturn *PurchaseReturn) ClearProductsPurchaseReturnHistory() error {
	//log.Printf("Clearing product purchase return history of purchase id:%s", purchase.Code)
	collection := db.GetDB("store_" + purchaseReturn.StoreID.Hex()).Collection("product_purchase_return_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"purchase_return_id": purchaseReturn.ID})
	if err != nil {
		return errors.New("error deleting product purchase return history: " + err.Error())
	}
	return nil
}

func (store *Store) ProcessPurchaseReturnHistory() error {
	log.Print("Processing purchase return history")
	totalCount, err := store.GetTotalCount(bson.M{}, "product_purchase_return_history")
	if err != nil {
		return err
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_purchase_return_history")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return errors.New("Error fetching purchase return history:" + err.Error())
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
		model := ProductPurchaseReturnHistory{}
		err = cur.Decode(&model)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		purchaseReturn, err := store.FindPurchaseReturnByID(model.PurchaseReturnID, map[string]interface{}{})
		if err != nil {
			return errors.New("Error finding order:" + err.Error())
		}
		model.Date = purchaseReturn.Date
		err = model.Update()
		if err != nil {
			return errors.New("Error updating purchase return history:" + err.Error())
		}
		bar.Add(1)
	}

	log.Print("Purchase return history DONE!")
	return nil
}

func (model *ProductPurchaseReturnHistory) Update() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_purchase_return_history")
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

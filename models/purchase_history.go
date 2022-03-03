package models

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

type ProductPurchaseHistory struct {
	ID              primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID         *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName       string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	ProductID       primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	VendorID        *primitive.ObjectID `json:"vendor_id,omitempty" bson:"vendor_id,omitempty"`
	VendorName      string              `json:"vendor_name,omitempty" bson:"vendor_name,omitempty"`
	PurchaseID      *primitive.ObjectID `json:"purchase_id,omitempty" bson:"purchase_id,omitempty"`
	PurchaseCode    string              `json:"purchase_code,omitempty" bson:"purchase_code,omitempty"`
	Quantity        float64             `json:"quantity,omitempty" bson:"quantity,omitempty"`
	UnitPrice       float64             `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	Price           float64             `bson:"price,omitempty" json:"price,omitempty"`
	NetPrice        float64             `bson:"net_price,omitempty" json:"net_price,omitempty"`
	RetailProfit    float64             `bson:"retail_profit" json:"retail_profit"`
	WholesaleProfit float64             `bson:"wholesale_profit" json:"wholesale_profit"`
	RetailLoss      float64             `bson:"retail_loss" json:"retail_loss"`
	WholesaleLoss   float64             `bson:"wholesale_loss" json:"wholesale_loss"`
	VatPercent      float64             `bson:"vat_percent,omitempty" json:"vat_percent,omitempty"`
	VatPrice        float64             `bson:"vat_price,omitempty" json:"vat_price,omitempty"`
	Unit            string              `bson:"unit,omitempty" json:"unit,omitempty"`
	Store           *Store              `json:"store,omitempty"`
	Vendor          *Vendor             `json:"vendor,omitempty"`
	CreatedAt       *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt       *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

type PurchaseHistoryStats struct {
	ID                   *primitive.ObjectID `json:"id" bson:"_id"`
	TotalPurchase        float64             `json:"total_purchase" bson:"total_purchase"`
	TotalRetailProfit    float64             `json:"total_retail_profit" bson:"total_retail_profit"`
	TotalWholesaleProfit float64             `json:"total_wholesale_profit" bson:"total_wholesale_profit"`
	TotalLoss            float64             `json:"total_loss" bson:"total_loss"`
	TotalVat             float64             `json:"total_vat" bson:"total_vat"`
}

func GetPurchaseHistoryStats(filter map[string]interface{}) (stats PurchaseHistoryStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product_purchase_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                    nil,
				"total_purchase":         bson.M{"$sum": "$net_price"},
				"total_retail_profit":    bson.M{"$sum": "$retail_profit"},
				"total_wholesale_profit": bson.M{"$sum": "$wholesale_profit"},
				"total_loss":             bson.M{"$sum": "$loss"},
				"total_vat":              bson.M{"$sum": "$vat_price"},
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
		stats.TotalPurchase = math.Round(stats.TotalPurchase*100) / 100
		stats.TotalRetailProfit = math.Round(stats.TotalRetailProfit*100) / 100
		stats.TotalWholesaleProfit = math.Round(stats.TotalWholesaleProfit*100) / 100
		stats.TotalLoss = math.Round(stats.TotalLoss*100) / 100
		stats.TotalVat = math.Round(stats.TotalVat*100) / 100
	}

	return stats, nil
}

func SearchPurchaseHistory(w http.ResponseWriter, r *http.Request) (models []ProductPurchaseHistory, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SortBy = map[string]interface{}{
		"created_at": -1,
	}

	criterias.SearchBy = make(map[string]interface{})

	var createdAtStartDate time.Time
	var createdAtEndDate time.Time

	keys, ok := r.URL.Query()["search[created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
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
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
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

	keys, ok = r.URL.Query()["search[customer_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["customer_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[price]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 32)
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

		value, err := strconv.ParseFloat(keys[0], 32)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["unit_price"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["unit_price"] = float64(value)
		}

	}

	keys, ok = r.URL.Query()["search[quantity]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 32)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["quantity"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["quantity"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 32)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["profit"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["profit"] = float64(value)
		}

	}

	keys, ok = r.URL.Query()["search[loss]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 32)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["loss"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["loss"] = float64(value)
		}

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

	collection := db.Client().Database(db.GetPosDB()).Collection("product_purchase_history")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)

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
		return models, criterias, errors.New("Error fetching product sales history:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		model := ProductPurchaseHistory{}
		err = cur.Decode(&model)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["store.id"]; ok {
			model.Store, _ = FindStoreByID(model.StoreID, storeSelectFields)
		}
		if _, ok := criterias.Select["vendor.id"]; ok {
			model.Vendor, _ = FindVendorByID(model.VendorID, vendorSelectFields)
		}

		models = append(models, model)
	} //end for loop

	return models, criterias, nil
}

func (purchase *Purchase) AddProductsPurchaseHistory() error {

	exists, err := IsPurchaseHistoryExistsByPurchaseID(&purchase.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.Client().Database(db.GetPosDB()).Collection("product_purchase_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, purchaseProduct := range purchase.Products {

		history := ProductPurchaseHistory{
			StoreID:      purchase.StoreID,
			StoreName:    purchase.StoreName,
			ProductID:    purchaseProduct.ProductID,
			VendorID:     purchase.VendorID,
			VendorName:   purchase.VendorName,
			PurchaseID:   &purchase.ID,
			PurchaseCode: purchase.Code,
			Quantity:     purchaseProduct.Quantity,
			UnitPrice:    purchaseProduct.PurchaseUnitPrice,
			Unit:         purchaseProduct.Unit,
			CreatedAt:    purchase.CreatedAt,
			UpdatedAt:    purchase.UpdatedAt,
		}

		history.UnitPrice = math.Round(purchaseProduct.PurchaseUnitPrice*100) / 100
		history.Price = math.Round((purchaseProduct.PurchaseUnitPrice*purchaseProduct.Quantity)*100) / 100

		history.RetailProfit = math.Round(purchase.ExpectedRetailProfit*100) / 100
		history.WholesaleProfit = math.Round(purchase.ExpectedWholesaleProfit*100) / 100
		history.RetailLoss = math.Round(purchase.ExpectedRetailLoss*100) / 100
		history.WholesaleLoss = math.Round(purchase.ExpectedWholesaleLoss*100) / 100

		history.VatPercent = math.Round(*purchase.VatPercent*100) / 100
		history.VatPrice = math.Round((history.Price*(history.VatPercent/100))*100) / 100
		history.NetPrice = math.Round((history.Price+history.VatPrice)*100) / 100

		history.ID = primitive.NewObjectID()

		_, err := collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}
	}
	return nil
}

func IsPurchaseHistoryExistsByPurchaseID(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product_purchase_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"purchase_id": ID,
	})

	return (count > 0), err
}

/*
func FindPurchaseHistoryByPurchaseID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (purchaseHistory *ProductPurchaseHistory, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("product_purchase_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"order_id": ID}, findOneOptions).
		Decode(&purchaseHistory)
	if err != nil {
		return nil, err
	}

	return purchaseHistory, err
}
*/

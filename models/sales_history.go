package models

import (
	"context"
	"errors"
	"log"
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

type ProductSalesHistory struct {
	ID                primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID           *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName         string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	ProductID         primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	CustomerID        *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	CustomerName      string              `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	OrderID           *primitive.ObjectID `json:"order_id,omitempty" bson:"order_id,omitempty"`
	OrderCode         string              `json:"order_code,omitempty" bson:"order_code,omitempty"`
	Quantity          float64             `json:"quantity,omitempty" bson:"quantity,omitempty"`
	PurchaseUnitPrice float64             `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	UnitPrice         float64             `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	Unit              string              `bson:"unit,omitempty" json:"unit,omitempty"`
	Price             float64             `bson:"price,omitempty" json:"price,omitempty"`
	NetPrice          float64             `bson:"net_price,omitempty" json:"net_price,omitempty"`
	Profit            float64             `bson:"profit" json:"profit"`
	Loss              float64             `bson:"loss" json:"loss"`
	VatPercent        float64             `bson:"vat_percent,omitempty" json:"vat_percent,omitempty"`
	VatPrice          float64             `bson:"vat_price,omitempty" json:"vat_price,omitempty"`
	Store             *Store              `json:"store,omitempty"`
	Customer          *Customer           `json:"customer,omitempty"`
	CreatedAt         *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt         *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

type SalesHistoryStats struct {
	ID          *primitive.ObjectID `json:"id" bson:"_id"`
	TotalSales  float64             `json:"total_sales" bson:"total_sales"`
	TotalProfit float64             `json:"total_profit" bson:"total_profit"`
	TotalLoss   float64             `json:"total_loss" bson:"total_loss"`
	TotalVat    float64             `json:"total_vat" bson:"total_vat"`
}

func GetSalesHistoryStats(filter map[string]interface{}) (stats SalesHistoryStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product_sales_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":          nil,
				"total_sales":  bson.M{"$sum": "$net_price"},
				"total_profit": bson.M{"$sum": "$profit"},
				"total_loss":   bson.M{"$sum": "$loss"},
				"total_vat":    bson.M{"$sum": "$vat_price"},
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
		stats.TotalSales = math.Round(stats.TotalSales*100) / 100
		stats.TotalProfit = math.Round(stats.TotalProfit*100) / 100
		stats.TotalLoss = math.Round(stats.TotalLoss*100) / 100
		stats.TotalVat = math.Round(stats.TotalVat*100) / 100
	}

	return stats, nil
}

func SearchSalesHistory(w http.ResponseWriter, r *http.Request) (models []ProductSalesHistory, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[order_id]"]
	if ok && len(keys[0]) >= 1 {
		orderID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["order_id"] = orderID
	}

	keys, ok = r.URL.Query()["search[order_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["order_code"] = keys[0]
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

	collection := db.Client().Database(db.GetPosDB()).Collection("product_sales_history")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)

	storeSelectFields := map[string]interface{}{}
	customerSelectFields := map[string]interface{}{}

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		//Relational Select Fields
		if _, ok := criterias.Select["store.id"]; ok {
			storeSelectFields = ParseRelationalSelectString(keys[0], "store")
		}

		if _, ok := criterias.Select["customer.id"]; ok {
			customerSelectFields = ParseRelationalSelectString(keys[0], "customer")
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
		model := ProductSalesHistory{}
		err = cur.Decode(&model)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["store.id"]; ok {
			model.Store, _ = FindStoreByID(model.StoreID, storeSelectFields)
		}
		if _, ok := criterias.Select["customer.id"]; ok {
			model.Customer, _ = FindCustomerByID(model.CustomerID, customerSelectFields)
		}

		models = append(models, model)
	} //end for loop

	return models, criterias, nil
}

func (order *Order) AddProductsSalesHistory() error {
	exists, err := IsSalesHistoryExistsByOrderID(&order.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.Client().Database(db.GetPosDB()).Collection("product_sales_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, orderProduct := range order.Products {

		history := ProductSalesHistory{
			StoreID:           order.StoreID,
			StoreName:         order.StoreName,
			ProductID:         orderProduct.ProductID,
			CustomerID:        order.CustomerID,
			CustomerName:      order.CustomerName,
			OrderID:           &order.ID,
			OrderCode:         order.Code,
			Quantity:          orderProduct.Quantity,
			PurchaseUnitPrice: orderProduct.PurchaseUnitPrice,
			Unit:              orderProduct.Unit,
			CreatedAt:         order.CreatedAt,
			UpdatedAt:         order.UpdatedAt,
		}

		history.UnitPrice = math.Round(orderProduct.UnitPrice*100) / 100
		history.Price = math.Round((orderProduct.UnitPrice*orderProduct.Quantity)*100) / 100
		history.Profit = math.Round(orderProduct.Profit*100) / 100
		history.Loss = math.Round(orderProduct.Loss*100) / 100

		history.VatPercent = math.Round(*order.VatPercent*100) / 100
		history.VatPrice = math.Round((history.Price*(history.VatPercent/100))*100) / 100
		history.NetPrice = math.Round((history.Price+history.VatPrice)*100) / 100

		history.ID = primitive.NewObjectID()

		_, err := collection.InsertOne(ctx, &history)
		if err != nil {
			log.Print(err)
			return err
		}
	}

	return nil
}

func IsSalesHistoryExistsByOrderID(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product_sales_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"order_id": ID,
	})

	return (count > 0), err
}

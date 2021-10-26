package models

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

type OrderProduct struct {
	ProductID primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Quantity  int                `json:"quantity,omitempty" bson:"quantity,omitempty"`
	Price     float32            `bson:"price,omitempty" json:"price,omitempty"`
}

//Order : Order structure
type Order struct {
	ID                     primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Code                   string             `bson:"code,omitempty" json:"code,omitempty"`
	BusinessID             primitive.ObjectID `json:"business_id,omitempty" bson:"business_id,omitempty"`
	CustomerID             primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	Products               []OrderProduct     `bson:"products,omitempty" json:"products,omitempty"`
	DeliveredBy            primitive.ObjectID `json:"delivered_by,omitempty" bson:"delivered_by,omitempty"`
	DeliveredBySignatureID primitive.ObjectID `json:"delivered_by_signature_id,omitempty" bson:"delivered_by_signature_id,omitempty"`
	VatPercent             *float32           `bson:"vat_percent,omitempty" json:"vat_percent,omitempty"`
	Discount               float32            `bson:"discount,omitempty" json:"discount,omitempty"`
	Deleted                bool               `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy              primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedAt              time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt              time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt              time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy              primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy              primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
}

func SearchOrder(w http.ResponseWriter, r *http.Request) (orders []Order, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})

	keys, ok := r.URL.Query()["search[business_id]"]
	if ok && len(keys[0]) >= 1 {
		businessID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return orders, criterias, err
		}
		criterias.SearchBy["business_id"] = businessID
	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {
		customerID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return orders, criterias, err
		}
		criterias.SearchBy["customer_id"] = customerID
	}

	keys, ok = r.URL.Query()["search[delivered_by]"]
	if ok && len(keys[0]) >= 1 {
		deliveredByID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return orders, criterias, err
		}
		criterias.SearchBy["delivered_by"] = deliveredByID
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

	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)

	//Fetch all device documents with (garbage:true AND (gc_processed:false if exist OR gc_processed not exist ))
	/* Note: Actual Record fetching will not happen here
	as it is using mongodb cursor and record fetching will
	start with we call cur.Next()
	*/
	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return orders, criterias, errors.New("Error fetching orders:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return orders, criterias, errors.New("Cursor error:" + err.Error())
		}
		order := Order{}
		err = cur.Decode(&order)
		if err != nil {
			return orders, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		orders = append(orders, order)
	} //end for loop

	return orders, criterias, nil
}

func (order *Order) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if order.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsOrderExists(order.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Order:" + order.ID.Hex()
		}

	}

	if order.BusinessID.IsZero() {
		errs["business_id"] = "Business is required"
	} else {
		exists, err := IsBusinessExists(order.BusinessID)
		if err != nil {
			errs["business_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["business_id"] = "Invalid business:" + order.BusinessID.Hex()
			return errs
		}
	}

	if order.CustomerID.IsZero() {
		errs["customer_id"] = "Customer is required"
	} else {
		exists, err := IsCustomerExists(order.CustomerID)
		if err != nil {
			errs["customer_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["customer_id"] = "Invalid Customer:" + order.CustomerID.Hex()
		}
	}

	if order.DeliveredBy.IsZero() {
		errs["delivered_by"] = "Delivered By is required"
	} else {
		exists, err := IsUserExists(order.DeliveredBy)
		if err != nil {
			errs["delivered_by"] = err.Error()
			return errs
		}

		if !exists {
			errs["delivered_by"] = "Invalid Delivered By:" + order.DeliveredBy.Hex()
		}
	}

	if len(order.Products) == 0 {
		errs["products"] = "Atleast 1 product is required for order"
	}

	if !order.DeliveredBySignatureID.IsZero() {
		exists, err := IsSignatureExists(order.DeliveredBySignatureID)
		if err != nil {
			errs["delivered_by_signature_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["delivered_by_signature_id"] = "Invalid Delivered By Signature:" + order.DeliveredBySignatureID.Hex()
		}
	}

	for _, product := range order.Products {
		if product.ProductID.IsZero() {
			errs["product_id"] = "Product is required for order"
		} else {
			exists, err := IsProductExists(product.ProductID)
			if err != nil {
				errs["product_id"] = err.Error()
				return errs
			}

			if !exists {
				errs["product_id"] = "Invalid product_id:" + product.ProductID.Hex() + " in products"
			}
		}

		if product.Quantity == 0 {
			errs["quantity"] = "Quantity is required"
		}

	}

	if order.VatPercent == nil {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func (order *Order) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	order.ID = primitive.NewObjectID()
	if len(order.Code) == 0 {
		for true {
			order.Code = strings.ToUpper(GenerateOrderCode(12))
			exists, err := order.IsCodeExists()
			if err != nil {
				return err
			}
			if !exists {
				break
			}
		}
	}
	_, err := collection.InsertOne(ctx, &order)
	if err != nil {
		return err
	}
	return nil
}

func (order *Order) IsCodeExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if order.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": order.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": order.Code,
			"_id":  bson.M{"$ne": order.ID},
		})
	}

	return (count == 1), err
}

func GenerateOrderCode(n int) string {
	letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func (order *Order) Update() (*Order, error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()
	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": order.ID},
		bson.M{"$set": order},
		updateOptions,
	)
	if err != nil {
		return nil, err
	}

	if updateResult.MatchedCount > 0 {
		return order, nil
	}
	return nil, nil
}

func (order *Order) DeleteOrder(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	order.Deleted = true
	order.DeletedBy = userID
	order.DeletedAt = time.Now().Local()

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": order.ID},
		bson.M{"$set": order},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func FindOrderByID(ID primitive.ObjectID) (order *Order, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}).
		Decode(&order)
	if err != nil {
		return nil, err
	}

	return order, err
}

func IsOrderExists(ID primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

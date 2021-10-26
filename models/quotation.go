package models

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

type QuotationProduct struct {
	ProductID primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Quantity  int                `json:"quantity,omitempty" bson:"quantity,omitempty"`
	Price     float32            `bson:"price,omitempty" json:"price,omitempty"`
}

//Quotation : Quotation structure
type Quotation struct {
	ID                     primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	BusinessID             primitive.ObjectID `json:"business_id,omitempty" bson:"business_id,omitempty"`
	CustomerID             primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	Products               []QuotationProduct `bson:"products,omitempty" json:"products,omitempty"`
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

func SearchQuotation(w http.ResponseWriter, r *http.Request) (quotations []Quotation, criterias SearchCriterias, err error) {

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
			return quotations, criterias, err
		}
		criterias.SearchBy["business_id"] = businessID
	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {
		customerID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return quotations, criterias, err
		}
		criterias.SearchBy["customer_id"] = customerID
	}

	keys, ok = r.URL.Query()["search[delivered_by]"]
	if ok && len(keys[0]) >= 1 {
		deliveredByID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return quotations, criterias, err
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

	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
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
		return quotations, criterias, errors.New("Error fetching quotations:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return quotations, criterias, errors.New("Cursor error:" + err.Error())
		}
		quotation := Quotation{}
		err = cur.Decode(&quotation)
		if err != nil {
			return quotations, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		quotations = append(quotations, quotation)
	} //end for loop

	return quotations, criterias, nil
}

func (quotation *Quotation) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if quotation.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsQuotationExists(quotation.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Quotation:" + quotation.ID.Hex()
		}

	}

	if quotation.BusinessID.IsZero() {
		errs["business_id"] = "Business is required"
	} else {
		exists, err := IsBusinessExists(quotation.BusinessID)
		if err != nil {
			errs["business_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["business_id"] = "Invalid business:" + quotation.BusinessID.Hex()
			return errs
		}
	}

	if quotation.CustomerID.IsZero() {
		errs["customer_id"] = "Customer is required"
	} else {
		exists, err := IsCustomerExists(quotation.CustomerID)
		if err != nil {
			errs["customer_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["customer_id"] = "Invalid Customer:" + quotation.CustomerID.Hex()
		}
	}

	if quotation.DeliveredBy.IsZero() {
		errs["delivered_by"] = "Delivered By is required"
	} else {
		exists, err := IsUserExists(quotation.DeliveredBy)
		if err != nil {
			errs["delivered_by"] = err.Error()
			return errs
		}

		if !exists {
			errs["delivered_by"] = "Invalid Delivered By:" + quotation.DeliveredBy.Hex()
		}
	}

	if len(quotation.Products) == 0 {
		errs["products"] = "Atleast 1 product is required for quotation"
	}

	if !quotation.DeliveredBySignatureID.IsZero() {
		exists, err := IsSignatureExists(quotation.DeliveredBySignatureID)
		if err != nil {
			errs["delivered_by_signature_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["delivered_by_signature_id"] = "Invalid Delivered By Signature:" + quotation.DeliveredBySignatureID.Hex()
		}
	}

	for _, product := range quotation.Products {
		if product.ProductID.IsZero() {
			errs["product_id"] = "Product is required for quotation"
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

	if quotation.VatPercent == nil {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func (quotation *Quotation) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	quotation.ID = primitive.NewObjectID()
	_, err := collection.InsertOne(ctx, &quotation)
	if err != nil {
		return err
	}
	return nil
}

func (quotation *Quotation) Update() (*Quotation, error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()
	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": quotation.ID},
		bson.M{"$set": quotation},
		updateOptions,
	)
	if err != nil {
		return nil, err
	}

	if updateResult.MatchedCount > 0 {
		return quotation, nil
	}
	return nil, nil
}

func (quotation *Quotation) DeleteQuotation(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	quotation.Deleted = true
	quotation.DeletedBy = userID
	quotation.DeletedAt = time.Now().Local()

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": quotation.ID},
		bson.M{"$set": quotation},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func FindQuotationByID(ID primitive.ObjectID) (quotation *Quotation, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}).
		Decode(&quotation)
	if err != nil {
		return nil, err
	}

	return quotation, err
}

func IsQuotationExists(ID primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

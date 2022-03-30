package models

import (
	"context"
	"errors"
	"fmt"
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

//SalesReturnPayment : SalesReturnPayment structure
type SalesReturnPayment struct {
	ID              primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	SalesReturnID   *primitive.ObjectID `json:"sales_return_id" bson:"sales_return_id"`
	SalesReturnCode string              `json:"sales_return_code" bson:"sales_return_code"`
	OrderID         *primitive.ObjectID `json:"order_id" bson:"order_id"`
	OrderCode       string              `json:"order_code" bson:"order_code"`
	Amount          float64             `json:"amount" bson:"amount"`
	Method          string              `json:"method" bson:"method"`
	CreatedAt       *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt       *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy       *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy       *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName   string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName   string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	StoreID         *primitive.ObjectID `json:"store_id" bson:"store_id"`
	StoreName       string              `json:"store_name" bson:"store_name"`
}

/*
func (salesreturnPayment *SalesReturnPayment) AttributesValueChangeEvent(salesreturnPaymentOld *SalesReturnPayment) error {

	if salesreturnPayment.Name != salesreturnPaymentOld.Name {
		err := UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": salesreturnPayment.ID},
			bson.M{"$pull": bson.M{
				"customer_name": salesreturnPaymentOld.Name,
			}},
		)
		if err != nil {
			return nil
		}

		err = UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": salesreturnPayment.ID},
			bson.M{"$push": bson.M{
				"customer_name": salesreturnPayment.Name,
			}},
		)
		if err != nil {
			return nil
		}
	}

	return nil
}
*/

func (salesreturnPayment *SalesReturnPayment) UpdateForeignLabelFields() error {

	if salesreturnPayment.StoreID != nil && !salesreturnPayment.StoreID.IsZero() {
		store, err := FindStoreByID(salesreturnPayment.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesreturnPayment.StoreName = store.Name
	} else {
		salesreturnPayment.StoreName = ""
	}

	if salesreturnPayment.SalesReturnID != nil && !salesreturnPayment.SalesReturnID.IsZero() {
		salesReturn, err := FindSalesReturnByID(salesreturnPayment.SalesReturnID, bson.M{"id": 1, "code": 1})
		if err != nil {
			return err
		}
		salesreturnPayment.SalesReturnCode = salesReturn.Code
	} else {
		salesreturnPayment.SalesReturnCode = ""
	}

	if salesreturnPayment.OrderID != nil && !salesreturnPayment.OrderID.IsZero() {
		order, err := FindOrderByID(salesreturnPayment.OrderID, bson.M{"id": 1, "code": 1})
		if err != nil {
			return err
		}
		salesreturnPayment.OrderCode = order.Code
	} else {
		salesreturnPayment.OrderCode = ""
	}

	if salesreturnPayment.CreatedBy != nil {
		createdByUser, err := FindUserByID(salesreturnPayment.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesreturnPayment.CreatedByName = createdByUser.Name
	}

	if salesreturnPayment.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(salesreturnPayment.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesreturnPayment.UpdatedByName = updatedByUser.Name
	}

	return nil
}

func SearchSalesReturnPayment(w http.ResponseWriter, r *http.Request) (models []SalesReturnPayment, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	keys, ok := r.URL.Query()["search[created_by_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["created_by_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[method]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["method"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 32)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["amount"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["amount"] = float64(value)
		}

	}

	keys, ok = r.URL.Query()["search[updated_by_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["updated_by_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[store_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["store_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[sales_return_id]"]
	if ok && len(keys[0]) >= 1 {
		salesReturnID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["order_id"] = salesReturnID
	}

	keys, ok = r.URL.Query()["search[sales_return_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["sales_return_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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
		criterias.SearchBy["order_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return models, criterias, err
			}
			objecIds = append(objecIds, userID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["created_by"] = bson.M{"$in": objecIds}
		}
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

	collection := db.Client().Database(db.GetPosDB()).Collection("sales_return_payment")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return models, criterias, errors.New("Error fetching sales return payment :" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		salesreturnPayment := SalesReturnPayment{}
		err = cur.Decode(&salesreturnPayment)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		models = append(models, salesreturnPayment)
	} //end for loop

	return models, criterias, nil
}

func (salesreturnPayment *SalesReturnPayment) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	var oldSalesReturnPayment *SalesReturnPayment

	if scenario == "update" {
		if salesreturnPayment.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsSalesReturnPaymentExists(&salesreturnPayment.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid sales return payment :" + salesreturnPayment.ID.Hex()
		}

		oldSalesReturnPayment, err = FindSalesReturnPaymentByID(&salesreturnPayment.ID, bson.M{})
		if err != nil {
			errs["sales_return_payment"] = err.Error()
			return errs
		}
	}

	if salesreturnPayment.Amount == 0 {
		errs["amount"] = "Amount is required"
	}

	if salesreturnPayment.Amount < 0 {
		errs["amount"] = "Amount should be > 0"
	}

	salesReturnPaymentStats, err := GetSalesReturnPaymentStats(bson.M{"sales_return_id": salesreturnPayment.SalesReturnID})
	if err != nil {
		return errs
	}

	salesReturn, err := FindSalesReturnByID(salesreturnPayment.SalesReturnID, bson.M{})
	if err != nil {
		return errs
	}

	if scenario == "update" {
		if ((salesReturnPaymentStats.TotalPayment - oldSalesReturnPayment.Amount) + salesreturnPayment.Amount) > salesReturn.NetTotal {
			if (salesReturn.NetTotal - (salesReturnPaymentStats.TotalPayment - oldSalesReturnPayment.Amount)) > 0 {
				errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", salesReturnPaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to " + fmt.Sprintf("%.02f", (salesReturn.NetTotal-(salesReturnPaymentStats.TotalPayment-oldSalesReturnPayment.Amount)))
			} else {
				errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", salesReturnPaymentStats.TotalPayment) + " SAR"
			}

		}
	} else {
		if (salesReturnPaymentStats.TotalPayment + salesreturnPayment.Amount) > salesReturn.NetTotal {
			if (salesReturn.NetTotal - salesReturnPaymentStats.TotalPayment) > 0 {
				errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", salesReturnPaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to  " + fmt.Sprintf("%.02f", (salesReturn.NetTotal-salesReturnPaymentStats.TotalPayment))
			} else {
				errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", salesReturnPaymentStats.TotalPayment) + " SAR"
			}

		}
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (salesreturnPayment *SalesReturnPayment) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := salesreturnPayment.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	salesreturnPayment.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, &salesreturnPayment)
	if err != nil {
		return err
	}
	return nil
}

func (salesreturnPayment *SalesReturnPayment) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := salesreturnPayment.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": salesreturnPayment.ID},
		bson.M{"$set": salesreturnPayment},
		updateOptions,
	)
	return err
}

func FindSalesReturnPaymentByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (salesreturnPayment *SalesReturnPayment, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&salesreturnPayment)
	if err != nil {
		return nil, err
	}

	return salesreturnPayment, err
}

func IsSalesReturnPaymentExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

type SalesReturnPaymentStats struct {
	ID           *primitive.ObjectID `json:"id" bson:"_id"`
	TotalPayment float64             `json:"total_payment" bson:"total_payment"`
}

func GetSalesReturnPaymentStats(filter map[string]interface{}) (stats SalesReturnPaymentStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":           nil,
				"total_payment": bson.M{"$sum": "$amount"},
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

		stats.TotalPayment = math.Round(stats.TotalPayment*100) / 100
	}

	return stats, nil
}

package models

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// SalesPayment : SalesPayment structure
type SalesPayment struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date          *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr       string              `json:"date_str,omitempty"`
	OrderID       *primitive.ObjectID `json:"order_id" bson:"order_id"`
	OrderCode     string              `json:"order_code" bson:"order_code"`
	Amount        float64             `json:"amount" bson:"amount"`
	Method        string              `json:"method" bson:"method"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy     *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	StoreID       *primitive.ObjectID `json:"store_id" bson:"store_id"`
	StoreName     string              `json:"store_name" bson:"store_name"`
}

/*
func (salesPayment *SalesPayment) AttributesValueChangeEvent(salesPaymentOld *SalesPayment) error {

	if salesPayment.Name != salesPaymentOld.Name {
		err := UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": salesPayment.ID},
			bson.M{"$pull": bson.M{
				"customer_name": salesPaymentOld.Name,
			}},
		)
		if err != nil {
			return nil
		}

		err = UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": salesPayment.ID},
			bson.M{"$push": bson.M{
				"customer_name": salesPayment.Name,
			}},
		)
		if err != nil {
			return nil
		}
	}

	return nil
}
*/

func (salesPayment *SalesPayment) UpdateForeignLabelFields() error {

	if salesPayment.StoreID != nil && !salesPayment.StoreID.IsZero() {
		store, err := FindStoreByID(salesPayment.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesPayment.StoreName = store.Name
	} else {
		salesPayment.StoreName = ""
	}

	if salesPayment.OrderID != nil && !salesPayment.OrderID.IsZero() {
		order, err := FindOrderByID(salesPayment.OrderID, bson.M{"id": 1, "code": 1})
		if err != nil && err != mongo.ErrNoDocuments {
			return err
		}

		if order != nil {
			salesPayment.OrderCode = order.Code
		}
	} else {
		salesPayment.OrderCode = ""
	}

	if salesPayment.CreatedBy != nil {
		createdByUser, err := FindUserByID(salesPayment.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesPayment.CreatedByName = createdByUser.Name
	}

	if salesPayment.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(salesPayment.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesPayment.UpdatedByName = updatedByUser.Name
	}

	return nil
}

func SearchSalesPayment(w http.ResponseWriter, r *http.Request) (models []SalesPayment, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	timeZoneOffset := 0.0
	keys, ok := r.URL.Query()["search[timezone_offset]"]
	if ok && len(keys[0]) >= 1 {
		if s, err := strconv.ParseFloat(keys[0], 64); err == nil {
			timeZoneOffset = s
		}
	}

	keys, ok = r.URL.Query()["search[created_by_name]"]
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

		value, err := strconv.ParseFloat(keys[0], 64)
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

	collection := db.Client().Database(db.GetPosDB()).Collection("sales_payment")
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
	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return models, criterias, errors.New("Error fetching sales payment :" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		salesPayment := SalesPayment{}
		err = cur.Decode(&salesPayment)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		models = append(models, salesPayment)
	} //end for loop

	return models, criterias, nil
}

func (salesPayment *SalesPayment) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	var oldSalesPayment *SalesPayment

	if govalidator.IsNull(salesPayment.Method) {
		errs["method"] = "Payment method is required"
	}

	if govalidator.IsNull(salesPayment.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, salesPayment.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		salesPayment.Date = &date
	}

	if scenario == "update" {
		if salesPayment.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsSalesPaymentExists(&salesPayment.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid sales payment :" + salesPayment.ID.Hex()
		}

		oldSalesPayment, err = FindSalesPaymentByID(&salesPayment.ID, bson.M{})
		if err != nil {
			errs["sales_payment"] = err.Error()
			return errs
		}
	}

	if salesPayment.Amount == 0 {
		errs["amount"] = "Amount is required"
	}

	if salesPayment.Amount < 0 {
		errs["amount"] = "Amount should be > 0"
	}

	salespaymentStats, err := GetSalesPaymentStats(bson.M{"order_id": salesPayment.OrderID})
	if err != nil {
		return errs
	}

	order, err := FindOrderByID(salesPayment.OrderID, bson.M{})
	if err != nil {
		return errs
	}

	if scenario == "update" {
		if ((salespaymentStats.TotalPayment - oldSalesPayment.Amount) + salesPayment.Amount) > order.NetTotal {
			if (order.NetTotal - (salespaymentStats.TotalPayment - oldSalesPayment.Amount)) > 0 {
				errs["amount"] = "Customer already paid " + fmt.Sprintf("%.02f", salespaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to " + fmt.Sprintf("%.02f", (order.NetTotal-(salespaymentStats.TotalPayment-oldSalesPayment.Amount)))
			} else {
				errs["amount"] = "Customer already paid " + fmt.Sprintf("%.02f", salespaymentStats.TotalPayment) + " SAR"
			}

		}
	} else {
		/*
			log.Print("salespaymentStats.TotalPayment: ")
			log.Print(salespaymentStats.TotalPayment)
			log.Print("order.TotalPaymentReceived: ")
			log.Print(order.TotalPaymentReceived)
			log.Print("salesPayment.Amount:")
			log.Print(salesPayment.Amount)
			log.Print("order.NetTotal:")
			log.Print(order.NetTotal)
			log.Print("ToFixed((salespaymentStats.TotalPayment+salesPayment.Amount), 2):")
			log.Print(ToFixed((salespaymentStats.TotalPayment + salesPayment.Amount), 2))
		*/

		if ToFixed((salespaymentStats.TotalPayment+salesPayment.Amount), 2) > order.NetTotal {
			if ToFixed((order.NetTotal-salespaymentStats.TotalPayment), 2) > 0 {
				errs["amount"] = "Customer already paid " + fmt.Sprintf("%.02f", salespaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to  " + fmt.Sprintf("%.02f", (order.NetTotal-salespaymentStats.TotalPayment))
			} else {
				errs["amount"] = "Customer already paid " + fmt.Sprintf("%.02f", salespaymentStats.TotalPayment) + " SAR"
			}

		}
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (salesPayment *SalesPayment) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := salesPayment.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	salesPayment.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, &salesPayment)
	if err != nil {
		return err
	}

	return nil
}

func (salesPayment *SalesPayment) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := salesPayment.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": salesPayment.ID},
		bson.M{"$set": salesPayment},
		updateOptions,
	)

	return err
}

func FindSalesPaymentByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (salesPayment *SalesPayment, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("sales_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&salesPayment)
	if err != nil {
		return nil, err
	}

	return salesPayment, err
}

func IsSalesPaymentExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

type SalesPaymentStats struct {
	ID           *primitive.ObjectID `json:"id" bson:"_id"`
	TotalPayment float64             `json:"total_payment" bson:"total_payment"`
}

func GetSalesPaymentStats(filter map[string]interface{}) (stats SalesPaymentStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_payment")
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

func ProcessSalesPayments() error {
	log.Print("Processing sales payments")
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_payment")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return errors.New("Error fetching quotations:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	//productCount := 1
	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		model := SalesPayment{}
		err = cur.Decode(&model)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		model.Date = model.CreatedAt
		//log.Print("Date updated")
		err = model.Update()
		if err != nil {
			log.Print(err)
			return err
		}
	}

	log.Print("DONE!")
	return nil
}

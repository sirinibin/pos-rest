package models

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SalesPayment : SalesPayment structure
type SalesPayment struct {
	ID                  primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date                *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr             string              `json:"date_str,omitempty" bson:"-"`
	OrderID             *primitive.ObjectID `json:"order_id" bson:"order_id"`
	OrderCode           string              `json:"order_code" bson:"order_code"`
	Amount              float64             `json:"amount" bson:"amount"`
	Method              string              `json:"method" bson:"method"`
	BankReference       *string             `json:"bank_reference" bson:"bank_reference"`
	Description         *string             `json:"description" bson:"description"`
	ReferenceType       string              `json:"reference_type" bson:"reference_type"`
	ReferenceCode       string              `json:"reference_code" bson:"reference_code"`
	ReferenceID         *primitive.ObjectID `json:"reference_id" bson:"reference_id"`
	CreatedAt           *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt           *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy           *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy           *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName       string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName       string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	StoreID             *primitive.ObjectID `json:"store_id" bson:"store_id"`
	StoreName           string              `json:"store_name" bson:"store_name"`
	Deleted             bool                `bson:"deleted" json:"deleted"`
	DeletedBy           *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser       *User               `json:"deleted_by_user" bson:"-"`
	DeletedAt           *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	ReceivableID        *primitive.ObjectID `json:"receivable_id" bson:"receivable_id"`
	ReceivablePaymentID *primitive.ObjectID `json:"receivable_payment_id" bson:"receivable_payment_id"`
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
	store, err := FindStoreByID(salesPayment.StoreID, bson.M{})
	if err != nil {
		return err
	}

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
		order, err := store.FindOrderByID(salesPayment.OrderID, bson.M{"id": 1, "code": 1})
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

func (store *Store) SearchSalesPayment(w http.ResponseWriter, r *http.Request) (models []SalesPayment, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	keys, ok := r.URL.Query()["search[deleted]"]
	if ok && len(keys[0]) >= 1 {
		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return models, criterias, err
		}

		if value == 1 {
			criterias.SearchBy["deleted"] = bson.M{"$eq": true}
		}
	}

	timeZoneOffset := 0.0
	keys, ok = r.URL.Query()["search[timezone_offset]"]
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

	keys, ok = r.URL.Query()["search[pay_from_account]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["pay_from_account"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_payment")
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

	store, err := FindStoreByID(salesPayment.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store id"
	}

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
		exists, err := store.IsSalesPaymentExists(&salesPayment.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid sales payment :" + salesPayment.ID.Hex()
		}

		oldSalesPayment, err = store.FindSalesPaymentByID(&salesPayment.ID, bson.M{})
		if err != nil {
			errs["sales_payment"] = err.Error()
			return errs
		}
	}

	if salesPayment.Amount == 0 {
		errs["amount"] = "Amount is required"
	}

	if ToFixed(salesPayment.Amount, 2) < 0 {
		errs["amount"] = "Amount should be > 0"
	}

	order, err := store.FindOrderByID(salesPayment.OrderID, bson.M{})
	if err != nil {
		errs["order"] = "error finding order" + err.Error()
	}

	if salesPayment.Amount > RoundTo2Decimals(order.NetTotal-order.CashDiscount) {
		errs["amount"] = "Amount should not exceed: " + fmt.Sprintf("%.02f", RoundTo2Decimals(order.NetTotal-order.CashDiscount)) + " (Net Total - Cash Discount)"
		return
	}

	if scenario == "update" {
		if (salesPayment.Amount + (order.TotalPaymentReceived - oldSalesPayment.Amount)) > (order.NetTotal - order.CashDiscount) {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", RoundTo2Decimals(order.NetTotal-order.CashDiscount)) + " (Net Total - Cash Discount)"
			return
		}
	} else {
		if (salesPayment.Amount + (order.TotalPaymentReceived)) > (order.NetTotal - order.CashDiscount) {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", RoundTo2Decimals(order.NetTotal-order.CashDiscount)) + " (Net Total - Cash Discount)"
			return
		}
	}

	/*
		salespaymentStats, err := GetSalesPaymentStats(bson.M{"order_id": salesPayment.OrderID})
		if err != nil {
			return errs
		}
	*/

	/*
		maxAllowedAmount := 0.00

		if scenario == "update" {
			maxAllowedAmount = (order.NetTotal - order.CashDiscount) - (salespaymentStats.TotalPayment - *oldSalesPayment.Amount)
		} else {
			maxAllowedAmount = (order.NetTotal - order.CashDiscount) - salespaymentStats.TotalPayment
		}

		if *salesPayment.Amount > RoundFloat(maxAllowedAmount, 2) {
			errs["amount"] = "The amount should not be greater than " + fmt.Sprintf("%.02f", maxAllowedAmount)
		}
	*/

	var customer *Customer
	if order.CustomerID != nil && !order.CustomerID.IsZero() {
		customer, err = store.FindCustomerByID(order.CustomerID, bson.M{})
		if err != nil {
			errs["customer_id"] = "Invalid Customer:" + order.CustomerID.Hex()
		}
	}

	if customer != nil {
		customerAccount, err := store.FindAccountByReferenceID(customer.ID, *order.StoreID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			errs["customer_account"] = "Error finding customer account: " + err.Error()
		}

		if salesPayment.Method == "customer_account" {
			log.Print("Checking customer account Balance")
			if customerAccount != nil {
				if scenario == "update" {
					extraAmount := 0.00
					if oldSalesPayment != nil && oldSalesPayment.Amount > salesPayment.Amount {
						extraAmount = oldSalesPayment.Amount - salesPayment.Amount
					}

					if extraAmount > 0 {
						if customerAccount.Balance == 0 {
							errs["payment_method"] = "customer account balance is zero, Please add " + fmt.Sprintf("%.02f", (extraAmount)) + " to customer account to continue"
						} else if customerAccount.Type == "asset" {
							errs["payment_method"] = "customer owe us: " + fmt.Sprintf("%.02f", customerAccount.Balance) + ", Please add " + fmt.Sprintf("%.02f", (customerAccount.Balance+extraAmount)) + " to customer account to continue"
						} else if customerAccount.Type == "liability" && customerAccount.Balance < extraAmount {
							errs["payment_method"] = "customer account balance is only: " + fmt.Sprintf("%.02f", customerAccount.Balance) + ", Please add " + fmt.Sprintf("%.02f", extraAmount) + " to customer account to continue"
						}
					}

				} else {

					if customerAccount.Balance == 0 {
						errs["payment_method"] = "customer account balance is zero"
					} else if customerAccount.Type == "asset" {
						errs["payment_method"] = "customer owe us: " + fmt.Sprintf("%.02f", customerAccount.Balance)
					} else if customerAccount.Type == "liability" && customerAccount.Balance < salesPayment.Amount {
						errs["payment_method"] = "customer account balance is only: " + fmt.Sprintf("%.02f", customerAccount.Balance)
					}

				}

			} else {
				errs["payment_method"] = "customer account balance is zero"
			}
		}
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (salesPayment *SalesPayment) Insert() error {
	collection := db.GetDB("store_" + salesPayment.StoreID.Hex()).Collection("sales_payment")
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
	collection := db.GetDB("store_" + salesPayment.StoreID.Hex()).Collection("sales_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
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

func (store *Store) FindSalesPaymentByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (salesPayment *SalesPayment, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"_id":      ID,
			"store_id": store.ID,
		}, findOneOptions).
		Decode(&salesPayment)
	if err != nil {
		return nil, err
	}

	return salesPayment, err
}

func (store *Store) IsSalesPaymentExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

type SalesPaymentStats struct {
	ID           *primitive.ObjectID `json:"id" bson:"_id"`
	TotalPayment float64             `json:"total_payment" bson:"total_payment"`
}

func (store *Store) GetSalesPaymentStats(filter map[string]interface{}) (stats SalesPaymentStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id": nil,
				//"total_payment": bson.M{"$sum": "$amount"},
				"total_payment": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$ne": []interface{}{"$deleted", true}},
					"$amount",
					0,
				}}},
				/*"total_payment": bson.M{"$sum": bson.M{
					"$cond": bson.M{
						"if": bson.M{
							"$ne": []interface{}{
								"$deleted",
								true,
							},
						},
						"then": "$amount",
						"else": 0,
					},
				}},*/
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

		stats.TotalPayment = RoundFloat(stats.TotalPayment, 2)
	}

	return stats, nil
}

func (store *Store) ProcessSalesPayments() error {
	log.Print("Processing sales payments")
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_payment")

	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)
	searchBy := make(map[string]interface{})
	searchBy["deleted"] = bson.M{"$ne": true}
	cur, err := collection.Find(ctx, searchBy, findOptions)
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

		if model.Method == "bank_account" {
			model.Method = "bank_card"
		}

		//model.Date = model.CreatedAt
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

func (salesPayment *SalesPayment) DeleteSalesPayment() (err error) {
	collection := db.GetDB("store_" + salesPayment.StoreID.Hex()).Collection("sales_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": salesPayment.ID},
		bson.M{"$set": salesPayment},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

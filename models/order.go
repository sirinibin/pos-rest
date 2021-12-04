package models

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

type OrderProduct struct {
	ProductID    primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name         string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode     string             `bson:"item_code,omitempty" json:"item_code,omitempty"`
	Quantity     int                `json:"quantity,omitempty" bson:"quantity,omitempty"`
	UnitPrice    float32            `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
}

//Order : Order structure
type Order struct {
	ID                       primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date                     *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr                  string              `json:"date_str,omitempty"`
	Code                     string              `bson:"code,omitempty" json:"code,omitempty"`
	StoreID                  *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CustomerID               *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	Store                    *Store              `json:"store,omitempty"`
	Customer                 *Customer           `json:"customer,omitempty"`
	Products                 []OrderProduct      `bson:"products,omitempty" json:"products,omitempty"`
	DeliveredBy              *primitive.ObjectID `json:"delivered_by,omitempty" bson:"delivered_by,omitempty"`
	DeliveredByUser          *User               `json:"delivered_by_user,omitempty"`
	DeliveredBySignatureID   *primitive.ObjectID `json:"delivered_by_signature_id,omitempty" bson:"delivered_by_signature_id,omitempty"`
	DeliveredBySignatureName string              `json:"delivered_by_signature_name,omitempty" bson:"delivered_by_signature_name,omitempty"`
	DeliveredBySignature     *Signature          `json:"delivered_by_signature,omitempty"`
	VatPercent               *float32            `bson:"vat_percent" json:"vat_percent"`
	Discount                 float32             `bson:"discount" json:"discount"`
	Status                   string              `bson:"status,omitempty" json:"status,omitempty"`
	StockRemoved             bool                `bson:"stock_removed,omitempty" json:"stock_removed,omitempty"`
	TotalQuantity            int                 `bson:"total_quantity" json:"total_quantity"`
	VatPrice                 float32             `bson:"vat_price" json:"vat_price"`
	Total                    float32             `bson:"total" json:"total"`
	NetTotal                 float32             `bson:"net_total" json:"net_total"`
	PartiaPaymentAmount      float32             `bson:"partial_payment_amount" json:"partial_payment_amount"`
	PaymentMethod            string              `bson:"payment_method" json:"payment_method"`
	PaymentStatus            string              `bson:"payment_status" json:"payment_status"`
	Deleted                  bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy                *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser            *User               `json:"deleted_by_user,omitempty"`
	DeletedAt                *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt                *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt                *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy                *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy                *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser            *User               `json:"created_by_user,omitempty"`
	UpdatedByUser            *User               `json:"updated_by_user,omitempty"`
	DeliveredByName          string              `json:"delivered_by_name,omitempty" bson:"delivered_by_name,omitempty"`
	CustomerName             string              `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	StoreName                string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	CreatedByName            string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName            string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName            string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	ChangeLog                []ChangeLog         `json:"change_log,omitempty" bson:"change_log,omitempty"`
}

func (order *Order) SetChangeLog(
	event string,
	name, oldValue, newValue interface{},
) {
	now := time.Now()
	description := ""
	if event == "create" {
		description = "Created by " + UserObject.Name
	} else if event == "update" {
		description = "Updated by " + UserObject.Name
	} else if event == "delete" {
		description = "Deleted by " + UserObject.Name
	} else if event == "view" {
		description = "Viewed by " + UserObject.Name
	} else if event == "attribute_value_change" && name != nil {
		description = name.(string) + " changed from " + oldValue.(string) + " to " + newValue.(string) + " by " + UserObject.Name
	} else if event == "remove_stock" && name != nil {
		description = "Stock of product: " + name.(string) + " reduced from " + strconv.Itoa(oldValue.(int)) + " to " + strconv.Itoa(newValue.(int))
	} else if event == "add_stock" && name != nil {
		description = "Stock of product: " + name.(string) + " raised from " + strconv.Itoa(oldValue.(int)) + " to " + strconv.Itoa(newValue.(int))
	}

	order.ChangeLog = append(
		order.ChangeLog,
		ChangeLog{
			Event:         event,
			Description:   description,
			CreatedBy:     &UserObject.ID,
			CreatedByName: UserObject.Name,
			CreatedAt:     &now,
		},
	)
}

func (order *Order) AttributesValueChangeEvent(orderOld *Order) error {

	if order.Status != orderOld.Status {
		order.SetChangeLog(
			"attribute_value_change",
			"status",
			orderOld.Status,
			order.Status,
		)

		//if order.Status == "delivered" || order.Status == "dispatched" {

		err := orderOld.AddStock()
		if err != nil {
			return err
		}

		err = order.RemoveStock()
		if err != nil {
			return err
		}
		//}
	}

	return nil
}

func (order *Order) UpdateForeignLabelFields() error {

	if order.StoreID != nil {
		store, err := FindStoreByID(order.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		order.StoreName = store.Name
	}

	if order.CustomerID != nil {
		customer, err := FindCustomerByID(order.CustomerID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		order.CustomerName = customer.Name
	}

	if order.DeliveredBy != nil {
		deliveredByUser, err := FindUserByID(order.DeliveredBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		order.DeliveredByName = deliveredByUser.Name
	}

	if order.DeliveredBySignatureID != nil {
		deliveredBySignature, err := FindSignatureByID(order.DeliveredBySignatureID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		order.DeliveredBySignatureName = deliveredBySignature.Name
	}

	if order.CreatedBy != nil {
		createdByUser, err := FindUserByID(order.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		order.CreatedByName = createdByUser.Name
	}

	if order.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(order.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		order.UpdatedByName = updatedByUser.Name
	}

	if order.DeletedBy != nil && !order.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(order.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		order.DeletedByName = deletedByUser.Name
	}

	for i, product := range order.Products {
		productObject, err := FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1})
		if err != nil {
			return err
		}
		order.Products[i].Name = productObject.Name
		order.Products[i].NameInArabic = productObject.NameInArabic
		order.Products[i].ItemCode = productObject.ItemCode
	}

	return nil
}

func (order *Order) FindNetTotal() {
	netTotal := float32(0.0)
	for _, product := range order.Products {
		netTotal += (float32(product.Quantity) * product.UnitPrice)
	}

	if order.VatPercent != nil {
		netTotal += netTotal * (*order.VatPercent / float32(100))
	}

	netTotal -= order.Discount
	order.NetTotal = float32(math.Floor(float64(netTotal*100)) / float64(100))
}

func (order *Order) FindTotal() {
	total := float32(0.0)
	for _, product := range order.Products {
		total += (float32(product.Quantity) * product.UnitPrice)
	}

	order.Total = float32(math.Floor(float64(total*100)) / 100)
}

func (order *Order) FindTotalQuantity() {
	totalQuantity := 0
	for _, product := range order.Products {
		totalQuantity += product.Quantity
	}
	order.TotalQuantity = totalQuantity
}

func (order *Order) FindVatPrice() {
	vatPrice := ((*order.VatPercent / 100) * order.Total)
	vatPrice = float32(math.Floor(float64(vatPrice*100)) / 100)
	order.VatPrice = vatPrice
}

func SearchOrder(w http.ResponseWriter, r *http.Request) (orders []Order, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	keys, ok := r.URL.Query()["search[date_str]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return orders, criterias, err
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
			return orders, criterias, err
		}
	}

	keys, ok = r.URL.Query()["search[to_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		endDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return orders, criterias, err
		}

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
			return orders, criterias, err
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
			return orders, criterias, err
		}
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return orders, criterias, err
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

	keys, ok = r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[net_total]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 32)
		if err != nil {
			return orders, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_total"] = bson.M{operator: float32(value)}
		} else {
			criterias.SearchBy["net_total"] = float32(value)
		}

	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {

		customerIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range customerIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return orders, criterias, err
			}
			objecIds = append(objecIds, customerID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["customer_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return orders, criterias, err
			}
			objecIds = append(objecIds, userID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["created_by"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[status]"]
	if ok && len(keys[0]) >= 1 {
		statusList := strings.Split(keys[0], ",")
		if len(statusList) > 0 {
			criterias.SearchBy["status"] = bson.M{"$in": statusList}
		}
	}

	keys, ok = r.URL.Query()["search[delivered_by]"]
	if ok && len(keys[0]) >= 1 {
		deliveredByID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return orders, criterias, err
		}
		criterias.SearchBy["delivered_by"] = deliveredByID
	}

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return orders, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
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

	storeSelectFields := map[string]interface{}{}
	customerSelectFields := map[string]interface{}{}
	createdByUserSelectFields := map[string]interface{}{}
	updatedByUserSelectFields := map[string]interface{}{}
	deletedByUserSelectFields := map[string]interface{}{}

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

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			createdByUserSelectFields = ParseRelationalSelectString(keys[0], "created_by_user")
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			updatedByUserSelectFields = ParseRelationalSelectString(keys[0], "updated_by_user")
		}

		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			deletedByUserSelectFields = ParseRelationalSelectString(keys[0], "deleted_by_user")
		}

	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

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

		if _, ok := criterias.Select["store.id"]; ok {
			order.Store, _ = FindStoreByID(order.StoreID, storeSelectFields)
		}
		if _, ok := criterias.Select["customer.id"]; ok {
			order.Customer, _ = FindCustomerByID(order.CustomerID, customerSelectFields)
		}
		if _, ok := criterias.Select["created_by_user.id"]; ok {
			order.CreatedByUser, _ = FindUserByID(order.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			order.UpdatedByUser, _ = FindUserByID(order.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			order.DeletedByUser, _ = FindUserByID(order.DeletedBy, deletedByUserSelectFields)
		}
		orders = append(orders, order)
	} //end for loop

	return orders, criterias, nil
}

func (order *Order) Validate(w http.ResponseWriter, r *http.Request, scenario string, oldOrder *Order) (errs map[string]string) {

	errs = make(map[string]string)

	if govalidator.IsNull(order.Status) {
		errs["status"] = "Status is required"
	}

	if govalidator.IsNull(order.PaymentMethod) {
		errs["payment_method"] = "Payment method is required"
	}

	if govalidator.IsNull(order.PaymentStatus) {
		errs["payment_status"] = "Payment status is required"
	}

	if govalidator.IsNull(order.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "Jan 02 2006"
		date, err := time.Parse(shortForm, order.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		order.Date = &date
	}

	if scenario == "update" {
		if order.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsOrderExists(&order.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Order:" + order.ID.Hex()
		}

		if oldOrder != nil {
			if oldOrder.Status == "delivered" || oldOrder.Status == "dispatched" {
				if order.Status == "pending" || order.Status == "cancelled" || order.Status == "order_placed" {
					errs["status"] = "Can't change the status from delivered/dispatched to pending/cancelled/order_placed"
				}
			}
		}
	}

	if order.StoreID == nil || order.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(order.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + order.StoreID.Hex()
			return errs
		}
	}

	if order.CustomerID == nil || order.CustomerID.IsZero() {
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

	if order.DeliveredBy == nil || order.DeliveredBy.IsZero() {
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
		errs["product_id"] = "Atleast 1 product is required for order"
	}

	if order.DeliveredBySignatureID != nil && !order.DeliveredBySignatureID.IsZero() {
		exists, err := IsSignatureExists(order.DeliveredBySignatureID)
		if err != nil {
			errs["delivered_by_signature_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["delivered_by_signature_id"] = "Invalid Delivered By Signature:" + order.DeliveredBySignatureID.Hex()
		}
	}

	for index, product := range order.Products {
		if product.ProductID.IsZero() {
			errs["product_id"] = "Product is required for order"
		} else {
			exists, err := IsProductExists(&product.ProductID)
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

		stock, err := GetProductStockInStore(&product.ProductID, order.StoreID, product.Quantity)
		if err != nil {
			errs["quantity"] = err.Error()
			return errs
		}

		if stock < product.Quantity {
			productObject, err := FindProductByID(&product.ProductID, bson.M{})
			if err != nil {
				errs["product"] = err.Error()
				return errs
			}

			storeObject, err := FindStoreByID(order.StoreID, nil)
			if err != nil {
				errs["store"] = err.Error()
				return errs
			}

			errs["quantity_"+strconv.Itoa(index)] = "Product: " + productObject.Name + " stock is only " + strconv.Itoa(stock) + " in Store: " + storeObject.Name
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

func GetProductStockInStore(
	productID *primitive.ObjectID,
	storeID *primitive.ObjectID,
	orderQuantity int,
) (stock int, err error) {
	product, err := FindProductByID(productID, bson.M{})
	if err != nil {
		return 0, err
	}

	if storeID == nil {
		return 0, err
	}

	for _, stock := range product.Stock {
		if stock.StoreID.Hex() == storeID.Hex() {
			return stock.Stock, nil
		}
	}

	return 0, err
}

func (order *Order) RemoveStock() (err error) {
	if len(order.Products) == 0 {
		return nil
	}

	for _, orderProduct := range order.Products {
		product, err := FindProductByID(&orderProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		for k, stock := range product.Stock {
			if stock.StoreID.Hex() == order.StoreID.Hex() {

				order.SetChangeLog(
					"remove_stock",
					product.Name,
					product.Stock[k].Stock,
					(product.Stock[k].Stock - orderProduct.Quantity),
				)

				product.SetChangeLog(
					"remove_stock",
					product.Name,
					product.Stock[k].Stock,
					(product.Stock[k].Stock - orderProduct.Quantity),
				)

				product.Stock[k].Stock -= orderProduct.Quantity
				order.StockRemoved = true
				break
			}
		}

		err = product.Update()
		if err != nil {
			return err
		}

	}

	err = order.Update()
	if err != nil {
		return err
	}
	return nil
}

func (order *Order) AddStock() (err error) {
	if len(order.Products) == 0 {
		return nil
	}

	for _, orderProduct := range order.Products {
		product, err := FindProductByID(&orderProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		storeExistInProductStock := false
		for k, stock := range product.Stock {
			if stock.StoreID.Hex() == order.StoreID.Hex() {
				/*
					order.SetChangeLog(
						"add_stock",
						product.Name,
						product.Stock[k].Stock,
						(product.Stock[k].Stock + orderProduct.Quantity),
					)

					product.SetChangeLog(
						"add_stock",
						product.Name,
						product.Stock[k].Stock,
						(product.Stock[k].Stock + orderProduct.Quantity),
					)*/

				product.Stock[k].Stock += orderProduct.Quantity
				storeExistInProductStock = true
				break
			}
		}

		if !storeExistInProductStock {
			productStock := ProductStock{
				StoreID: *order.StoreID,
				Stock:   orderProduct.Quantity,
			}
			product.Stock = append(product.Stock, productStock)
		}

		err = product.Update()
		if err != nil {
			return err
		}
	}

	order.StockRemoved = false
	err = order.Update()
	if err != nil {
		return err
	}

	return nil
}

func (order *Order) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := order.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

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

	order.SetChangeLog("create", nil, nil, nil)

	_, err = collection.InsertOne(ctx, &order)
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
	//letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789")
	letterRunes := []rune("0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func (order *Order) UpdateOrderStatus(status string) (*Order, error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()
	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": order.ID},
		bson.M{"$set": bson.M{"status": status}},
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

func (order *Order) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := order.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	order.SetChangeLog("update", nil, nil, nil)

	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": order.ID},
		bson.M{"$set": order},
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

func (order *Order) DeleteOrder(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = order.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	order.Deleted = true
	order.DeletedBy = &userID
	now := time.Now()
	order.DeletedAt = &now

	order.SetChangeLog("delete", nil, nil, nil)

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

func (order *Order) HardDelete() (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = collection.DeleteOne(ctx, bson.M{"_id": order.ID})
	if err != nil {
		return err
	}
	return nil
}

func FindOrderByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (order *Order, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&order)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["store.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "store")
		order.Store, _ = FindStoreByID(order.StoreID, fields)
	}

	if _, ok := selectFields["customer.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "customer")
		order.Customer, _ = FindCustomerByID(order.CustomerID, fields)
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		order.CreatedByUser, _ = FindUserByID(order.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		order.UpdatedByUser, _ = FindUserByID(order.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		order.DeletedByUser, _ = FindUserByID(order.DeletedBy, fields)
	}

	return order, err
}

func IsOrderExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

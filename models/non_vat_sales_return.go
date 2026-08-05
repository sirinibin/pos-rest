package models

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// NonVATSalesReturn is a standalone non-VAT sales return stored in the non_vat_sales_return collection.
type NonVATSalesReturn struct {
	ID                 primitive.ObjectID            `json:"id,omitempty" bson:"_id,omitempty"`
	Code               string                        `bson:"code,omitempty" json:"code,omitempty"`
	NonVATSalesID      *primitive.ObjectID           `json:"non_vat_sales_id,omitempty" bson:"non_vat_sales_id,omitempty"`
	NonVATSalesCode    string                        `bson:"non_vat_sales_code,omitempty" json:"non_vat_sales_code,omitempty"`
	Date               *time.Time                    `bson:"date,omitempty" json:"date,omitempty"`
	DateStr            string                        `json:"date_str,omitempty" bson:"-"`
	StoreID            *primitive.ObjectID           `json:"store_id,omitempty" bson:"store_id,omitempty"`
	Store              *Store                        `json:"store,omitempty" bson:"-"`
	CustomerID         *primitive.ObjectID           `json:"customer_id" bson:"customer_id"`
	Customer           *Customer                     `json:"customer,omitempty" bson:"-"`
	CustomerName       string                        `json:"customer_name" bson:"customer_name"`
	CustomerNameArabic string                        `json:"customer_name_arabic" bson:"customer_name_arabic"`
	Products           []QuotationSalesReturnProduct `bson:"products,omitempty" json:"products,omitempty"`
	VatPercent         *float64                      `bson:"vat_percent" json:"vat_percent"`
	Discount           float64                       `bson:"discount" json:"discount"`
	DiscountPercent    float64                       `bson:"discount_percent" json:"discount_percent"`
	DiscountWithVAT    float64                       `bson:"discount_with_vat" json:"discount_with_vat"`
	Total              float64                       `bson:"total" json:"total"`
	TotalWithVAT       float64                       `bson:"total_with_vat" json:"total_with_vat"`
	NetTotal           float64                       `bson:"net_total" json:"net_total"`
	ActualTotal        float64                       `bson:"actual_total" json:"actual_total"`
	ActualTotalWithVAT float64                       `bson:"actual_total_with_vat" json:"actual_total_with_vat"`
	ActualNetTotal     float64                       `bson:"actual_net_total" json:"actual_net_total"`
	VatPrice           float64                       `bson:"vat_price" json:"vat_price"`
	ActualVatPrice     float64                       `bson:"actual_vat_price" json:"actual_vat_price"`
	RoundingAmount     float64                       `bson:"rounding_amount" json:"rounding_amount"`
	AutoRoundingAmount bool                          `bson:"auto_rounding_amount" json:"auto_rounding_amount"`
	TotalQuantity      float64                       `bson:"total_quantity" json:"total_quantity"`
	PaymentStatus      string                        `bson:"payment_status" json:"payment_status"`
	Payments           []QuotationSalesReturnPayment `bson:"payments" json:"payments"`
	PaymentsInput      []QuotationSalesReturnPayment `bson:"-" json:"payments_input"`
	PaymentsCount      int64                         `bson:"payments_count" json:"payments_count"`
	TotalPaymentPaid   float64                       `bson:"total_payment_paid" json:"total_payment_paid"`
	BalanceAmount      float64                       `bson:"balance_amount" json:"balance_amount"`
	CashDiscount       float64                       `bson:"cash_discount" json:"cash_discount"`
	PaymentMethods     []string                      `json:"payment_methods" bson:"payment_methods"`
	Remarks            string                        `bson:"remarks" json:"remarks"`
	Phone              string                        `bson:"phone" json:"phone"`
	VatNo              string                        `bson:"vat_no" json:"vat_no"`
	Address            string                        `bson:"address" json:"address"`
	Status             string                        `bson:"status,omitempty" json:"status,omitempty"`
	StockAdded         bool                          `bson:"stock_added,omitempty" json:"stock_added,omitempty"`
	Profit             float64                       `bson:"profit" json:"profit"`
	NetProfit          float64                       `bson:"net_profit" json:"net_profit"`
	Loss               float64                       `bson:"loss" json:"loss"`
	NetLoss            float64                       `bson:"net_loss" json:"net_loss"`
	CreatedAt          *time.Time                    `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt          *time.Time                    `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy          *primitive.ObjectID           `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy          *primitive.ObjectID           `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser      *User                         `json:"created_by_user,omitempty" bson:"-"`
	UpdatedByUser      *User                         `json:"updated_by_user,omitempty" bson:"-"`
	StoreName          string                        `json:"store_name,omitempty" bson:"store_name,omitempty"`
	CreatedByName      string                        `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName      string                        `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName      string                        `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	Deleted            bool                          `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy          *primitive.ObjectID           `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedAt          *time.Time                    `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	ExcludeServiceTax  bool                          `json:"exclude_service_tax" bson:"exclude_service_tax"`
	ExcludeProductTax  bool                          `json:"exclude_product_tax" bson:"exclude_product_tax"`
}

// ---- Calculation helpers ----

func (r *NonVATSalesReturn) FindTotalQuantity() {
	total := float64(0)
	for _, p := range r.Products {
		total += p.Quantity
	}
	r.TotalQuantity = total
}

func (r *NonVATSalesReturn) FindTotal() {
	total := float64(0)
	totalWithVAT := float64(0)
	actualTotal := float64(0)
	actualTotalWithVAT := float64(0)
	for i, p := range r.Products {
		excludeVAT := (p.IsService && r.ExcludeServiceTax) || (!p.IsService && r.ExcludeProductTax)
		if excludeVAT {
			r.Products[i].UnitPriceWithVAT = r.Products[i].UnitPrice
			r.Products[i].UnitDiscountWithVAT = r.Products[i].UnitDiscount
		} else if p.UnitPriceWithVAT == 0 && p.UnitPrice > 0 && r.VatPercent != nil && *r.VatPercent > 0 {
			r.Products[i].UnitPriceWithVAT = RoundTo2Decimals(p.UnitPrice * (1 + (*r.VatPercent / 100)))
		}
		total += p.Quantity * (r.Products[i].UnitPrice - p.UnitDiscount)
		total = RoundTo2Decimals(total)
		totalWithVAT += p.Quantity * (r.Products[i].UnitPriceWithVAT - p.UnitDiscountWithVAT)
		totalWithVAT = RoundTo2Decimals(totalWithVAT)

		//Actual
		actualTotal += p.Quantity * (r.Products[i].UnitPrice - p.UnitDiscount)
		actualTotal = RoundTo8Decimals(actualTotal)
		actualTotalWithVAT += p.Quantity * (r.Products[i].UnitPriceWithVAT - p.UnitDiscountWithVAT)
		actualTotalWithVAT = RoundTo8Decimals(actualTotalWithVAT)
	}
	r.Total = total
	r.TotalWithVAT = totalWithVAT
	r.ActualTotal = actualTotal
	r.ActualTotalWithVAT = actualTotalWithVAT
}

func (r *NonVATSalesReturn) FindVatPrice() {
	r.VatPrice = RoundTo2Decimals(r.TotalWithVAT - r.Total)
	r.ActualVatPrice = RoundTo2Decimals(r.ActualTotalWithVAT - r.ActualTotal)
}

func (r *NonVATSalesReturn) FindNetTotal() error {
	store, err := FindStoreByID(r.StoreID, bson.M{})
	if err != nil {
		return err
	}
	if r.VatPercent == nil {
		vatPercent := store.VatPercent
		r.VatPercent = &vatPercent
	}

	r.Discount = RoundTo2Decimals(r.Discount)

	r.FindTotal()
	r.FindVatPrice()

	netTotal := r.TotalWithVAT - r.Discount - r.CashDiscount
	r.NetTotal = RoundTo2Decimals(netTotal)

	//Actual
	actualNetTotal := r.ActualTotalWithVAT - r.Discount - r.CashDiscount
	r.ActualNetTotal = RoundTo2Decimals(actualNetTotal)

	if r.AutoRoundingAmount {
		r.RoundingAmount = RoundTo2Decimals(r.ActualNetTotal - r.NetTotal)
	}

	r.NetTotal = RoundTo2Decimals(r.NetTotal + r.RoundingAmount)

	return nil
}

// ---- Foreign label update ----

func (ret *NonVATSalesReturn) UpdateForeignLabelFields() error {
	if ret.StoreID != nil {
		store, err := FindStoreByID(ret.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		ret.StoreName = store.Name
	}

	store, err := FindStoreByID(ret.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if ret.CustomerID != nil && !ret.CustomerID.IsZero() {
		customer, err := store.FindCustomerByID(ret.CustomerID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1})
		if err != nil {
			return err
		}
		ret.CustomerName = customer.Name
		ret.CustomerNameArabic = customer.NameInArabic
	}

	if ret.CreatedBy != nil {
		u, err := FindUserByID(ret.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		ret.CreatedByName = u.Name
	}

	if ret.UpdatedBy != nil {
		u, err := FindUserByID(ret.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		ret.UpdatedByName = u.Name
	}

	for i, product := range ret.Products {
		productObject, err := store.FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1, "part_number": 1, "prefix_part_number": 1, "is_service": 1})
		if err != nil {
			return err
		}
		ret.Products[i].NameInArabic = productObject.NameInArabic
		ret.Products[i].ItemCode = productObject.ItemCode
		if productObject.PartNumber != "" {
			ret.Products[i].PartNumber = productObject.PartNumber
		}
		ret.Products[i].PrefixPartNumber = productObject.PrefixPartNumber
	}

	return nil
}

// ---- Validation ----

func (ret *NonVATSalesReturn) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(ret.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store id"
		return errs
	}

	customer, err := store.FindCustomerByID(ret.CustomerID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		errs["customer_id"] = "Invalid customer"
		return errs
	}
	if customer == nil && govalidator.IsNull(ret.CustomerName) {
		ret.CustomerID = nil
	}

	if govalidator.IsNull(ret.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, ret.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		ret.Date = &date
	}

	if scenario == "update" {
		if ret.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsNonVATSalesReturnExists(&ret.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}
		if !exists {
			errs["id"] = "Invalid Non VAT Sales Return:" + ret.ID.Hex()
		}
	}

	return errs
}

func (store *Store) IsNonVATSalesReturnExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("non_vat_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count, err := collection.CountDocuments(ctx, bson.M{"_id": ID, "store_id": store.ID, "deleted": bson.M{"$ne": true}})
	return count > 0, err
}

// ---- Serial number / code generation ----

func (ret *NonVATSalesReturn) MakeRedisCode() error {
	store, err := FindStoreByID(ret.StoreID, bson.M{})
	if err != nil {
		return err
	}

	serialNum := store.NonVATSalesReturnSerialNumber
	if serialNum.StartFromCount <= 0 {
		serialNum.StartFromCount = 1
	}
	if serialNum.PaddingCount <= 0 {
		serialNum.PaddingCount = 3
	}
	redisKey := ret.StoreID.Hex() + "_non_vat_sales_return_counter"

	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	baseTime := ret.CreatedAt.In(location)

	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		count, err := store.GetCountByCollection("non_vat_sales_return")
		if err != nil {
			return err
		}
		startFrom := serialNum.StartFromCount
		err = db.RedisClient.Set(redisKey, startFrom+count-1, 0).Err()
		if err != nil {
			return err
		}
	}

	globalIncr, err := db.RedisClient.Incr(redisKey).Result()
	if err != nil {
		return err
	}

	useMonthly := strings.Contains(serialNum.Prefix, "DATE")
	serialNumber := globalIncr

	if useMonthly {
		monthKey := baseTime.Format("200601")
		monthlyRedisKey := redisKey + "_" + monthKey

		monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result()
		if err != nil {
			return err
		}
		if monthlyExists == 0 {
			startFrom := serialNum.StartFromCount
			fromDate := time.Date(baseTime.Year(), baseTime.Month(), 1, 0, 0, 0, 0, location)
			toDate := fromDate.AddDate(0, 1, 0).Add(-time.Nanosecond)
			monthlyCount, err := store.GetCountByCollectionInRange(fromDate, toDate, "non_vat_sales_return")
			if err != nil {
				return err
			}
			err = db.RedisClient.Set(monthlyRedisKey, startFrom+monthlyCount-1, 0).Err()
			if err != nil {
				return err
			}
		}
		monthlyIncr, err := db.RedisClient.Incr(monthlyRedisKey).Result()
		if err != nil {
			return err
		}
		if store.Settings.EnableMonthlySerialNumber {
			serialNumber = monthlyIncr
		}
	}

	paddingCount := serialNum.PaddingCount
	if serialNum.Prefix != "" {
		ret.Code = fmt.Sprintf("%s-%0*d", serialNum.Prefix, paddingCount, serialNumber)
	} else {
		ret.Code = fmt.Sprintf("%0*d", paddingCount, serialNumber)
	}

	if strings.Contains(ret.Code, "DATE") {
		ret.Code = strings.ReplaceAll(ret.Code, "DATE", baseTime.Format("20060102"))
	}

	return nil
}

func (ret *NonVATSalesReturn) UnMakeRedisCode() error {
	store, err := FindStoreByID(ret.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := ret.StoreID.Hex() + "_non_vat_sales_return_counter"

	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}
	baseTime := ret.CreatedAt.In(location)

	if exists, err := db.RedisClient.Exists(redisKey).Result(); err == nil && exists != 0 {
		if _, err := db.RedisClient.Decr(redisKey).Result(); err != nil {
			return err
		}
	}

	if strings.Contains(store.NonVATSalesReturnSerialNumber.Prefix, "DATE") {
		monthKey := baseTime.Format("200601")
		monthlyRedisKey := redisKey + "_" + monthKey
		if monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result(); err == nil && monthlyExists != 0 {
			if _, err := db.RedisClient.Decr(monthlyRedisKey).Result(); err != nil {
				return err
			}
		}
	}

	return nil
}

// ---- Payment processing ----

func (ret *NonVATSalesReturn) ProcessPayments() {
	payments := []QuotationSalesReturnPayment{}
	methods := []string{}
	methodSeen := map[string]bool{}
	total := float64(0)

	for _, p := range ret.PaymentsInput {
		if p.Deleted || p.Amount <= 0 {
			continue
		}
		if p.Date == nil && p.DateStr != "" {
			const shortForm = "2006-01-02T15:04:05Z07:00"
			if t, err := time.Parse(shortForm, p.DateStr); err == nil {
				p.Date = &t
			}
		}
		if p.ID.IsZero() {
			p.ID = primitive.NewObjectID()
		}
		payments = append(payments, p)
		total += p.Amount
		if p.Method != "" && !methodSeen[p.Method] {
			methods = append(methods, p.Method)
			methodSeen[p.Method] = true
		}
	}

	ret.Payments = payments
	ret.PaymentsCount = int64(len(payments))
	ret.TotalPaymentPaid = RoundTo2Decimals(total)
	ret.PaymentMethods = methods

	effective := RoundTo2Decimals(ret.NetTotal - ret.CashDiscount)
	ret.BalanceAmount = RoundTo2Decimals(effective - total)
	if total <= 0 {
		ret.PaymentStatus = "not_paid"
	} else if ret.BalanceAmount <= 0 {
		ret.PaymentStatus = "paid"
	} else {
		ret.PaymentStatus = "paid_partially"
	}
}

// ---- CRUD ----

func (ret *NonVATSalesReturn) Insert() error {
	collection := db.GetDB("store_" + ret.StoreID.Hex()).Collection("non_vat_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ret.ID = primitive.NewObjectID()
	_, err := collection.InsertOne(ctx, ret)
	return err
}

func (ret *NonVATSalesReturn) Update() error {
	collection := db.GetDB("store_" + ret.StoreID.Hex()).Collection("non_vat_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	_, err := collection.UpdateOne(ctx, bson.M{"_id": ret.ID}, bson.M{"$set": ret}, updateOptions)
	return err
}

func (ret *NonVATSalesReturn) Delete(tokenClaims TokenClaims) error {
	err := ret.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	ret.Deleted = true
	ret.DeletedBy = &userID
	now := time.Now()
	ret.DeletedAt = &now

	collection := db.GetDB("store_" + ret.StoreID.Hex()).Collection("non_vat_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	_, err = collection.UpdateOne(ctx, bson.M{"_id": ret.ID}, bson.M{"$set": ret}, updateOptions)
	return err
}

func (store *Store) FindNonVATSalesReturnByID(ID *primitive.ObjectID, selectFields map[string]interface{}) (*NonVATSalesReturn, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("non_vat_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	var ret NonVATSalesReturn
	err := collection.FindOne(ctx, bson.M{"_id": ID, "store_id": store.ID}, findOneOptions).Decode(&ret)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["store.id"]; ok {
		storeSelectFields := ParseRelationalSelectString(selectFields, "store")
		ret.Store, _ = FindStoreByID(ret.StoreID, storeSelectFields)
	}
	if _, ok := selectFields["customer.id"]; ok {
		customerSelectFields := ParseRelationalSelectString(selectFields, "customer")
		ret.Customer, _ = store.FindCustomerByID(ret.CustomerID, customerSelectFields)
	}
	if _, ok := selectFields["created_by_user.id"]; ok {
		createdBySelectFields := ParseRelationalSelectString(selectFields, "created_by_user")
		ret.CreatedByUser, _ = FindUserByID(ret.CreatedBy, createdBySelectFields)
	}

	return &ret, nil
}

// ---- Search / List ----

func (store *Store) SearchNonVATSalesReturn(w http.ResponseWriter, r *http.Request) (items []NonVATSalesReturn, criterias SearchCriterias, err error) {
	criterias = InitSearchCriterias()
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	timeZoneOffset := CountryTimezoneOffset(store.CountryCode)

	if err = ParseExactDateFilter(r, &criterias, "search[date_str]", "date", timeZoneOffset); err != nil {
		return items, criterias, err
	}
	if err = ParseDateRangeFilter(r, &criterias, "search[from_date]", "search[to_date]", "date", timeZoneOffset); err != nil {
		return items, criterias, err
	}

	if err = ParseObjectIDFilter(r, &criterias, "search[store_id]", "store_id"); err != nil {
		return items, criterias, err
	}
	if err = ParseObjectIDFilter(r, &criterias, "search[customer_id]", "customer_id"); err != nil {
		return items, criterias, err
	}
	if err = ParseObjectIDFilter(r, &criterias, "search[non_vat_sales_id]", "non_vat_sales_id"); err != nil {
		return items, criterias, err
	}
	if err = ParseObjectIDFilter(r, &criterias, "search[created_by]", "created_by"); err != nil {
		return items, criterias, err
	}

	keys, ok := r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["code"] = bson.M{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[customer_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["$or"] = []bson.M{
			{"customer_name": bson.M{"$regex": keys[0], "$options": "i"}},
			{"customer_name_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
		}
	}

	keys, ok = r.URL.Query()["search[payment_status]"]
	if ok && len(keys[0]) >= 1 {
		paymentStatusList := strings.Split(keys[0], ",")
		if len(paymentStatusList) > 0 {
			criterias.SearchBy["payment_status"] = bson.M{"$in": paymentStatusList}
		}
	}

	ParsePaginationAndSort(r, &criterias)

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("non_vat_sales_return")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	storeSelectFields := map[string]interface{}{}
	customerSelectFields := map[string]interface{}{}
	createdByUserSelectFields := map[string]interface{}{}

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		if _, ok := criterias.Select["store.id"]; ok {
			storeSelectFields = ParseRelationalSelectString(keys[0], "store")
		}
		if _, ok := criterias.Select["customer.id"]; ok {
			customerSelectFields = ParseRelationalSelectString(keys[0], "customer")
		}
		if _, ok := criterias.Select["created_by_user.id"]; ok {
			createdByUserSelectFields = ParseRelationalSelectString(keys[0], "created_by_user")
		}
	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return items, criterias, errors.New("Error fetching non_vat_sales_return:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for cur != nil && cur.Next(ctx) {
		if err := cur.Err(); err != nil {
			return items, criterias, errors.New("Cursor error:" + err.Error())
		}
		var item NonVATSalesReturn
		if err := cur.Decode(&item); err != nil {
			return items, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		if _, ok := criterias.Select["store.id"]; ok {
			item.Store, _ = FindStoreByID(item.StoreID, storeSelectFields)
		}
		if _, ok := criterias.Select["customer.id"]; ok {
			item.Customer, _ = store.FindCustomerByID(item.CustomerID, customerSelectFields)
		}
		if _, ok := criterias.Select["created_by_user.id"]; ok {
			item.CreatedByUser, _ = FindUserByID(item.CreatedBy, createdByUserSelectFields)
		}
		items = append(items, item)
	}

	return items, criterias, nil
}

type ProductNonVATSalesReturnStats struct {
	NonVATSalesReturnQuantity float64 `json:"non_vat_sales_return_quantity" bson:"non_vat_sales_return_quantity"`
}

func (product *Product) SetProductNonVATSalesReturnQuantityByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_non_vat_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductNonVATSalesReturnStats

	filter := map[string]interface{}{
		"store_id":   storeID,
		"product_id": product.ID,
	}

	pipeline := []bson.M{
		{"$match": filter},
		{"$group": bson.M{
			"_id":                           nil,
			"non_vat_sales_return_quantity": bson.M{"$sum": "$quantity"},
		}},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)

	if cur.Next(ctx) {
		if err := cur.Decode(&stats); err != nil {
			return err
		}
	}

	if productStoreTemp, ok := product.ProductStores[storeID.Hex()]; ok {
		productStoreTemp.NonVATSalesReturnQuantity = stats.NonVATSalesReturnQuantity
		product.ProductStores[storeID.Hex()] = productStoreTemp
	}

	return nil
}

func (ret *NonVATSalesReturn) SetProductsStock() error {
	store, err := FindStoreByID(ret.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if len(ret.Products) == 0 {
		return nil
	}

	for _, p := range ret.Products {
		if !p.Selected {
			continue
		}

		product, err := store.FindProductByID(&p.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if err = product.SetStock(); err != nil {
			return err
		}

		if err = product.Update(&store.ID); err != nil {
			return err
		}

		for _, setProduct := range product.Set.Products {
			setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
			if err != nil {
				return err
			}
			if err = setProductObj.SetStock(); err != nil {
				return err
			}
			if err = setProductObj.Update(&store.ID); err != nil {
				return err
			}
		}
	}

	return nil
}

// Accounting

var totalNonVATSalesReturnPaidAmount float64
var extraNonVATSalesReturnAmountPaid float64
var extraNonVATSalesReturnPayments []QuotationSalesReturnPayment

func (ret *NonVATSalesReturn) AdjustPayments() error {
	if len(ret.Payments) == 0 || ret.Date == nil {
		return nil
	}

	firstPayment := ret.Payments[0]
	if firstPayment.Date != nil && firstPayment.Date.Equal(*ret.Date) {
		newTime := ret.Date.Add(1 * time.Minute)
		ret.Payments[0].Date = &newTime
	}

	for i := 1; i < len(ret.Payments); i++ {
		prev := ret.Payments[i-1].Date
		curr := ret.Payments[i].Date
		if prev != nil && curr != nil && (curr.Equal(*prev) || curr.Before(*prev)) {
			newTime := prev.Add(1 * time.Minute)
			ret.Payments[i].Date = &newTime
		}
	}

	return ret.Update()
}

func (ret *NonVATSalesReturn) DoAccounting() error {
	err := ret.AdjustPayments()
	if err != nil {
		return errors.New("error adjusting payments: " + err.Error())
	}

	ledger, err := ret.CreateLedger()
	if err != nil {
		return errors.New("error creating ledger: " + err.Error())
	}

	_, err = ledger.CreatePostings()
	if err != nil {
		return errors.New("error creating postings: " + err.Error())
	}

	return nil
}

func (ret *NonVATSalesReturn) UndoAccounting() error {
	store, err := FindStoreByID(ret.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledger, err := store.FindLedgerByReferenceID(ret.ID, *ret.StoreID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return errors.New("Error finding ledger by reference id: " + err.Error())
	}

	if err == mongo.ErrNoDocuments {
		return nil
	}

	ledgerAccounts := map[string]Account{}

	if ledger != nil {
		ledgerAccounts, err = ledger.GetRelatedAccounts()
		if err != nil && err != mongo.ErrNoDocuments {
			return errors.New("Error getting related accounts: " + err.Error())
		}
	}

	err = store.RemoveLedgerByReferenceID(ret.ID)
	if err != nil {
		return errors.New("Error removing ledger by reference id: " + err.Error())
	}

	err = store.RemovePostingsByReferenceID(ret.ID)
	if err != nil {
		return errors.New("Error removing postings by reference id: " + err.Error())
	}

	err = SetAccountBalances(ledgerAccounts)
	if err != nil {
		return errors.New("Error setting account balances: " + err.Error())
	}

	return nil
}

func (ret *NonVATSalesReturn) CreateLedger() (ledger *Ledger, err error) {
	store, err := FindStoreByID(ret.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var customer *Customer

	if ret.CustomerID != nil && !ret.CustomerID.IsZero() {
		customer, err = store.FindCustomerByID(ret.CustomerID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}
	}

	cashAccount, err := store.CreateAccountIfNotExists(ret.StoreID, nil, nil, "Cash", nil, nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := store.CreateAccountIfNotExists(ret.StoreID, nil, nil, "Bank", nil, nil)
	if err != nil {
		return nil, err
	}

	nonVATSalesReturnAccount, err := store.CreateAccountIfNotExists(ret.StoreID, nil, nil, "Non VAT Sales Return", nil, nil)
	if err != nil {
		return nil, err
	}

	cashDiscountReceivedAccount, err := store.CreateAccountIfNotExists(ret.StoreID, nil, nil, "Cash discount received", nil, nil)
	if err != nil {
		return nil, err
	}

	journals := []Journal{}

	var firstPaymentDate *time.Time
	if len(ret.Payments) > 0 {
		firstPaymentDate = ret.Payments[0].Date
	}

	if len(ret.Payments) == 0 || (firstPaymentDate != nil && !IsDateTimesEqual(firstPaymentDate, ret.Date)) {
		referenceModel := "customer"
		customerName := ""
		var referenceID *primitive.ObjectID
		var customerVATNo *string
		var customerPhone *string
		if customer != nil {
			customerName = customer.Name
			referenceID = &customer.ID
			customerVATNo = &customer.VATNo
			customerPhone = &customer.Phone
		} else {
			customerName = "Customer Accounts - Unknown"
			referenceID = nil
		}

		customerAccount, err := store.CreateAccountIfNotExists(
			ret.StoreID,
			referenceID,
			&referenceModel,
			customerName,
			customerPhone,
			customerVATNo,
		)
		if err != nil {
			return nil, err
		}
		journals = append(journals, MakeJournalsForUnpaidNonVATSalesReturn(
			ret,
			customerAccount,
			nonVATSalesReturnAccount,
			cashDiscountReceivedAccount,
		)...)
	}

	if len(ret.Payments) > 0 {
		totalNonVATSalesReturnPaidAmount = float64(0.00)
		extraNonVATSalesReturnAmountPaid = float64(0.00)
		extraNonVATSalesReturnPayments = []QuotationSalesReturnPayment{}

		paymentsByDatetimeNumber := 1
		paymentsByDatetime := RegroupNonVATSalesReturnPaymentsByDatetime(ret.Payments)

		for _, paymentByDatetime := range paymentsByDatetime {
			newJournals, err := MakeJournalsForNonVATSalesReturnPaymentsByDatetime(
				ret,
				customer,
				cashAccount,
				bankAccount,
				nonVATSalesReturnAccount,
				paymentByDatetime,
				cashDiscountReceivedAccount,
				paymentsByDatetimeNumber,
			)
			if err != nil {
				return nil, err
			}

			journals = append(journals, newJournals...)
			paymentsByDatetimeNumber++
		}

		if ret.BalanceAmount < 0 && len(extraNonVATSalesReturnPayments) > 0 {
			newJournals, err := MakeJournalsForNonVATSalesReturnExtraPayments(
				ret,
				customer,
				cashAccount,
				bankAccount,
				extraNonVATSalesReturnPayments,
			)
			if err != nil {
				return nil, err
			}
			journals = append(journals, newJournals...)
		}

		totalNonVATSalesReturnPaidAmount = float64(0.00)
		extraNonVATSalesReturnAmountPaid = float64(0.00)
	}

	ledger = &Ledger{
		StoreID:        ret.StoreID,
		ReferenceID:    ret.ID,
		ReferenceModel: "non_vat_sales_return",
		ReferenceCode:  ret.Code,
		Journals:       journals,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}

	err = ledger.Insert()
	if err != nil {
		return nil, err
	}

	return ledger, nil
}

func MakeJournalsForUnpaidNonVATSalesReturn(
	ret *NonVATSalesReturn,
	customerAccount *Account,
	nonVATSalesReturnAccount *Account,
	cashDiscountReceivedAccount *Account,
) []Journal {
	now := time.Now()
	groupID := primitive.NewObjectID()
	journals := []Journal{}

	balanceAmount := RoundFloat((ret.NetTotal - ret.CashDiscount), 2)

	journals = append(journals, Journal{
		Date:          ret.Date,
		AccountID:     nonVATSalesReturnAccount.ID,
		AccountNumber: nonVATSalesReturnAccount.Number,
		AccountName:   nonVATSalesReturnAccount.Name,
		DebitOrCredit: "debit",
		Debit:         ret.NetTotal,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	if ret.CashDiscount > 0 {
		journals = append(journals, Journal{
			Date:          ret.Date,
			AccountID:     cashDiscountReceivedAccount.ID,
			AccountNumber: cashDiscountReceivedAccount.Number,
			AccountName:   cashDiscountReceivedAccount.Name,
			DebitOrCredit: "credit",
			Credit:        ret.CashDiscount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	}

	journals = append(journals, Journal{
		Date:          ret.Date,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "credit",
		Credit:        balanceAmount,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals
}

func RegroupNonVATSalesReturnPaymentsByDatetime(payments []QuotationSalesReturnPayment) [][]QuotationSalesReturnPayment {
	paymentsByDatetime := map[string][]QuotationSalesReturnPayment{}
	for _, payment := range payments {
		paymentsByDatetime[payment.Date.Format("2006-01-02T15:04")] = append(paymentsByDatetime[payment.Date.Format("2006-01-02T15:04")], payment)
	}

	paymentsByDatetime2 := [][]QuotationSalesReturnPayment{}
	for _, v := range paymentsByDatetime {
		paymentsByDatetime2 = append(paymentsByDatetime2, v)
	}

	sort.Slice(paymentsByDatetime2, func(i, j int) bool {
		return paymentsByDatetime2[i][0].Date.Before(*paymentsByDatetime2[j][0].Date)
	})

	return paymentsByDatetime2
}

func MakeJournalsForNonVATSalesReturnPaymentsByDatetime(
	ret *NonVATSalesReturn,
	customer *Customer,
	cashAccount *Account,
	bankAccount *Account,
	nonVATSalesReturnAccount *Account,
	payments []QuotationSalesReturnPayment,
	cashDiscountReceivedAccount *Account,
	paymentsByDatetimeNumber int,
) ([]Journal, error) {
	store, err := FindStoreByID(ret.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	groupID := primitive.NewObjectID()
	journals := []Journal{}
	totalPayment := float64(0.00)

	var firstPaymentDate *time.Time
	if len(payments) > 0 {
		firstPaymentDate = payments[0].Date
	}

	// First pass: compute totalPayment (for debit side)
	totalNonVATSalesReturnPaidAmountTemp := totalNonVATSalesReturnPaidAmount
	extraNonVATSalesReturnAmountPaidTemp := extraNonVATSalesReturnAmountPaid

	for _, payment := range payments {
		totalNonVATSalesReturnPaidAmount += payment.Amount
		if totalNonVATSalesReturnPaidAmount > (ret.NetTotal - ret.CashDiscount) {
			extraNonVATSalesReturnAmountPaid = RoundFloat((totalNonVATSalesReturnPaidAmount - (ret.NetTotal - ret.CashDiscount)), 2)
		}
		amount := payment.Amount

		if extraNonVATSalesReturnAmountPaid > 0 {
			skip := false
			if extraNonVATSalesReturnAmountPaid < payment.Amount {
				amount = RoundFloat((payment.Amount - extraNonVATSalesReturnAmountPaid), 2)
				extraNonVATSalesReturnAmountPaid = 0
			} else if extraNonVATSalesReturnAmountPaid >= payment.Amount {
				skip = true
				extraNonVATSalesReturnAmountPaid = RoundFloat((extraNonVATSalesReturnAmountPaid - payment.Amount), 2)
			}
			if skip {
				continue
			}
		}
		totalPayment += amount
	}

	totalNonVATSalesReturnPaidAmount = totalNonVATSalesReturnPaidAmountTemp
	extraNonVATSalesReturnAmountPaid = extraNonVATSalesReturnAmountPaidTemp

	// Debits
	if paymentsByDatetimeNumber == 1 && IsDateTimesEqual(ret.Date, firstPaymentDate) {
		journals = append(journals, Journal{
			Date:          ret.Date,
			AccountID:     nonVATSalesReturnAccount.ID,
			AccountNumber: nonVATSalesReturnAccount.Number,
			AccountName:   nonVATSalesReturnAccount.Name,
			DebitOrCredit: "debit",
			Debit:         ret.NetTotal,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	} else if paymentsByDatetimeNumber > 1 || !IsDateTimesEqual(ret.Date, firstPaymentDate) {
		referenceModel := "customer"
		customerName := ""
		var referenceID *primitive.ObjectID
		var customerVATNo *string
		var customerPhone *string
		if customer != nil {
			customerName = customer.Name
			referenceID = &customer.ID
			customerVATNo = &customer.VATNo
			customerPhone = &customer.Phone
		} else {
			customerName = "Customer Accounts - Unknown"
			referenceID = nil
		}
		customerAccount, err := store.CreateAccountIfNotExists(
			ret.StoreID,
			referenceID,
			&referenceModel,
			customerName,
			customerPhone,
			customerVATNo,
		)
		if err != nil {
			return nil, err
		}

		totalPayment = RoundFloat(totalPayment, 2)
		if totalPayment > 0 {
			journals = append(journals, Journal{
				Date:          firstPaymentDate,
				AccountID:     customerAccount.ID,
				AccountNumber: customerAccount.Number,
				AccountName:   customerAccount.Name,
				DebitOrCredit: "debit",
				Debit:         totalPayment,
				GroupID:       groupID,
				CreatedAt:     &now,
				UpdatedAt:     &now,
			})
		}
	}

	// Credits (second pass)
	totalPayment = float64(0.00)
	for _, payment := range payments {
		totalNonVATSalesReturnPaidAmount += payment.Amount
		if totalNonVATSalesReturnPaidAmount > (ret.NetTotal - ret.CashDiscount) {
			extraNonVATSalesReturnAmountPaid = RoundFloat((totalNonVATSalesReturnPaidAmount - (ret.NetTotal - ret.CashDiscount)), 2)
		}
		amount := payment.Amount

		if extraNonVATSalesReturnAmountPaid > 0 {
			skip := false
			if extraNonVATSalesReturnAmountPaid < payment.Amount {
				extraNonVATSalesReturnPayments = append(extraNonVATSalesReturnPayments, QuotationSalesReturnPayment{
					Date:   payment.Date,
					Amount: extraNonVATSalesReturnAmountPaid,
					Method: payment.Method,
				})
				amount = RoundFloat((payment.Amount - extraNonVATSalesReturnAmountPaid), 2)
				extraNonVATSalesReturnAmountPaid = 0
			} else if extraNonVATSalesReturnAmountPaid >= payment.Amount {
				extraNonVATSalesReturnPayments = append(extraNonVATSalesReturnPayments, QuotationSalesReturnPayment{
					Date:   payment.Date,
					Amount: payment.Amount,
					Method: payment.Method,
				})
				skip = true
				extraNonVATSalesReturnAmountPaid = RoundFloat((extraNonVATSalesReturnAmountPaid - payment.Amount), 2)
			}
			if skip {
				continue
			}
		}

		cashPayingAccount := Account{}
		if payment.ReferenceType == "customer_withdrawal" || payment.ReferenceType == "non_vat_sales" {
			continue
		} else if payment.Method == "cash" {
			cashPayingAccount = *cashAccount
		} else if slices.Contains(BANK_PAYMENT_METHODS, payment.Method) {
			cashPayingAccount = *bankAccount
		} else if payment.Method == "customer_account" && customer != nil {
			continue
		}

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     cashPayingAccount.ID,
			AccountNumber: cashPayingAccount.Number,
			AccountName:   cashPayingAccount.Name,
			DebitOrCredit: "credit",
			Credit:        amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
		totalPayment += amount
	}

	if ret.CashDiscount > 0 && paymentsByDatetimeNumber == 1 && IsDateTimesEqual(ret.Date, firstPaymentDate) {
		journals = append(journals, Journal{
			Date:          ret.Date,
			AccountID:     cashDiscountReceivedAccount.ID,
			AccountNumber: cashDiscountReceivedAccount.Number,
			AccountName:   cashDiscountReceivedAccount.Name,
			DebitOrCredit: "credit",
			Credit:        ret.CashDiscount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	}

	balanceAmount := RoundFloat(((ret.NetTotal - ret.CashDiscount) - totalPayment), 2)

	if paymentsByDatetimeNumber == 1 && balanceAmount > 0 && IsDateTimesEqual(ret.Date, firstPaymentDate) {
		referenceModel := "customer"
		customerName := ""
		var referenceID *primitive.ObjectID
		var customerVATNo *string
		var customerPhone *string
		if customer != nil {
			customerName = customer.Name
			referenceID = &customer.ID
			customerVATNo = &customer.VATNo
			customerPhone = &customer.Phone
		} else {
			customerName = "Customer Accounts - Unknown"
			referenceID = nil
		}

		customerAccount, err := store.CreateAccountIfNotExists(
			ret.StoreID,
			referenceID,
			&referenceModel,
			customerName,
			customerPhone,
			customerVATNo,
		)
		if err != nil {
			return nil, err
		}

		journals = append(journals, Journal{
			Date:          ret.Date,
			AccountID:     customerAccount.ID,
			AccountNumber: customerAccount.Number,
			AccountName:   customerAccount.Name,
			DebitOrCredit: "credit",
			Credit:        balanceAmount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	}

	return journals, nil
}

type NonVATSalesReturnStats struct {
	NetTotal                     float64 `json:"net_total" bson:"net_total"`
	VatPrice                     float64 `json:"vat_price" bson:"vat_price"`
	Discount                     float64 `json:"discount" bson:"discount"`
	CashDiscount                 float64 `json:"cash_discount" bson:"cash_discount"`
	NetProfit                    float64 `json:"net_profit" bson:"net_profit"`
	NetLoss                      float64 `json:"net_loss" bson:"net_loss"`
	PaidNonVATSalesReturn        float64 `json:"paid_non_vat_sales_return" bson:"paid_non_vat_sales_return"`
	UnPaidNonVATSalesReturn      float64 `json:"unpaid_non_vat_sales_return" bson:"unpaid_non_vat_sales_return"`
	CashNonVATSalesReturn        float64 `json:"cash_non_vat_sales_return" bson:"cash_non_vat_sales_return"`
	BankAccountNonVATSalesReturn float64 `json:"bank_account_non_vat_sales_return" bson:"bank_account_non_vat_sales_return"`
	ShippingOrHandlingFees       float64 `json:"shipping_handling_fees" bson:"shipping_handling_fees"`
	NonVATSalesReturnCount       int64   `json:"non_vat_sales_return_count" bson:"non_vat_sales_return_count"`
	NonVATSalesNonVATSalesReturn float64 `json:"non_vat_sales_non_vat_sales_return" bson:"non_vat_sales_non_vat_sales_return"`
}

func (store *Store) GetNonVATSalesReturnStats(filter map[string]interface{}) (stats NonVATSalesReturnStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("non_vat_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		{
			"$match": filter,
		},
		{
			"$group": bson.M{
				"_id":                    nil,
				"net_total":              bson.M{"$sum": "$net_total"},
				"vat_price":              bson.M{"$sum": "$vat_price"},
				"discount":               bson.M{"$sum": "$discount"},
				"cash_discount":          bson.M{"$sum": "$cash_discount"},
				"net_profit":             bson.M{"$sum": "$net_profit"},
				"net_loss":               bson.M{"$sum": "$net_loss"},
				"shipping_handling_fees": bson.M{"$sum": "$shipping_handling_fees"},
				"paid_non_vat_sales_return": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$payments",
						"as":    "payment",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
								"$$payment.amount",
								0,
							},
						},
					},
				}}},
				"unpaid_non_vat_sales_return": bson.M{"$sum": "$balance_amount"},
				"cash_non_vat_sales_return": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$payments",
						"as":    "payment",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$eq": []interface{}{"$$payment.method", "cash"}},
									bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
								}},
								"$$payment.amount",
								0,
							},
						},
					},
				}}},
				"bank_account_non_vat_sales_return": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$payments",
						"as":    "payment",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$or": []interface{}{
									bson.M{"$and": []interface{}{
										bson.M{"$eq": []interface{}{"$$payment.method", "debit_card"}},
										bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
									}},
									bson.M{"$and": []interface{}{
										bson.M{"$eq": []interface{}{"$$payment.method", "credit_card"}},
										bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
									}},
									bson.M{"$and": []interface{}{
										bson.M{"$eq": []interface{}{"$$payment.method", "bank_card"}},
										bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
									}},
									bson.M{"$and": []interface{}{
										bson.M{"$eq": []interface{}{"$$payment.method", "bank_transfer"}},
										bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
									}},
									bson.M{"$and": []interface{}{
										bson.M{"$eq": []interface{}{"$$payment.method", "bank_cheque"}},
										bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
									}},
								}},
								"$$payment.amount",
								0,
							},
						},
					},
				}}},
				"non_vat_sales_non_vat_sales_return": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$payments",
						"as":    "payment",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$eq": []interface{}{"$$payment.method", "non_vat_sales"}},
									bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
								}},
								"$$payment.amount",
								0,
							},
						},
					},
				}}},
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
		stats.NetTotal = RoundFloat(stats.NetTotal, 2)
		stats.NetProfit = RoundFloat(stats.NetProfit, 2)
		stats.NetLoss = RoundFloat(stats.NetLoss, 2)
		stats.CashDiscount = RoundFloat(stats.CashDiscount, 2)
	}
	return stats, nil
}

func MakeJournalsForNonVATSalesReturnExtraPayments(
	ret *NonVATSalesReturn,
	customer *Customer,
	cashAccount *Account,
	bankAccount *Account,
	extraPayments []QuotationSalesReturnPayment,
) ([]Journal, error) {
	store, err := FindStoreByID(ret.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	journals := []Journal{}
	groupID := primitive.NewObjectID()

	var lastPaymentDate *time.Time
	if len(extraPayments) > 0 {
		lastPaymentDate = extraPayments[len(extraPayments)-1].Date
	}

	referenceModel := "customer"
	customerName := ""
	var referenceID *primitive.ObjectID
	var customerVATNo *string
	var customerPhone *string
	if customer != nil {
		customerName = customer.Name
		referenceID = &customer.ID
		customerVATNo = &customer.VATNo
		customerPhone = &customer.Phone
	} else {
		customerName = "Customer Accounts - Unknown"
		referenceID = nil
	}

	customerAccount, err := store.CreateAccountIfNotExists(
		ret.StoreID,
		referenceID,
		&referenceModel,
		customerName,
		customerPhone,
		customerVATNo,
	)
	if err != nil {
		return nil, err
	}

	journals = append(journals, Journal{
		Date:          lastPaymentDate,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "debit",
		Debit:         ret.BalanceAmount * (-1),
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	for _, payment := range extraPayments {
		cashPayingAccount := Account{}
		if payment.ReferenceType == "customer_withdrawal" || payment.ReferenceType == "non_vat_sales" {
			continue
		} else if payment.Method == "cash" {
			cashPayingAccount = *cashAccount
		} else if slices.Contains(BANK_PAYMENT_METHODS, payment.Method) {
			cashPayingAccount = *bankAccount
		} else if payment.Method == "customer_account" && customer != nil {
			continue
		}

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     cashPayingAccount.ID,
			AccountNumber: cashPayingAccount.Number,
			AccountName:   cashPayingAccount.Name,
			DebitOrCredit: "credit",
			Credit:        payment.Amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	}

	return journals, nil
}

type CustomerNonVATSalesReturnStats struct {
	NonVATSalesReturnCount              int64   `json:"non_vat_sales_return_count" bson:"non_vat_sales_return_count"`
	NonVATSalesReturnAmount             float64 `json:"non_vat_sales_return_amount" bson:"non_vat_sales_return_amount"`
	NonVATSalesReturnPaidAmount         float64 `json:"non_vat_sales_return_paid_amount" bson:"non_vat_sales_return_paid_amount"`
	NonVATSalesReturnBalanceAmount      float64 `json:"non_vat_sales_return_balance_amount" bson:"non_vat_sales_return_balance_amount"`
	NonVATSalesReturnProfit             float64 `json:"non_vat_sales_return_profit" bson:"non_vat_sales_return_profit"`
	NonVATSalesReturnLoss               float64 `json:"non_vat_sales_return_loss" bson:"non_vat_sales_return_loss"`
	NonVATSalesReturnPaidCount          int64   `json:"non_vat_sales_return_paid_count" bson:"non_vat_sales_return_paid_count"`
	NonVATSalesReturnNotPaidCount       int64   `json:"non_vat_sales_return_not_paid_count" bson:"non_vat_sales_return_not_paid_count"`
	NonVATSalesReturnPaidPartiallyCount int64   `json:"non_vat_sales_return_paid_partially_count" bson:"non_vat_sales_return_paid_partially_count"`
}

func (customer *Customer) SetCustomerNonVATSalesReturnStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("non_vat_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats CustomerNonVATSalesReturnStats

	filter := map[string]interface{}{
		"store_id":    storeID,
		"customer_id": customer.ID,
		"deleted":     bson.M{"$ne": true},
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                                 nil,
				"non_vat_sales_return_count":          bson.M{"$sum": 1},
				"non_vat_sales_return_amount":         bson.M{"$sum": "$net_total"},
				"non_vat_sales_return_paid_amount":    bson.M{"$sum": "$total_payment_paid"},
				"non_vat_sales_return_balance_amount": bson.M{"$sum": "$balance_amount"},
				"non_vat_sales_return_profit":         bson.M{"$sum": "$net_profit"},
				"non_vat_sales_return_loss":           bson.M{"$sum": "$loss"},
				"non_vat_sales_return_paid_count": bson.M{"$sum": bson.M{
					"$cond": bson.M{
						"if": bson.M{
							"$eq": []string{
								"$payment_status",
								"paid",
							},
						},
						"then": 1,
						"else": 0,
					},
				}},
				"non_vat_sales_return_not_paid_count": bson.M{"$sum": bson.M{
					"$cond": bson.M{
						"if": bson.M{
							"$eq": []string{
								"$payment_status",
								"not_paid",
							},
						},
						"then": 1,
						"else": 0,
					},
				}},
				"non_vat_sales_return_paid_partially_count": bson.M{"$sum": bson.M{
					"$cond": bson.M{
						"if": bson.M{
							"$eq": []string{
								"$payment_status",
								"paid_partially",
							},
						},
						"then": 1,
						"else": 0,
					},
				}},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}

	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return err
		}
		stats.NonVATSalesReturnAmount = RoundFloat(stats.NonVATSalesReturnAmount, 2)
		stats.NonVATSalesReturnPaidAmount = RoundFloat(stats.NonVATSalesReturnPaidAmount, 2)
		stats.NonVATSalesReturnBalanceAmount = RoundFloat(stats.NonVATSalesReturnBalanceAmount, 2)
		stats.NonVATSalesReturnProfit = RoundFloat(stats.NonVATSalesReturnProfit, 2)
		stats.NonVATSalesReturnLoss = RoundFloat(stats.NonVATSalesReturnLoss, 2)
	}

	store, err := FindStoreByID(&storeID, bson.M{})
	if err != nil {
		return errors.New("error finding store: " + err.Error())
	}

	if len(customer.Stores) == 0 {
		customer.Stores = map[string]CustomerStore{}
	}

	if customerStore, ok := customer.Stores[storeID.Hex()]; ok {
		customerStore.StoreID = storeID
		customerStore.StoreName = store.Name
		customerStore.StoreNameInArabic = store.NameInArabic
		customerStore.NonVATSalesReturnCount = stats.NonVATSalesReturnCount
		customerStore.NonVATSalesReturnAmount = stats.NonVATSalesReturnAmount
		customerStore.NonVATSalesReturnPaidAmount = stats.NonVATSalesReturnPaidAmount
		customerStore.NonVATSalesReturnBalanceAmount = stats.NonVATSalesReturnBalanceAmount
		customerStore.NonVATSalesReturnProfit = stats.NonVATSalesReturnProfit
		customerStore.NonVATSalesReturnLoss = stats.NonVATSalesReturnLoss
		customerStore.NonVATSalesReturnPaidCount = stats.NonVATSalesReturnPaidCount
		customerStore.NonVATSalesReturnNotPaidCount = stats.NonVATSalesReturnNotPaidCount
		customerStore.NonVATSalesReturnPaidPartiallyCount = stats.NonVATSalesReturnPaidPartiallyCount
		customer.Stores[storeID.Hex()] = customerStore
	} else {
		customer.Stores[storeID.Hex()] = CustomerStore{
			StoreID:                             storeID,
			StoreName:                           store.Name,
			StoreNameInArabic:                   store.NameInArabic,
			NonVATSalesReturnCount:              stats.NonVATSalesReturnCount,
			NonVATSalesReturnAmount:             stats.NonVATSalesReturnAmount,
			NonVATSalesReturnPaidAmount:         stats.NonVATSalesReturnPaidAmount,
			NonVATSalesReturnBalanceAmount:      stats.NonVATSalesReturnBalanceAmount,
			NonVATSalesReturnProfit:             stats.NonVATSalesReturnProfit,
			NonVATSalesReturnLoss:               stats.NonVATSalesReturnLoss,
			NonVATSalesReturnPaidCount:          stats.NonVATSalesReturnPaidCount,
			NonVATSalesReturnNotPaidCount:       stats.NonVATSalesReturnNotPaidCount,
			NonVATSalesReturnPaidPartiallyCount: stats.NonVATSalesReturnPaidPartiallyCount,
		}
	}

	err = customer.Update()
	if err != nil {
		return errors.New("Error updating customer: " + err.Error())
	}

	return nil
}

func (ret *NonVATSalesReturn) SetCustomerNonVATSalesReturnStats() error {
	store, err := FindStoreByID(ret.StoreID, bson.M{})
	if err != nil {
		return err
	}

	customer, err := store.FindCustomerByID(ret.CustomerID, map[string]interface{}{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	if customer != nil {
		err = customer.SetCustomerNonVATSalesReturnStatsByStoreID(*ret.StoreID)
		if err != nil {
			return err
		}
	}

	return nil
}

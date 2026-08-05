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

// NonVATSales is a standalone non-VAT sales document stored in the non_vat_sales collection.
// Its JSON shape mirrors Quotation so that QuotationType3Form can be reused on the frontend.
type NonVATSales struct {
	ID                     primitive.ObjectID   `json:"id,omitempty" bson:"_id,omitempty"`
	Code                   string               `bson:"code,omitempty" json:"code,omitempty"`
	Date                   *time.Time           `bson:"date,omitempty" json:"date,omitempty"`
	DateStr                string               `json:"date_str,omitempty" bson:"-"`
	StoreID                *primitive.ObjectID  `json:"store_id,omitempty" bson:"store_id,omitempty"`
	Store                  *Store               `json:"store,omitempty" bson:"-"`
	CustomerID             *primitive.ObjectID  `json:"customer_id" bson:"customer_id"`
	Customer               *Customer            `json:"customer" bson:"-"`
	CustomerName           string               `json:"customer_name" bson:"customer_name"`
	CustomerNameArabic     string               `json:"customer_name_arabic" bson:"customer_name_arabic"`
	Products               []QuotationProduct   `bson:"products,omitempty" json:"products,omitempty"`
	VatPercent             *float64             `bson:"vat_percent" json:"vat_percent"`
	Discount               float64              `bson:"discount" json:"discount"`
	DiscountPercent        float64              `bson:"discount_percent" json:"discount_percent"`
	DiscountWithVAT        float64              `bson:"discount_with_vat" json:"discount_with_vat"`
	DiscountPercentWithVAT float64              `bson:"discount_percent_with_vat" json:"discount_percent_with_vat"`
	Total                  float64              `bson:"total" json:"total"`
	TotalWithVAT           float64              `bson:"total_with_vat" json:"total_with_vat"`
	NetTotal               float64              `bson:"net_total" json:"net_total"`
	ActualTotal            float64              `bson:"actual_total" json:"actual_total"`
	ActualTotalWithVAT     float64              `bson:"actual_total_with_vat" json:"actual_total_with_vat"`
	ActualNetTotal         float64              `bson:"actual_net_total" json:"actual_net_total"`
	VatPrice               float64              `bson:"vat_price" json:"vat_price"`
	ActualVatPrice         float64              `bson:"actual_vat_price" json:"actual_vat_price"`
	RoundingAmount         float64              `bson:"rounding_amount" json:"rounding_amount"`
	AutoRoundingAmount     bool                 `bson:"auto_rounding_amount" json:"auto_rounding_amount"`
	TotalQuantity          float64              `bson:"total_quantity" json:"total_quantity"`
	PaymentStatus          string               `bson:"payment_status" json:"payment_status"`
	Payments               []QuotationPayment   `bson:"payments" json:"payments"`
	PaymentsInput          []QuotationPayment   `bson:"-" json:"payments_input"`
	PaymentsCount          int64                `bson:"payments_count" json:"payments_count"`
	TotalPaymentReceived   float64              `bson:"total_payment_received" json:"total_payment_received"`
	BalanceAmount          float64              `bson:"balance_amount" json:"balance_amount"`
	CashDiscount           float64              `bson:"cash_discount" json:"cash_discount"`
	PaymentMethods         []string             `json:"payment_methods" bson:"payment_methods"`
	ShippingOrHandlingFees float64              `bson:"shipping_handling_fees" json:"shipping_handling_fees"`
	Remarks                string               `bson:"remarks" json:"remarks"`
	Phone                  string               `bson:"phone" json:"phone"`
	VatNo                  string               `bson:"vat_no" json:"vat_no"`
	Address                string               `bson:"address" json:"address"`
	Status                 string               `bson:"status,omitempty" json:"status,omitempty"`
	ReturnCount            int64                `bson:"return_count" json:"return_count"`
	ReturnAmount           float64              `bson:"return_amount" json:"return_amount"`
	Profit                 float64              `bson:"profit" json:"profit"`
	NetProfit              float64              `bson:"net_profit" json:"net_profit"`
	Loss                   float64              `bson:"loss" json:"loss"`
	NetLoss                float64              `bson:"net_loss" json:"net_loss"`
	CreatedAt              *time.Time           `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt              *time.Time           `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy              *primitive.ObjectID  `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy              *primitive.ObjectID  `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser          *User                `json:"created_by_user,omitempty" bson:"-"`
	UpdatedByUser          *User                `json:"updated_by_user,omitempty" bson:"-"`
	StoreName              string               `json:"store_name,omitempty" bson:"store_name,omitempty"`
	CreatedByName          string               `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName          string               `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName          string               `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	Deleted                bool                 `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy              *primitive.ObjectID  `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser          *User                `json:"deleted_by_user,omitempty" bson:"-"`
	DeletedAt              *time.Time           `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	VehicleID              *primitive.ObjectID  `json:"vehicle_id,omitempty" bson:"vehicle_id,omitempty"`
	VehicleSnapshot        *VehicleSnapshot     `json:"vehicle_snapshot,omitempty" bson:"vehicle_snapshot,omitempty"`
	KmDriven               float64              `json:"km_driven" bson:"km_driven"`
	RepairJobID            *primitive.ObjectID  `json:"repair_job_id,omitempty" bson:"repair_job_id,omitempty"`
	RepairJobIDs           []primitive.ObjectID `json:"repair_job_ids,omitempty" bson:"repair_job_ids,omitempty"`
	ExcludeServiceTax      bool                 `json:"exclude_service_tax" bson:"exclude_service_tax"`
	ExcludeProductTax      bool                 `json:"exclude_product_tax" bson:"exclude_product_tax"`
}

// ---- Calculation helpers ----

func (s *NonVATSales) FindTotalQuantity() {
	total := float64(0)
	for _, p := range s.Products {
		total += p.Quantity
	}
	s.TotalQuantity = total
}

func (s *NonVATSales) FindTotal() {
	total := float64(0)
	totalWithVAT := float64(0)
	actualTotal := float64(0)
	actualTotalWithVAT := float64(0)
	for i, p := range s.Products {
		excludeVAT := (p.IsService && s.ExcludeServiceTax) || (!p.IsService && s.ExcludeProductTax)
		if excludeVAT {
			s.Products[i].UnitPriceWithVAT = s.Products[i].UnitPrice
			s.Products[i].UnitDiscountWithVAT = s.Products[i].UnitDiscount
		} else if p.UnitPriceWithVAT == 0 && p.UnitPrice > 0 && s.VatPercent != nil && *s.VatPercent > 0 {
			s.Products[i].UnitPriceWithVAT = RoundTo2Decimals(p.UnitPrice * (1 + (*s.VatPercent / 100)))
		}
		total += p.Quantity * (s.Products[i].UnitPrice - p.UnitDiscount)
		total = RoundTo2Decimals(total)
		totalWithVAT += p.Quantity * (s.Products[i].UnitPriceWithVAT - p.UnitDiscountWithVAT)
		totalWithVAT = RoundTo2Decimals(totalWithVAT)

		//Actual
		actualTotal += p.Quantity * (s.Products[i].UnitPrice - p.UnitDiscount)
		actualTotal = RoundTo8Decimals(actualTotal)
		actualTotalWithVAT += p.Quantity * (s.Products[i].UnitPriceWithVAT - p.UnitDiscountWithVAT)
		actualTotalWithVAT = RoundTo8Decimals(actualTotalWithVAT)
	}
	s.Total = total
	s.TotalWithVAT = totalWithVAT
	s.ActualTotal = actualTotal
	s.ActualTotalWithVAT = actualTotalWithVAT
}

func (s *NonVATSales) FindVatPrice() {
	s.VatPrice = RoundTo2Decimals(s.TotalWithVAT - s.Total)
	s.ActualVatPrice = RoundTo2Decimals(s.ActualTotalWithVAT - s.ActualTotal)
}

func (s *NonVATSales) FindNetTotal() error {
	store, err := FindStoreByID(s.StoreID, bson.M{})
	if err != nil {
		return err
	}
	if s.VatPercent == nil {
		vatPercent := store.VatPercent
		s.VatPercent = &vatPercent
	}

	s.ShippingOrHandlingFees = RoundTo2Decimals(s.ShippingOrHandlingFees)
	s.Discount = RoundTo2Decimals(s.Discount)

	s.FindTotal()
	s.FindVatPrice()

	netTotal := s.TotalWithVAT - s.Discount - s.CashDiscount + s.ShippingOrHandlingFees
	s.NetTotal = RoundTo2Decimals(netTotal)

	//Actual
	actualNetTotal := s.ActualTotalWithVAT - s.Discount - s.CashDiscount + s.ShippingOrHandlingFees
	s.ActualNetTotal = RoundTo2Decimals(actualNetTotal)

	if s.AutoRoundingAmount {
		s.RoundingAmount = RoundTo2Decimals(s.ActualNetTotal - s.NetTotal)
	}

	s.NetTotal = RoundTo2Decimals(s.NetTotal + s.RoundingAmount)

	return nil
}

func (s *NonVATSales) CalculateProfit() {
	totalProfit := 0.0
	totalLoss := 0.0
	for i, p := range s.Products {
		quantity := p.Quantity
		salesPrice := quantity * (p.UnitPrice - p.UnitDiscount)
		purchaseUnitPrice := p.PurchaseUnitPrice
		profit := 0.0
		if purchaseUnitPrice > 0 {
			profit = salesPrice - (quantity * purchaseUnitPrice)
		}
		if profit >= 0 {
			s.Products[i].Profit = profit
			s.Products[i].Loss = 0
			totalProfit += profit
		} else {
			s.Products[i].Profit = 0
			loss := profit * -1
			s.Products[i].Loss = loss
			totalLoss += loss
		}
	}
	s.Profit = totalProfit
	s.NetProfit = totalProfit - s.Discount
	s.Loss = totalLoss
	s.NetLoss = totalLoss
	if s.NetProfit < 0 {
		s.NetLoss += (s.NetProfit * -1)
		s.NetProfit = 0
	}
}

// ---- Foreign label update ----

func (s *NonVATSales) UpdateForeignLabelFields() error {
	if s.StoreID != nil {
		store, err := FindStoreByID(s.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		s.StoreName = store.Name
	}

	store, err := FindStoreByID(s.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if s.CustomerID != nil && !s.CustomerID.IsZero() {
		customer, err := store.FindCustomerByID(s.CustomerID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1})
		if err != nil {
			return err
		}
		s.CustomerName = customer.Name
		s.CustomerNameArabic = customer.NameInArabic
	}

	if s.CreatedBy != nil {
		u, err := FindUserByID(s.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		s.CreatedByName = u.Name
	}

	if s.UpdatedBy != nil {
		u, err := FindUserByID(s.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		s.UpdatedByName = u.Name
	}

	if s.DeletedBy != nil && !s.DeletedBy.IsZero() {
		u, err := FindUserByID(s.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		s.DeletedByName = u.Name
	}

	for i, product := range s.Products {
		productObject, err := store.FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1, "part_number": 1, "prefix_part_number": 1, "is_service": 1})
		if err != nil {
			return err
		}
		s.Products[i].NameInArabic = productObject.NameInArabic
		s.Products[i].ItemCode = productObject.ItemCode
		if productObject.PartNumber != "" {
			s.Products[i].PartNumber = productObject.PartNumber
		}
		s.Products[i].PrefixPartNumber = productObject.PrefixPartNumber
		s.Products[i].IsService = productObject.IsService
	}

	return nil
}

// ---- Validation ----

func (s *NonVATSales) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(s.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store id"
		return errs
	}

	customer, err := store.FindCustomerByID(s.CustomerID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		errs["customer_id"] = "Invalid customer"
		return errs
	}
	if customer == nil && govalidator.IsNull(s.CustomerName) {
		s.CustomerID = nil
	}

	if govalidator.IsNull(s.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, s.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		s.Date = &date
	}

	if !govalidator.IsNull(strings.TrimSpace(s.Phone)) && !ValidateSaudiPhone(strings.TrimSpace(s.Phone)) {
		errs["phone"] = "Invalid phone no."
		return
	}

	if scenario == "update" {
		if s.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsNonVATSalesExists(&s.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}
		if !exists {
			errs["id"] = "Invalid Non VAT Sales:" + s.ID.Hex()
		}
	}

	totalPayment := float64(0.00)
	for _, payment := range s.PaymentsInput {
		if payment.Amount > 0 {
			totalPayment += payment.Amount
		}
	}
	if totalPayment > RoundTo2Decimals(s.NetTotal-s.CashDiscount) {
		errs["total_payment"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", RoundTo2Decimals(s.NetTotal-s.CashDiscount))
	}

	return errs
}

func (store *Store) IsNonVATSalesExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("non_vat_sales")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count, err := collection.CountDocuments(ctx, bson.M{"_id": ID, "store_id": store.ID, "deleted": bson.M{"$ne": true}})
	return count > 0, err
}

// ---- Serial number / code generation ----

func (s *NonVATSales) MakeRedisCode() error {
	store, err := FindStoreByID(s.StoreID, bson.M{})
	if err != nil {
		return err
	}

	serialNum := store.NonVATSalesSerialNumber
	if serialNum.StartFromCount <= 0 {
		serialNum.StartFromCount = 1
	}
	if serialNum.PaddingCount <= 0 {
		serialNum.PaddingCount = 3
	}
	redisKey := s.StoreID.Hex() + "_non_vat_sales_counter"

	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	baseTime := s.CreatedAt.In(location)

	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		count, err := store.GetCountByCollection("non_vat_sales")
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
			monthlyCount, err := store.GetCountByCollectionInRange(fromDate, toDate, "non_vat_sales")
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
		s.Code = fmt.Sprintf("%s-%0*d", serialNum.Prefix, paddingCount, serialNumber)
	} else {
		s.Code = fmt.Sprintf("%0*d", paddingCount, serialNumber)
	}

	if strings.Contains(s.Code, "DATE") {
		s.Code = strings.ReplaceAll(s.Code, "DATE", baseTime.Format("20060102"))
	}

	return nil
}

func (s *NonVATSales) UnMakeRedisCode() error {
	store, err := FindStoreByID(s.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := s.StoreID.Hex() + "_non_vat_sales_counter"

	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}
	baseTime := s.CreatedAt.In(location)

	if exists, err := db.RedisClient.Exists(redisKey).Result(); err == nil && exists != 0 {
		if _, err := db.RedisClient.Decr(redisKey).Result(); err != nil {
			return err
		}
	}

	if strings.Contains(store.NonVATSalesSerialNumber.Prefix, "DATE") {
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

// ProcessPayments copies PaymentsInput → Payments and computes payment summary fields.
func (s *NonVATSales) ProcessPayments() {
	payments := []QuotationPayment{}
	methods := []string{}
	methodSeen := map[string]bool{}
	total := float64(0)

	for _, p := range s.PaymentsInput {
		if p.Deleted || p.Amount <= 0 {
			continue
		}
		// Parse DateStr into Date when the time.Time pointer is not already set.
		if p.Date == nil && p.DateStr != "" {
			const shortForm = "2006-01-02T15:04:05Z07:00"
			if t, err := time.Parse(shortForm, p.DateStr); err == nil {
				p.Date = &t
			}
		}
		// Ensure the payment has an ID (for future edits to reference it).
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

	s.Payments = payments
	s.PaymentsCount = int64(len(payments))
	s.TotalPaymentReceived = RoundTo2Decimals(total)
	s.PaymentMethods = methods

	effective := RoundTo2Decimals(s.NetTotal - s.CashDiscount)
	s.BalanceAmount = RoundTo2Decimals(effective - total)
	if total <= 0 {
		s.PaymentStatus = "not_paid"
	} else if s.BalanceAmount <= 0 {
		s.PaymentStatus = "paid"
	} else {
		s.PaymentStatus = "paid_partially"
	}
}

// ---- CRUD ----

func (s *NonVATSales) Insert() error {
	collection := db.GetDB("store_" + s.StoreID.Hex()).Collection("non_vat_sales")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.ID = primitive.NewObjectID()
	_, err := collection.InsertOne(ctx, s)
	return err
}

func (s *NonVATSales) Update() error {
	collection := db.GetDB("store_" + s.StoreID.Hex()).Collection("non_vat_sales")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	_, err := collection.UpdateOne(ctx, bson.M{"_id": s.ID}, bson.M{"$set": s}, updateOptions)
	return err
}

func (s *NonVATSales) Delete(tokenClaims TokenClaims) error {
	err := s.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	s.Deleted = true
	s.DeletedBy = &userID
	now := time.Now()
	s.DeletedAt = &now

	collection := db.GetDB("store_" + s.StoreID.Hex()).Collection("non_vat_sales")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	_, err = collection.UpdateOne(ctx, bson.M{"_id": s.ID}, bson.M{"$set": s}, updateOptions)
	return err
}

func (store *Store) FindNonVATSalesByID(ID *primitive.ObjectID, selectFields map[string]interface{}) (*NonVATSales, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("non_vat_sales")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	var s NonVATSales
	err := collection.FindOne(ctx, bson.M{"_id": ID, "store_id": store.ID}, findOneOptions).Decode(&s)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["store.id"]; ok {
		storeSelectFields := ParseRelationalSelectString(selectFields, "store")
		s.Store, _ = FindStoreByID(s.StoreID, storeSelectFields)
	}
	if _, ok := selectFields["customer.id"]; ok {
		customerSelectFields := ParseRelationalSelectString(selectFields, "customer")
		s.Customer, _ = store.FindCustomerByID(s.CustomerID, customerSelectFields)
	}
	if _, ok := selectFields["created_by_user.id"]; ok {
		createdBySelectFields := ParseRelationalSelectString(selectFields, "created_by_user")
		s.CreatedByUser, _ = FindUserByID(s.CreatedBy, createdBySelectFields)
	}

	return &s, nil
}

// ---- Search / List ----

type NonVATSalesStats struct {
	NetTotal  float64 `json:"net_total" bson:"net_total"`
	NetProfit float64 `json:"net_profit" bson:"net_profit"`
	NetLoss   float64 `json:"net_loss" bson:"net_loss"`
	VatPrice  float64 `json:"vat_price" bson:"vat_price"`
	Discount  float64 `json:"discount" bson:"discount"`
}

func (store *Store) GetNonVATSalesStats(filter map[string]interface{}) (stats NonVATSalesStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("non_vat_sales")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		{"$match": filter},
		{"$group": bson.M{
			"_id":        nil,
			"net_total":  bson.M{"$sum": "$net_total"},
			"net_profit": bson.M{"$sum": "$net_profit"},
			"net_loss":   bson.M{"$sum": "$net_loss"},
			"vat_price":  bson.M{"$sum": "$vat_price"},
			"discount":   bson.M{"$sum": "$discount"},
		}},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return stats, err
	}
	defer cur.Close(ctx)
	if cur.Next(ctx) {
		cur.Decode(&stats)
	}
	return stats, nil
}

func (store *Store) SearchNonVATSales(w http.ResponseWriter, r *http.Request) (items []NonVATSales, criterias SearchCriterias, err error) {
	criterias = InitSearchCriterias()
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	timeZoneOffset := CountryTimezoneOffset(store.CountryCode)

	if err = ParseExactDateFilter(r, &criterias, "search[date_str]", "date", timeZoneOffset); err != nil {
		return items, criterias, err
	}
	if err = ParseDateRangeFilter(r, &criterias, "search[from_date]", "search[to_date]", "date", timeZoneOffset); err != nil {
		return items, criterias, err
	}
	if err = ParseExactDateFilter(r, &criterias, "search[created_at]", "created_at", timeZoneOffset); err != nil {
		return items, criterias, err
	}

	if err = ParseObjectIDFilter(r, &criterias, "search[store_id]", "store_id"); err != nil {
		return items, criterias, err
	}
	if err = ParseObjectIDFilter(r, &criterias, "search[customer_id]", "customer_id"); err != nil {
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("non_vat_sales")
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
	updatedByUserSelectFields := map[string]interface{}{}

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
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			updatedByUserSelectFields = ParseRelationalSelectString(keys[0], "updated_by_user")
		}
	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return items, criterias, errors.New("Error fetching non_vat_sales:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for cur != nil && cur.Next(ctx) {
		if err := cur.Err(); err != nil {
			return items, criterias, errors.New("Cursor error:" + err.Error())
		}
		var item NonVATSales
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
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			item.UpdatedByUser, _ = FindUserByID(item.UpdatedBy, updatedByUserSelectFields)
		}
		items = append(items, item)
	}

	return items, criterias, nil
}

func (store *Store) FindLastNonVATSaleByStoreID(storeID *primitive.ObjectID, selectFields map[string]interface{}) (sale *NonVATSales, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("non_vat_sales")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx, bson.M{"store_id": store.ID}, findOneOptions).Decode(&sale)
	if err != nil {
		return nil, err
	}
	return sale, err
}

func (s *NonVATSales) FindNextNonVATSale(selectFields map[string]interface{}) (next *NonVATSales, err error) {
	collection := db.GetDB("store_" + s.StoreID.Hex()).Collection("non_vat_sales")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	findOneOptions.SetSort(bson.M{"date": 1})
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx, bson.M{
		"date":     bson.M{"$gte": s.Date},
		"_id":      bson.M{"$ne": s.ID},
		"store_id": s.StoreID,
	}, findOneOptions).Decode(&next)
	if err != nil {
		return nil, err
	}
	return next, nil
}

func (s *NonVATSales) FindPreviousNonVATSale(selectFields map[string]interface{}) (prev *NonVATSales, err error) {
	collection := db.GetDB("store_" + s.StoreID.Hex()).Collection("non_vat_sales")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	findOneOptions.SetSort(bson.M{"date": -1})
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx, bson.M{
		"date":     bson.M{"$lte": s.Date},
		"_id":      bson.M{"$ne": s.ID},
		"store_id": s.StoreID,
	}, findOneOptions).Decode(&prev)
	if err != nil {
		return nil, err
	}
	return prev, nil
}

type ProductNonVATSalesStats struct {
	NonVATSalesQuantity float64 `json:"non_vat_sales_quantity" bson:"non_vat_sales_quantity"`
}

func (product *Product) SetProductNonVATSalesQuantityByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_non_vat_sales_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductNonVATSalesStats

	filter := map[string]interface{}{
		"store_id":   storeID,
		"product_id": product.ID,
	}

	pipeline := []bson.M{
		{"$match": filter},
		{"$group": bson.M{
			"_id":                    nil,
			"non_vat_sales_quantity": bson.M{"$sum": "$quantity"},
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
		productStoreTemp.NonVATSalesQuantity = stats.NonVATSalesQuantity
		product.ProductStores[storeID.Hex()] = productStoreTemp
	}

	return nil
}

func (s *NonVATSales) SetProductsStock() error {
	store, err := FindStoreByID(s.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if len(s.Products) == 0 {
		return nil
	}

	for _, p := range s.Products {
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

var extraNonVATSalesPayments []QuotationPayment
var totalNonVATSalesPaidAmount float64
var extraNonVATSalesAmountPaid float64

func (s *NonVATSales) AdjustPayments() error {
	if len(s.Payments) == 0 || s.Date == nil {
		return nil
	}

	firstPayment := s.Payments[0]
	if firstPayment.Date != nil && firstPayment.Date.Equal(*s.Date) {
		newTime := s.Date.Add(1 * time.Minute)
		s.Payments[0].Date = &newTime
	}

	for i := 1; i < len(s.Payments); i++ {
		prev := s.Payments[i-1].Date
		curr := s.Payments[i].Date
		if prev != nil && curr != nil && (curr.Equal(*prev) || curr.Before(*prev)) {
			newTime := prev.Add(1 * time.Minute)
			s.Payments[i].Date = &newTime
		}
	}

	return s.Update()
}

func (s *NonVATSales) DoAccounting() error {
	err := s.AdjustPayments()
	if err != nil {
		return errors.New("error adjusting payments: " + err.Error())
	}

	ledger, err := s.CreateLedger()
	if err != nil {
		return errors.New("error creating ledger: " + err.Error())
	}

	_, err = ledger.CreatePostings()
	if err != nil {
		return errors.New("error creating postings: " + err.Error())
	}

	return nil
}

func (s *NonVATSales) UndoAccounting() error {
	store, err := FindStoreByID(s.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledger, err := store.FindLedgerByReferenceID(s.ID, *s.StoreID, bson.M{})
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

	err = store.RemoveLedgerByReferenceID(s.ID)
	if err != nil {
		return errors.New("Error removing ledger by reference id: " + err.Error())
	}

	err = store.RemovePostingsByReferenceID(s.ID)
	if err != nil {
		return errors.New("Error removing postings by reference id: " + err.Error())
	}

	err = SetAccountBalances(ledgerAccounts)
	if err != nil {
		return errors.New("Error setting account balances: " + err.Error())
	}

	return nil
}

func (s *NonVATSales) CreateLedger() (ledger *Ledger, err error) {
	store, err := FindStoreByID(s.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var customer *Customer

	if s.CustomerID != nil && !s.CustomerID.IsZero() {
		customer, err = store.FindCustomerByID(s.CustomerID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}
	}

	cashAccount, err := store.CreateAccountIfNotExists(s.StoreID, nil, nil, "Cash", nil, nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := store.CreateAccountIfNotExists(s.StoreID, nil, nil, "Bank", nil, nil)
	if err != nil {
		return nil, err
	}

	nonVATSalesAccount, err := store.CreateAccountIfNotExists(s.StoreID, nil, nil, "Non VAT Sales", nil, nil)
	if err != nil {
		return nil, err
	}

	cashDiscountAllowedAccount, err := store.CreateAccountIfNotExists(s.StoreID, nil, nil, "Cash discount allowed", nil, nil)
	if err != nil {
		return nil, err
	}

	journals := []Journal{}

	var firstPaymentDate *time.Time
	if len(s.Payments) > 0 {
		firstPaymentDate = s.Payments[0].Date
	}

	if len(s.Payments) == 0 || (firstPaymentDate != nil && !IsDateTimesEqual(s.Date, firstPaymentDate)) {
		customerName := ""
		var referenceID *primitive.ObjectID
		customerVATNo := ""
		customerPhone := ""
		if customer != nil {
			customerName = customer.Name
			referenceID = &customer.ID
			customerVATNo = customer.VATNo
			customerPhone = customer.Phone
		} else {
			customerName = "Customer Accounts - Unknown"
			referenceID = nil
		}

		referenceModel := "customer"
		customerAccount, err := store.CreateAccountIfNotExists(
			s.StoreID,
			referenceID,
			&referenceModel,
			customerName,
			&customerPhone,
			&customerVATNo,
		)
		if err != nil {
			return nil, err
		}
		journals = append(journals, MakeJournalsForUnpaidNonVATSale(
			s,
			customerAccount,
			nonVATSalesAccount,
			cashDiscountAllowedAccount,
		)...)
	}

	if len(s.Payments) > 0 {
		totalNonVATSalesPaidAmount = float64(0.00)
		extraNonVATSalesAmountPaid = float64(0.00)
		extraNonVATSalesPayments = []QuotationPayment{}

		paymentsByDatetimeNumber := 1
		paymentsByDatetime := RegroupNonVATSalesPaymentsByDatetime(s.Payments)

		for _, paymentByDatetime := range paymentsByDatetime {
			newJournals, err := MakeJournalsForNonVATSalesPaymentsByDatetime(
				s,
				customer,
				cashAccount,
				bankAccount,
				nonVATSalesAccount,
				paymentByDatetime,
				cashDiscountAllowedAccount,
				paymentsByDatetimeNumber,
			)
			if err != nil {
				return nil, err
			}

			journals = append(journals, newJournals...)
			paymentsByDatetimeNumber++
		}

		if s.BalanceAmount < 0 && len(extraNonVATSalesPayments) > 0 {
			newJournals, err := MakeJournalsForNonVATSalesExtraPayments(
				s,
				customer,
				cashAccount,
				bankAccount,
				extraNonVATSalesPayments,
			)
			if err != nil {
				return nil, err
			}
			journals = append(journals, newJournals...)
		}

		totalNonVATSalesPaidAmount = float64(0.00)
		extraNonVATSalesAmountPaid = float64(0.00)
	}

	ledger = &Ledger{
		StoreID:        s.StoreID,
		ReferenceID:    s.ID,
		ReferenceModel: "non_vat_sales",
		ReferenceCode:  s.Code,
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

func MakeJournalsForUnpaidNonVATSale(
	s *NonVATSales,
	customerAccount *Account,
	nonVATSalesAccount *Account,
	cashDiscountAllowedAccount *Account,
) []Journal {
	now := time.Now()
	groupID := primitive.NewObjectID()
	journals := []Journal{}

	balanceAmount := RoundFloat((s.NetTotal - s.CashDiscount), 2)

	journals = append(journals, Journal{
		Date:          s.Date,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "debit",
		Debit:         balanceAmount,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	if s.CashDiscount > 0 {
		journals = append(journals, Journal{
			Date:          s.Date,
			AccountID:     cashDiscountAllowedAccount.ID,
			AccountNumber: cashDiscountAllowedAccount.Number,
			AccountName:   cashDiscountAllowedAccount.Name,
			DebitOrCredit: "debit",
			Debit:         s.CashDiscount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	}

	journals = append(journals, Journal{
		Date:          s.Date,
		AccountID:     nonVATSalesAccount.ID,
		AccountNumber: nonVATSalesAccount.Number,
		AccountName:   nonVATSalesAccount.Name,
		DebitOrCredit: "credit",
		Credit:        s.NetTotal,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals
}

func RegroupNonVATSalesPaymentsByDatetime(payments []QuotationPayment) [][]QuotationPayment {
	paymentsByDatetime := map[string][]QuotationPayment{}
	for _, payment := range payments {
		paymentsByDatetime[payment.Date.Format("2006-01-02T15:04")] = append(paymentsByDatetime[payment.Date.Format("2006-01-02T15:04")], payment)
	}

	paymentsByDatetime2 := [][]QuotationPayment{}
	for _, v := range paymentsByDatetime {
		paymentsByDatetime2 = append(paymentsByDatetime2, v)
	}

	sort.Slice(paymentsByDatetime2, func(i, j int) bool {
		return paymentsByDatetime2[i][0].Date.Before(*paymentsByDatetime2[j][0].Date)
	})

	return paymentsByDatetime2
}

func MakeJournalsForNonVATSalesPaymentsByDatetime(
	s *NonVATSales,
	customer *Customer,
	cashAccount *Account,
	bankAccount *Account,
	nonVATSalesAccount *Account,
	payments []QuotationPayment,
	cashDiscountAllowedAccount *Account,
	paymentsByDatetimeNumber int,
) ([]Journal, error) {
	store, err := FindStoreByID(s.StoreID, bson.M{})
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

	for _, payment := range payments {
		totalNonVATSalesPaidAmount += payment.Amount
		if totalNonVATSalesPaidAmount > (s.NetTotal - s.CashDiscount) {
			extraNonVATSalesAmountPaid = RoundFloat((totalNonVATSalesPaidAmount - (s.NetTotal - s.CashDiscount)), 2)
		}
		amount := payment.Amount

		if extraNonVATSalesAmountPaid > 0 {
			skip := false
			if extraNonVATSalesAmountPaid < payment.Amount {
				extraNonVATSalesPayments = append(extraNonVATSalesPayments, QuotationPayment{
					Date:   payment.Date,
					Amount: extraNonVATSalesAmountPaid,
					Method: payment.Method,
				})
				amount = RoundFloat((payment.Amount - extraNonVATSalesAmountPaid), 2)
				extraNonVATSalesAmountPaid = 0
			} else if extraNonVATSalesAmountPaid >= payment.Amount {
				extraNonVATSalesPayments = append(extraNonVATSalesPayments, QuotationPayment{
					Date:   payment.Date,
					Amount: payment.Amount,
					Method: payment.Method,
				})
				skip = true
				extraNonVATSalesAmountPaid = RoundFloat((extraNonVATSalesAmountPaid - payment.Amount), 2)
			}

			if skip {
				continue
			}
		}

		cashReceivingAccount := Account{}
		if payment.ReferenceType == "customer_deposit" || payment.ReferenceType == "non_vat_sales_return" {
			continue
		} else if payment.Method == "cash" {
			cashReceivingAccount = *cashAccount
		} else if slices.Contains(BANK_PAYMENT_METHODS, payment.Method) {
			cashReceivingAccount = *bankAccount
		} else if payment.Method == "customer_account" && customer != nil {
			continue
		}

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     cashReceivingAccount.ID,
			AccountNumber: cashReceivingAccount.Number,
			AccountName:   cashReceivingAccount.Name,
			DebitOrCredit: "debit",
			Debit:         amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
		totalPayment += amount
	}

	if s.CashDiscount > 0 && paymentsByDatetimeNumber == 1 && IsDateTimesEqual(s.Date, firstPaymentDate) {
		journals = append(journals, Journal{
			Date:          s.Date,
			AccountID:     cashDiscountAllowedAccount.ID,
			AccountNumber: cashDiscountAllowedAccount.Number,
			AccountName:   cashDiscountAllowedAccount.Name,
			DebitOrCredit: "debit",
			Debit:         s.CashDiscount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	}

	balanceAmount := RoundFloat(((s.NetTotal - s.CashDiscount) - totalPayment), 2)

	if paymentsByDatetimeNumber == 1 && balanceAmount > 0 && IsDateTimesEqual(s.Date, firstPaymentDate) {
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
			s.StoreID,
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
			Date:          s.Date,
			AccountID:     customerAccount.ID,
			AccountNumber: customerAccount.Number,
			AccountName:   customerAccount.Name,
			DebitOrCredit: "debit",
			Debit:         balanceAmount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	}

	if paymentsByDatetimeNumber == 1 && IsDateTimesEqual(s.Date, firstPaymentDate) {
		journals = append(journals, Journal{
			Date:          s.Date,
			AccountID:     nonVATSalesAccount.ID,
			AccountNumber: nonVATSalesAccount.Number,
			AccountName:   nonVATSalesAccount.Name,
			DebitOrCredit: "credit",
			Credit:        s.NetTotal,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	} else if paymentsByDatetimeNumber > 1 || !IsDateTimesEqual(s.Date, firstPaymentDate) {
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
			s.StoreID,
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
				DebitOrCredit: "credit",
				Credit:        totalPayment,
				GroupID:       groupID,
				CreatedAt:     &now,
				UpdatedAt:     &now,
			})
		}
	}

	return journals, nil
}

func MakeJournalsForNonVATSalesExtraPayments(
	s *NonVATSales,
	customer *Customer,
	cashAccount *Account,
	bankAccount *Account,
	extraPayments []QuotationPayment,
) ([]Journal, error) {
	store, err := FindStoreByID(s.StoreID, bson.M{})
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

	for _, payment := range extraPayments {
		cashReceivingAccount := Account{}
		if payment.ReferenceType == "customer_deposit" || payment.ReferenceType == "non_vat_sales_return" {
			continue
		} else if payment.Method == "cash" {
			cashReceivingAccount = *cashAccount
		} else if slices.Contains(BANK_PAYMENT_METHODS, payment.Method) {
			cashReceivingAccount = *bankAccount
		} else if payment.Method == "customer_account" && customer != nil {
			continue
		}

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     cashReceivingAccount.ID,
			AccountNumber: cashReceivingAccount.Number,
			AccountName:   cashReceivingAccount.Name,
			DebitOrCredit: "debit",
			Debit:         payment.Amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
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
		s.StoreID,
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
		DebitOrCredit: "credit",
		Credit:        s.BalanceAmount * (-1),
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals, nil
}

type CustomerNonVATSalesStats struct {
	NonVATSalesCount              int64   `json:"non_vat_sales_count" bson:"non_vat_sales_count"`
	NonVATSalesAmount             float64 `json:"non_vat_sales_amount" bson:"non_vat_sales_amount"`
	NonVATSalesPaidAmount         float64 `json:"non_vat_sales_paid_amount" bson:"non_vat_sales_paid_amount"`
	NonVATSalesBalanceAmount      float64 `json:"non_vat_sales_balance_amount" bson:"non_vat_sales_balance_amount"`
	NonVATSalesProfit             float64 `json:"non_vat_sales_profit" bson:"non_vat_sales_profit"`
	NonVATSalesLoss               float64 `json:"non_vat_sales_loss" bson:"non_vat_sales_loss"`
	NonVATSalesPaidCount          int64   `json:"non_vat_sales_paid_count" bson:"non_vat_sales_paid_count"`
	NonVATSalesNotPaidCount       int64   `json:"non_vat_sales_not_paid_count" bson:"non_vat_sales_not_paid_count"`
	NonVATSalesPaidPartiallyCount int64   `json:"non_vat_sales_paid_partially_count" bson:"non_vat_sales_paid_partially_count"`
}

func (customer *Customer) SetCustomerNonVATSalesStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("non_vat_sales")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats CustomerNonVATSalesStats

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
				"_id":                          nil,
				"non_vat_sales_count":          bson.M{"$sum": 1},
				"non_vat_sales_amount":         bson.M{"$sum": "$net_total"},
				"non_vat_sales_paid_amount":    bson.M{"$sum": "$total_payment_received"},
				"non_vat_sales_balance_amount": bson.M{"$sum": "$balance_amount"},
				"non_vat_sales_profit":         bson.M{"$sum": "$net_profit"},
				"non_vat_sales_loss":           bson.M{"$sum": "$loss"},
				"non_vat_sales_paid_count": bson.M{"$sum": bson.M{
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
				"non_vat_sales_not_paid_count": bson.M{"$sum": bson.M{
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
				"non_vat_sales_paid_partially_count": bson.M{"$sum": bson.M{
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
		stats.NonVATSalesAmount = RoundFloat(stats.NonVATSalesAmount, 2)
		stats.NonVATSalesPaidAmount = RoundFloat(stats.NonVATSalesPaidAmount, 2)
		stats.NonVATSalesBalanceAmount = RoundFloat(stats.NonVATSalesBalanceAmount, 2)
		stats.NonVATSalesProfit = RoundFloat(stats.NonVATSalesProfit, 2)
		stats.NonVATSalesLoss = RoundFloat(stats.NonVATSalesLoss, 2)
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
		customerStore.NonVATSalesCount = stats.NonVATSalesCount
		customerStore.NonVATSalesAmount = stats.NonVATSalesAmount
		customerStore.NonVATSalesPaidAmount = stats.NonVATSalesPaidAmount
		customerStore.NonVATSalesBalanceAmount = stats.NonVATSalesBalanceAmount
		customerStore.NonVATSalesProfit = stats.NonVATSalesProfit
		customerStore.NonVATSalesLoss = stats.NonVATSalesLoss
		customerStore.NonVATSalesPaidCount = stats.NonVATSalesPaidCount
		customerStore.NonVATSalesNotPaidCount = stats.NonVATSalesNotPaidCount
		customerStore.NonVATSalesPaidPartiallyCount = stats.NonVATSalesPaidPartiallyCount
		customer.Stores[storeID.Hex()] = customerStore
	} else {
		customer.Stores[storeID.Hex()] = CustomerStore{
			StoreID:                       storeID,
			StoreName:                     store.Name,
			StoreNameInArabic:             store.NameInArabic,
			NonVATSalesCount:              stats.NonVATSalesCount,
			NonVATSalesAmount:             stats.NonVATSalesAmount,
			NonVATSalesPaidAmount:         stats.NonVATSalesPaidAmount,
			NonVATSalesBalanceAmount:      stats.NonVATSalesBalanceAmount,
			NonVATSalesProfit:             stats.NonVATSalesProfit,
			NonVATSalesLoss:               stats.NonVATSalesLoss,
			NonVATSalesPaidCount:          stats.NonVATSalesPaidCount,
			NonVATSalesNotPaidCount:       stats.NonVATSalesNotPaidCount,
			NonVATSalesPaidPartiallyCount: stats.NonVATSalesPaidPartiallyCount,
		}
	}

	err = customer.Update()
	if err != nil {
		return errors.New("Error updating customer: " + err.Error())
	}

	return nil
}

func (s *NonVATSales) SetCustomerNonVATSalesStats() error {
	store, err := FindStoreByID(s.StoreID, bson.M{})
	if err != nil {
		return err
	}

	customer, err := store.FindCustomerByID(s.CustomerID, map[string]interface{}{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	if customer != nil {
		err = customer.SetCustomerNonVATSalesStatsByStoreID(*s.StoreID)
		if err != nil {
			return err
		}
	}

	return nil
}

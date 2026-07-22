package models

import (
	"context"
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

type PurchaseOrderProduct struct {
	ProductID                  primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	WarehouseID                *primitive.ObjectID `json:"warehouse_id" bson:"warehouse_id"`
	WarehouseCode              *string             `json:"warehouse_code" bson:"warehouse_code"`
	Name                       string              `bson:"name" json:"name"`
	NameInArabic               string              `bson:"name_in_arabic" json:"name_in_arabic"`
	ItemCode                   string              `bson:"item_code" json:"item_code"`
	PrefixPartNumber           string              `bson:"prefix_part_number" json:"prefix_part_number"`
	PartNumber                 string              `bson:"part_number" json:"part_number"`
	Quantity                   float64             `json:"quantity" bson:"quantity"`
	Unit                       string              `bson:"unit" json:"unit"`
	PurchaseUnitPrice          float64             `bson:"purchase_unit_price" json:"purchase_unit_price"`
	PurchaseUnitPriceWithVAT   float64             `bson:"purchase_unit_price_with_vat" json:"purchase_unit_price_with_vat"`
	UnitDiscount               float64             `bson:"unit_discount" json:"unit_discount"`
	UnitDiscountWithVAT        float64             `bson:"unit_discount_with_vat" json:"unit_discount_with_vat"`
	UnitDiscountPercent        float64             `bson:"unit_discount_percent" json:"unit_discount_percent"`
	UnitDiscountPercentWithVAT float64             `bson:"unit_discount_percent_with_vat" json:"unit_discount_percent_with_vat"`
	IsService                  bool                `bson:"is_service" json:"is_service"`
}

// PurchaseOrder : Purchase Order structure (no stock/payment effects)
type PurchaseOrder struct {
	ID                  primitive.ObjectID     `json:"id,omitempty" bson:"_id,omitempty"`
	Date                *time.Time             `bson:"date,omitempty" json:"date,omitempty"`
	DateStr             string                 `json:"date_str,omitempty" bson:"-"`
	ExpectedDate        *time.Time             `bson:"expected_date,omitempty" json:"expected_date,omitempty"`
	ExpectedDateStr     string                 `json:"expected_date_str,omitempty" bson:"-"`
	Code                string                 `bson:"code,omitempty" json:"code,omitempty"`
	StoreID             *primitive.ObjectID    `json:"store_id,omitempty" bson:"store_id,omitempty"`
	VendorID            *primitive.ObjectID    `json:"vendor_id" bson:"vendor_id"`
	VendorInvoiceNumber string                 `bson:"vendor_invoice_no,omitempty" json:"vendor_invoice_no,omitempty"`
	Store               *Store                 `json:"store,omitempty" bson:"-"`
	Vendor              *Vendor                `json:"vendor,omitempty" bson:"-"`
	Products            []PurchaseOrderProduct `bson:"products,omitempty" json:"products,omitempty"`
	OrderPlacedBy       *primitive.ObjectID    `json:"order_placed_by,omitempty" bson:"order_placed_by,omitempty"`
	// Status: draft | sent | confirmed | partially_received | received | cancelled
	Status                 string   `bson:"status,omitempty" json:"status,omitempty"`
	VatPercent             *float64 `bson:"vat_percent" json:"vat_percent"`
	Discount               float64  `bson:"discount" json:"discount"`
	DiscountWithVAT        float64  `bson:"discount_with_vat" json:"discount_with_vat"`
	DiscountPercent        float64  `bson:"discount_percent" json:"discount_percent"`
	DiscountPercentWithVAT float64  `bson:"discount_percent_with_vat" json:"discount_percent_with_vat"`
	ShippingOrHandlingFees float64  `bson:"shipping_handling_fees" json:"shipping_handling_fees"`
	TotalQuantity          float64  `bson:"total_quantity" json:"total_quantity"`
	VatPrice               float64  `bson:"vat_price" json:"vat_price"`
	Total                  float64  `bson:"total" json:"total"`
	TotalWithVAT           float64  `bson:"total_with_vat" json:"total_with_vat"`
	NetTotal               float64  `bson:"net_total" json:"net_total"`
	ActualVatPrice         float64  `bson:"actual_vat_price" json:"actual_vat_price"`
	ActualTotal            float64  `bson:"actual_total" json:"actual_total"`
	ActualTotalWithVAT     float64  `bson:"actual_total_with_vat" json:"actual_total_with_vat"`
	ActualNetTotal         float64  `bson:"actual_net_total" json:"actual_net_total"`
	RoundingAmount         float64  `bson:"rounding_amount" json:"rounding_amount"`
	AutoRoundingAmount     bool     `bson:"auto_rounding_amount" json:"auto_rounding_amount"`
	CreatedAt              *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt              *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy              *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy              *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	OrderPlacedByName      string              `json:"order_placed_by_name,omitempty" bson:"order_placed_by_name,omitempty"`
	VendorName             string              `json:"vendor_name" bson:"vendor_name"`
	VendorNameArabic       string              `json:"vendor_name_arabic" bson:"vendor_name_arabic"`
	StoreName              string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	CreatedByName          string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName          string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	Remarks                string              `bson:"remarks" json:"remarks"`
	Phone                  string              `bson:"phone" json:"phone"`
	VatNo                  string              `bson:"vat_no" json:"vat_no"`
	Address                string              `bson:"address" json:"address"`
	// Link to Purchase after conversion
	PurchaseID   *primitive.ObjectID `json:"purchase_id" bson:"purchase_id"`
	PurchaseCode *string             `json:"purchase_code" bson:"purchase_code"`
	// Link to Purchase Request that originated this PO
	PurchaseRequestID   *primitive.ObjectID `json:"purchase_request_id" bson:"purchase_request_id"`
	PurchaseRequestCode *string             `json:"purchase_request_code" bson:"purchase_request_code"`
}

type PurchaseOrderStats struct {
	NetTotal  float64 `bson:"net_total"`
	VatPrice  float64 `bson:"vat_price"`
	Discount  float64 `bson:"discount"`
	Count     int64   `bson:"count"`
}

func (po *PurchaseOrder) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(po.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if po.StoreID != nil {
		s, err := FindStoreByID(po.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		po.StoreName = s.Name
	}

	if po.VendorID != nil && !po.VendorID.IsZero() {
		vendor, err := store.FindVendorByID(po.VendorID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1})
		if err != nil {
			return err
		}
		po.VendorName = vendor.Name
		po.VendorNameArabic = vendor.NameInArabic
	} else {
		po.VendorName = ""
		po.VendorNameArabic = ""
	}

	if po.OrderPlacedBy != nil {
		user, err := FindUserByID(po.OrderPlacedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		po.OrderPlacedByName = user.Name
	}

	if po.CreatedBy != nil {
		user, err := FindUserByID(po.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		po.CreatedByName = user.Name
	}

	if po.UpdatedBy != nil {
		user, err := FindUserByID(po.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		po.UpdatedByName = user.Name
	}

	for i, product := range po.Products {
		productObject, err := store.FindProductByID(&product.ProductID, bson.M{
			"id":                 1,
			"name":               1,
			"name_in_arabic":     1,
			"item_code":          1,
			"part_number":        1,
			"prefix_part_number": 1,
			"is_service":         1,
		})
		if err != nil {
			return err
		}
		po.Products[i].NameInArabic = productObject.NameInArabic
		po.Products[i].ItemCode = productObject.ItemCode
		po.Products[i].PrefixPartNumber = productObject.PrefixPartNumber
		po.Products[i].IsService = productObject.IsService
	}

	return nil
}

func (po *PurchaseOrder) FindNetTotal() {
	po.ShippingOrHandlingFees = RoundTo2Decimals(po.ShippingOrHandlingFees)
	po.Discount = RoundTo2Decimals(po.Discount)

	po.FindTotal()

	baseTotal := po.Total + po.ShippingOrHandlingFees - po.Discount
	baseTotal = RoundTo2Decimals(baseTotal)

	if po.VatPercent != nil {
		po.VatPrice = RoundTo2Decimals(baseTotal * (*po.VatPercent / 100))
	}
	po.NetTotal = RoundTo2Decimals(baseTotal + po.VatPrice)

	actualBaseTotal := po.ActualTotal + po.ShippingOrHandlingFees - po.Discount
	actualBaseTotal = RoundTo8Decimals(actualBaseTotal)
	if po.VatPercent != nil {
		po.ActualVatPrice = RoundTo2Decimals(actualBaseTotal * (*po.VatPercent / 100))
	}
	po.ActualNetTotal = RoundTo2Decimals(actualBaseTotal + po.ActualVatPrice)

	if po.AutoRoundingAmount {
		po.RoundingAmount = RoundTo2Decimals(po.ActualNetTotal - po.NetTotal)
	}
	po.NetTotal = RoundTo2Decimals(po.NetTotal + po.RoundingAmount)

	po.CalculateDiscountPercentage()
}

func (po *PurchaseOrder) FindTotal() {
	total := float64(0.0)
	totalWithVAT := float64(0.0)
	actualTotal := float64(0.0)
	actualTotalWithVAT := float64(0.0)

	for i, product := range po.Products {
		total += product.Quantity * (po.Products[i].PurchaseUnitPrice - po.Products[i].UnitDiscount)
		totalWithVAT += product.Quantity * (po.Products[i].PurchaseUnitPriceWithVAT - po.Products[i].UnitDiscountWithVAT)
		total = RoundTo2Decimals(total)
		totalWithVAT = RoundTo2Decimals(totalWithVAT)
		actualTotal += product.Quantity * (po.Products[i].PurchaseUnitPrice - po.Products[i].UnitDiscount)
		actualTotal = RoundTo8Decimals(actualTotal)
		actualTotalWithVAT += product.Quantity * (po.Products[i].PurchaseUnitPriceWithVAT - po.Products[i].UnitDiscountWithVAT)
		actualTotalWithVAT = RoundTo8Decimals(actualTotalWithVAT)
	}

	po.Total = total
	po.TotalWithVAT = totalWithVAT
	po.ActualTotal = actualTotal
	po.ActualTotalWithVAT = actualTotalWithVAT
}

func (po *PurchaseOrder) CalculateDiscountPercentage() {
	if po.Discount <= 0 {
		po.DiscountPercent = 0.00
		po.DiscountPercentWithVAT = 0.00
		return
	}
	base := po.NetTotal + po.Discount
	if base == 0 {
		po.DiscountPercent = 0.00
		po.DiscountPercentWithVAT = 0.00
		return
	}
	po.DiscountPercent = RoundTo2Decimals((po.Discount / base) * 100)
	baseWithVAT := po.NetTotal + po.DiscountWithVAT
	if baseWithVAT == 0 {
		po.DiscountPercentWithVAT = 0.00
		return
	}
	po.DiscountPercentWithVAT = RoundTo2Decimals((po.DiscountWithVAT / baseWithVAT) * 100)
}

func (po *PurchaseOrder) FindTotalQuantity() {
	total := float64(0.0)
	for _, p := range po.Products {
		total += p.Quantity
	}
	po.TotalQuantity = total
}

func (po *PurchaseOrder) MakeCode() error {
	store, err := FindStoreByID(po.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := po.StoreID.Hex() + "_purchase_order_counter"

	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		count, err := store.GetCountByCollection("purchase_order")
		if err != nil {
			return err
		}
		startFrom := store.PurchaseOrderSerialNumber.StartFromCount
		if startFrom <= 0 {
			startFrom = 1
		}
		err = db.RedisClient.Set(redisKey, startFrom+count-1, 0).Err()
		if err != nil {
			return err
		}
	}

	incr, err := db.RedisClient.Incr(redisKey).Result()
	if err != nil {
		return err
	}

	paddingCount := store.PurchaseOrderSerialNumber.PaddingCount
	if paddingCount <= 0 {
		paddingCount = 4
	}
	prefix := store.PurchaseOrderSerialNumber.Prefix
	if prefix == "" {
		prefix = "PO"
	}
	po.Code = fmt.Sprintf("%s-%0*d", prefix, paddingCount, incr)

	if strings.Contains(po.Code, "DATE") {
		baseTime := time.Now()
		if po.Date != nil {
			baseTime = *po.Date
		}
		orderDate := baseTime.Format("20060102")
		po.Code = strings.ReplaceAll(po.Code, "DATE", orderDate)
	}

	return nil
}

func (po *PurchaseOrder) Insert() error {
	collection := db.GetDB("store_" + po.StoreID.Hex()).Collection("purchase_order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	po.ID = primitive.NewObjectID()
	_, err := collection.InsertOne(ctx, &po)
	return err
}

func (po *PurchaseOrder) Update() error {
	collection := db.GetDB("store_" + po.StoreID.Hex()).Collection("purchase_order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.ReplaceOne(ctx, bson.M{"_id": po.ID}, &po)
	return err
}

func (store *Store) FindPurchaseOrderByID(id *primitive.ObjectID, selectFields bson.M) (*PurchaseOrder, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var po PurchaseOrder
	opts := options.FindOne()
	if len(selectFields) > 0 {
		opts.SetProjection(selectFields)
	}
	err := collection.FindOne(ctx, bson.M{"_id": id}, opts).Decode(&po)
	if err != nil {
		return nil, err
	}
	return &po, nil
}

func (po *PurchaseOrder) FindNextPurchaseOrder(selectFields map[string]interface{}) (*PurchaseOrder, error) {
	collection := db.GetDB("store_" + po.StoreID.Hex()).Collection("purchase_order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	findOneOptions.SetSort(bson.M{"created_at": 1})
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	var next PurchaseOrder
	err := collection.FindOne(ctx, bson.M{
		"created_at": bson.M{"$gt": po.CreatedAt},
		"_id":        bson.M{"$ne": po.ID},
		"store_id":   po.StoreID,
	}, findOneOptions).Decode(&next)
	if err != nil {
		return nil, err
	}
	return &next, nil
}

func (po *PurchaseOrder) FindPreviousPurchaseOrder(selectFields map[string]interface{}) (*PurchaseOrder, error) {
	collection := db.GetDB("store_" + po.StoreID.Hex()).Collection("purchase_order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	findOneOptions.SetSort(bson.M{"created_at": -1})
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	var prev PurchaseOrder
	err := collection.FindOne(ctx, bson.M{
		"created_at": bson.M{"$lt": po.CreatedAt},
		"store_id":   po.StoreID,
	}, findOneOptions).Decode(&prev)
	if err != nil {
		return nil, err
	}
	return &prev, nil
}

func (po *PurchaseOrder) Validate(
	w http.ResponseWriter,
	r *http.Request,
	scenario string,
) (errs map[string]string) {
	errs = make(map[string]string)

	if govalidator.IsNull(po.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, po.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		} else {
			po.Date = &date
		}
	}

	if !govalidator.IsNull(po.ExpectedDateStr) {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, po.ExpectedDateStr)
		if err != nil {
			errs["expected_date_str"] = "Invalid expected date format"
		} else {
			po.ExpectedDate = &date
		}
	}

	if len(po.Products) == 0 {
		errs["product_id"] = "At least 1 product is required"
	}

	store, err := FindStoreByID(po.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "Invalid store id"
		return
	}

	for i, product := range po.Products {
		if product.ProductID.IsZero() {
			errs["product_id_"+strconv.Itoa(i)] = "Product is required"
		} else {
			exists, err := store.IsProductExists(&product.ProductID)
			if err != nil {
				errs["product_id_"+strconv.Itoa(i)] = err.Error()
				return errs
			}
			if !exists {
				errs["product_id_"+strconv.Itoa(i)] = "Invalid product"
			}
		}
	}

	return errs
}

func (po *PurchaseOrder) CreateNewVendorFromName() error {
	if po.VendorID != nil && !po.VendorID.IsZero() {
		return nil
	}
	if govalidator.IsNull(po.VendorName) {
		return nil
	}

	store, err := FindStoreByID(po.StoreID, bson.M{})
	if err != nil {
		return err
	}

	now := time.Now()
	newVendor := Vendor{
		Name:      po.VendorName,
		Phone:     po.Phone,
		VATNo:     po.VatNo,
		Remarks:   po.Remarks,
		CreatedBy: po.CreatedBy,
		UpdatedBy: po.CreatedBy,
		CreatedAt: &now,
		UpdatedAt: &now,
		StoreID:   po.StoreID,
	}

	err = newVendor.MakeCode()
	if err != nil {
		return err
	}

	newVendor.GenerateSearchWords()
	newVendor.SetSearchLabel()
	newVendor.SetAdditionalkeywords()

	err = newVendor.Insert()
	if err != nil {
		return err
	}
	_ = store
	err = newVendor.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	po.VendorID = &newVendor.ID
	return nil
}

func (po *PurchaseOrder) SetUnKnownVendorIfNoVendorSelected() error {
	if po.VendorID != nil && !po.VendorID.IsZero() {
		return nil
	}

	store, err := FindStoreByID(po.StoreID, bson.M{})
	if err != nil {
		return err
	}

	vendor, err := store.FindVendorByName("Unknown", bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	if vendor == nil {
		now := time.Now()
		unknownVendor := Vendor{
			Name:      "Unknown",
			CreatedBy: po.CreatedBy,
			UpdatedBy: po.CreatedBy,
			CreatedAt: &now,
			UpdatedAt: &now,
			StoreID:   po.StoreID,
		}
		err = unknownVendor.MakeCode()
		if err != nil {
			return err
		}
		unknownVendor.GenerateSearchWords()
		unknownVendor.SetSearchLabel()
		err = unknownVendor.Insert()
		if err != nil {
			return err
		}
		po.VendorID = &unknownVendor.ID
		po.VendorName = unknownVendor.Name
	} else {
		po.VendorID = &vendor.ID
		po.VendorName = vendor.Name
	}

	return nil
}

type SearchCriteriasPurchaseOrder struct {
	SearchBy  map[string]interface{}
	SortBy    map[string]interface{}
	Page      int
	Offset    int64
	Limit     int64
	Select    []string
}

func (store *Store) SearchPurchaseOrder(w http.ResponseWriter, r *http.Request) (pos []PurchaseOrder, criterias SearchCriteriasPurchaseOrder, err error) {
	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["store_id"] = store.ID
	criterias.Page = 1
	criterias.Limit = 10

	var createdAtStartDate time.Time
	var createdAtEndDate time.Time

	keys, ok := r.URL.Query()["search[created_by_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["created_by_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {
		ids := strings.Split(keys[0], ",")
		var objectIDs []primitive.ObjectID
		for _, idStr := range ids {
			id, err := primitive.ObjectIDFromHex(strings.TrimSpace(idStr))
			if err == nil {
				objectIDs = append(objectIDs, id)
			}
		}
		if len(objectIDs) == 1 {
			criterias.SearchBy["created_by"] = objectIDs[0]
		} else if len(objectIDs) > 1 {
			criterias.SearchBy["created_by"] = bson.M{"$in": objectIDs}
		}
	}

	keys, ok = r.URL.Query()["search[vendor_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["$or"] = []bson.M{
			{"vendor_name": bson.M{"$regex": keys[0], "$options": "i"}},
			{"vendor_name_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
		}
	}

	keys, ok = r.URL.Query()["search[vendor_id]"]
	if ok && len(keys[0]) >= 1 {
		vendorID, err := primitive.ObjectIDFromHex(keys[0])
		if err == nil {
			criterias.SearchBy["vendor_id"] = vendorID
		}
	}

	keys, ok = r.URL.Query()["search[status]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["status"] = keys[0]
	}

	keys, ok = r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[date_str]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err == nil {
			endDate := startDate.Add(24 * time.Hour)
			criterias.SearchBy["date"] = bson.M{"$gte": startDate, "$lt": endDate}
		}
	}

	keys, ok = r.URL.Query()["search[from_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		fromDate, err := time.Parse(shortForm, keys[0])
		if err == nil {
			criterias.SearchBy["date"] = bson.M{"$gte": fromDate}
		}
	}

	keys, ok = r.URL.Query()["search[to_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		toDate, err := time.Parse(shortForm, keys[0])
		if err == nil {
			toDate = toDate.Add(24 * time.Hour)
			if existing, ok := criterias.SearchBy["date"].(bson.M); ok {
				existing["$lt"] = toDate
			} else {
				criterias.SearchBy["date"] = bson.M{"$lt": toDate}
			}
		}
	}

	keys, ok = r.URL.Query()["search[created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtStartDate, err = time.Parse(shortForm, keys[0])
		if err == nil {
			createdAtEndDate = createdAtStartDate.Add(24 * time.Hour)
			criterias.SearchBy["created_at"] = bson.M{"$gte": createdAtStartDate, "$lt": createdAtEndDate}
		}
	}

	keys, ok = r.URL.Query()["search[from_created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtStartDate, err = time.Parse(shortForm, keys[0])
		if err == nil {
			criterias.SearchBy["created_at"] = bson.M{"$gte": createdAtStartDate}
		}
	}

	keys, ok = r.URL.Query()["search[to_created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err == nil {
			createdAtEndDate = createdAtEndDate.Add(24 * time.Hour)
			if existing, ok := criterias.SearchBy["created_at"].(bson.M); ok {
				existing["$lt"] = createdAtEndDate
			} else {
				criterias.SearchBy["created_at"] = bson.M{"$lt": createdAtEndDate}
			}
		}
	}

	keys, ok = r.URL.Query()["limit"]
	if ok && len(keys[0]) >= 1 {
		criterias.Limit, _ = strconv.ParseInt(keys[0], 10, 64)
	}

	keys, ok = r.URL.Query()["page"]
	if ok && len(keys[0]) >= 1 {
		criterias.Page, _ = strconv.Atoi(keys[0])
	}
	criterias.Offset = int64(criterias.Page-1) * criterias.Limit

	criterias.SortBy = map[string]interface{}{}
	keys, ok = r.URL.Query()["sort"]
	if ok && len(keys[0]) >= 1 {
		sortField := keys[0]
		sortOrder := 1
		if strings.HasPrefix(sortField, "-") {
			sortOrder = -1
			sortField = strings.TrimPrefix(sortField, "-")
		}
		criterias.SortBy[sortField] = sortOrder
	} else {
		criterias.SortBy["created_at"] = -1
	}

	selectFields := bson.M{}
	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		for _, field := range strings.Split(keys[0], ",") {
			field = strings.TrimSpace(field)
			if field == "id" {
				field = "_id"
			}
			selectFields[field] = 1
		}
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOptions := options.Find()
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetSkip(criterias.Offset)
	findOptions.SetLimit(criterias.Limit)
	if len(selectFields) > 0 {
		findOptions.SetProjection(selectFields)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return pos, criterias, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var po PurchaseOrder
		if err := cur.Decode(&po); err != nil {
			return pos, criterias, err
		}
		pos = append(pos, po)
	}

	return pos, criterias, nil
}

func (store *Store) GetPurchaseOrderStats(filter map[string]interface{}) (stats PurchaseOrderStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		{"$match": filter},
		{"$group": bson.M{
			"_id":       nil,
			"net_total": bson.M{"$sum": "$net_total"},
			"vat_price": bson.M{"$sum": "$vat_price"},
			"discount":  bson.M{"$sum": "$discount"},
			"count":     bson.M{"$sum": 1},
		}},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return stats, err
	}
	defer cur.Close(ctx)

	if cur.Next(ctx) {
		if err := cur.Decode(&stats); err != nil {
			return stats, err
		}
	}

	return stats, nil
}

func (store *Store) GetPurchaseOrderTotalCount(filter map[string]interface{}) (int64, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return collection.CountDocuments(ctx, filter)
}

func (po *PurchaseOrder) Delete() error {
	collection := db.GetDB("store_" + po.StoreID.Hex()).Collection("purchase_order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := collection.DeleteOne(ctx, bson.M{"_id": po.ID})
	return err
}

// ConvertToPurchase converts a PurchaseOrder into a Purchase (draft — caller still needs to call SetProductsStock etc.)
func (po *PurchaseOrder) ConvertToPurchase(userID *primitive.ObjectID) (*Purchase, error) {
	now := time.Now()
	vatPercent := float64(0)
	if po.VatPercent != nil {
		vatPercent = *po.VatPercent
	}
	purchase := &Purchase{
		Date:                   po.Date,
		DateStr:                po.DateStr,
		StoreID:                po.StoreID,
		VendorID:               po.VendorID,
		VendorName:             po.VendorName,
		VendorNameArabic:       po.VendorNameArabic,
		VendorInvoiceNumber:    po.VendorInvoiceNumber,
		StoreName:              po.StoreName,
		VatPercent:             &vatPercent,
		Discount:               po.Discount,
		DiscountWithVAT:        po.DiscountWithVAT,
		DiscountPercent:        po.DiscountPercent,
		DiscountPercentWithVAT: po.DiscountPercentWithVAT,
		ShippingOrHandlingFees: po.ShippingOrHandlingFees,
		TotalQuantity:          po.TotalQuantity,
		VatPrice:               po.VatPrice,
		Total:                  po.Total,
		TotalWithVAT:           po.TotalWithVAT,
		NetTotal:               po.NetTotal,
		ActualVatPrice:         po.ActualVatPrice,
		ActualTotal:            po.ActualTotal,
		ActualTotalWithVAT:     po.ActualTotalWithVAT,
		ActualNetTotal:         po.ActualNetTotal,
		RoundingAmount:         po.RoundingAmount,
		AutoRoundingAmount:     po.AutoRoundingAmount,
		Remarks:                po.Remarks,
		Phone:                  po.Phone,
		VatNo:                  po.VatNo,
		Address:                po.Address,
		Status:                 "delivered",
		PaymentStatus:          "not_paid",
		CreatedBy:              userID,
		UpdatedBy:              userID,
		CreatedAt:              &now,
		UpdatedAt:              &now,
	}

	for _, p := range po.Products {
		purchase.Products = append(purchase.Products, PurchaseProduct{
			ProductID:                  p.ProductID,
			WarehouseID:                p.WarehouseID,
			WarehouseCode:              p.WarehouseCode,
			Name:                       p.Name,
			NameInArabic:               p.NameInArabic,
			ItemCode:                   p.ItemCode,
			PrefixPartNumber:           p.PrefixPartNumber,
			PartNumber:                 p.PartNumber,
			Quantity:                   p.Quantity,
			Unit:                       p.Unit,
			PurchaseUnitPrice:          p.PurchaseUnitPrice,
			PurchaseUnitPriceWithVAT:   p.PurchaseUnitPriceWithVAT,
			UnitDiscount:               p.UnitDiscount,
			UnitDiscountWithVAT:        p.UnitDiscountWithVAT,
			UnitDiscountPercent:        p.UnitDiscountPercent,
			UnitDiscountPercentWithVAT: p.UnitDiscountPercentWithVAT,
			IsService:                  p.IsService,
		})
	}

	return purchase, nil
}

func init() {
	log.Println("purchase_order model loaded")
}

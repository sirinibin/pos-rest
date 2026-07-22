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
	"go.mongodb.org/mongo-driver/mongo/options"
)

type PurchaseRequestProduct struct {
	ProductID        primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name             string             `bson:"name" json:"name"`
	NameInArabic     string             `bson:"name_in_arabic" json:"name_in_arabic"`
	ItemCode         string             `bson:"item_code" json:"item_code"`
	PartNumber       string             `bson:"part_number" json:"part_number"`
	PrefixPartNumber string             `bson:"prefix_part_number" json:"prefix_part_number"`
	Quantity         float64            `json:"quantity" bson:"quantity"`
	Unit             string             `bson:"unit" json:"unit"`
	PurchaseUnitPrice float64           `bson:"purchase_unit_price" json:"purchase_unit_price"`
	UnitDiscount     float64            `bson:"unit_discount" json:"unit_discount"`
	IsService        bool               `bson:"is_service" json:"is_service"`
}

// PurchaseRequest status: pending | accepted | partially_accepted | rejected
type PurchaseRequest struct {
	ID                primitive.ObjectID       `json:"id,omitempty" bson:"_id,omitempty"`
	Date              *time.Time               `bson:"date,omitempty" json:"date,omitempty"`
	DateStr           string                   `json:"date_str,omitempty" bson:"-"`
	Code              string                   `bson:"code,omitempty" json:"code,omitempty"`
	StoreID           *primitive.ObjectID      `json:"store_id,omitempty" bson:"store_id,omitempty"`
	AssignedTo        *primitive.ObjectID      `json:"assigned_to,omitempty" bson:"assigned_to,omitempty"`
	Products          []PurchaseRequestProduct `bson:"products,omitempty" json:"products,omitempty"`
	Status            string                   `bson:"status,omitempty" json:"status,omitempty"`
	Notes             string                   `bson:"notes" json:"notes"`
	TotalQuantity     float64                  `bson:"total_quantity" json:"total_quantity"`
	Total             float64                  `bson:"total" json:"total"`
	NetTotal          float64                  `bson:"net_total" json:"net_total"`
	VatPercent        *float64                 `bson:"vat_percent" json:"vat_percent"`
	VatPrice          float64                  `bson:"vat_price" json:"vat_price"`
	Discount          float64                  `bson:"discount" json:"discount"`
	ShippingOrHandlingFees float64             `bson:"shipping_handling_fees" json:"shipping_handling_fees"`
	CreatedAt         *time.Time               `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt         *time.Time               `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy         *primitive.ObjectID      `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy         *primitive.ObjectID      `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	AssignedToName    string                   `json:"assigned_to_name,omitempty" bson:"assigned_to_name,omitempty"`
	StoreName         string                   `json:"store_name,omitempty" bson:"store_name,omitempty"`
	CreatedByName     string                   `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName     string                   `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	PurchaseOrderID   *primitive.ObjectID      `json:"purchase_order_id" bson:"purchase_order_id"`
	PurchaseOrderCode *string                  `json:"purchase_order_code" bson:"purchase_order_code"`
}

type PurchaseRequestStats struct {
	NetTotal float64 `bson:"net_total"`
	Count    int64   `bson:"count"`
}

func (pr *PurchaseRequest) UpdateForeignLabelFields() error {
	if pr.StoreID != nil {
		s, err := FindStoreByID(pr.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		pr.StoreName = s.Name
	}

	if pr.AssignedTo != nil {
		user, err := FindUserByID(pr.AssignedTo, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		pr.AssignedToName = user.Name
	}

	if pr.CreatedBy != nil {
		user, err := FindUserByID(pr.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		pr.CreatedByName = user.Name
	}

	if pr.UpdatedBy != nil {
		user, err := FindUserByID(pr.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		pr.UpdatedByName = user.Name
	}

	store, err := FindStoreByID(pr.StoreID, bson.M{})
	if err != nil {
		return err
	}

	for i, product := range pr.Products {
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
		pr.Products[i].Name = productObject.Name
		pr.Products[i].NameInArabic = productObject.NameInArabic
		pr.Products[i].ItemCode = productObject.ItemCode
		pr.Products[i].PartNumber = productObject.PartNumber
		pr.Products[i].PrefixPartNumber = productObject.PrefixPartNumber
		pr.Products[i].IsService = productObject.IsService
	}

	return nil
}

func (pr *PurchaseRequest) FindNetTotal() {
	pr.ShippingOrHandlingFees = RoundTo2Decimals(pr.ShippingOrHandlingFees)
	pr.Discount = RoundTo2Decimals(pr.Discount)
	pr.FindTotal()

	baseTotal := pr.Total + pr.ShippingOrHandlingFees - pr.Discount
	baseTotal = RoundTo2Decimals(baseTotal)

	if pr.VatPercent != nil {
		pr.VatPrice = RoundTo2Decimals(baseTotal * (*pr.VatPercent / 100))
	}
	pr.NetTotal = RoundTo2Decimals(baseTotal + pr.VatPrice)
}

func (pr *PurchaseRequest) FindTotal() {
	total := float64(0.0)
	for _, product := range pr.Products {
		total += product.Quantity * (product.PurchaseUnitPrice - product.UnitDiscount)
		total = RoundTo2Decimals(total)
	}
	pr.Total = total
}

func (pr *PurchaseRequest) FindTotalQuantity() {
	total := float64(0.0)
	for _, p := range pr.Products {
		total += p.Quantity
	}
	pr.TotalQuantity = total
}

func (pr *PurchaseRequest) MakeCode() error {
	store, err := FindStoreByID(pr.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := pr.StoreID.Hex() + "_purchase_request_counter"

	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		count, err := store.GetCountByCollection("purchase_request")
		if err != nil {
			return err
		}
		startFrom := store.PurchaseRequestSerialNumber.StartFromCount
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

	paddingCount := store.PurchaseRequestSerialNumber.PaddingCount
	if paddingCount <= 0 {
		paddingCount = 4
	}
	prefix := store.PurchaseRequestSerialNumber.Prefix
	if prefix == "" {
		prefix = "PR"
	}
	pr.Code = fmt.Sprintf("%s-%0*d", prefix, paddingCount, incr)

	if strings.Contains(pr.Code, "DATE") {
		baseTime := time.Now()
		if pr.Date != nil {
			baseTime = *pr.Date
		}
		pr.Code = strings.ReplaceAll(pr.Code, "DATE", baseTime.Format("20060102"))
	}

	return nil
}

func (pr *PurchaseRequest) Insert() error {
	collection := db.GetDB("store_" + pr.StoreID.Hex()).Collection("purchase_request")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pr.ID = primitive.NewObjectID()
	_, err := collection.InsertOne(ctx, &pr)
	return err
}

func (pr *PurchaseRequest) Update() error {
	collection := db.GetDB("store_" + pr.StoreID.Hex()).Collection("purchase_request")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.ReplaceOne(ctx, bson.M{"_id": pr.ID}, &pr)
	return err
}

func (pr *PurchaseRequest) Delete() error {
	collection := db.GetDB("store_" + pr.StoreID.Hex()).Collection("purchase_request")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.DeleteOne(ctx, bson.M{"_id": pr.ID})
	return err
}

func (store *Store) FindPurchaseRequestByID(id *primitive.ObjectID, selectFields bson.M) (*PurchaseRequest, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_request")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var pr PurchaseRequest
	opts := options.FindOne()
	if len(selectFields) > 0 {
		opts.SetProjection(selectFields)
	}
	err := collection.FindOne(ctx, bson.M{"_id": id}, opts).Decode(&pr)
	if err != nil {
		return nil, err
	}
	return &pr, nil
}

func (pr *PurchaseRequest) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	if govalidator.IsNull(pr.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, pr.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		} else {
			pr.Date = &date
		}
	}

	if pr.AssignedTo == nil || pr.AssignedTo.IsZero() {
		errs["assigned_to"] = "Assigned to user is required"
	}

	if len(pr.Products) == 0 {
		errs["product_id"] = "At least 1 product is required"
	}

	store, err := FindStoreByID(pr.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "Invalid store id"
		return
	}

	for i, product := range pr.Products {
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

type SearchCriteriasPurchaseRequest struct {
	SearchBy map[string]interface{}
	SortBy   map[string]interface{}
	Page     int
	Offset   int64
	Limit    int64
	Select   []string
}

func (store *Store) SearchPurchaseRequest(w http.ResponseWriter, r *http.Request) (prs []PurchaseRequest, criterias SearchCriteriasPurchaseRequest, err error) {
	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["store_id"] = store.ID

	keys, ok := r.URL.Query()["search[id]"]
	if ok && len(keys[0]) >= 1 {
		id, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return prs, criterias, err
		}
		criterias.SearchBy["_id"] = id
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {
		id, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return prs, criterias, err
		}
		criterias.SearchBy["created_by"] = id
	}

	keys, ok = r.URL.Query()["search[assigned_to]"]
	if ok && len(keys[0]) >= 1 {
		id, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return prs, criterias, err
		}
		criterias.SearchBy["assigned_to"] = id
	}

	keys, ok = r.URL.Query()["search[status]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["status"] = keys[0]
	}

	keys, ok = r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	criterias.SortBy = make(map[string]interface{})
	keys, ok = r.URL.Query()["search[sort_by]"]
	if ok && len(keys[0]) >= 1 {
		keys2, ok := r.URL.Query()["search[sort_order]"]
		if ok && len(keys2[0]) >= 1 {
			sortOrder := -1
			if keys2[0] == "asc" {
				sortOrder = 1
			}
			criterias.SortBy[keys[0]] = sortOrder
		}
	}
	if len(criterias.SortBy) == 0 {
		criterias.SortBy["created_at"] = -1
	}

	criterias.Page = 1
	keys, ok = r.URL.Query()["search[page]"]
	if ok && len(keys[0]) >= 1 {
		page, err := strconv.Atoi(keys[0])
		if err == nil && page > 0 {
			criterias.Page = page
		}
	}

	keys, ok = r.URL.Query()["search[limit]"]
	if ok && len(keys[0]) >= 1 {
		limit, err := strconv.ParseInt(keys[0], 10, 64)
		if err == nil && limit > 0 {
			criterias.Limit = limit
		}
	}
	if criterias.Limit == 0 {
		criterias.Limit = 10
	}

	criterias.Offset = int64(criterias.Page-1) * criterias.Limit

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_request")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(criterias.Offset)
	findOptions.SetLimit(criterias.Limit)
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return prs, criterias, err
	}
	if err = cur.All(ctx, &prs); err != nil {
		return prs, criterias, err
	}

	return prs, criterias, nil
}

func (store *Store) GetPurchaseRequestTotalCount(filter map[string]interface{}) (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_request")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return collection.CountDocuments(ctx, filter)
}

func (store *Store) GetPurchaseRequestStats(filter map[string]interface{}) (stats PurchaseRequestStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_request")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		{"$match": filter},
		{"$group": bson.M{
			"_id":       nil,
			"net_total": bson.M{"$sum": "$net_total"},
			"count":     bson.M{"$sum": 1},
		}},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return stats, err
	}
	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err = cur.Decode(&stats)
	}
	return stats, err
}

func (pr *PurchaseRequest) ConvertToPurchaseOrder(userID *primitive.ObjectID) (*PurchaseOrder, error) {
	now := time.Now()
	dateStr := now.Format("2006-01-02T15:04:05Z07:00")

	var vatPercent float64
	if pr.VatPercent != nil {
		vatPercent = *pr.VatPercent
	}

	po := &PurchaseOrder{
		StoreID:    pr.StoreID,
		Status:     "draft",
		Remarks:    pr.Notes,
		VatPercent: &vatPercent,
		Discount:   pr.Discount,
		ShippingOrHandlingFees: pr.ShippingOrHandlingFees,
		CreatedBy:  userID,
		UpdatedBy:  userID,
		CreatedAt:  &now,
		UpdatedAt:  &now,
		DateStr:    dateStr,
	}

	for _, p := range pr.Products {
		po.Products = append(po.Products, PurchaseOrderProduct{
			ProductID:         p.ProductID,
			Name:              p.Name,
			NameInArabic:      p.NameInArabic,
			ItemCode:          p.ItemCode,
			PartNumber:        p.PartNumber,
			PrefixPartNumber:  p.PrefixPartNumber,
			Quantity:          p.Quantity,
			Unit:              p.Unit,
			PurchaseUnitPrice: p.PurchaseUnitPrice,
			UnitDiscount:      p.UnitDiscount,
			IsService:         p.IsService,
		})
	}

	// Parse date
	const shortForm = "2006-01-02T15:04:05Z07:00"
	date, err := time.Parse(shortForm, dateStr)
	if err != nil {
		log.Print("Error parsing date:", err)
	} else {
		po.Date = &date
	}

	po.FindNetTotal()
	po.FindTotalQuantity()

	err = po.UpdateForeignLabelFields()
	if err != nil {
		return nil, err
	}

	err = po.MakeCode()
	if err != nil {
		return nil, err
	}

	err = po.SetUnKnownVendorIfNoVendorSelected()
	if err != nil {
		return nil, err
	}

	err = po.Insert()
	if err != nil {
		return nil, err
	}

	return po, nil
}

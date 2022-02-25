package models

import (
	"context"
	"errors"
	"fmt"
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

type PurchaseReturnProduct struct {
	ProductID               primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name                    string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic            string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode                string             `bson:"item_code,omitempty" json:"item_code,omitempty"`
	Quantity                float64            `json:"quantity,omitempty" bson:"quantity,omitempty"`
	Unit                    string             `bson:"unit,omitempty" json:"unit,omitempty"`
	PurchaseReturnUnitPrice float64            `bson:"purchasereturn_unit_price,omitempty" json:"purchasereturn_unit_price,omitempty"`
}

//PurchaseReturn : PurchaseReturn structure
type PurchaseReturn struct {
	ID                              primitive.ObjectID      `json:"id,omitempty" bson:"_id,omitempty"`
	PurchaseID                      *primitive.ObjectID     `json:"purchase_id,omitempty" bson:"purchase_id,omitempty"`
	PurchaseCode                    string                  `bson:"purchase_code,omitempty" json:"purchase_code,omitempty"`
	Date                            *time.Time              `bson:"date,omitempty" json:"date,omitempty"`
	DateStr                         string                  `json:"date_str,omitempty"`
	Code                            string                  `bson:"code,omitempty" json:"code,omitempty"`
	StoreID                         *primitive.ObjectID     `json:"store_id,omitempty" bson:"store_id,omitempty"`
	VendorID                        *primitive.ObjectID     `json:"vendor_id,omitempty" bson:"vendor_id,omitempty"`
	Store                           *Store                  `json:"store,omitempty"`
	Vendor                          *Vendor                 `json:"vendor,omitempty"`
	Products                        []PurchaseReturnProduct `bson:"products,omitempty" json:"products,omitempty"`
	PurchaseReturnedBy              *primitive.ObjectID     `json:"purchase_returned_by,omitempty" bson:"purchase_returned_by,omitempty"`
	PurchaseReturnedBySignatureID   *primitive.ObjectID     `json:"purchase_returned_by_signature_id,omitempty" bson:"purchase_returned_signature_id,omitempty"`
	PurchaseReturnedBySignatureName string                  `json:"purchase_returned_by_signature_name,omitempty" bson:"purchase_returned_by_signature_name,omitempty"`
	PurchaseReturnedByUser          *User                   `json:"purchase_returned_by_user,omitempty"`
	PurchaseReturnedBySignature     *Signature              `json:"purchase_returned_by_signature,omitempty"`
	SignatureDate                   *time.Time              `bson:"signature_date,omitempty" json:"signature_date,omitempty"`
	SignatureDateStr                string                  `json:"signature_date_str,omitempty"`
	VatPercent                      *float64                `bson:"vat_percent" json:"vat_percent"`
	Discount                        float64                 `bson:"discount" json:"discount"`
	DiscountPercent                 float64                 `bson:"discount_percent" json:"discount_percent"`
	IsDiscountPercent               bool                    `bson:"is_discount_percent" json:"is_discount_percent"`
	Status                          string                  `bson:"status,omitempty" json:"status,omitempty"`
	TotalQuantity                   float64                 `bson:"total_quantity" json:"total_quantity"`
	VatPrice                        float64                 `bson:"vat_price" json:"vat_price"`
	Total                           float64                 `bson:"total" json:"total"`
	NetTotal                        float64                 `bson:"net_total" json:"net_total"`
	Deleted                         bool                    `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy                       *primitive.ObjectID     `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser                   *User                   `json:"deleted_by_user,omitempty"`
	DeletedAt                       *time.Time              `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt                       *time.Time              `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt                       *time.Time              `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy                       *primitive.ObjectID     `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy                       *primitive.ObjectID     `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser                   *User                   `json:"created_by_user,omitempty"`
	UpdatedByUser                   *User                   `json:"updated_by_user,omitempty"`
	PurchaseReturnedByName          string                  `json:"purchase_returned_by_name,omitempty" bson:"purchase_returned_by_name,omitempty"`
	VendorName                      string                  `json:"vendor_name,omitempty" bson:"vendor_name,omitempty"`
	StoreName                       string                  `json:"store_name,omitempty" bson:"store_name,omitempty"`
	CreatedByName                   string                  `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName                   string                  `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName                   string                  `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	ChangeLog                       []ChangeLog             `json:"change_log,omitempty" bson:"change_log,omitempty"`
}

type PurchaseReturnStats struct {
	ID       *primitive.ObjectID `json:"id" bson:"_id"`
	NetTotal float64             `json:"net_total" bson:"net_total"`
}

func GetPurchaseReturnStats(filter map[string]interface{}) (stats PurchaseReturnStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":       nil,
				"net_total": bson.M{"$sum": "$net_total"},
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
		stats.NetTotal = math.Round(stats.NetTotal*100) / 100
	}
	return stats, nil
}

func (purchasereturn *PurchaseReturn) SetChangeLog(
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

	purchasereturn.ChangeLog = append(
		purchasereturn.ChangeLog,
		ChangeLog{
			Event:         event,
			Description:   description,
			CreatedBy:     &UserObject.ID,
			CreatedByName: UserObject.Name,
			CreatedAt:     &now,
		},
	)
}

func (purchasereturn *PurchaseReturn) AttributesValueChangeEvent(purchasereturnOld *PurchaseReturn) error {

	if purchasereturn.Status != purchasereturnOld.Status {
		purchasereturn.SetChangeLog(
			"attribute_value_change",
			"status",
			purchasereturnOld.Status,
			purchasereturn.Status,
		)

		//if purchasereturn.Status == "delivered" {

		/*
			err := purchasereturnOld.RemoveStock()
			if err != nil {
				return err
			}

			err = purchasereturn.AddStock()
			if err != nil {
				return err
			}

			err = purchasereturn.UpdateProductUnitPriceInStore()
			if err != nil {
				return err
			}
		*/
		//}
	}

	return nil
}

func (purchasereturn *PurchaseReturn) UpdateForeignLabelFields() error {

	if purchasereturn.StoreID != nil {
		store, err := FindStoreByID(purchasereturn.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchasereturn.StoreName = store.Name
	}

	if purchasereturn.VendorID != nil {
		vendor, err := FindVendorByID(purchasereturn.VendorID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchasereturn.VendorName = vendor.Name
	}

	if purchasereturn.PurchaseReturnedBy != nil {
		PurchaseReturnedByUser, err := FindUserByID(purchasereturn.PurchaseReturnedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchasereturn.PurchaseReturnedByName = PurchaseReturnedByUser.Name
	}

	if purchasereturn.PurchaseReturnedBySignatureID != nil {
		PurchaseReturnedBySignature, err := FindSignatureByID(purchasereturn.PurchaseReturnedBySignatureID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchasereturn.PurchaseReturnedBySignatureName = PurchaseReturnedBySignature.Name
	}

	if purchasereturn.CreatedBy != nil {
		createdByUser, err := FindUserByID(purchasereturn.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchasereturn.CreatedByName = createdByUser.Name
	}

	if purchasereturn.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(purchasereturn.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchasereturn.UpdatedByName = updatedByUser.Name
	}

	if purchasereturn.DeletedBy != nil && !purchasereturn.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(purchasereturn.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchasereturn.DeletedByName = deletedByUser.Name
	}

	for i, product := range purchasereturn.Products {
		productObject, err := FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1})
		if err != nil {
			return err
		}
		purchasereturn.Products[i].Name = productObject.Name
		purchasereturn.Products[i].NameInArabic = productObject.NameInArabic
		purchasereturn.Products[i].ItemCode = productObject.ItemCode
	}

	return nil
}

func (purchasereturn *PurchaseReturn) FindNetTotal() {
	netTotal := float64(0.0)
	for _, product := range purchasereturn.Products {
		netTotal += (float64(product.Quantity) * product.PurchaseReturnUnitPrice)
	}

	if purchasereturn.VatPercent != nil {
		netTotal += netTotal * (*purchasereturn.VatPercent / float64(100))
	}

	netTotal -= purchasereturn.Discount
	purchasereturn.NetTotal = math.Round(netTotal*100) / 100
}

func (purchasereturn *PurchaseReturn) FindTotal() {
	total := float64(0.0)
	for _, product := range purchasereturn.Products {
		total += product.Quantity * product.PurchaseReturnUnitPrice
	}

	purchasereturn.Total = math.Round(total*100) / 100
}

func (purchasereturn *PurchaseReturn) FindTotalQuantity() {
	totalQuantity := float64(0.00)
	for _, product := range purchasereturn.Products {
		totalQuantity += product.Quantity
	}
	purchasereturn.TotalQuantity = totalQuantity
}

func (purchasereturn *PurchaseReturn) FindVatPrice() {
	vatPrice := ((*purchasereturn.VatPercent / 100) * purchasereturn.Total)
	vatPrice = math.Round(vatPrice*100) / 100
	purchasereturn.VatPrice = vatPrice
}

func SearchPurchaseReturn(w http.ResponseWriter, r *http.Request) (purchasereturns []PurchaseReturn, criterias SearchCriterias, err error) {

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
			return purchasereturns, criterias, err
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
			return purchasereturns, criterias, err
		}
	}

	keys, ok = r.URL.Query()["search[to_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		endDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return purchasereturns, criterias, err
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
			return purchasereturns, criterias, err
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
			return purchasereturns, criterias, err
		}
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return purchasereturns, criterias, err
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

	keys, ok = r.URL.Query()["search[purchase_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["purchase_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[net_total]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return purchasereturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_total"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["net_total"] = value
		}

	}

	keys, ok = r.URL.Query()["search[vendor_id]"]
	if ok && len(keys[0]) >= 1 {

		vendorIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range vendorIds {
			vendorID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return purchasereturns, criterias, err
			}
			objecIds = append(objecIds, vendorID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["vendor_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return purchasereturns, criterias, err
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
			return purchasereturns, criterias, err
		}
		criterias.SearchBy["delivered_by"] = deliveredByID
	}

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return purchasereturns, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[purchase_returned_by]"]
	if ok && len(keys[0]) >= 1 {
		purchaseReturnedByID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return purchasereturns, criterias, err
		}
		criterias.SearchBy["purchase_returned_by"] = purchaseReturnedByID
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

	collection := db.Client().Database(db.GetPosDB()).Collection("purchasereturn")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)

	storeSelectFields := map[string]interface{}{}
	vendorSelectFields := map[string]interface{}{}
	purchaseReturnedByUserSelectFields := map[string]interface{}{}
	purchaseReturnedBySignatureSelectFields := map[string]interface{}{}
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

		if _, ok := criterias.Select["vendor.id"]; ok {
			vendorSelectFields = ParseRelationalSelectString(keys[0], "vendor")
		}

		if _, ok := criterias.Select["purchase_returned_by_user.id"]; ok {
			purchaseReturnedByUserSelectFields = ParseRelationalSelectString(keys[0], "purchase_returned_by_user")
		}

		if _, ok := criterias.Select["purchase_returned_signature.id"]; ok {
			purchaseReturnedBySignatureSelectFields = ParseRelationalSelectString(keys[0], "purchase_returned_signature")
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
		return purchasereturns, criterias, errors.New("Error fetching purchasereturns:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return purchasereturns, criterias, errors.New("Cursor error:" + err.Error())
		}
		purchasereturn := PurchaseReturn{}
		err = cur.Decode(&purchasereturn)
		if err != nil {
			return purchasereturns, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["store.id"]; ok {
			purchasereturn.Store, _ = FindStoreByID(purchasereturn.StoreID, storeSelectFields)
		}

		if _, ok := criterias.Select["vendor.id"]; ok {
			purchasereturn.Vendor, _ = FindVendorByID(purchasereturn.VendorID, vendorSelectFields)
		}

		if _, ok := criterias.Select["purchase_returned_by_user.id"]; ok {
			purchasereturn.PurchaseReturnedByUser, _ = FindUserByID(purchasereturn.PurchaseReturnedBy, purchaseReturnedByUserSelectFields)
		}

		if _, ok := criterias.Select["purchase_returned_by_signature.id"]; ok {
			purchasereturn.PurchaseReturnedBySignature, _ = FindSignatureByID(purchasereturn.PurchaseReturnedBySignatureID, purchaseReturnedBySignatureSelectFields)
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			purchasereturn.CreatedByUser, _ = FindUserByID(purchasereturn.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			purchasereturn.UpdatedByUser, _ = FindUserByID(purchasereturn.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			purchasereturn.DeletedByUser, _ = FindUserByID(purchasereturn.DeletedBy, deletedByUserSelectFields)
		}
		purchasereturns = append(purchasereturns, purchasereturn)
	} //end for loop

	return purchasereturns, criterias, nil
}

func (purchasereturn *PurchaseReturn) Validate(
	w http.ResponseWriter,
	r *http.Request,
	scenario string,
	oldPurchaseReturn *PurchaseReturn,
) (errs map[string]string) {

	errs = make(map[string]string)

	if purchasereturn.PurchaseID == nil || purchasereturn.PurchaseID.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
		errs["purchase_id"] = "Purchase ID is required"
		return errs
	}

	purchase, err := FindPurchaseByID(purchasereturn.PurchaseID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["purchase_id"] = err.Error()
		return errs
	}

	purchasereturn.PurchaseCode = purchase.Code

	if govalidator.IsNull(purchasereturn.Status) {
		errs["status"] = "Status is required"
	}

	if govalidator.IsNull(purchasereturn.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "Jan 02 2006"
		date, err := time.Parse(shortForm, purchasereturn.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		purchasereturn.Date = &date
	}

	if !govalidator.IsNull(purchasereturn.SignatureDateStr) {
		const shortForm = "Jan 02 2006"
		date, err := time.Parse(shortForm, purchasereturn.SignatureDateStr)
		if err != nil {
			errs["signature_date_str"] = "Invalid date format"
		}
		purchasereturn.SignatureDate = &date
	}

	if scenario == "update" {
		if purchasereturn.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsPurchaseReturnExists(&purchasereturn.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid PurchaseReturn:" + purchasereturn.ID.Hex()
		}

		if oldPurchaseReturn != nil {
			if oldPurchaseReturn.Status == "delivered" {

				if purchasereturn.Status == "pending" ||
					purchasereturn.Status == "cancelled" ||
					purchasereturn.Status == "purchase_returned" ||
					purchasereturn.Status == "dispatched" {
					errs["status"] = "Can't change the status from delivered to pending/cancelled/order_placed/dispatched"
				}
			}
		}

	}

	if purchasereturn.StoreID == nil || purchasereturn.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(purchasereturn.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + purchasereturn.StoreID.Hex()
			return errs
		}
	}

	if purchasereturn.VendorID == nil || purchasereturn.VendorID.IsZero() {
		errs["vendor_id"] = "Vendor is required"
	} else {
		exists, err := IsVendorExists(purchasereturn.VendorID)
		if err != nil {
			errs["vendor_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["vendor_id"] = "Invalid Vendor:" + purchasereturn.VendorID.Hex()
		}
	}

	if purchasereturn.PurchaseReturnedBy == nil || purchasereturn.PurchaseReturnedBy.IsZero() {
		errs["purchase_returned_by"] = "Purchase Returnd By is required"
	} else {
		exists, err := IsUserExists(purchasereturn.PurchaseReturnedBy)
		if err != nil {
			errs["purchase_returned_by"] = err.Error()
			return errs
		}

		if !exists {
			errs["purchase_returned_by"] = "Invalid Purchase Returned By:" + purchasereturn.PurchaseReturnedBy.Hex()
		}
	}

	if purchasereturn.PurchaseReturnedBySignatureID != nil && !purchasereturn.PurchaseReturnedBySignatureID.IsZero() {
		exists, err := IsSignatureExists(purchasereturn.PurchaseReturnedBySignatureID)
		if err != nil {
			errs["order_placed_by_signature_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["order_placed_by_signature_id"] = "Invalid Order Placed By Signature:" + purchasereturn.PurchaseReturnedBySignatureID.Hex()
		}
	}

	if len(purchasereturn.Products) == 0 {
		errs["product_id"] = "Atleast 1 product is required for purchase return"
	}

	for index, purchaseReturnProduct := range purchasereturn.Products {
		if purchaseReturnProduct.ProductID.IsZero() {
			errs["product_id_"+strconv.Itoa(index)] = "Product is required for purchasereturn"
		} else {
			exists, err := IsProductExists(&purchaseReturnProduct.ProductID)
			if err != nil {
				errs["product_id_"+strconv.Itoa(index)] = err.Error()
				return errs
			}

			if !exists {
				errs["product_id_"+strconv.Itoa(index)] = "Invalid product_id:" + purchaseReturnProduct.ProductID.Hex() + " in products"
			}
		}

		if purchaseReturnProduct.Quantity == 0 {
			errs["quantity_"+strconv.Itoa(index)] = "Quantity is required"
		}

		for _, purchaseProduct := range purchase.Products {
			if purchaseProduct.ProductID == purchaseReturnProduct.ProductID {
				purchasedQty := math.Round((purchaseProduct.Quantity-purchaseProduct.QuantityReturned)*100) / float64(100)
				if purchasedQty == 0 {
					errs["quantity_"+strconv.Itoa(index)] = "Already returned all purchased quantities"
				} else if purchaseReturnProduct.Quantity > float64(purchasedQty) {
					errs["quantity_"+strconv.Itoa(index)] = "Quantity should not be greater than purchased quantity: " + fmt.Sprintf("%.02f", purchasedQty) + " " + purchaseProduct.Unit
				}
			}
		}

		if purchaseReturnProduct.PurchaseReturnUnitPrice == 0 {
			errs["purchasereturn_unit_price_"+strconv.Itoa(index)] = "Purchase Return Unit Price is required"
		}
	}

	if purchasereturn.VatPercent == nil {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func (purchaseReturn *PurchaseReturn) UpdateReturnedQuantityInPurchaseProduct() error {
	purchase, err := FindPurchaseByID(purchaseReturn.PurchaseID, bson.M{})
	if err != nil {
		return err
	}
	for _, purchaseReturnProduct := range purchaseReturn.Products {
		for index2, purchaseProduct := range purchase.Products {
			if purchaseProduct.ProductID == purchaseReturnProduct.ProductID {
				purchase.Products[index2].QuantityReturned += purchaseReturnProduct.Quantity
			}
		}
	}

	err = purchase.CalculatePurchaseExpectedProfit()
	if err != nil {
		return err
	}

	err = purchase.Update()
	if err != nil {
		return err
	}

	return nil
}

func (purchasereturn *PurchaseReturn) AddStock() (err error) {
	for _, purchasereturnProduct := range purchasereturn.Products {
		product, err := FindProductByID(&purchasereturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		storeExistInProductStock := false
		for k, stock := range product.Stock {
			if stock.StoreID.Hex() == purchasereturn.StoreID.Hex() {
				purchasereturn.SetChangeLog(
					"add_stock",
					product.Name,
					product.Stock[k].Stock,
					(product.Stock[k].Stock + purchasereturnProduct.Quantity),
				)

				product.SetChangeLog(
					"add_stock",
					product.Name,
					product.Stock[k].Stock,
					(product.Stock[k].Stock + purchasereturnProduct.Quantity),
				)

				product.Stock[k].Stock += purchasereturnProduct.Quantity
				storeExistInProductStock = true
				break
			}
		}

		if !storeExistInProductStock {
			productStock := ProductStock{
				StoreID: *purchasereturn.StoreID,
				Stock:   purchasereturnProduct.Quantity,
			}
			product.Stock = append(product.Stock, productStock)
		}

		err = product.Update()
		if err != nil {
			return err
		}
	}
	err = purchasereturn.Update()
	if err != nil {
		return err
	}

	return nil
}

func (purchasereturn *PurchaseReturn) RemoveStock() (err error) {
	for _, purchasereturnProduct := range purchasereturn.Products {
		product, err := FindProductByID(&purchasereturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		for k, stock := range product.Stock {
			if stock.StoreID.Hex() == purchasereturn.StoreID.Hex() {
				/*
					purchasereturn.SetChangeLog(
						"remove_stock",
						product.Name,
						product.Stock[k].Stock,
						(product.Stock[k].Stock - purchasereturnProduct.Quantity),
					)

					product.SetChangeLog(
						"remove_stock",
						product.Name,
						product.Stock[k].Stock,
						(product.Stock[k].Stock - purchasereturnProduct.Quantity),
					)
				*/

				product.Stock[k].Stock -= purchasereturnProduct.Quantity
				break
			}
		}

		err = product.Update()
		if err != nil {
			return err
		}
	}
	return nil
}

func (purchasereturn *PurchaseReturn) UpdateProductUnitPriceInStore() (err error) {

	for _, purchasereturnProduct := range purchasereturn.Products {
		product, err := FindProductByID(&purchasereturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		storeExistInProductUnitPrice := false
		for k, unitPrice := range product.UnitPrices {
			if unitPrice.StoreID.Hex() == purchasereturn.StoreID.Hex() {
				product.UnitPrices[k].PurchaseUnitPrice = purchasereturnProduct.PurchaseReturnUnitPrice
				storeExistInProductUnitPrice = true
				break
			}
		}

		if !storeExistInProductUnitPrice {
			productUnitPrice := ProductUnitPrice{
				StoreID:           *purchasereturn.StoreID,
				PurchaseUnitPrice: purchasereturnProduct.PurchaseReturnUnitPrice,
			}
			product.UnitPrices = append(product.UnitPrices, productUnitPrice)
		}
		err = product.Update()
		if err != nil {
			return err
		}
	}

	return nil
}

func (purchasereturn *PurchaseReturn) GenerateCode(startFrom int, storeCode string) (string, error) {
	count, err := GetTotalCount(bson.M{"store_id": purchasereturn.StoreID}, "purchasereturn")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return storeCode + "-" + strconv.Itoa(code+1), nil
}

func (purchasereturn *PurchaseReturn) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := purchasereturn.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	store, err := FindStoreByID(purchasereturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	purchasereturn.ID = primitive.NewObjectID()
	if len(purchasereturn.Code) == 0 {
		startAt := 400000
		for true {
			code, err := purchasereturn.GenerateCode(startAt, store.Code)
			if err != nil {
				return err
			}
			purchasereturn.Code = code
			exists, err := purchasereturn.IsCodeExists()
			if err != nil {
				return err
			}
			if !exists {
				break
			}
			startAt++
		}
	}

	purchasereturn.SetChangeLog("create", nil, nil, nil)

	_, err = collection.InsertOne(ctx, &purchasereturn)
	if err != nil {
		return err
	}

	return nil
}

func (purchasereturn *PurchaseReturn) IsCodeExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if purchasereturn.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": purchasereturn.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": purchasereturn.Code,
			"_id":  bson.M{"$ne": purchasereturn.ID},
		})
	}

	return (count == 1), err
}

func GeneratePurchaseReturnCode(n int) string {
	//letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789")
	letterRunes := []rune("0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func (purchasereturn *PurchaseReturn) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := purchasereturn.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	purchasereturn.SetChangeLog("update", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": purchasereturn.ID},
		bson.M{"$set": purchasereturn},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (purchasereturn *PurchaseReturn) DeletePurchaseReturn(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = purchasereturn.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	purchasereturn.Deleted = true
	purchasereturn.DeletedBy = &userID
	now := time.Now()
	purchasereturn.DeletedAt = &now

	purchasereturn.SetChangeLog("delete", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": purchasereturn.ID},
		bson.M{"$set": purchasereturn},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (purchasereturn *PurchaseReturn) HardDelete() (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = collection.DeleteOne(ctx, bson.M{"_id": purchasereturn.ID})
	if err != nil {
		return err
	}
	return nil
}

func FindPurchaseReturnByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (purchasereturn *PurchaseReturn, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&purchasereturn)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["purchase_returned_by.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "purchase_returned_by")
		purchasereturn.PurchaseReturnedByUser, _ = FindUserByID(purchasereturn.PurchaseReturnedBy, fields)
	}

	if _, ok := selectFields["purchase_returned_by_signature.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "purchase_returned_by_signature")
		purchasereturn.PurchaseReturnedBySignature, _ = FindSignatureByID(purchasereturn.PurchaseReturnedBySignatureID, fields)
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		purchasereturn.CreatedByUser, _ = FindUserByID(purchasereturn.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		purchasereturn.UpdatedByUser, _ = FindUserByID(purchasereturn.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		purchasereturn.DeletedByUser, _ = FindUserByID(purchasereturn.DeletedBy, fields)
	}

	return purchasereturn, err
}

func IsPurchaseReturnExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

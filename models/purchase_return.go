package models

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/schollz/progressbar/v3"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/exp/slices"
	"gopkg.in/mgo.v2/bson"
)

type PurchaseReturnProduct struct {
	ProductID               primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name                    string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic            string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode                string             `bson:"item_code,omitempty" json:"item_code,omitempty"`
	PartNumber              string             `bson:"part_number,omitempty" json:"part_number,omitempty"`
	Quantity                float64            `json:"quantity" bson:"quantity"`
	Unit                    string             `bson:"unit,omitempty" json:"unit,omitempty"`
	PurchaseReturnUnitPrice float64            `bson:"purchasereturn_unit_price,omitempty" json:"purchasereturn_unit_price,omitempty"`
	Discount                float64            `bson:"discount" json:"discount"`
	DiscountPercent         float64            `bson:"discount_percent" json:"discount_percent"`
	UnitDiscount            float64            `bson:"unit_discount" json:"unit_discount"`
	UnitDiscountPercent     float64            `bson:"unit_discount_percent" json:"unit_discount_percent"`
	Selected                bool               `bson:"selected" json:"selected"`
}

// PurchaseReturn : PurchaseReturn structure
type PurchaseReturn struct {
	ID                     primitive.ObjectID      `json:"id,omitempty" bson:"_id,omitempty"`
	PurchaseID             *primitive.ObjectID     `json:"purchase_id,omitempty" bson:"purchase_id,omitempty"`
	PurchaseCode           string                  `bson:"purchase_code,omitempty" json:"purchase_code,omitempty"`
	Date                   *time.Time              `bson:"date,omitempty" json:"date,omitempty"`
	DateStr                string                  `json:"date_str,omitempty" bson:"-"`
	Code                   string                  `bson:"code,omitempty" json:"code,omitempty"`
	StoreID                *primitive.ObjectID     `json:"store_id,omitempty" bson:"store_id,omitempty"`
	VendorID               *primitive.ObjectID     `json:"vendor_id,omitempty" bson:"vendor_id,omitempty"`
	VendorInvoiceNumber    string                  `bson:"vendor_invoice_no,omitempty" json:"vendor_invoice_no,omitempty"`
	Store                  *Store                  `json:"store,omitempty"`
	Vendor                 *Vendor                 `json:"vendor,omitempty"`
	Products               []PurchaseReturnProduct `bson:"products,omitempty" json:"products,omitempty"`
	PurchaseReturnedBy     *primitive.ObjectID     `json:"purchase_returned_by,omitempty" bson:"purchase_returned_by,omitempty"`
	PurchaseReturnedByUser *User                   `json:"purchase_returned_by_user,omitempty"`
	/*
		PurchaseReturnedBySignatureID   *primitive.ObjectID     `json:"purchase_returned_by_signature_id,omitempty" bson:"purchase_returned_signature_id,omitempty"`
		PurchaseReturnedBySignatureName string                  `json:"purchase_returned_by_signature_name,omitempty" bson:"purchase_returned_by_signature_name,omitempty"`
		PurchaseReturnedBySignature *Signature              `json:"purchase_returned_by_signature,omitempty"`
		SignatureDate               *time.Time              `bson:"signature_date,omitempty" json:"signature_date,omitempty"`
		SignatureDateStr            string                  `json:"signature_date_str,omitempty"`
	*/
	VatPercent             *float64 `bson:"vat_percent" json:"vat_percent"`
	ShippingOrHandlingFees float64  `bson:"shipping_handling_fees" json:"shipping_handling_fees"`
	Discount               float64  `bson:"discount" json:"discount"`
	DiscountPercent        float64  `bson:"discount_percent" json:"discount_percent"`
	IsDiscountPercent      bool     `bson:"is_discount_percent" json:"is_discount_percent"`
	Status                 string   `bson:"status,omitempty" json:"status,omitempty"`
	TotalQuantity          float64  `bson:"total_quantity" json:"total_quantity"`
	VatPrice               float64  `bson:"vat_price" json:"vat_price"`
	Total                  float64  `bson:"total" json:"total"`
	NetTotal               float64  `bson:"net_total" json:"net_total"`
	CashDiscount           float64  `bson:"cash_discount" json:"cash_discount"`
	PaymentStatus          string   `bson:"payment_status" json:"payment_status"`
	/*
		Deleted                bool                    `bson:"deleted,omitempty" json:"deleted,omitempty"`
		DeletedBy              *primitive.ObjectID     `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
		DeletedByUser          *User                   `json:"deleted_by_user,omitempty"`
		DeletedAt              *time.Time              `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	*/
	CreatedAt              *time.Time              `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt              *time.Time              `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy              *primitive.ObjectID     `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy              *primitive.ObjectID     `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser          *User                   `json:"created_by_user,omitempty"`
	UpdatedByUser          *User                   `json:"updated_by_user,omitempty"`
	PurchaseReturnedByName string                  `json:"purchase_returned_by_name,omitempty" bson:"purchase_returned_by_name,omitempty"`
	VendorName             string                  `json:"vendor_name,omitempty" bson:"vendor_name,omitempty"`
	StoreName              string                  `json:"store_name,omitempty" bson:"store_name,omitempty"`
	CreatedByName          string                  `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName          string                  `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName          string                  `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	TotalPaymentPaid       float64                 `bson:"total_payment_paid" json:"total_payment_paid"`
	BalanceAmount          float64                 `bson:"balance_amount" json:"balance_amount"`
	Payments               []PurchaseReturnPayment `bson:"payments" json:"payments"`
	PaymentsInput          []PurchasePayment       `bson:"-" json:"payments_input"`
	PaymentsCount          int64                   `bson:"payments_count" json:"payments_count"`
	PaymentMethods         []string                `json:"payment_methods" bson:"payment_methods"`
}

func (model *PurchaseReturn) AddPayments() error {
	for _, payment := range model.PaymentsInput {
		purchaseReturnPayment := PurchaseReturnPayment{
			PurchaseReturnID:   &model.ID,
			PurchaseReturnCode: model.Code,
			PurchaseID:         model.PurchaseID,
			PurchaseCode:       model.PurchaseCode,
			Amount:             payment.Amount,
			Method:             payment.Method,
			Date:               payment.Date,
			CreatedAt:          model.CreatedAt,
			UpdatedAt:          model.UpdatedAt,
			CreatedBy:          model.CreatedBy,
			CreatedByName:      model.CreatedByName,
			UpdatedBy:          model.UpdatedBy,
			UpdatedByName:      model.UpdatedByName,
			StoreID:            model.StoreID,
			StoreName:          model.StoreName,
		}
		err := purchaseReturnPayment.Insert()
		if err != nil {
			return err
		}
	}
	return nil
}

func (model *PurchaseReturn) UpdatePayments() error {
	store, err := FindStoreByID(model.StoreID, bson.M{})
	if err != nil {
		return err
	}

	model.GetPayments()
	now := time.Now()
	for _, payment := range model.PaymentsInput {
		if payment.ID.IsZero() {
			//Create new
			purchaseReturnPayment := PurchaseReturnPayment{
				PurchaseReturnID:   &model.ID,
				PurchaseReturnCode: model.Code,
				PurchaseID:         model.PurchaseID,
				PurchaseCode:       model.PurchaseCode,
				Amount:             payment.Amount,
				Method:             payment.Method,
				Date:               payment.Date,
				CreatedAt:          &now,
				UpdatedAt:          &now,
				CreatedBy:          model.CreatedBy,
				CreatedByName:      model.CreatedByName,
				UpdatedBy:          model.UpdatedBy,
				UpdatedByName:      model.UpdatedByName,
				StoreID:            model.StoreID,
				StoreName:          model.StoreName,
			}
			err := purchaseReturnPayment.Insert()
			if err != nil {
				return err
			}
		} else {
			//Update
			purchaseReturnPayment, err := store.FindPurchaseReturnPaymentByID(&payment.ID, bson.M{})
			if err != nil {
				return err
			}

			purchaseReturnPayment.Date = payment.Date
			purchaseReturnPayment.Amount = payment.Amount
			purchaseReturnPayment.Method = payment.Method
			purchaseReturnPayment.UpdatedAt = &now
			purchaseReturnPayment.UpdatedBy = model.UpdatedBy
			purchaseReturnPayment.UpdatedByName = model.UpdatedByName
			err = purchaseReturnPayment.Update()
			if err != nil {
				return err
			}
		}

	}

	//Deleting payments

	paymentsToDelete := []PurchaseReturnPayment{}

	for _, payment := range model.Payments {
		found := false
		for _, paymentInput := range model.PaymentsInput {
			if paymentInput.ID.Hex() == payment.ID.Hex() {
				found = true
				break
			}
		}
		if !found {
			paymentsToDelete = append(paymentsToDelete, payment)
		}
	}

	for _, payment := range paymentsToDelete {
		payment.Deleted = true
		payment.DeletedAt = &now
		payment.DeletedBy = model.UpdatedBy
		err := payment.Update()
		if err != nil {
			return err
		}
	}

	return nil
}

type PurchaseReturnStats struct {
	ID                        *primitive.ObjectID `json:"id" bson:"_id"`
	NetTotal                  float64             `json:"net_total" bson:"net_total"`
	VatPrice                  float64             `json:"vat_price" bson:"vat_price"`
	Discount                  float64             `json:"discount" bson:"discount"`
	CashDiscount              float64             `json:"cash_discount" bson:"cash_discount"`
	PaidPurchaseReturn        float64             `json:"paid_purchase_return" bson:"paid_purchase_return"`
	UnPaidPurchaseReturn      float64             `json:"unpaid_purchase_return" bson:"unpaid_purchase_return"`
	CashPurchaseReturn        float64             `json:"cash_purchase_return" bson:"cash_purchase_return"`
	BankAccountPurchaseReturn float64             `json:"bank_account_purchase_return" bson:"bank_account_purchase_return"`
	ShippingOrHandlingFees    float64             `json:"shipping_handling_fees" bson:"shipping_handling_fees"`
}

func (store *Store) GetPurchaseReturnStats(filter map[string]interface{}) (stats PurchaseReturnStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                    nil,
				"net_total":              bson.M{"$sum": "$net_total"},
				"vat_price":              bson.M{"$sum": "$vat_price"},
				"discount":               bson.M{"$sum": "$discount"},
				"cash_discount":          bson.M{"$sum": "$cash_discount"},
				"shipping_handling_fees": bson.M{"$sum": "$shipping_handling_fees"},
				"paid_purchase_return": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$payments",
						"as":    "payment",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
									bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
								}},
								"$$payment.amount",
								0,
							},
						},
					},
				}}},
				"unpaid_purchase_return": bson.M{"$sum": "$balance_amount"},
				"cash_purchase_return": bson.M{"$sum": bson.M{"$sum": bson.M{
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
				"bank_account_purchase_return": bson.M{"$sum": bson.M{"$sum": bson.M{
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
	}
	return stats, nil
}

func (purchasereturn *PurchaseReturn) AttributesValueChangeEvent(purchasereturnOld *PurchaseReturn) error {

	if purchasereturn.Status != purchasereturnOld.Status {
		/*
			purchasereturn.SetChangeLog(
				"attribute_value_change",
				"status",
				purchasereturnOld.Status,
				purchasereturn.Status,
			)
		*/

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
	store, err := FindStoreByID(purchasereturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if purchasereturn.StoreID != nil {
		store, err := FindStoreByID(purchasereturn.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchasereturn.StoreName = store.Name
	}

	if purchasereturn.VendorID != nil {
		vendor, err := store.FindVendorByID(purchasereturn.VendorID, bson.M{"id": 1, "name": 1})
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

	/*
		if purchasereturn.PurchaseReturnedBySignatureID != nil {
			PurchaseReturnedBySignature, err := FindSignatureByID(purchasereturn.PurchaseReturnedBySignatureID, bson.M{"id": 1, "name": 1})
			if err != nil {
				return err
			}
			purchasereturn.PurchaseReturnedBySignatureName = PurchaseReturnedBySignature.Name
		}*/

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

	/*
		if purchasereturn.DeletedBy != nil && !purchasereturn.DeletedBy.IsZero() {
			deletedByUser, err := FindUserByID(purchasereturn.DeletedBy, bson.M{"id": 1, "name": 1})
			if err != nil {
				return err
			}
			purchasereturn.DeletedByName = deletedByUser.Name
		}*/

	for i, product := range purchasereturn.Products {
		productObject, err := store.FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1, "part_number": 1})
		if err != nil {
			return err
		}
		purchasereturn.Products[i].Name = productObject.Name
		purchasereturn.Products[i].NameInArabic = productObject.NameInArabic
		purchasereturn.Products[i].ItemCode = productObject.ItemCode
		purchasereturn.Products[i].PartNumber = productObject.PartNumber
	}

	return nil
}

func (purchaseReturn *PurchaseReturn) FindNetTotal() {
	netTotal := float64(0.0)
	purchaseReturn.FindTotal()
	netTotal = purchaseReturn.Total
	purchaseReturn.ShippingOrHandlingFees = RoundTo2Decimals(purchaseReturn.ShippingOrHandlingFees)
	purchaseReturn.Discount = RoundTo2Decimals(purchaseReturn.Discount)
	netTotal += purchaseReturn.ShippingOrHandlingFees
	netTotal -= purchaseReturn.Discount

	purchaseReturn.FindVatPrice()
	netTotal += purchaseReturn.VatPrice

	purchaseReturn.NetTotal = RoundTo2Decimals(netTotal)
	purchaseReturn.CalculateDiscountPercentage()
}

func (purchaseReturn *PurchaseReturn) CalculateDiscountPercentage() {
	if purchaseReturn.NetTotal == 0 {
		purchaseReturn.DiscountPercent = 0
	}

	if purchaseReturn.Discount <= 0 {
		purchaseReturn.DiscountPercent = 0.00
		return
	}

	percentage := (purchaseReturn.Discount / purchaseReturn.NetTotal) * 100
	purchaseReturn.DiscountPercent = RoundTo2Decimals(percentage) // Use rounding here
}

func (purchaseReturn *PurchaseReturn) FindTotal() {
	total := float64(0.0)
	for i, product := range purchaseReturn.Products {
		if !product.Selected {
			continue
		}

		purchaseReturn.Products[i].PurchaseReturnUnitPrice = RoundTo2Decimals(product.PurchaseReturnUnitPrice)
		purchaseReturn.Products[i].UnitDiscount = RoundTo2Decimals(product.UnitDiscount)
		if purchaseReturn.Products[i].UnitDiscount > 0 {
			purchaseReturn.Products[i].UnitDiscountPercent = RoundTo2Decimals((purchaseReturn.Products[i].UnitDiscount / purchaseReturn.Products[i].PurchaseReturnUnitPrice) * 100)
		}

		total += RoundTo2Decimals(product.Quantity * (purchaseReturn.Products[i].PurchaseReturnUnitPrice - purchaseReturn.Products[i].UnitDiscount))
	}

	purchaseReturn.Total = RoundTo2Decimals(total)
}

func (purchaseReturn *PurchaseReturn) FindVatPrice() {
	vatPrice := ((*purchaseReturn.VatPercent / float64(100.00)) * ((purchaseReturn.Total + purchaseReturn.ShippingOrHandlingFees) - purchaseReturn.Discount))
	purchaseReturn.VatPrice = RoundTo2Decimals(vatPrice)
}

func (purchasereturn *PurchaseReturn) FindTotalQuantity() {
	totalQuantity := float64(0.00)
	for _, product := range purchasereturn.Products {
		if !product.Selected {
			continue
		}

		totalQuantity += product.Quantity
	}
	purchasereturn.TotalQuantity = totalQuantity
}

func (store *Store) SearchPurchaseReturn(w http.ResponseWriter, r *http.Request) (purchasereturns []PurchaseReturn, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[purchase_id]"]
	if ok && len(keys[0]) >= 1 {
		orderID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return purchasereturns, criterias, err
		}
		criterias.SearchBy["purchase_id"] = orderID
	}

	keys, ok = r.URL.Query()["search[cash_discount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return purchasereturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["cash_discount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["cash_discount"] = value
		}
	}

	keys, ok = r.URL.Query()["search[payment_status]"]
	if ok && len(keys[0]) >= 1 {
		paymentStatusList := strings.Split(keys[0], ",")
		if len(paymentStatusList) > 0 {
			criterias.SearchBy["payment_status"] = bson.M{"$in": paymentStatusList}
		}
	}

	keys, ok = r.URL.Query()["search[payment_methods]"]
	if ok && len(keys[0]) >= 1 {
		paymentMethods := strings.Split(keys[0], ",")

		if len(paymentMethods) > 0 {
			criterias.SearchBy["payment_methods"] = bson.M{"$in": paymentMethods}
		}
	}

	keys, ok = r.URL.Query()["search[total_payment_paid]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return purchasereturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["total_payment_paid"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["total_payment_paid"] = value
		}

	}

	keys, ok = r.URL.Query()["search[balance_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return purchasereturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["balance_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["balance_amount"] = value
		}
	}

	keys, ok = r.URL.Query()["search[payments_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return purchasereturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["payments_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["payments_count"] = value
		}
	}

	keys, ok = r.URL.Query()["search[date_str]"]
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

		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}
	}

	keys, ok = r.URL.Query()["search[to_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		endDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return purchasereturns, criterias, err
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
			return purchasereturns, criterias, err
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
			return purchasereturns, criterias, err
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
			return purchasereturns, criterias, err
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

	keys, ok = r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[vendor_invoice_no]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["vendor_invoice_no"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchasereturn")

	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	storeSelectFields := map[string]interface{}{}
	vendorSelectFields := map[string]interface{}{}
	purchaseReturnedByUserSelectFields := map[string]interface{}{}
	//purchaseReturnedBySignatureSelectFields := map[string]interface{}{}
	createdByUserSelectFields := map[string]interface{}{}
	updatedByUserSelectFields := map[string]interface{}{}
	//deletedByUserSelectFields := map[string]interface{}{}

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

		/*
			if _, ok := criterias.Select["purchase_returned_signature.id"]; ok {
				purchaseReturnedBySignatureSelectFields = ParseRelationalSelectString(keys[0], "purchase_returned_signature")
			}*/

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			createdByUserSelectFields = ParseRelationalSelectString(keys[0], "created_by_user")
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			updatedByUserSelectFields = ParseRelationalSelectString(keys[0], "updated_by_user")
		}

		/*
			if _, ok := criterias.Select["deleted_by_user.id"]; ok {
				deletedByUserSelectFields = ParseRelationalSelectString(keys[0], "deleted_by_user")
			}*/

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
			purchasereturn.Vendor, _ = store.FindVendorByID(purchasereturn.VendorID, vendorSelectFields)
		}

		if _, ok := criterias.Select["purchase_returned_by_user.id"]; ok {
			purchasereturn.PurchaseReturnedByUser, _ = FindUserByID(purchasereturn.PurchaseReturnedBy, purchaseReturnedByUserSelectFields)
		}

		/*
			if _, ok := criterias.Select["purchase_returned_by_signature.id"]; ok {
				purchasereturn.PurchaseReturnedBySignature, _ = FindSignatureByID(purchasereturn.PurchaseReturnedBySignatureID, purchaseReturnedBySignatureSelectFields)
			}*/

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			purchasereturn.CreatedByUser, _ = FindUserByID(purchasereturn.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			purchasereturn.UpdatedByUser, _ = FindUserByID(purchasereturn.UpdatedBy, updatedByUserSelectFields)
		}
		/*
			if _, ok := criterias.Select["deleted_by_user.id"]; ok {
				purchasereturn.DeletedByUser, _ = FindUserByID(purchasereturn.DeletedBy, deletedByUserSelectFields)
			}*/

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

	store, err := FindStoreByID(purchasereturn.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = "invalid store id"
		return errs
	}

	purchase, err := store.FindPurchaseByID(purchasereturn.PurchaseID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["purchase_id"] = err.Error()
		return errs
	}

	if purchasereturn.PurchaseID == nil || purchasereturn.PurchaseID.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
		errs["purchase_id"] = "Purchase ID is required"
		return errs
	}

	/*
		if purchasereturn.NetTotal <= 0 {
			errs["net_total"] = "Net total should be greater than 0.00 "
		}
	*/

	if purchasereturn.NetTotal > 0 && purchasereturn.CashDiscount >= purchasereturn.NetTotal {
		errs["cash_discount"] = "Cash discount should not be >= " + fmt.Sprintf("%.02f", purchasereturn.NetTotal)
	}

	maxDiscountAllowed := 0.00
	if scenario == "update" {
		maxDiscountAllowed = purchasereturn.Discount - (purchase.ReturnDiscount - oldPurchaseReturn.Discount)
	} else {
		maxDiscountAllowed = purchasereturn.Discount - purchase.ReturnDiscount
	}

	if purchasereturn.Discount > maxDiscountAllowed {
		errs["discount"] = "Discount shouldn't greater than " + fmt.Sprintf("%.2f", (maxDiscountAllowed))
	}

	maxCashDiscountAllowed := 0.00
	if scenario == "update" {
		maxCashDiscountAllowed = purchasereturn.CashDiscount - (purchase.ReturnCashDiscount - oldPurchaseReturn.CashDiscount)
	} else {
		maxCashDiscountAllowed = purchasereturn.CashDiscount - purchase.ReturnCashDiscount
	}

	if purchasereturn.CashDiscount > maxCashDiscountAllowed {
		errs["cash_discount"] = "Cash discount shouldn't greater than " + fmt.Sprintf("%.2f", (maxCashDiscountAllowed))
	}

	totalPayment := float64(0.00)
	for _, payment := range purchasereturn.PaymentsInput {
		if payment.Amount != nil {
			totalPayment += *payment.Amount
		}
	}

	for index, payment := range purchasereturn.PaymentsInput {
		if govalidator.IsNull(payment.DateStr) {
			errs["payment_date_"+strconv.Itoa(index)] = "Payment date is required"
		} else {
			const shortForm = "2006-01-02T15:04:05Z07:00"
			date, err := time.Parse(shortForm, payment.DateStr)
			if err != nil {
				errs["payment_date_"+strconv.Itoa(index)] = "Invalid date format"
			}

			purchasereturn.PaymentsInput[index].Date = &date
			payment.Date = &date

			if purchasereturn.Date != nil && IsAfter(purchasereturn.Date, purchasereturn.PaymentsInput[index].Date) {
				errs["payment_date_"+strconv.Itoa(index)] = "Payment date time should be greater than or equal to sales return date time"
			}
		}

		if payment.Amount == nil {
			errs["payment_amount_"+strconv.Itoa(index)] = "Payment amount is required"
		} else if *payment.Amount == 0 {
			errs["payment_amount_"+strconv.Itoa(index)] = "Payment amount should be greater than zero"
		}

		if payment.Method == "" {
			errs["payment_method_"+strconv.Itoa(index)] = "Payment method is required"
		}

		if payment.DateStr != "" && payment.Amount != nil && payment.Method != "" {
			if *payment.Amount <= 0 {
				errs["payment_amount_"+strconv.Itoa(index)] = "Payment amount should be greater than zero"
			}

			/*
				maxAllowedAmount := (salesreturn.NetTotal - salesreturn.CashDiscount) - (totalPayment - *payment.Amount)

				if maxAllowedAmount < 0 {
					maxAllowedAmount = 0
				}

				if maxAllowedAmount == 0 {
					errs["payment_amount_"+strconv.Itoa(index)] = "Total amount should not exceed " + fmt.Sprintf("%.02f", (salesreturn.NetTotal-salesreturn.CashDiscount)) + ", Please delete this payment"
				} else if *payment.Amount > RoundFloat(maxAllowedAmount, 2) {
					errs["payment_amount_"+strconv.Itoa(index)] = "Amount should not be greater than " + fmt.Sprintf("%.02f", (maxAllowedAmount)) + ", Please delete or edit this payment"
				}
			*/

		}
	} //end for

	purchasereturn.PurchaseCode = purchase.Code

	if govalidator.IsNull(purchasereturn.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, purchasereturn.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		purchasereturn.Date = &date
	}

	/*
		if !govalidator.IsNull(purchasereturn.SignatureDateStr) {
			const shortForm = "Jan 02 2006"
			date, err := time.Parse(shortForm, purchasereturn.SignatureDateStr)
			if err != nil {
				errs["signature_date_str"] = "Invalid date format"
			}
			purchasereturn.SignatureDate = &date
		}*/

	if scenario == "update" {
		if purchasereturn.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsPurchaseReturnExists(&purchasereturn.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid PurchaseReturn:" + purchasereturn.ID.Hex()
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
		exists, err := store.IsVendorExists(purchasereturn.VendorID)
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

	/*
		if purchasereturn.PurchaseReturnedBySignatureID != nil && !purchasereturn.PurchaseReturnedBySignatureID.IsZero() {
			exists, err := IsSignatureExists(purchasereturn.PurchaseReturnedBySignatureID)
			if err != nil {
				errs["order_placed_by_signature_id"] = err.Error()
				return errs
			}

			if !exists {
				errs["order_placed_by_signature_id"] = "Invalid Order Placed By Signature:" + purchasereturn.PurchaseReturnedBySignatureID.Hex()
			}
		}*/

	if len(purchasereturn.Products) == 0 {
		errs["product_id"] = "Atleast 1 product is required for purchase return"
	}

	for index, purchaseReturnProduct := range purchasereturn.Products {
		if !purchaseReturnProduct.Selected {
			continue
		}

		if purchaseReturnProduct.ProductID.IsZero() {
			errs["product_id_"+strconv.Itoa(index)] = "Product is required for purchase return"
		} else {
			exists, err := store.IsProductExists(&purchaseReturnProduct.ProductID)
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

		if purchaseReturnProduct.UnitDiscount > purchaseReturnProduct.PurchaseReturnUnitPrice && purchaseReturnProduct.PurchaseReturnUnitPrice > 0 {
			errs["unit_discount_"+strconv.Itoa(index)] = "Unit discount should not be greater than unit price"
		}

		for _, purchaseProduct := range purchase.Products {
			if purchaseProduct.ProductID == purchaseReturnProduct.ProductID {

				maxAllowedQuantity := 0.00
				if scenario == "update" {
					maxAllowedQuantity = purchaseProduct.Quantity - (purchaseProduct.QuantityReturned - oldPurchaseReturn.Products[index].Quantity)
				} else {
					maxAllowedQuantity = purchaseProduct.Quantity - purchaseProduct.QuantityReturned
				}

				if purchaseReturnProduct.Quantity > maxAllowedQuantity {
					errs["quantity_"+strconv.Itoa(index)] = "Quantity should not be greater than purchased quantity: " + fmt.Sprintf("%.02f", maxAllowedQuantity) + " " + purchaseProduct.Unit
				}
				/*
					purchasedQty := 0.0

					if scenario == "update" {
						//purchasedQty = RoundFloat(purchaseProduct.Quantity, 2)
						purchasedQty = RoundFloat((purchaseProduct.Quantity - (purchaseProduct.QuantityReturned - purchaseReturnProduct.Quantity)), 2)
					} else {
						purchasedQty = RoundFloat((purchaseProduct.Quantity - purchaseProduct.QuantityReturned), 2)
					}

					if purchasedQty == 0 {
						errs["quantity_"+strconv.Itoa(index)] = "Already returned all purchased quantities"
					} else if purchaseReturnProduct.Quantity > float64(purchasedQty) {
						errs["quantity_"+strconv.Itoa(index)] = "Quantity should not be greater than purchased quantity: " + fmt.Sprintf("%.02f", purchasedQty) + " " + purchaseProduct.Unit
					}

					if purchaseReturnProduct.PurchaseReturnUnitPrice > purchaseProduct.PurchaseUnitPrice {
						errs["purchasereturned_unit_price_"+strconv.Itoa(index)] = "Purchase Return Unit Price should not be greater than purchase Unit Price: " + fmt.Sprintf("%.02f", purchaseProduct.PurchaseUnitPrice)
					}
				*/
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

func (purchaseReturn *PurchaseReturn) UpdateReturnedQuantityInPurchaseProduct(purchaseReturnOld *PurchaseReturn) error {
	store, err := FindStoreByID(purchaseReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	purchase, err := store.FindPurchaseByID(purchaseReturn.PurchaseID, bson.M{})
	if err != nil {
		return err
	}

	if purchaseReturnOld != nil {
		for _, purchaseReturnProduct := range purchaseReturnOld.Products {
			if !purchaseReturnProduct.Selected {
				continue
			}

			for index2, purchaseProduct := range purchase.Products {
				if purchaseProduct.ProductID == purchaseReturnProduct.ProductID {
					purchase.Products[index2].QuantityReturned -= purchaseReturnProduct.Quantity
				}
			}
		}
	}

	for _, purchaseReturnProduct := range purchaseReturn.Products {
		if !purchaseReturnProduct.Selected {
			continue
		}

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
	store, err := FindStoreByID(purchasereturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	for _, purchasereturnProduct := range purchasereturn.Products {
		if !purchasereturnProduct.Selected {
			continue
		}

		product, err := store.FindProductByID(&purchasereturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if productStoreTemp, ok := product.ProductStores[purchasereturn.StoreID.Hex()]; ok {
			productStoreTemp.Stock += purchasereturnProduct.Quantity
			product.ProductStores[purchasereturn.StoreID.Hex()] = productStoreTemp
		} else {
			product.ProductStores = map[string]ProductStore{}
			product.ProductStores[purchasereturn.StoreID.Hex()] = ProductStore{
				StoreID: *purchasereturn.StoreID,
				Stock:   purchasereturnProduct.Quantity,
			}
		}

		/*
			storeExistInProductStore := false
			for k, productStore := range product.ProductStores {
				if productStore.StoreID.Hex() == purchasereturn.StoreID.Hex() {
					product.ProductStores[k].Stock += purchasereturnProduct.Quantity
					storeExistInProductStore = true
					break
				}
			}

			if !storeExistInProductStore {
				productStore := ProductStore{
					StoreID: *purchasereturn.StoreID,
					Stock:   purchasereturnProduct.Quantity,
				}
				product.Stores = append(product.Stores, productStore)
			}
		*/

		err = product.Update()
		if err != nil {
			return err
		}
	}
	return nil
}

func (purchasereturn *PurchaseReturn) RemoveStock() (err error) {
	store, err := FindStoreByID(purchasereturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	for _, purchasereturnProduct := range purchasereturn.Products {
		if !purchasereturnProduct.Selected {
			continue
		}

		product, err := store.FindProductByID(&purchasereturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		for k, productStore := range product.ProductStores {
			if productStore.StoreID.Hex() == purchasereturn.StoreID.Hex() {
				if productStoreTemp, ok := product.ProductStores[k]; ok {
					productStoreTemp.Stock -= purchasereturnProduct.Quantity
					product.ProductStores[k] = productStoreTemp
				}
				//product.Stores[k].Stock -= purchasereturnProduct.Quantity
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
	store, err := FindStoreByID(purchasereturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	for _, purchasereturnProduct := range purchasereturn.Products {
		if !purchasereturnProduct.Selected {
			continue
		}

		product, err := store.FindProductByID(&purchasereturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}
		//storeExistInProductUnitPrice := false

		if productStoreTemp, ok := product.ProductStores[purchasereturn.StoreID.Hex()]; ok {
			//storeExistInProductUnitPrice = true
			productStoreTemp.PurchaseUnitPrice = purchasereturnProduct.PurchaseReturnUnitPrice
			product.ProductStores[purchasereturn.StoreID.Hex()] = productStoreTemp
		} else {
			product.ProductStores = map[string]ProductStore{}
			product.ProductStores[purchasereturn.StoreID.Hex()] = ProductStore{
				StoreID:           *purchasereturn.StoreID,
				PurchaseUnitPrice: purchasereturnProduct.PurchaseReturnUnitPrice,
			}
		}

		/*
			for k, productStore := range product.ProductStores {
				if productStore.StoreID.Hex() == purchasereturn.StoreID.Hex() {
					if productStoreTemp, ok := product.ProductStores[k]; ok {
						productStoreTemp.PurchaseUnitPrice = purchasereturnProduct.PurchaseReturnUnitPrice
						product.ProductStores[k] = productStoreTemp
					}
					//product.ProductStores[k].PurchaseUnitPrice = purchasereturnProduct.PurchaseReturnUnitPrice
					storeExistInProductUnitPrice = true
					break
				}
			}
		*/

		/*
			if !storeExistInProductUnitPrice {
				productStore := ProductStore{
					StoreID:           *purchasereturn.StoreID,
					PurchaseUnitPrice: purchasereturnProduct.PurchaseReturnUnitPrice,
				}

				product.ProductStores = map[string]ProductStore{}
				product.ProductStores[purchasereturn.StoreID.Hex()] = productStore
				//product.ProductStores = append(product.ProductStores, productStore)
			}
		*/
		err = product.Update()
		if err != nil {
			return err
		}
	}

	return nil
}

func (purchasereturn *PurchaseReturn) GenerateCode(startFrom int, storeCode string) (string, error) {
	store, err := FindStoreByID(purchasereturn.StoreID, bson.M{})
	if err != nil {
		return "", err
	}

	count, err := store.GetTotalCount(bson.M{"store_id": purchasereturn.StoreID}, "purchasereturn")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return storeCode + "-" + strconv.Itoa(code+1), nil
}

func (purchasereturn *PurchaseReturn) Insert() error {
	collection := db.GetDB("store_" + purchasereturn.StoreID.Hex()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	purchasereturn.ID = primitive.NewObjectID()

	_, err := collection.InsertOne(ctx, &purchasereturn)
	if err != nil {
		return err
	}

	return nil
}

func (model *PurchaseReturn) MakeRedisCode() error {
	store, err := FindStoreByID(model.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := model.StoreID.Hex() + "_purchase_return_invoice_counter"

	// Check if counter exists, if not set it to the custom startFrom - 1
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}

	if exists == 0 {
		count, err := store.GetPurchaseReturnCount()
		if err != nil {
			return err
		}

		startFrom := store.PurchaseReturnSerialNumber.StartFromCount

		startFrom += count
		// Set the initial counter value (startFrom - 1) so that the first increment gives startFrom
		err = db.RedisClient.Set(redisKey, startFrom-1, 0).Err()
		if err != nil {
			return err
		}
	}

	incr, err := db.RedisClient.Incr(redisKey).Result()
	if err != nil {
		return err
	}

	paddingCount := store.PurchaseReturnSerialNumber.PaddingCount

	invoiceID := fmt.Sprintf("%s-%0*d", store.PurchaseReturnSerialNumber.Prefix, paddingCount, incr)
	model.Code = invoiceID
	return nil
}

func (purchaseReturn *PurchaseReturn) MakeCode() error {
	store, err := FindStoreByID(purchaseReturn.StoreID, bson.M{"code": 1})
	if err != nil {
		return err
	}

	if store.Code != "GUOCJ" && store.Code != "GUOJ" {
		return purchaseReturn.MakeRedisCode()
	}

	lastQuotation, err := store.FindLastPurchaseReturnByStoreID(purchaseReturn.StoreID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}
	if lastQuotation == nil {
		store, err := FindStoreByID(purchaseReturn.StoreID, bson.M{})
		if err != nil {
			return err
		}
		purchaseReturn.Code = store.Code + "-400000"
	} else {
		splits := strings.Split(lastQuotation.Code, "-")
		if len(splits) == 2 {
			storeCode := splits[0]
			codeStr := splits[1]
			codeInt, err := strconv.Atoi(codeStr)
			if err != nil {
				return err
			}
			codeInt++
			purchaseReturn.Code = storeCode + "-" + strconv.Itoa(codeInt)
		}
	}

	for {
		exists, err := purchaseReturn.IsCodeExists()
		if err != nil {
			return err
		}
		if !exists {
			break
		}

		splits := strings.Split(lastQuotation.Code, "-")
		storeCode := splits[0]
		codeStr := splits[1]
		codeInt, err := strconv.Atoi(codeStr)
		if err != nil {
			return err
		}
		codeInt++
		purchaseReturn.Code = storeCode + "-" + strconv.Itoa(codeInt)
	}

	return nil
}

func (store *Store) FindLastPurchaseReturnByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (purchaseReturn *PurchaseReturn, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": storeID}, findOneOptions).
		Decode(&purchaseReturn)
	if err != nil {
		return nil, err
	}

	return purchaseReturn, err
}

func (purchasereturn *PurchaseReturn) Update() error {
	collection := db.GetDB("store_" + purchasereturn.StoreID.Hex()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	_, err := collection.UpdateOne(
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

func (purchasereturn *PurchaseReturn) UpdatePurchaseReturnDiscount(replace bool) error {

	store, err := FindStoreByID(purchasereturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	purchase, err := store.FindPurchaseByID(purchasereturn.PurchaseID, bson.M{})
	if err != nil {
		return err
	}
	if replace {
		purchase.ReturnDiscount = purchasereturn.Discount
	} else {
		purchase.ReturnDiscount += purchasereturn.Discount
	}

	return purchase.Update()
}

func (purchasereturn *PurchaseReturn) IsCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + purchasereturn.StoreID.Hex()).Collection("purchasereturn")
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

func (purchasereturn *PurchaseReturn) DeletePurchaseReturn(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + purchasereturn.StoreID.Hex()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = purchasereturn.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	/*
		userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
		if err != nil {
			return err
		}

		purchasereturn.Deleted = true
		purchasereturn.DeletedBy = &userID
		now := time.Now()
		purchasereturn.DeletedAt = &now
	*/

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
	collection := db.GetDB("store_" + purchasereturn.StoreID.Hex()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = collection.DeleteOne(ctx, bson.M{"_id": purchasereturn.ID})
	if err != nil {
		return err
	}
	return nil
}

func (store *Store) FindPurchaseReturnByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (purchasereturn *PurchaseReturn, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchasereturn")
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
		Decode(&purchasereturn)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["purchase_returned_by.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "purchase_returned_by")
		purchasereturn.PurchaseReturnedByUser, _ = FindUserByID(purchasereturn.PurchaseReturnedBy, fields)
	}

	/*
		if _, ok := selectFields["purchase_returned_by_signature.id"]; ok {
			fields := ParseRelationalSelectString(selectFields, "purchase_returned_by_signature")
			purchasereturn.PurchaseReturnedBySignature, _ = FindSignatureByID(purchasereturn.PurchaseReturnedBySignatureID, fields)
		}*/

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		purchasereturn.CreatedByUser, _ = FindUserByID(purchasereturn.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		purchasereturn.UpdatedByUser, _ = FindUserByID(purchasereturn.UpdatedBy, fields)
	}

	/*
		if _, ok := selectFields["deleted_by_user.id"]; ok {
			fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
			purchasereturn.DeletedByUser, _ = FindUserByID(purchasereturn.DeletedBy, fields)
		}*/

	return purchasereturn, err
}

func (store *Store) IsPurchaseReturnExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

func (store *Store) ProcessPurchaseReturns() error {
	log.Print("Processing purchase returns")
	totalCount, err := store.GetTotalCount(bson.M{}, "purchasereturn")
	if err != nil {
		return err
	}
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchasereturn")
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

	bar := progressbar.Default(totalCount)
	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		model := PurchaseReturn{}
		err = cur.Decode(&model)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		/*
			for i, paymentMethod := range model.PaymentMethods {
				if paymentMethod == "bank_account" {
					model.PaymentMethods[i] = "bank_card"
				}
			}

			for i, payment := range model.Payments {
				if payment.Method == "bank_account" {
					model.Payments[i].Method = "bank_card"
				}
			}*/

		for i, product := range model.Products {
			if product.Discount > 0 {
				model.Products[i].UnitDiscount = product.Discount / product.Quantity
				model.Products[i].UnitDiscountPercent = product.DiscountPercent
			}
		}

		err = model.Update()
		if err != nil {
			return errors.New("Error updating: " + err.Error())
		}

		/*
			err = model.ClearProductsPurchaseReturnHistory()
			if err != nil {
				return errors.New("error deleting product purchase return history: " + err.Error())
			}

			err = model.AddProductsPurchaseReturnHistory()
			if err != nil {
				return errors.New("error Adding product purchase return history: " + err.Error())
			}
		*/

		/*
			model.PaymentStatus = "paid"
			model.PaymentMethod = "cash"
			err = model.Update()
			if err != nil {
				return err
			}

			count, _ := model.GetPaymentsCount()
			if count == 0 {
				model.AddPayment()
			}
		*/

		/*


			model.GetPayments()

			err = model.SetProductsPurchaseReturnStats()
			if err != nil {
				return errors.New("error setting products purchase return stats: " + err.Error())
			}
		*/

		/*
			if model.PaymentStatus == "" {
				model.PaymentStatus = "paid"
			}

			if model.PaymentMethod == "" {
				model.PaymentMethod = "cash"
			}

			totalPaymentsCount, err := GetTotalCount(bson.M{"purchase_return_id": model.ID}, "purchase_return_payment")
			if err != nil {
				return err
			}

			if totalPaymentsCount == 0 {
				err = model.AddPayment()
				if err != nil {
					return err
				}
			}
		*/

		/*
			d := model.Date.Add(time.Hour * time.Duration(-3))
			model.Date = &d
		*/
		//model.Date = model.CreatedAt

		/*
			err = model.SetVendorPurchaseReturnStats()
			if err != nil {
				return err
			}
		*/

		/*
			model.UpdatePurchaseReturnCount()

			err = model.Update()
			if err != nil {
				return err
			}
		*/
		/*
			model.GetPayments()

			err = model.UndoAccounting()
			if err != nil {
				return errors.New("error undo accounting: " + err.Error())
			}

			err = model.DoAccounting()
			if err != nil {
				return errors.New("error doing accounting: " + err.Error())
			}
		*/

		/*
			err = model.Update()
			if err != nil {
				return err
			}*/

		bar.Add(1)
	}
	log.Print("Purchase returns DONE!")
	return nil
}

func (model *PurchaseReturn) GetPaymentsCount() (count int64, err error) {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("purchase_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"purchase_return_id": model.ID,
		"deleted":            bson.M{"$ne": true},
	})
}

func (model *PurchaseReturn) GetPayments() (payments []PurchaseReturnPayment, err error) {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("purchase_return_payment")
	ctx := context.Background()
	findOptions := options.Find()
	sortBy := map[string]interface{}{}
	sortBy["date"] = 1
	findOptions.SetSort(sortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{"purchase_return_id": model.ID, "deleted": bson.M{"$ne": true}}, findOptions)
	if err != nil {
		return payments, errors.New("Error fetching purchase return payment history" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	totalPaymentPaid := float64(0.0)
	paymentMethods := []string{}

	//	log.Print("Starting for")
	for i := 0; cur != nil && cur.Next(ctx); i++ {
		//log.Print("Loop")
		err := cur.Err()
		if err != nil {
			return payments, errors.New("Cursor error:" + err.Error())
		}
		payment := PurchaseReturnPayment{}
		err = cur.Decode(&payment)
		if err != nil {
			return payments, errors.New("Cursor decode error:" + err.Error())
		}

		payments = append(payments, payment)

		totalPaymentPaid += *payment.Amount

		if !slices.Contains(paymentMethods, payment.Method) {
			paymentMethods = append(paymentMethods, payment.Method)
		}
	} //end for loop

	model.TotalPaymentPaid = ToFixed(totalPaymentPaid, 2)
	model.BalanceAmount = ToFixed((model.NetTotal-model.CashDiscount)-totalPaymentPaid, 2)
	model.PaymentMethods = paymentMethods
	model.Payments = payments
	model.PaymentsCount = int64(len(payments))

	if ToFixed((model.NetTotal-model.CashDiscount), 2) <= ToFixed(totalPaymentPaid, 2) {
		model.PaymentStatus = "paid"
	} else if ToFixed(totalPaymentPaid, 2) > 0 {
		model.PaymentStatus = "paid_partially"
	} else if ToFixed(totalPaymentPaid, 2) <= 0 {
		model.PaymentStatus = "not_paid"
	}

	return payments, err
}

func (model *PurchaseReturn) ClearPayments() error {
	//log.Printf("Clearing Purchase payment history of purchase id:%s", model.Code)
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("purchase_return_payment")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"purchase_return_id": model.ID})
	if err != nil {
		return err
	}
	return nil
}

type ProductPurchaseReturnStats struct {
	PurchaseReturnCount    int64   `json:"purchase_return_count" bson:"purchase_return_count"`
	PurchaseReturnQuantity float64 `json:"purchase_return_quantity" bson:"purchase_return_quantity"`
	PurchaseReturn         float64 `json:"purchase_return" bson:"purchase_return"`
}

func (product *Product) SetProductPurchaseReturnStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_purchase_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductPurchaseReturnStats

	filter := map[string]interface{}{
		"store_id":   storeID,
		"product_id": product.ID,
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                      nil,
				"purchase_return_count":    bson.M{"$sum": 1},
				"purchase_return_quantity": bson.M{"$sum": "$quantity"},
				"purchase_return":          bson.M{"$sum": "$net_price"},
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

		stats.PurchaseReturn = RoundFloat(stats.PurchaseReturn, 2)
	}

	for storeIndex, store := range product.ProductStores {
		if store.StoreID.Hex() == storeID.Hex() {
			if productStoreTemp, ok := product.ProductStores[storeIndex]; ok {
				productStoreTemp.PurchaseReturnCount = stats.PurchaseReturnCount
				productStoreTemp.PurchaseReturnQuantity = stats.PurchaseReturnQuantity
				productStoreTemp.PurchaseReturn = stats.PurchaseReturn
				product.ProductStores[storeIndex] = productStoreTemp
			}
			//product.Stores[storeIndex].PurchaseReturnCount = stats.PurchaseReturnCount
			//product.Stores[storeIndex].PurchaseReturnQuantity = stats.PurchaseReturnQuantity
			//product.Stores[storeIndex].PurchaseReturn = stats.PurchaseReturn
			err = product.Update()
			if err != nil {
				return err
			}
			break
		}
	}

	return nil
}

func (purchaseReturn *PurchaseReturn) SetProductsPurchaseReturnStats() error {
	store, err := FindStoreByID(purchaseReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	for _, purchaseReturnProduct := range purchaseReturn.Products {
		product, err := store.FindProductByID(&purchaseReturnProduct.ProductID, map[string]interface{}{})
		if err != nil {
			return err
		}

		err = product.SetProductPurchaseReturnStatsByStoreID(*purchaseReturn.StoreID)
		if err != nil {
			return err
		}

	}
	return nil
}

//Vendor

type VendorPurchaseReturnStats struct {
	PurchaseReturnCount              int64   `json:"purchase_return_count" bson:"purchase_return_count"`
	PurchaseReturnAmount             float64 `json:"purchase_return_amount" bson:"purchase_return_amount"`
	PurchaseReturnPaidAmount         float64 `json:"purchase_return_paid_amount" bson:"purchase_return_paid_amount"`
	PurchaseReturnBalanceAmount      float64 `json:"purchase_return_balance_amount" bson:"purchase_return_balance_amount"`
	PurchaseReturnPaidCount          int64   `json:"purchase_return_paid_count" bson:"purchase_return_paid_count"`
	PurchaseReturnNotPaidCount       int64   `json:"purchase_return_not_paid_count" bson:"purchase_return_not_paid_count"`
	PurchaseReturnPaidPartiallyCount int64   `json:"purchase_return_paid_partially_count" bson:"purchase_return_paid_partially_count"`
}

func (vendor *Vendor) SetVendorPurchaseReturnStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + storeID.Hex()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats VendorPurchaseReturnStats

	filter := map[string]interface{}{
		"store_id":  storeID,
		"vendor_id": vendor.ID,
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                            nil,
				"purchase_return_count":          bson.M{"$sum": 1},
				"purchase_return_amount":         bson.M{"$sum": "$net_total"},
				"purchase_return_paid_amount":    bson.M{"$sum": "$total_payment_paid"},
				"purchase_return_balance_amount": bson.M{"$sum": "$balance_amount"},
				"purchase_return_paid_count": bson.M{"$sum": bson.M{
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
				"purchase_return_not_paid_count": bson.M{"$sum": bson.M{
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
				"purchase_return_paid_partially_count": bson.M{"$sum": bson.M{
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
		stats.PurchaseReturnAmount = RoundFloat(stats.PurchaseReturnAmount, 2)
		stats.PurchaseReturnPaidAmount = RoundFloat(stats.PurchaseReturnPaidAmount, 2)
		stats.PurchaseReturnBalanceAmount = RoundFloat(stats.PurchaseReturnBalanceAmount, 2)

	}

	store, err := FindStoreByID(&storeID, bson.M{})
	if err != nil {
		return errors.New("error finding store: " + err.Error())
	}

	if len(vendor.Stores) == 0 {
		vendor.Stores = map[string]VendorStore{}
	}

	if vendorStore, ok := vendor.Stores[storeID.Hex()]; ok {
		vendorStore.StoreID = storeID
		vendorStore.StoreName = store.Name
		vendorStore.StoreNameInArabic = store.NameInArabic
		vendorStore.PurchaseReturnCount = stats.PurchaseReturnCount
		vendorStore.PurchaseReturnPaidCount = stats.PurchaseReturnPaidCount
		vendorStore.PurchaseReturnNotPaidCount = stats.PurchaseReturnNotPaidCount
		vendorStore.PurchaseReturnPaidPartiallyCount = stats.PurchaseReturnPaidPartiallyCount
		vendorStore.PurchaseReturnAmount = stats.PurchaseReturnAmount
		vendorStore.PurchaseReturnPaidAmount = stats.PurchaseReturnPaidAmount
		vendorStore.PurchaseReturnBalanceAmount = stats.PurchaseReturnBalanceAmount
		vendor.Stores[storeID.Hex()] = vendorStore
	} else {
		vendor.Stores[storeID.Hex()] = VendorStore{
			StoreID:                          storeID,
			StoreName:                        store.Name,
			StoreNameInArabic:                store.NameInArabic,
			PurchaseReturnCount:              stats.PurchaseReturnCount,
			PurchaseReturnPaidCount:          stats.PurchaseReturnPaidCount,
			PurchaseReturnNotPaidCount:       stats.PurchaseReturnNotPaidCount,
			PurchaseReturnPaidPartiallyCount: stats.PurchaseReturnPaidPartiallyCount,
			PurchaseReturnAmount:             stats.PurchaseReturnAmount,
			PurchaseReturnPaidAmount:         stats.PurchaseReturnPaidAmount,
			PurchaseReturnBalanceAmount:      stats.PurchaseReturnBalanceAmount,
		}
	}

	err = vendor.Update()
	if err != nil {
		return err
	}

	return nil
}

func (purchaseReturn *PurchaseReturn) SetVendorPurchaseReturnStats() error {
	store, err := FindStoreByID(purchaseReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	vendor, err := store.FindVendorByID(purchaseReturn.VendorID, map[string]interface{}{})
	if err != nil {
		return err
	}

	err = vendor.SetVendorPurchaseReturnStatsByStoreID(*purchaseReturn.StoreID)
	if err != nil {
		return err
	}

	return nil
}

func (purchaseReturnPayment *PurchaseReturnPayment) DeletePurchaseReturnPayment() (err error) {
	collection := db.GetDB("store_" + purchaseReturnPayment.StoreID.Hex()).Collection("purchase_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": purchaseReturnPayment.ID},
		bson.M{"$set": purchaseReturnPayment},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (purchaseReturn *PurchaseReturn) UpdatePurchaseReturnCount() (count int64, err error) {
	store, err := FindStoreByID(purchaseReturn.StoreID, bson.M{})
	if err != nil {
		return 0, err
	}

	collection := db.GetDB("store_" + purchaseReturn.StoreID.Hex()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	returnCount, err := collection.CountDocuments(ctx, bson.M{
		"purchase_id": purchaseReturn.PurchaseID,
		"deleted":     bson.M{"$ne": true},
	})
	if err != nil {
		return 0, err
	}

	purchase, err := store.FindPurchaseByID(purchaseReturn.PurchaseID, bson.M{})
	if err != nil {
		return 0, err
	}

	purchase.ReturnCount = returnCount
	err = purchase.Update()
	if err != nil {
		return 0, err
	}

	return returnCount, nil
}

// Accounting
// Journal entries
func MakeJournalsForUnpaidPurchaseReturn(
	purchaseReturn *PurchaseReturn,
	vendorAccount *Account,
	purchaseReturnAccount *Account,
	cashDiscountAllowedAccount *Account,
) []Journal {
	now := time.Now()

	groupID := primitive.NewObjectID()

	journals := []Journal{}

	balanceAmount := RoundFloat((purchaseReturn.NetTotal - purchaseReturn.CashDiscount), 2)

	journals = append(journals, Journal{
		Date:          purchaseReturn.Date,
		AccountID:     vendorAccount.ID,
		AccountNumber: vendorAccount.Number,
		AccountName:   vendorAccount.Name,
		DebitOrCredit: "debit",
		Debit:         balanceAmount,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	if purchaseReturn.CashDiscount > 0 {
		journals = append(journals, Journal{
			Date:          purchaseReturn.Date,
			AccountID:     cashDiscountAllowedAccount.ID,
			AccountNumber: cashDiscountAllowedAccount.Number,
			AccountName:   cashDiscountAllowedAccount.Name,
			DebitOrCredit: "debit",
			Debit:         purchaseReturn.CashDiscount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	}

	journals = append(journals, Journal{
		Date:          purchaseReturn.Date,
		AccountID:     purchaseReturnAccount.ID,
		AccountNumber: purchaseReturnAccount.Number,
		AccountName:   purchaseReturnAccount.Name,
		DebitOrCredit: "credit",
		Credit:        purchaseReturn.NetTotal,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals
}

var totalPurchaseReturnPaidAmount float64
var extraPurchaseReturnAmountPaid float64
var extraPurchaseReturnPayments []PurchaseReturnPayment

func MakeJournalsForPurchaseReturnPaymentsByDatetime(
	purchaseReturn *PurchaseReturn,
	vendor *Vendor,
	cashAccount *Account,
	bankAccount *Account,
	purchaseReturnAccount *Account,
	payments []PurchaseReturnPayment,
	cashDiscountAllowedAccount *Account,
	paymentsByDatetimeNumber int,
) ([]Journal, error) {

	store, err := FindStoreByID(purchaseReturn.StoreID, bson.M{})
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
		totalPurchaseReturnPaidAmount += *payment.Amount
		if totalPurchaseReturnPaidAmount > (purchaseReturn.NetTotal - purchaseReturn.CashDiscount) {
			extraPurchaseReturnAmountPaid = RoundFloat((totalPurchaseReturnPaidAmount - (purchaseReturn.NetTotal - purchaseReturn.CashDiscount)), 2)
		}
		amount := *payment.Amount

		if extraPurchaseReturnAmountPaid > 0 {
			skip := false
			if extraPurchaseReturnAmountPaid < *payment.Amount {
				extraAmount := extraPurchaseReturnAmountPaid
				extraPurchaseReturnPayments = append(extraPurchaseReturnPayments, PurchaseReturnPayment{
					Date:   payment.Date,
					Amount: &extraAmount,
					Method: payment.Method,
				})
				amount = RoundFloat((*payment.Amount - extraPurchaseReturnAmountPaid), 2)
				//totalPaidAmount -= *payment.Amount
				//totalPaidAmount += amount
				extraPurchaseReturnAmountPaid = 0
			} else if extraPurchaseReturnAmountPaid >= *payment.Amount {
				extraPurchaseReturnPayments = append(extraPurchaseReturnPayments, PurchaseReturnPayment{
					Date:   payment.Date,
					Amount: payment.Amount,
					Method: payment.Method,
				})

				skip = true
				extraPurchaseReturnAmountPaid = RoundFloat((extraPurchaseReturnAmountPaid - *payment.Amount), 2)
			}

			if skip {
				continue
			}

		}

		cashReceivingAccount := Account{}
		if payment.Method == "cash" {
			cashReceivingAccount = *cashAccount
		} else if slices.Contains(BANK_PAYMENT_METHODS, payment.Method) {
			cashReceivingAccount = *bankAccount
		} else if payment.Method == "vendor_account" {
			referenceModel := "vendor"
			vendorAccount, err := store.CreateAccountIfNotExists(
				purchaseReturn.StoreID,
				&vendor.ID,
				&referenceModel,
				vendor.Name,
				&vendor.Phone,
			)
			if err != nil {
				return nil, err
			}
			cashReceivingAccount = *vendorAccount
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

	if purchaseReturn.CashDiscount > 0 && paymentsByDatetimeNumber == 1 && IsDateTimesEqual(purchaseReturn.Date, firstPaymentDate) {
		journals = append(journals, Journal{
			Date:          purchaseReturn.Date,
			AccountID:     cashDiscountAllowedAccount.ID,
			AccountNumber: cashDiscountAllowedAccount.Number,
			AccountName:   cashDiscountAllowedAccount.Name,
			DebitOrCredit: "debit",
			Debit:         purchaseReturn.CashDiscount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	}

	balanceAmount := RoundFloat(((purchaseReturn.NetTotal - purchaseReturn.CashDiscount) - totalPayment), 2)

	//Asset or debt increased
	if paymentsByDatetimeNumber == 1 && balanceAmount > 0 && IsDateTimesEqual(purchaseReturn.Date, firstPaymentDate) {
		referenceModel := "vendor"
		vendorAccount, err := store.CreateAccountIfNotExists(
			purchaseReturn.StoreID,
			&vendor.ID,
			&referenceModel,
			vendor.Name,
			&vendor.Phone,
		)
		if err != nil {
			return nil, err
		}

		journals = append(journals, Journal{
			Date:          purchaseReturn.Date,
			AccountID:     vendorAccount.ID,
			AccountNumber: vendorAccount.Number,
			AccountName:   vendorAccount.Name,
			DebitOrCredit: "debit",
			Debit:         balanceAmount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	}

	if paymentsByDatetimeNumber == 1 && IsDateTimesEqual(purchaseReturn.Date, firstPaymentDate) {
		journals = append(journals, Journal{
			Date:          purchaseReturn.Date,
			AccountID:     purchaseReturnAccount.ID,
			AccountNumber: purchaseReturnAccount.Number,
			AccountName:   purchaseReturnAccount.Name,
			DebitOrCredit: "credit",
			Credit:        purchaseReturn.NetTotal,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	} else if paymentsByDatetimeNumber > 1 || !IsDateTimesEqual(purchaseReturn.Date, firstPaymentDate) {
		referenceModel := "vendor"
		vendorAccount, err := store.CreateAccountIfNotExists(
			purchaseReturn.StoreID,
			&vendor.ID,
			&referenceModel,
			vendor.Name,
			&vendor.Phone,
		)
		if err != nil {
			return nil, err
		}

		totalPayment = RoundFloat(totalPayment, 2)

		if totalPayment > 0 {
			journals = append(journals, Journal{
				Date:          firstPaymentDate,
				AccountID:     vendorAccount.ID,
				AccountNumber: vendorAccount.Number,
				AccountName:   vendorAccount.Name,
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

func MakeJournalsForPurchaseReturnExtraPayments(
	purchaseReturn *PurchaseReturn,
	vendor *Vendor,
	cashAccount *Account,
	bankAccount *Account,
	extraPayments []PurchaseReturnPayment,
) ([]Journal, error) {
	store, err := FindStoreByID(purchaseReturn.StoreID, bson.M{})
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
		if payment.Method == "cash" {
			cashReceivingAccount = *cashAccount
		} else if slices.Contains(BANK_PAYMENT_METHODS, payment.Method) {
			cashReceivingAccount = *bankAccount
		} else if payment.Method == "vendor_account" {
			referenceModel := "vendor"
			vendorAccount, err := store.CreateAccountIfNotExists(
				purchaseReturn.StoreID,
				&vendor.ID,
				&referenceModel,
				vendor.Name,
				&vendor.Phone,
			)
			if err != nil {
				return nil, err
			}
			cashReceivingAccount = *vendorAccount
		}

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     cashReceivingAccount.ID,
			AccountNumber: cashReceivingAccount.Number,
			AccountName:   cashReceivingAccount.Name,
			DebitOrCredit: "debit",
			Debit:         *payment.Amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	} //end for

	referenceModel := "vendor"
	vendorAccount, err := store.CreateAccountIfNotExists(
		purchaseReturn.StoreID,
		&vendor.ID,
		&referenceModel,
		vendor.Name,
		&vendor.Phone,
	)
	if err != nil {
		return nil, err
	}
	journals = append(journals, Journal{
		Date:          lastPaymentDate,
		AccountID:     vendorAccount.ID,
		AccountNumber: vendorAccount.Number,
		AccountName:   vendorAccount.Name,
		DebitOrCredit: "credit",
		Credit:        purchaseReturn.BalanceAmount * (-1),
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals, nil
}

// Regroup payments by datetime
func RegroupPurchaseReturnPaymentsByDatetime(payments []PurchaseReturnPayment) [][]PurchaseReturnPayment {
	paymentsByDatetime := map[string][]PurchaseReturnPayment{}
	for _, payment := range payments {
		paymentsByDatetime[payment.Date.Format("2006-01-02T15:04")] = append(paymentsByDatetime[payment.Date.Format("2006-01-02T15:04")], payment)
	}

	paymentsByDatetime2 := [][]PurchaseReturnPayment{}
	for _, v := range paymentsByDatetime {
		paymentsByDatetime2 = append(paymentsByDatetime2, v)
	}

	sort.Slice(paymentsByDatetime2, func(i, j int) bool {
		return paymentsByDatetime2[i][0].Date.Before(*paymentsByDatetime2[j][0].Date)
	})

	return paymentsByDatetime2
}

func (purchaseReturn *PurchaseReturn) CreateLedger() (ledger *Ledger, err error) {
	store, err := FindStoreByID(purchaseReturn.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()

	vendor, err := store.FindVendorByID(purchaseReturn.VendorID, bson.M{})
	if err != nil {
		return nil, err
	}

	cashAccount, err := store.CreateAccountIfNotExists(purchaseReturn.StoreID, nil, nil, "Cash", nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := store.CreateAccountIfNotExists(purchaseReturn.StoreID, nil, nil, "Bank", nil)
	if err != nil {
		return nil, err
	}

	purchaseReturnAccount, err := store.CreateAccountIfNotExists(purchaseReturn.StoreID, nil, nil, "Purchase Return", nil)
	if err != nil {
		return nil, err
	}

	cashDiscountAllowedAccount, err := store.CreateAccountIfNotExists(purchaseReturn.StoreID, nil, nil, "Cash discount allowed", nil)
	if err != nil {
		return nil, err
	}

	journals := []Journal{}

	var firstPaymentDate *time.Time
	if len(purchaseReturn.Payments) > 0 {
		firstPaymentDate = purchaseReturn.Payments[0].Date
	}

	if len(purchaseReturn.Payments) == 0 || (firstPaymentDate != nil && !IsDateTimesEqual(purchaseReturn.Date, firstPaymentDate)) {
		//Case: UnPaid
		referenceModel := "vendor"
		vendorAccount, err := store.CreateAccountIfNotExists(
			purchaseReturn.StoreID,
			&vendor.ID,
			&referenceModel,
			vendor.Name,
			&vendor.Phone,
		)
		if err != nil {
			return nil, err
		}
		journals = append(journals, MakeJournalsForUnpaidPurchaseReturn(
			purchaseReturn,
			vendorAccount,
			purchaseReturnAccount,
			cashDiscountAllowedAccount,
		)...)
	}

	if len(purchaseReturn.Payments) > 0 {
		totalPurchaseReturnPaidAmount = float64(0.00)
		extraPurchaseReturnAmountPaid = float64(0.00)
		extraPurchaseReturnPayments = []PurchaseReturnPayment{}

		paymentsByDatetimeNumber := 1
		paymentsByDatetime := RegroupPurchaseReturnPaymentsByDatetime(purchaseReturn.Payments)
		//fmt.Printf("%+v", paymentsByDatetime)

		for _, paymentByDatetime := range paymentsByDatetime {
			newJournals, err := MakeJournalsForPurchaseReturnPaymentsByDatetime(
				purchaseReturn,
				vendor,
				cashAccount,
				bankAccount,
				purchaseReturnAccount,
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

		if purchaseReturn.BalanceAmount < 0 && len(extraPurchaseReturnPayments) > 0 {
			newJournals, err := MakeJournalsForPurchaseReturnExtraPayments(
				purchaseReturn,
				vendor,
				cashAccount,
				bankAccount,
				extraPurchaseReturnPayments,
			)
			if err != nil {
				return nil, err
			}

			journals = append(journals, newJournals...)
		}

		totalPurchaseReturnPaidAmount = float64(0.00)
		extraPurchaseReturnAmountPaid = float64(0.00)

	}

	ledger = &Ledger{
		StoreID:        purchaseReturn.StoreID,
		ReferenceID:    purchaseReturn.ID,
		ReferenceModel: "purchase_return",
		ReferenceCode:  purchaseReturn.Code,
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

func (purchaseReturn *PurchaseReturn) DoAccounting() error {
	ledger, err := purchaseReturn.CreateLedger()
	if err != nil {
		return errors.New("error creating ledger: " + err.Error())
	}

	_, err = ledger.CreatePostings()
	if err != nil {
		return errors.New("error creating postings: " + err.Error())
	}

	return nil
}

func (purchaseReturn *PurchaseReturn) UndoAccounting() error {
	store, err := FindStoreByID(purchaseReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledger, err := store.FindLedgerByReferenceID(purchaseReturn.ID, *purchaseReturn.StoreID, bson.M{})
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

	err = store.RemoveLedgerByReferenceID(purchaseReturn.ID)
	if err != nil {
		return errors.New("Error removing ledger by reference id: " + err.Error())
	}

	err = store.RemovePostingsByReferenceID(purchaseReturn.ID)
	if err != nil {
		return errors.New("Error removing postings by reference id: " + err.Error())
	}

	err = SetAccountBalances(ledgerAccounts)
	if err != nil {
		return errors.New("Error setting account balances: " + err.Error())
	}

	return nil
}

//End Accounting

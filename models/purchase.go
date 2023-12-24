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
	"golang.org/x/exp/slices"
	"gopkg.in/mgo.v2/bson"
)

type PurchaseProduct struct {
	ProductID               primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name                    string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic            string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode                string             `bson:"item_code,omitempty" json:"item_code,omitempty"`
	PartNumber              string             `bson:"part_number,omitempty" json:"part_number,omitempty"`
	Quantity                float64            `json:"quantity,omitempty" bson:"quantity,omitempty"`
	QuantityReturned        float64            `json:"quantity_returned" bson:"quantity_returned"`
	Unit                    string             `bson:"unit,omitempty" json:"unit,omitempty"`
	PurchaseUnitPrice       float64            `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	RetailUnitPrice         float64            `bson:"retail_unit_price,omitempty" json:"retail_unit_price,omitempty"`
	WholesaleUnitPrice      float64            `bson:"wholesale_unit_price,omitempty" json:"wholesale_unit_price,omitempty"`
	ExpectedRetailProfit    float64            `bson:"retail_profit" json:"retail_profit"`
	ExpectedWholesaleProfit float64            `bson:"wholesale_profit" json:"wholesale_profit"`
	ExpectedWholesaleLoss   float64            `bson:"wholesale_loss" json:"wholesale_loss"`
	ExpectedRetailLoss      float64            `bson:"retail_loss" json:"retail_loss"`
}

// Purchase : Purchase structure
type Purchase struct {
	ID                         primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date                       *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr                    string              `json:"date_str,omitempty"`
	Code                       string              `bson:"code,omitempty" json:"code,omitempty"`
	StoreID                    *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	VendorID                   *primitive.ObjectID `json:"vendor_id,omitempty" bson:"vendor_id,omitempty"`
	VendorInvoiceNumber        string              `bson:"vendor_invoice_no,omitempty" json:"vendor_invoice_no,omitempty"`
	Store                      *Store              `json:"store,omitempty"`
	Vendor                     *Vendor             `json:"vendor,omitempty"`
	Products                   []PurchaseProduct   `bson:"products,omitempty" json:"products,omitempty"`
	OrderPlacedBy              *primitive.ObjectID `json:"order_placed_by,omitempty" bson:"order_placed,omitempty"`
	OrderPlacedBySignatureID   *primitive.ObjectID `json:"order_placed_by_signature_id,omitempty" bson:"order_placed_signature_id,omitempty"`
	OrderPlacedBySignatureName string              `json:"order_placed_by_signature_name,omitempty" bson:"order_placed_by_signature_name,omitempty"`
	OrderPlacedByUser          *User               `json:"order_placed_by_user,omitempty"`
	OrderPlacedBySignature     *Signature          `json:"order_placed_by_signature,omitempty"`
	SignatureDate              *time.Time          `bson:"signature_date,omitempty" json:"signature_date,omitempty"`
	SignatureDateStr           string              `json:"signature_date_str,omitempty"`
	VatPercent                 *float64            `bson:"vat_percent" json:"vat_percent"`
	Discount                   float64             `bson:"discount" json:"discount"`
	ReturnDiscount             float64             `bson:"return_discount" json:"return_discount"`
	DiscountPercent            float64             `bson:"discount_percent" json:"discount_percent"`
	IsDiscountPercent          bool                `bson:"is_discount_percent" json:"is_discount_percent"`
	DiscountProfit             float64             `bson:"discount_profit" json:"discount_profit"`
	Status                     string              `bson:"status,omitempty" json:"status,omitempty"`
	TotalQuantity              float64             `bson:"total_quantity" json:"total_quantity"`
	VatPrice                   float64             `bson:"vat_price" json:"vat_price"`
	Total                      float64             `bson:"total" json:"total"`
	NetTotal                   float64             `bson:"net_total" json:"net_total"`
	PartiaPaymentAmount        float64             `bson:"partial_payment_amount" json:"partial_payment_amount"`
	PaymentMethod              string              `bson:"payment_method" json:"payment_method"`
	PaymentStatus              string              `bson:"payment_status" json:"payment_status"`
	ShippingOrHandlingFees     float64             `bson:"shipping_handling_fees" json:"shipping_handling_fees"`
	ExpectedRetailProfit       float64             `bson:"retail_profit" json:"retail_profit"`
	ExpectedWholesaleProfit    float64             `bson:"wholesale_profit" json:"wholesale_profit"`
	ExpectedNetRetailProfit    float64             `bson:"net_retail_profit" json:"net_retail_profit"`
	ExpectedNetWholesaleProfit float64             `bson:"net_wholesale_profit" json:"net_wholesale_profit"`
	ExpectedWholesaleLoss      float64             `bson:"wholesale_loss" json:"wholesale_loss"`
	ExpectedRetailLoss         float64             `bson:"retail_loss" json:"retail_loss"`
	ReturnedAll                bool                `json:"returned_all"`
	Deleted                    bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy                  *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser              *User               `json:"deleted_by_user,omitempty"`
	DeletedAt                  *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt                  *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt                  *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy                  *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy                  *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser              *User               `json:"created_by_user,omitempty"`
	UpdatedByUser              *User               `json:"updated_by_user,omitempty"`
	OrderPlacedByName          string              `json:"order_placed_by_name,omitempty" bson:"order_placed_by_name,omitempty"`
	VendorName                 string              `json:"vendor_name,omitempty" bson:"vendor_name,omitempty"`
	StoreName                  string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	CreatedByName              string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName              string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName              string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	ChangeLog                  []ChangeLog         `json:"change_log,omitempty" bson:"change_log,omitempty"`
	TotalPaymentPaid           float64             `bson:"total_payment_paid" json:"total_payment_paid"`
	BalanceAmount              float64             `bson:"balance_amount" json:"balance_amount"`
	Payments                   []PurchasePayment   `bson:"payments" json:"payments"`
	PaymentsCount              int64               `bson:"payments_count" json:"payments_count"`
	PaymentMethods             []string            `json:"payment_methods" bson:"payment_methods"`
}

func UpdatePurchaseProfit() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
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

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		model := Purchase{}
		err = cur.Decode(&model)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		err = model.Update()
		if err != nil {
			return err
		}
	}

	return nil
}

type PurchaseStats struct {
	ID                     *primitive.ObjectID `json:"id" bson:"_id"`
	NetTotal               float64             `json:"net_total" bson:"net_total"`
	VatPrice               float64             `json:"vat_price" bson:"vat_price"`
	Discount               float64             `json:"discount" bson:"discount"`
	ShippingOrHandlingFees float64             `json:"shipping_handling_fees" bson:"shipping_handling_fees"`
	NetRetailProfit        float64             `json:"net_retail_net_profit" bson:"net_retail_profit"`
	NetWholesaleProfit     float64             `json:"net_wholesale_profit" bson:"net_wholesale_profit"`
	PaidPurchase           float64             `json:"paid_purchase" bson:"paid_purchase"`
	UnPaidPurchase         float64             `json:"unpaid_purchase" bson:"unpaid_purchase"`
	CashPurchase           float64             `json:"cash_purchase" bson:"cash_purchase"`
	BankAccountPurchase    float64             `json:"bank_account_purchase" bson:"bank_account_purchase"`
}

func GetPurchaseStats(filter map[string]interface{}) (stats PurchaseStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
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
				"shipping_handling_fees": bson.M{"$sum": "$shipping_handling_fees"},
				"net_retail_profit":      bson.M{"$sum": "$net_retail_profit"},
				"net_wholesale_profit":   bson.M{"$sum": "$net_wholesale_profit"},
				"paid_purchase": bson.M{"$sum": bson.M{"$sum": bson.M{
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
				"unpaid_purchase": bson.M{"$sum": "$balance_amount"},
				"cash_purchase": bson.M{"$sum": bson.M{"$sum": bson.M{
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
				"bank_account_purchase": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$payments",
						"as":    "payment",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$eq": []interface{}{"$$payment.method", "bank_account"}},
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
		stats.NetTotal = math.Round(stats.NetTotal*100) / 100
		stats.NetRetailProfit = math.Round(stats.NetRetailProfit*100) / 100
		stats.NetWholesaleProfit = math.Round(stats.NetWholesaleProfit*100) / 100
	}
	return stats, nil
}

func (purchase *Purchase) CalculatePurchaseExpectedProfit() error {
	totalRetailProfit := 0.0
	totalWholesaleProfit := 0.0

	totalRetailLoss := 0.0
	totalWholesaleLoss := 0.0

	purchase.ReturnedAll = true

	for index, purchaseProduct := range purchase.Products {
		quantity := (purchaseProduct.Quantity - purchaseProduct.QuantityReturned)

		if quantity > 0 {
			purchase.ReturnedAll = false
		}

		purchasePrice := quantity * purchaseProduct.PurchaseUnitPrice
		retailPrice := quantity * purchaseProduct.RetailUnitPrice
		wholesalePrice := quantity * purchaseProduct.WholesaleUnitPrice

		expectedRetailProfit := retailPrice - purchasePrice
		expectedWholesaleProfit := wholesalePrice - purchasePrice

		if expectedRetailProfit > 0 {
			purchase.Products[index].ExpectedRetailProfit = expectedRetailProfit
			totalRetailProfit += expectedRetailProfit
		} else {
			purchase.Products[index].ExpectedRetailLoss = expectedRetailProfit * (-1)
			totalRetailLoss += expectedRetailProfit * (-1)
		}

		if expectedWholesaleProfit > 0 {
			purchase.Products[index].ExpectedWholesaleProfit = expectedWholesaleProfit
			totalWholesaleProfit += expectedWholesaleProfit
		} else {
			purchase.Products[index].ExpectedWholesaleLoss = expectedWholesaleProfit * (-1)
			totalWholesaleLoss += expectedWholesaleProfit * (-1)
		}

	}

	if purchase.ReturnedAll {
		purchase.ExpectedRetailProfit = math.Round(0.00*100) / 100
		purchase.ExpectedWholesaleProfit = math.Round(0.00*100) / 100
		purchase.ExpectedNetRetailProfit = math.Round(0.00*100) / 100
		purchase.ExpectedNetWholesaleProfit = math.Round(0.00*100) / 100
		purchase.ExpectedRetailLoss = math.Round(0.00*100) / 100
		purchase.ExpectedWholesaleLoss = math.Round(0.00*100) / 100
	} else {
		purchase.ExpectedRetailProfit = math.Round((totalRetailProfit)*100) / 100
		purchase.ExpectedWholesaleProfit = math.Round((totalWholesaleProfit)*100) / 100

		purchase.ExpectedNetRetailProfit = math.Round((totalRetailProfit+purchase.Discount-purchase.ReturnDiscount)*100) / 100
		purchase.ExpectedNetWholesaleProfit = math.Round((totalWholesaleProfit+purchase.Discount-purchase.ReturnDiscount)*100) / 100

		purchase.ExpectedRetailLoss = math.Round(totalRetailLoss*100) / 100
		purchase.ExpectedWholesaleLoss = math.Round(totalWholesaleLoss*100) / 100
	}

	return nil
}

func (purchase *Purchase) SetChangeLog(
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
		description = "Stock of product: " + name.(string) + " reduced from " + fmt.Sprintf("%.02f", oldValue.(float64)) + " to " + fmt.Sprintf("%.02f", newValue.(float64))
	} else if event == "add_stock" && name != nil {
		description = "Stock of product: " + name.(string) + " raised from " + fmt.Sprintf("%.02f", oldValue.(float64)) + " to " + fmt.Sprintf("%.02f", newValue.(float64))
	}

	purchase.ChangeLog = append(
		purchase.ChangeLog,
		ChangeLog{
			Event:         event,
			Description:   description,
			CreatedBy:     &UserObject.ID,
			CreatedByName: UserObject.Name,
			CreatedAt:     &now,
		},
	)
}

func (purchase *Purchase) AttributesValueChangeEvent(purchaseOld *Purchase) error {

	if purchase.Status != purchaseOld.Status {
		/*
			purchase.SetChangeLog(
				"attribute_value_change",
				"status",
				purchaseOld.Status,
				purchase.Status,
			)
		*/
		/*

				err := purchaseOld.RemoveStock()
				if err != nil {
					return err
				}

				err = purchase.AddStock()
				if err != nil {
					return err
				}

			err := purchase.UpdateProductUnitPriceInStore()
			if err != nil {
				return err
			}
		*/
	}

	return nil
}

func (purchase *Purchase) UpdateForeignLabelFields() error {

	if purchase.StoreID != nil {
		store, err := FindStoreByID(purchase.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchase.StoreName = store.Name
	}

	if purchase.VendorID != nil {
		vendor, err := FindVendorByID(purchase.VendorID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchase.VendorName = vendor.Name
	}

	if purchase.OrderPlacedBy != nil {
		orderPlacedByUser, err := FindUserByID(purchase.OrderPlacedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchase.OrderPlacedByName = orderPlacedByUser.Name
	}

	if purchase.OrderPlacedBySignatureID != nil {
		orderPlacedBySignature, err := FindSignatureByID(purchase.OrderPlacedBySignatureID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchase.OrderPlacedBySignatureName = orderPlacedBySignature.Name
	}

	if purchase.CreatedBy != nil {
		createdByUser, err := FindUserByID(purchase.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchase.CreatedByName = createdByUser.Name
	}

	if purchase.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(purchase.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchase.UpdatedByName = updatedByUser.Name
	}

	if purchase.DeletedBy != nil && !purchase.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(purchase.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchase.DeletedByName = deletedByUser.Name
	}

	for i, product := range purchase.Products {
		productObject, err := FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1, "part_number": 1})
		if err != nil {
			return err
		}
		purchase.Products[i].Name = productObject.Name
		purchase.Products[i].NameInArabic = productObject.NameInArabic
		purchase.Products[i].ItemCode = productObject.ItemCode
		purchase.Products[i].PartNumber = productObject.PartNumber
	}

	return nil
}

func (purchase *Purchase) FindNetTotal() {
	netTotal := float64(0.0)
	for _, product := range purchase.Products {
		netTotal += (float64(product.Quantity) * product.PurchaseUnitPrice)
	}

	netTotal -= purchase.Discount
	netTotal += purchase.ShippingOrHandlingFees

	if purchase.VatPercent != nil {
		netTotal += netTotal * (*purchase.VatPercent / float64(100))
	}

	purchase.NetTotal = math.Round(netTotal*100) / 100
}

func (purchase *Purchase) FindTotal() {
	total := float64(0.0)
	for _, product := range purchase.Products {
		total += (float64(product.Quantity) * product.PurchaseUnitPrice)
	}

	purchase.Total = math.Round(total*100) / 100
}

func (purchase *Purchase) FindTotalQuantity() {
	totalQuantity := float64(0)
	for _, product := range purchase.Products {
		totalQuantity += product.Quantity
	}
	purchase.TotalQuantity = totalQuantity
}

func (purchase *Purchase) FindVatPrice() {
	vatPrice := ((*purchase.VatPercent / 100) * (purchase.Total - purchase.Discount + purchase.ShippingOrHandlingFees))
	vatPrice = math.Round(vatPrice*100) / 100
	purchase.VatPrice = vatPrice
}

func SearchPurchase(w http.ResponseWriter, r *http.Request) (purchases []Purchase, criterias SearchCriterias, err error) {

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
			return purchases, criterias, err
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
			return purchases, criterias, err
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
			return purchases, criterias, err
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
			return purchases, criterias, err
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
			return purchases, criterias, err
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
			return purchases, criterias, err
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
			return purchases, criterias, err
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
			return purchases, criterias, err
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
			return purchases, criterias, err
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

	keys, ok = r.URL.Query()["search[net_total]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return purchases, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_total"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["net_total"] = float64(value)
		}

	}

	keys, ok = r.URL.Query()["search[vat_price]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return purchases, criterias, err
		}
		log.Print("value:")
		log.Print(value)

		if operator != "" {
			criterias.SearchBy["vat_price"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["vat_price"] = float64(value)
		}

	}

	keys, ok = r.URL.Query()["search[vendor_id]"]
	if ok && len(keys[0]) >= 1 {

		vendorIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range vendorIds {
			vendorID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return purchases, criterias, err
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
				return purchases, criterias, err
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
			return purchases, criterias, err
		}
		criterias.SearchBy["delivered_by"] = deliveredByID
	}

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return purchases, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[order_placed_by]"]
	if ok && len(keys[0]) >= 1 {
		orderPlacedByID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return purchases, criterias, err
		}
		criterias.SearchBy["order_placed_by"] = orderPlacedByID
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

	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	storeSelectFields := map[string]interface{}{}
	vendorSelectFields := map[string]interface{}{}
	orderPlacedByUserSelectFields := map[string]interface{}{}
	orderPlacedBySignatureSelectFields := map[string]interface{}{}
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

		if _, ok := criterias.Select["order_placed_by_user.id"]; ok {
			orderPlacedByUserSelectFields = ParseRelationalSelectString(keys[0], "order_placed_by_user")
		}

		if _, ok := criterias.Select["order_placed_signature.id"]; ok {
			orderPlacedBySignatureSelectFields = ParseRelationalSelectString(keys[0], "order_placed_signature")
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
		return purchases, criterias, errors.New("Error fetching purchases:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return purchases, criterias, errors.New("Cursor error:" + err.Error())
		}
		purchase := Purchase{}
		err = cur.Decode(&purchase)
		if err != nil {
			return purchases, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["store.id"]; ok {
			purchase.Store, _ = FindStoreByID(purchase.StoreID, storeSelectFields)
		}

		if _, ok := criterias.Select["vendor.id"]; ok {
			purchase.Vendor, _ = FindVendorByID(purchase.VendorID, vendorSelectFields)
			log.Print(purchase.Vendor.VATNo)
		}

		if _, ok := criterias.Select["order_placed_by_user.id"]; ok {
			purchase.OrderPlacedByUser, _ = FindUserByID(purchase.OrderPlacedBy, orderPlacedByUserSelectFields)
		}

		if _, ok := criterias.Select["order_placed_by_signature.id"]; ok {
			purchase.OrderPlacedBySignature, _ = FindSignatureByID(purchase.OrderPlacedBySignatureID, orderPlacedBySignatureSelectFields)
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			purchase.CreatedByUser, _ = FindUserByID(purchase.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			purchase.UpdatedByUser, _ = FindUserByID(purchase.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			purchase.DeletedByUser, _ = FindUserByID(purchase.DeletedBy, deletedByUserSelectFields)
		}
		purchases = append(purchases, purchase)
	} //end for loop

	return purchases, criterias, nil
}

func (purchase *Purchase) Validate(
	w http.ResponseWriter,
	r *http.Request,
	scenario string,
	oldPurchase *Purchase,
) (errs map[string]string) {

	errs = make(map[string]string)

	if govalidator.IsNull(purchase.PaymentStatus) {
		errs["payment_status"] = "Payment status is required"
	}

	if govalidator.IsNull(purchase.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		//const shortForm = "Jan 02 2006"
		//const shortForm = "	January 02, 2006T3:04PM"
		//from js:Thu Apr 14 2022 03:53:15 GMT+0300 (Arabian Standard Time)
		//	const shortForm = "Monday Jan 02 2006 15:04:05 GMT-0700 (MST)"
		//const shortForm = "Mon Jan 02 2006 15:04:05 GMT-0700 (MST)"
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, purchase.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		purchase.Date = &date
	}

	if !govalidator.IsNull(purchase.SignatureDateStr) {
		const shortForm = "Jan 02 2006"
		date, err := time.Parse(shortForm, purchase.SignatureDateStr)
		if err != nil {
			errs["signature_date_str"] = "Invalid date format"
		}
		purchase.SignatureDate = &date
	}

	if scenario == "update" {
		if purchase.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsPurchaseExists(&purchase.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Purchase:" + purchase.ID.Hex()
		}

	} else {
		if purchase.PaymentStatus != "not_paid" {
			if govalidator.IsNull(purchase.PaymentMethod) {
				errs["payment_method"] = "Payment method is required"
			}
		}
	}

	if purchase.StoreID == nil || purchase.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(purchase.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + purchase.StoreID.Hex()
			return errs
		}
	}

	if purchase.VendorID == nil || purchase.VendorID.IsZero() {
		errs["vendor_id"] = "Vendor is required"
	} else {
		exists, err := IsVendorExists(purchase.VendorID)
		if err != nil {
			errs["vendor_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["vendor_id"] = "Invalid Vendor:" + purchase.VendorID.Hex()
		}
	}

	if purchase.OrderPlacedBy == nil || purchase.OrderPlacedBy.IsZero() {
		errs["order_placed_by"] = "Order Placed By is required"
	} else {
		exists, err := IsUserExists(purchase.OrderPlacedBy)
		if err != nil {
			errs["order_placed_by"] = err.Error()
			return errs
		}

		if !exists {
			errs["order_placed_by"] = "Invalid Order Placed By:" + purchase.OrderPlacedBy.Hex()
		}
	}

	if purchase.OrderPlacedBySignatureID != nil && !purchase.OrderPlacedBySignatureID.IsZero() {
		exists, err := IsSignatureExists(purchase.OrderPlacedBySignatureID)
		if err != nil {
			errs["order_placed_by_signature_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["order_placed_by_signature_id"] = "Invalid Order Placed By Signature:" + purchase.OrderPlacedBySignatureID.Hex()
		}
	}

	if len(purchase.Products) == 0 {
		errs["product_id"] = "Atleast 1 product is required for purchase"
	}

	for i, product := range purchase.Products {
		if product.ProductID.IsZero() {
			errs["product_id_"+strconv.Itoa(i)] = "Product is required for purchase"
		} else {
			exists, err := IsProductExists(&product.ProductID)
			if err != nil {
				errs["product_id_"+strconv.Itoa(i)] = err.Error()
				return errs
			}

			if !exists {
				errs["product_id_"+strconv.Itoa(i)] = "Invalid product_id:" + product.ProductID.Hex() + " in products"
			}
		}

		if product.Quantity == 0 {
			errs["quantity_"+strconv.Itoa(i)] = "Quantity is required"
		}

		if product.PurchaseUnitPrice == 0 {
			errs["purchase_unit_price_"+strconv.Itoa(i)] = "Purchase Unit Price is required"
		}

		if product.RetailUnitPrice == 0 {
			errs["retail_unit_price_"+strconv.Itoa(i)] = "Retail Unit Price is required"
		}

		if product.WholesaleUnitPrice == 0 {
			errs["wholesale_unit_price_"+strconv.Itoa(i)] = "Wholesale Unit Price is required"
		}

	}

	if purchase.VatPercent == nil {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func (purchase *Purchase) AddStock() (err error) {
	for _, purchaseProduct := range purchase.Products {
		product, err := FindProductByID(&purchaseProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if productStoreTemp, ok := product.ProductStores[purchase.StoreID.Hex()]; ok {
			productStoreTemp.Stock += (purchaseProduct.Quantity - purchaseProduct.QuantityReturned)
			product.ProductStores[purchase.StoreID.Hex()] = productStoreTemp
		} else {
			product.ProductStores = map[string]ProductStore{}
			product.ProductStores[purchase.StoreID.Hex()] = ProductStore{
				StoreID: *purchase.StoreID,
				Stock:   (purchaseProduct.Quantity - purchaseProduct.QuantityReturned),
			}
		}

		/*
			storeExistInProductStore := false
			for k, productStore := range product.Stores {
				if productStore.StoreID.Hex() == purchase.StoreID.Hex() {
					product.Stores[k].Stock += (purchaseProduct.Quantity - purchaseProduct.QuantityReturned)
					storeExistInProductStore = true
					break
				}
			}

			if !storeExistInProductStore {
				productStore := ProductStore{
					StoreID: *purchase.StoreID,
					Stock:   (purchaseProduct.Quantity - purchaseProduct.QuantityReturned),
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

func (purchase *Purchase) RemoveStock() (err error) {
	for _, purchaseProduct := range purchase.Products {
		product, err := FindProductByID(&purchaseProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if productStoreTemp, ok := product.ProductStores[purchase.StoreID.Hex()]; ok {
			productStoreTemp.Stock -= (purchaseProduct.Quantity - purchaseProduct.QuantityReturned)
			product.ProductStores[purchase.StoreID.Hex()] = productStoreTemp
		}

		/*
			for k, productStore := range product.Stores {
				if productStore.StoreID.Hex() == purchase.StoreID.Hex() {
					product.Stores[k].Stock -= (purchaseProduct.Quantity - purchaseProduct.QuantityReturned)
					break
				}
			}*/

		err = product.Update()
		if err != nil {
			return err
		}
	}
	return nil
}

func (purchase *Purchase) UpdateProductUnitPriceInStore() (err error) {

	for _, purchaseProduct := range purchase.Products {
		product, err := FindProductByID(&purchaseProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if productStoreTemp, ok := product.ProductStores[purchase.StoreID.Hex()]; ok {
			productStoreTemp.PurchaseUnitPrice = purchaseProduct.PurchaseUnitPrice
			product.ProductStores[purchase.StoreID.Hex()] = productStoreTemp
		} else {
			product.ProductStores = map[string]ProductStore{}
			product.ProductStores[purchase.StoreID.Hex()] = ProductStore{
				StoreID:            *purchase.StoreID,
				PurchaseUnitPrice:  purchaseProduct.PurchaseUnitPrice,
				WholesaleUnitPrice: purchaseProduct.WholesaleUnitPrice,
				RetailUnitPrice:    purchaseProduct.RetailUnitPrice,
			}
		}

		/*
			storeExistInProductUnitPrice := false
			for k, productStore := range product.Stores {
				if productStore.StoreID.Hex() == purchase.StoreID.Hex() {
					product.Stores[k].PurchaseUnitPrice = purchaseProduct.PurchaseUnitPrice
					product.Stores[k].WholesaleUnitPrice = purchaseProduct.WholesaleUnitPrice
					product.Stores[k].RetailUnitPrice = purchaseProduct.RetailUnitPrice
					storeExistInProductUnitPrice = true
					break
				}
			}

			if !storeExistInProductUnitPrice {
				productStore := ProductStore{
					StoreID:            *purchase.StoreID,
					PurchaseUnitPrice:  purchaseProduct.PurchaseUnitPrice,
					WholesaleUnitPrice: purchaseProduct.WholesaleUnitPrice,
					RetailUnitPrice:    purchaseProduct.RetailUnitPrice,
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

func (purchase *Purchase) GenerateCode(startFrom int, storeCode string) (string, error) {
	count, err := GetTotalCount(bson.M{"store_id": purchase.StoreID}, "purchase")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return storeCode + "-" + strconv.Itoa(code+1), nil
}

func (purchase *Purchase) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.InsertOne(ctx, &purchase)
	if err != nil {
		return err
	}

	return nil
}

func (purchase *Purchase) MakeCode() error {
	lastPurchase, err := FindLastPurchaseByStoreID(purchase.StoreID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}
	if lastPurchase == nil {
		store, err := FindStoreByID(purchase.StoreID, bson.M{})
		if err != nil {
			return err
		}
		purchase.Code = store.Code + "-300000"
	} else {
		splits := strings.Split(lastPurchase.Code, "-")
		if len(splits) == 2 {
			storeCode := splits[0]
			codeStr := splits[1]
			codeInt, err := strconv.Atoi(codeStr)
			if err != nil {
				return err
			}
			codeInt++
			purchase.Code = storeCode + "-" + strconv.Itoa(codeInt)
		}
	}

	for {
		exists, err := purchase.IsCodeExists()
		if err != nil {
			return err
		}
		if !exists {
			break
		}

		splits := strings.Split(lastPurchase.Code, "-")
		storeCode := splits[0]
		codeStr := splits[1]
		codeInt, err := strconv.Atoi(codeStr)
		if err != nil {
			return err
		}
		codeInt++

		purchase.Code = storeCode + "-" + strconv.Itoa(codeInt)
	}

	return nil
}

func FindLastPurchaseByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (purchase *Purchase, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": storeID}, findOneOptions).
		Decode(&purchase)
	if err != nil {
		return nil, err
	}

	return purchase, err
}

func (purchase *Purchase) AddPayment() error {
	amount := float64(0.0)
	if purchase.PaymentStatus == "paid" {
		amount = purchase.NetTotal
	} else if purchase.PaymentStatus == "paid_partially" {
		amount = purchase.PartiaPaymentAmount
	} else {
		return nil
	}

	payment := PurchasePayment{
		PurchaseID:    &purchase.ID,
		Date:          purchase.Date,
		PurchaseCode:  purchase.Code,
		Amount:        &amount,
		Method:        purchase.PaymentMethod,
		CreatedAt:     purchase.CreatedAt,
		UpdatedAt:     purchase.UpdatedAt,
		CreatedBy:     purchase.CreatedBy,
		CreatedByName: purchase.CreatedByName,
		UpdatedBy:     purchase.UpdatedBy,
		UpdatedByName: purchase.UpdatedByName,
		StoreID:       purchase.StoreID,
		StoreName:     purchase.StoreName,
	}
	err := payment.Insert()
	if err != nil {
		return err
	}
	return nil
}

func (purchase *Purchase) IsCodeExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if purchase.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": purchase.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": purchase.Code,
			"_id":  bson.M{"$ne": purchase.ID},
		})
	}

	return (count == 1), err
}

func GeneratePurchaseCode(startFrom int) (string, error) {

	count, err := GetTotalCount(bson.M{}, "purchase")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return strconv.Itoa(code + 1), nil
}

func (purchase *Purchase) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": purchase.ID},
		bson.M{"$set": purchase},
		updateOptions,
	)

	if err != nil {
		return err
	}

	return nil
}

func (purchase *Purchase) DeletePurchase(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = purchase.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	purchase.Deleted = true
	purchase.DeletedBy = &userID
	now := time.Now()
	purchase.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": purchase.ID},
		bson.M{"$set": purchase},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (purchase *Purchase) HardDelete() (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = collection.DeleteOne(ctx, bson.M{"_id": purchase.ID})
	if err != nil {
		return err
	}
	return nil
}

func FindPurchaseByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (purchase *Purchase, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&purchase)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["order_placed_by.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "order_placed_by")
		purchase.OrderPlacedByUser, _ = FindUserByID(purchase.OrderPlacedBy, fields)
	}

	if _, ok := selectFields["order_placed_by_signature.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "order_placed_by_signature")
		purchase.OrderPlacedBySignature, _ = FindSignatureByID(purchase.OrderPlacedBySignatureID, fields)
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		purchase.CreatedByUser, _ = FindUserByID(purchase.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		purchase.UpdatedByUser, _ = FindUserByID(purchase.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		purchase.DeletedByUser, _ = FindUserByID(purchase.DeletedBy, fields)
	}

	return purchase, err
}

func IsPurchaseExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

func ProcessPurchases() error {
	log.Print("Processing purchases")
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return errors.New("Error fetching purchases:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	/*
		orders, err := GetAllOrders()
		if err != nil {
			return errors.New("Error fetching orders:" + err.Error())
		}
	*/

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		model := Purchase{}
		err = cur.Decode(&model)
		if err != nil {
			return errors.New("Cursor decoding purchase error:" + err.Error())
		}

		/*
			err = model.ClearProductsPurchaseHistory()
			if err != nil {
				return errors.New("error deleting product purchase history: " + err.Error())
			}

			err = model.AddProductsPurchaseHistory()
			if err != nil {
				return errors.New("error Adding product purchase history: " + err.Error())
			}

			model.GetPayments()
		*/

		/*
			err = model.SetProductsPurchaseStats()
			if err != nil {
				return errors.New("error set product purchase stats: " + err.Error())
			}
		*/

		err = model.SetVendorPurchaseStats()
		if err != nil {
			return errors.New("Error setting vendor purchase stats: " + err.Error())
		}

		err = model.Update()
		if err != nil {
			return errors.New("error updating purchase: " + err.Error())
		}
	}
	log.Print("DONE!")

	return nil
}

func (model *Purchase) GetPayments() (payments []PurchasePayment, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_payment")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{"purchase_id": model.ID, "deleted": bson.M{"$ne": true}}, findOptions)
	if err != nil {
		return payments, errors.New("Error fetching purchase payment history" + err.Error())
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
		payment := PurchasePayment{}
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
	model.BalanceAmount = ToFixed(model.NetTotal-totalPaymentPaid, 2)
	model.PaymentMethods = paymentMethods
	model.Payments = payments
	model.PaymentsCount = int64(len(payments))

	if ToFixed(model.NetTotal, 2) == ToFixed(totalPaymentPaid, 2) {
		model.PaymentStatus = "paid"
	} else if ToFixed(totalPaymentPaid, 2) > 0 {
		model.PaymentStatus = "paid_partially"
		model.PartiaPaymentAmount = totalPaymentPaid
	} else if ToFixed(totalPaymentPaid, 2) <= 0 {
		model.PaymentStatus = "not_paid"
	}

	return payments, err
}

func (model *Purchase) GetPaymentsCount() (count int64, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"purchase_id": model.ID,
		"deleted":     bson.M{"$ne": true},
	})
}

func (model *Purchase) ClearPayments() error {
	//log.Printf("Clearing Purchase payment history of purchase id:%s", model.Code)
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_payment")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"purchase_id": model.ID})
	if err != nil {
		return err
	}
	return nil
}

type ProductPurchaseStats struct {
	PurchaseCount    int64   `json:"purchase_count" bson:"purchase_count"`
	PurchaseQuantity float64 `json:"purchase_quantity" bson:"purchase_quantity"`
	Purchase         float64 `json:"purchase" bson:"purchase"`
}

func (product *Product) SetProductPurchaseStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.Client().Database(db.GetPosDB()).Collection("product_purchase_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductPurchaseStats

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
				"_id":               nil,
				"purchase_count":    bson.M{"$sum": 1},
				"purchase_quantity": bson.M{"$sum": "$quantity"},
				"purchase":          bson.M{"$sum": "$net_price"},
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

		stats.Purchase = math.Round(stats.Purchase*100) / 100
	}

	if productStoreTemp, ok := product.ProductStores[storeID.Hex()]; ok {
		productStoreTemp.PurchaseCount = stats.PurchaseCount
		productStoreTemp.PurchaseQuantity = stats.PurchaseQuantity
		productStoreTemp.Purchase = stats.Purchase
		product.ProductStores[storeID.Hex()] = productStoreTemp
	}

	err = product.Update()
	if err != nil {
		return err
	}

	/*
		for storeIndex, store := range product.Stores {
			if store.StoreID.Hex() == storeID.Hex() {
				product.Stores[storeIndex].PurchaseCount = stats.PurchaseCount
				product.Stores[storeIndex].PurchaseQuantity = stats.PurchaseQuantity
				product.Stores[storeIndex].Purchase = stats.Purchase
				err = product.Update()
				if err != nil {
					return err
				}
				break
			}
		}
	*/

	return nil
}

func (purchase *Purchase) SetProductsPurchaseStats() error {
	for _, purchaseProduct := range purchase.Products {
		product, err := FindProductByID(&purchaseProduct.ProductID, map[string]interface{}{})
		if err != nil {
			return err
		}

		err = product.SetProductPurchaseStatsByStoreID(*purchase.StoreID)
		if err != nil {
			return err
		}

	}
	return nil
}

//Vendor

type VendorPurchaseStats struct {
	PurchaseCount              int64   `json:"purchase_count" bson:"purchase_count"`
	PurchaseAmount             float64 `json:"purchase_amount" bson:"purchase_amount"`
	PurchasePaidAmount         float64 `json:"purchase_paid_amount" bson:"purchase_paid_amount"`
	PurchaseBalanceAmount      float64 `json:"purchase_balance_amount" bson:"purchase_balance_amount"`
	PurchaseRetailProfit       float64 `json:"purchase_retail_profit" bson:"purchase_retail_profit"`
	PurchaseWholesaleProfit    float64 `json:"purchase_wholesale_profit" bson:"purchase_wholesale_profit"`
	PurchaseRetailLoss         float64 `json:"purchase_retail_loss" bson:"purchase_retail_loss"`
	PurchaseWholesaleLoss      float64 `json:"purchase_wholesale_loss" bson:"purchase_wholesale_loss"`
	PurchasePaidCount          int64   `json:"purchase_paid_count" bson:"purchase_paid_count"`
	PurchaseNotPaidCount       int64   `json:"purchase_not_paid_count" bson:"purchase_not_paid_count"`
	PurchasePaidPartiallyCount int64   `json:"purchase_paid_partially_count" bson:"purchase_paid_partially_count"`
}

func (vendor *Vendor) SetVendorPurchaseStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats VendorPurchaseStats

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
				"_id":                       nil,
				"purchase_count":            bson.M{"$sum": 1},
				"purchase_amount":           bson.M{"$sum": "$net_total"},
				"purchase_paid_amount":      bson.M{"$sum": "$total_payment_paid"},
				"purchase_balance_amount":   bson.M{"$sum": "$balance_amount"},
				"purchase_retail_profit":    bson.M{"$sum": "$net_retail_profit"},
				"purchase_wholesale_profit": bson.M{"$sum": "$net_wholesale_profit"},
				"purchase_retail_loss":      bson.M{"$sum": "$retail_loss"},
				"purchase_wholesale_loss":   bson.M{"$sum": "$wholesale_loss"},
				"purchase_paid_count": bson.M{"$sum": bson.M{
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
				"purchase_not_paid_count": bson.M{"$sum": bson.M{
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
				"purchase_paid_partially_count": bson.M{"$sum": bson.M{
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
		return errors.New("error finding purchase stats aggregate: " + err.Error())
	}

	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return errors.New("Error decoding purchase stats: " + err.Error())
		}
		stats.PurchaseAmount = math.Round(stats.PurchaseAmount*100) / 100
		stats.PurchasePaidAmount = math.Round(stats.PurchasePaidAmount*100) / 100
		stats.PurchaseBalanceAmount = math.Round(stats.PurchaseBalanceAmount*100) / 100
		stats.PurchaseRetailProfit = math.Round(stats.PurchaseRetailProfit*100) / 100
		stats.PurchaseWholesaleProfit = math.Round(stats.PurchaseWholesaleProfit*100) / 100
		stats.PurchaseRetailLoss = math.Round(stats.PurchaseRetailLoss*100) / 100
		stats.PurchaseWholesaleLoss = math.Round(stats.PurchaseWholesaleLoss*100) / 100
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
		vendorStore.PurchaseCount = stats.PurchaseCount
		vendorStore.PurchasePaidCount = stats.PurchasePaidCount
		vendorStore.PurchaseNotPaidCount = stats.PurchaseNotPaidCount
		vendorStore.PurchasePaidPartiallyCount = stats.PurchasePaidPartiallyCount
		vendorStore.PurchaseAmount = stats.PurchaseAmount
		vendorStore.PurchasePaidAmount = stats.PurchasePaidAmount
		vendorStore.PurchaseBalanceAmount = stats.PurchaseBalanceAmount
		vendorStore.PurchaseRetailProfit = stats.PurchaseRetailProfit
		vendorStore.PurchaseWholesaleProfit = stats.PurchaseWholesaleProfit
		vendorStore.PurchaseRetailLoss = stats.PurchaseRetailLoss
		vendorStore.PurchaseWholesaleLoss = stats.PurchaseWholesaleLoss
		vendor.Stores[storeID.Hex()] = vendorStore
	} else {
		vendor.Stores[storeID.Hex()] = VendorStore{
			StoreID:                    storeID,
			StoreName:                  store.Name,
			StoreNameInArabic:          store.NameInArabic,
			PurchaseCount:              stats.PurchaseCount,
			PurchasePaidCount:          stats.PurchasePaidCount,
			PurchaseNotPaidCount:       stats.PurchaseNotPaidCount,
			PurchasePaidPartiallyCount: stats.PurchasePaidPartiallyCount,
			PurchaseAmount:             stats.PurchaseAmount,
			PurchasePaidAmount:         stats.PurchasePaidAmount,
			PurchaseBalanceAmount:      stats.PurchaseBalanceAmount,
			PurchaseRetailProfit:       stats.PurchaseRetailProfit,
			PurchaseWholesaleProfit:    stats.PurchaseWholesaleProfit,
			PurchaseRetailLoss:         stats.PurchaseRetailLoss,
			PurchaseWholesaleLoss:      stats.PurchaseWholesaleLoss,
		}
	}

	err = vendor.Update()
	if err != nil {
		return errors.New("Error updating vendor: " + err.Error())
	}

	return nil
}

func (purchase *Purchase) SetVendorPurchaseStats() error {

	vendor, err := FindVendorByID(purchase.VendorID, map[string]interface{}{})
	if err != nil {
		return errors.New("Error finding vendor: " + err.Error())
	}

	err = vendor.SetVendorPurchaseStatsByStoreID(*purchase.StoreID)
	if err != nil {
		return err
	}

	return nil
}

func (purchasePayment *PurchasePayment) DeletePurchasePayment() (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": purchasePayment.ID},
		bson.M{"$set": purchasePayment},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

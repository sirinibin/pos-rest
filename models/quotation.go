package models

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/jung-kurt/gofpdf"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

type QuotationProduct struct {
	ProductID         primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name              string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic      string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode          string             `bson:"item_code,omitempty" json:"item_code,omitempty"`
	PartNumber        string             `bson:"part_number,omitempty" json:"part_number,omitempty"`
	Quantity          float64            `json:"quantity,omitempty" bson:"quantity,omitempty"`
	Unit              string             `bson:"unit,omitempty" json:"unit,omitempty"`
	UnitPrice         float64            `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	PurchaseUnitPrice float64            `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	Profit            float64            `bson:"profit" json:"profit"`
	Loss              float64            `bson:"loss" json:"loss"`
}

// Quotation : Quotation structure
type Quotation struct {
	ID                       primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Code                     string              `bson:"code,omitempty" json:"code,omitempty"`
	Date                     *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr                  string              `json:"date_str,omitempty"`
	StoreID                  *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CustomerID               *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	Store                    *Store              `json:"store,omitempty"`
	Customer                 *Customer           `json:"customer,omitempty"`
	Products                 []QuotationProduct  `bson:"products,omitempty" json:"products,omitempty"`
	DeliveredBy              *primitive.ObjectID `json:"delivered_by,omitempty" bson:"delivered_by,omitempty"`
	DeliveredBySignatureID   *primitive.ObjectID `json:"delivered_by_signature_id,omitempty" bson:"delivered_by_signature_id,omitempty"`
	DeliveredBySignatureName string              `json:"delivered_by_signature_name,omitempty" bson:"delivered_by_signature_name,omitempty"`
	SignatureDate            *time.Time          `bson:"signature_date,omitempty" json:"signature_date,omitempty"`
	SignatureDateStr         string              `json:"signature_date_str,omitempty"`
	DeliveredByUser          *User               `json:"delivered_by_user,omitempty"`
	DeliveredBySignature     *Signature          `json:"delivered_by_signature,omitempty"`
	VatPercent               *float64            `bson:"vat_percent" json:"vat_percent"`
	Discount                 float64             `bson:"discount" json:"discount"`
	DiscountPercent          float64             `bson:"discount_percent" json:"discount_percent"`
	IsDiscountPercent        bool                `bson:"is_discount_percent" json:"is_discount_percent"`
	Status                   string              `bson:"status,omitempty" json:"status,omitempty"`
	TotalQuantity            float64             `bson:"total_quantity" json:"total_quantity"`
	VatPrice                 float64             `bson:"vat_price" json:"vat_price"`
	Total                    float64             `bson:"total" json:"total"`
	NetTotal                 float64             `bson:"net_total" json:"net_total"`
	ShippingOrHandlingFees   float64             `bson:"shipping_handling_fees" json:"shipping_handling_fees"`
	Profit                   float64             `bson:"profit" json:"profit"`
	NetProfit                float64             `bson:"net_profit" json:"net_profit"`
	Loss                     float64             `bson:"loss" json:"loss"`
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
	CustomerName             string              `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	StoreName                string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	DeliveredByName          string              `json:"delivered_by_name,omitempty" bson:"delivered_by_name,omitempty"`
	CreatedByName            string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName            string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName            string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	ValidityDays             *int64              `bson:"validity_days,omitempty" json:"validity_days,omitempty"`
	DeliveryDays             *int64              `bson:"delivery_days,omitempty" json:"delivery_days,omitempty"`
	ChangeLog                []ChangeLog         `json:"change_log,omitempty" bson:"change_log,omitempty"`
}

func UpdateQuotationProfit() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
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
		quotation := Quotation{}
		err = cur.Decode(&quotation)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		err = quotation.Update()
		if err != nil {
			return err
		}
	}

	return nil
}

type QuotationStats struct {
	ID        *primitive.ObjectID `json:"id" bson:"_id"`
	NetTotal  float64             `json:"net_total" bson:"net_total"`
	NetProfit float64             `json:"net_profit" bson:"net_profit"`
	Loss      float64             `json:"loss" bson:"loss"`
}

func (quotation *Quotation) CalculateQuotationProfit() error {
	totalProfit := float64(0.0)
	totalLoss := float64(0.0)
	for i, quotationProduct := range quotation.Products {
		product, err := FindProductByID(&quotationProduct.ProductID, map[string]interface{}{})
		if err != nil {
			return err
		}
		quantity := quotationProduct.Quantity

		salesPrice := quantity * quotationProduct.UnitPrice

		purchaseUnitPrice := quotation.Products[i].PurchaseUnitPrice

		if purchaseUnitPrice == 0 {
			for _, store := range product.Stores {
				if store.StoreID == *quotation.StoreID {
					purchaseUnitPrice = store.PurchaseUnitPrice
					quotation.Products[i].PurchaseUnitPrice = purchaseUnitPrice
					break
				}
			}
		}

		profit := salesPrice - (quantity * purchaseUnitPrice)
		profit = math.Round(profit*100) / 100

		if profit >= 0 {
			quotation.Products[i].Profit = profit
			quotation.Products[i].Loss = 0.0
			totalProfit += quotation.Products[i].Profit
		} else {
			quotation.Products[i].Profit = 0
			quotation.Products[i].Loss = (profit * -1)
			totalLoss += quotation.Products[i].Loss
		}

	}

	quotation.Profit = math.Round(totalProfit*100) / 100
	quotation.NetProfit = math.Round((totalProfit-quotation.Discount)*100) / 100
	quotation.Loss = totalLoss
	return nil
}

func GetQuotationStats(filter map[string]interface{}) (stats QuotationStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":        nil,
				"net_total":  bson.M{"$sum": "$net_total"},
				"net_profit": bson.M{"$sum": "$net_profit"},
				"loss":       bson.M{"$sum": "$loss"},
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
		stats.NetProfit = math.Round(stats.NetProfit*100) / 100
		stats.Loss = math.Round(stats.Loss*100) / 100

		return stats, nil
	}
	return stats, nil
}

func (quotation *Quotation) GeneratePDF() error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 8)
	//pdf.Cell(float64(10), float64(5), "GULF UNION OZONE CO.")
	pdf.CellFormat(50, 7, "GULF UNION OZONE CO.", "1", 0, "LM", false, 0, "")
	pdf.AddLayer("layer1", true)
	//pdf.Rect(float64(5), float64(5), float64(201), float64(286), "")
	pdf.SetCreator("Sirin K", true)
	pdf.SetMargins(float64(2), float64(2), float64(2))
	pdf.Rect(float64(5), float64(5), float64(201), float64(286), "")
	pdf.AddPage()
	pdf.Cell(40, 10, "Hello, world2")

	filename := "pdfs/quotations/quotation_" + quotation.Code + ".pdf"

	return pdf.OutputFileAndClose(filename)
}

/*
func (quotation *Quotation) GeneratePDF() error {

	pdfg, err := wkhtml.NewPDFGenerator()
	if err != nil {
		return err
	}

	//	htmlStr := quotation.getHTML()

	html, err := ioutil.ReadFile("html-templates/quotation.html")
	//html, err := ioutil.ReadFile("html-templates/test.html")

	if err != nil {
		return err
	}

	//log.Print(html)

	//page := wkhtmltopdf.NewPageReader(bytes.NewReader(html))
	//pdfg.AddPage(page)
	//	page.NoBackground.Set(true)
	//	page.DisableExternalLinks.Set(false)

	// create a new instance of the PDF generator


	//pdfg.AddPage(wkhtml.NewPageReader(strings.NewReader(htmlStr)))

	//pageReader.PageOptions.EnableLocalFileAccess.Set(true)
	pdfg.Cover.EnableLocalFileAccess.Set(true)
	pdfg.AddPage(wkhtml.NewPageReader(bytes.NewReader(html)))

	// Create PDF document in internal buffer
	err = pdfg.Create()
	if err != nil {
		return err
	}

	filename := "pdfs/quotations/quotation_" + quotation.Code + ".pdf"

	//Your Pdf Name
	err = pdfg.WriteFile(filename)
	if err != nil {
		return err
	}
	return nil
}
*/

func (quotation *Quotation) SetChangeLog(
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
	}

	quotation.ChangeLog = append(
		quotation.ChangeLog,
		ChangeLog{
			Event:         event,
			Description:   description,
			CreatedBy:     &UserObject.ID,
			CreatedByName: UserObject.Name,
			CreatedAt:     &now,
		},
	)
}

func (quotation *Quotation) AttributesValueChangeEvent(quotationOld *Quotation) error {

	if quotation.Status != quotationOld.Status {
		/*
			quotation.SetChangeLog(
				"attribute_value_change",
				"status",
				quotationOld.Status,
				quotation.Status,
			)
		*/
	}

	return nil
}

func (quotation *Quotation) UpdateForeignLabelFields() error {

	if quotation.StoreID != nil {
		store, err := FindStoreByID(quotation.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.StoreName = store.Name
	}

	if quotation.CustomerID != nil {
		customer, err := FindCustomerByID(quotation.CustomerID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.CustomerName = customer.Name
	}

	if quotation.DeliveredBy != nil {
		deliveredByUser, err := FindUserByID(quotation.DeliveredBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.DeliveredByName = deliveredByUser.Name
	}

	if quotation.DeliveredBySignatureID != nil {
		deliveredBySignature, err := FindSignatureByID(quotation.DeliveredBySignatureID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.DeliveredBySignatureName = deliveredBySignature.Name
	}

	if quotation.CreatedBy != nil {
		createdByUser, err := FindUserByID(quotation.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.CreatedByName = createdByUser.Name
	}

	if quotation.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(quotation.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.UpdatedByName = updatedByUser.Name
	}

	if quotation.DeletedBy != nil && !quotation.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(quotation.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.DeletedByName = deletedByUser.Name
	}

	for i, product := range quotation.Products {
		productObject, err := FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1, "part_number": 1})
		if err != nil {
			return err
		}
		quotation.Products[i].Name = productObject.Name
		quotation.Products[i].NameInArabic = productObject.NameInArabic
		quotation.Products[i].ItemCode = productObject.ItemCode
		quotation.Products[i].PartNumber = productObject.PartNumber
	}

	return nil
}

func (quotation *Quotation) FindNetTotal() {
	netTotal := float64(0.0)
	for _, product := range quotation.Products {
		netTotal += (float64(product.Quantity) * product.UnitPrice)
	}

	netTotal -= quotation.Discount
	netTotal -= quotation.ShippingOrHandlingFees

	if quotation.VatPercent != nil {
		netTotal += netTotal * (*quotation.VatPercent / float64(100))
	}

	quotation.NetTotal = math.Round(netTotal*100) / 100
}

func (quotation *Quotation) FindTotal() {
	total := float64(0.0)
	for _, product := range quotation.Products {
		total += (float64(product.Quantity) * product.UnitPrice)
	}

	quotation.Total = math.Round(total*100) / 100
}

func (quotation *Quotation) FindTotalQuantity() {
	totalQuantity := float64(0)
	for _, product := range quotation.Products {
		totalQuantity += product.Quantity
	}
	quotation.TotalQuantity = totalQuantity
}

func (quotation *Quotation) FindVatPrice() {
	vatPrice := ((*quotation.VatPercent / 100) * (quotation.Total - quotation.Discount + quotation.ShippingOrHandlingFees))
	vatPrice = math.Round(vatPrice*100) / 100
	quotation.VatPrice = vatPrice
}

func SearchQuotation(w http.ResponseWriter, r *http.Request) (quotations []Quotation, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[date_str]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return quotations, criterias, err
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
			return quotations, criterias, err
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
			return quotations, criterias, err
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
			return quotations, criterias, err
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
			return quotations, criterias, err
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
			return quotations, criterias, err
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

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return quotations, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[net_total]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotations, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_total"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["net_total"] = float64(value)
		}

	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {

		customerIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range customerIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return quotations, criterias, err
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
				return quotations, criterias, err
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
			return quotations, criterias, err
		}
		criterias.SearchBy["delivered_by"] = deliveredByID
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

	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
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
		return quotations, criterias, errors.New("Error fetching quotations:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return quotations, criterias, errors.New("Cursor error:" + err.Error())
		}
		quotation := Quotation{}
		err = cur.Decode(&quotation)
		if err != nil {
			return quotations, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["store.id"]; ok {
			quotation.Store, _ = FindStoreByID(quotation.StoreID, storeSelectFields)
		}
		if _, ok := criterias.Select["customer.id"]; ok {
			quotation.Customer, _ = FindCustomerByID(quotation.CustomerID, customerSelectFields)
		}
		if _, ok := criterias.Select["created_by_user.id"]; ok {
			quotation.CreatedByUser, _ = FindUserByID(quotation.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			quotation.UpdatedByUser, _ = FindUserByID(quotation.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			quotation.DeletedByUser, _ = FindUserByID(quotation.DeletedBy, deletedByUserSelectFields)
		}

		quotations = append(quotations, quotation)
	} //end for loop

	return quotations, criterias, nil
}

func (quotation *Quotation) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if govalidator.IsNull(quotation.Status) {
		errs["status"] = "Status is required"
	}

	/*
		if govalidator.IsNull(quotation.DateStr) {
			errs["date_str"] = "date_str is required"
		} else {
			const shortForm = "Jan 02 2006"
			date, err := time.Parse(shortForm, quotation.DateStr)
			if err != nil {
				errs["date_str"] = "Invalid date format"
			}
			quotation.Date = &date
		}
	*/

	if govalidator.IsNull(quotation.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		/*
			const shortForm = "Jan 02 2006"
			date, err := time.Parse(shortForm, order.DateStr)
			if err != nil {
				errs["date_str"] = "Invalid date format"
			}
			order.Date = &date
		*/

		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, quotation.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		quotation.Date = &date
		//quotation.CreatedAt = &date
	}

	if !govalidator.IsNull(quotation.SignatureDateStr) {
		const shortForm = "Jan 02 2006"
		date, err := time.Parse(shortForm, quotation.SignatureDateStr)
		if err != nil {
			errs["signature_date_str"] = "Invalid date format"
		}
		quotation.SignatureDate = &date
	}

	if scenario == "update" {
		if quotation.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsQuotationExists(&quotation.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Quotation:" + quotation.ID.Hex()
		}

	}

	if quotation.StoreID == nil || quotation.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(quotation.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + quotation.StoreID.Hex()
		}
	}

	if quotation.CustomerID == nil || quotation.CustomerID.IsZero() {
		errs["customer_id"] = "Customer is required"
	} else {
		exists, err := IsCustomerExists(quotation.CustomerID)
		if err != nil {
			errs["customer_id"] = err.Error()
		}

		if !exists {
			errs["customer_id"] = "Invalid Customer:" + quotation.CustomerID.Hex()
		}
	}

	if quotation.DeliveredBy == nil || quotation.DeliveredBy.IsZero() {
		errs["delivered_by"] = "Delivered By is required"
	} else {
		exists, err := IsUserExists(quotation.DeliveredBy)
		if err != nil {
			errs["delivered_by"] = err.Error()
			return errs
		}

		if !exists {
			errs["delivered_by"] = "Invalid Delivered By:" + quotation.DeliveredBy.Hex()
		}
	}

	if len(quotation.Products) == 0 {
		errs["product_id"] = "Atleast 1 product is required for quotation"
	}

	if quotation.DeliveredBySignatureID != nil && !quotation.DeliveredBySignatureID.IsZero() {
		exists, err := IsSignatureExists(quotation.DeliveredBySignatureID)
		if err != nil {
			errs["delivered_by_signature_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["delivered_by_signature_id"] = "Invalid Delivered By Signature:" + quotation.DeliveredBySignatureID.Hex()
		}
	}

	for index, product := range quotation.Products {
		if product.ProductID.IsZero() {
			errs["product_id_"+strconv.Itoa(index)] = "Product is required for quotation"
		} else {
			exists, err := IsProductExists(&product.ProductID)
			if err != nil {
				errs["product_id_"+strconv.Itoa(index)] = err.Error()
				return errs
			}

			if !exists {
				errs["product_id_"+strconv.Itoa(index)] = "Invalid product_id:" + product.ProductID.Hex() + " in products"
			}
		}

		if product.Quantity == 0 {
			errs["quantity_"+strconv.Itoa(index)] = "Quantity is required"
		}

		if product.UnitPrice == 0 {
			errs["unit_price_"+strconv.Itoa(index)] = "Unit Price is required"
		}

		if product.PurchaseUnitPrice == 0 {
			errs["purchase_unit_price_"+strconv.Itoa(index)] = "Purchase Unit Price is required"
		}

	}

	if quotation.VatPercent == nil {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if quotation.ValidityDays == nil {
		errs["validity_days"] = "Validity days are required"
	} else if *quotation.ValidityDays < 1 {
		errs["validity_days"] = "Validity days should be greater than 0"
	}

	if quotation.DeliveryDays == nil {
		errs["delivery_days"] = "Delivery days are required"
	} else if *quotation.DeliveryDays < 1 {
		errs["delivery_days"] = "Delivery days should be greater than 0"
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func (quotation *Quotation) GenerateCode(startFrom int, storeCode string) (string, error) {
	count, err := GetTotalCount(bson.M{"store_id": quotation.StoreID}, "quotation")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return storeCode + "-" + strconv.Itoa(code+1), nil
}

func (quotation *Quotation) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.InsertOne(ctx, &quotation)
	if err != nil {
		return err
	}
	return nil
}

func (quotation *Quotation) MakeCode() error {
	lastQuotation, err := FindLastQuotationByStoreID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}
	if lastQuotation == nil {
		store, err := FindStoreByID(quotation.StoreID, bson.M{})
		if err != nil {
			return err
		}
		quotation.Code = store.Code + "-50000"
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
			quotation.Code = storeCode + "-" + strconv.Itoa(codeInt)
		}
	}

	for {
		exists, err := quotation.IsCodeExists()
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
		quotation.Code = storeCode + "-" + strconv.Itoa(codeInt)
	}

	return nil
}

func FindLastQuotationByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (quotation *Quotation, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": storeID}, findOneOptions).
		Decode(&quotation)
	if err != nil {
		return nil, err
	}

	return quotation, err
}

func (quotation *Quotation) IsCodeExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if quotation.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": quotation.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": quotation.Code,
			"_id":  bson.M{"$ne": quotation.ID},
		})
	}

	return (count == 1), err
}

func (quotation *Quotation) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": quotation.ID},
		bson.M{"$set": quotation},
		updateOptions,
	)
	if err != nil {
		return err
	}
	return nil
}

func (quotation *Quotation) DeleteQuotation(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = quotation.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	quotation.Deleted = true
	quotation.DeletedBy = &userID
	now := time.Now()
	quotation.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": quotation.ID},
		bson.M{"$set": quotation},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func FindQuotationByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (quotation *Quotation, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&quotation)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["store.id"]; ok {
		storeSelectFields := ParseRelationalSelectString(selectFields, "store")
		quotation.Store, _ = FindStoreByID(quotation.StoreID, storeSelectFields)
	}

	if _, ok := selectFields["customer.id"]; ok {
		customerSelectFields := ParseRelationalSelectString(selectFields, "customer")
		quotation.Customer, _ = FindCustomerByID(quotation.CustomerID, customerSelectFields)
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		quotation.CreatedByUser, _ = FindUserByID(quotation.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		quotation.UpdatedByUser, _ = FindUserByID(quotation.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		quotation.DeletedByUser, _ = FindUserByID(quotation.DeletedBy, fields)
	}

	return quotation, err
}

func IsQuotationExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

func ProcessQuotations() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
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
		quotation := Quotation{}
		err = cur.Decode(&quotation)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		quotation.ClearProductsQuotationHistory()
		err = quotation.AddProductsQuotationHistory()
		if err != nil {
			return err
		}

		err = quotation.SetProductsQuotationStats()
		if err != nil {
			return err
		}

		//quotation.Date = quotation.CreatedAt
		err = quotation.Update()
		if err != nil {
			return err
		}
	}

	return nil
}

type ProductQuotationStats struct {
	QuotationCount    int64   `json:"quotation_count" bson:"quotation_count"`
	QuotationQuantity float64 `json:"quotation_quantity" bson:"quotation_quantity"`
	Quotation         float64 `json:"quotation" bson:"quotation"`
}

func (product *Product) SetProductQuotationStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.Client().Database(db.GetPosDB()).Collection("product_quotation_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductQuotationStats

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
				"_id":                nil,
				"quotation_count":    bson.M{"$sum": 1},
				"quotation_quantity": bson.M{"$sum": "$quantity"},
				"quotation":          bson.M{"$sum": "$net_price"},
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
	}

	for storeIndex, store := range product.Stores {
		if store.StoreID.Hex() == storeID.Hex() {
			product.Stores[storeIndex].QuotationCount = stats.QuotationCount
			product.Stores[storeIndex].QuotationQuantity = stats.QuotationQuantity
			product.Stores[storeIndex].Quotation = stats.Quotation
			err = product.Update()
			if err != nil {
				return err
			}
			break
		}
	}

	return nil
}

func (quotation *Quotation) SetProductsQuotationStats() error {
	for _, quotationProduct := range quotation.Products {
		product, err := FindProductByID(&quotationProduct.ProductID, map[string]interface{}{})
		if err != nil {
			return err
		}

		err = product.SetProductQuotationStatsByStoreID(*quotation.StoreID)
		if err != nil {
			return err
		}
	}
	return nil
}

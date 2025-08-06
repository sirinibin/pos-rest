package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	documentai "cloud.google.com/go/documentai/apiv1"
	documentaipb "cloud.google.com/go/documentai/apiv1/documentaipb"

	"github.com/gorilla/mux"
	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/api/option"
)

// ListPurchase : handler for GET /purchase
func ListPurchase(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	purchases, criterias, err := store.SearchPurchase(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find purchases:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "purchase")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of purchases:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	var purchaseStats models.PurchaseStats

	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] == "1" {
			purchaseStats, err = store.GetPurchaseStats(criterias.SearchBy)
			if err != nil {
				response.Status = false
				response.Errors["purchase_stats"] = "Unable to find purchase stats:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_purchase"] = purchaseStats.NetTotal
	response.Meta["vat_price"] = purchaseStats.VatPrice
	response.Meta["discount"] = purchaseStats.Discount
	response.Meta["cash_discount"] = purchaseStats.CashDiscount
	response.Meta["shipping_handling_fees"] = purchaseStats.ShippingOrHandlingFees
	response.Meta["net_retail_profit"] = purchaseStats.NetRetailProfit
	response.Meta["net_wholesale_profit"] = purchaseStats.NetWholesaleProfit
	response.Meta["paid_purchase"] = purchaseStats.PaidPurchase
	response.Meta["unpaid_purchase"] = purchaseStats.UnPaidPurchase
	response.Meta["cash_purchase"] = purchaseStats.CashPurchase
	response.Meta["bank_account_purchase"] = purchaseStats.BankAccountPurchase
	response.Meta["return_count"] = purchaseStats.ReturnCount
	response.Meta["return_amount"] = purchaseStats.ReturnAmount

	if len(purchases) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = purchases
	}

	json.NewEncoder(w).Encode(response)

}

// CreatePurchase : handler for POST /purchase
func CreatePurchase(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var purchase *models.Purchase
	// Decode data
	if !utils.Decode(w, r, &purchase) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.CreatedBy = &userID
	purchase.UpdatedBy = &userID
	now := time.Now()
	purchase.CreatedAt = &now
	purchase.UpdatedAt = &now
	purchase.FindNetTotal()

	// Validate data
	if errs := purchase.Validate(w, r, "create", nil); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	//Queue
	queue := GetOrCreateQueue(store.ID.Hex(), "purchase")
	queueToken := generateQueueToken()
	queue.Enqueue(Request{Token: queueToken})
	queue.WaitUntilMyTurn(queueToken)

	err = purchase.CreateNewVendorFromName()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "purchase")
		response.Status = false
		response.Errors["new_vendor_from_name"] = "error creating new vendor from name: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.FindTotalQuantity()
	err = purchase.UpdateForeignLabelFields()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "purchase")
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update_foreign_fields"] = "error updating foreign fields: " + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}
	err = purchase.MakeCode()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "purchase")
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["code"] = "error making code: " + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.Insert()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "purchase")

		redisErr := purchase.UnMakeRedisCode()
		if redisErr != nil {
			response.Errors["error_unmaking_code"] = "error_unmaking_code: " + redisErr.Error()
		}
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	queue.Pop()
	CleanupQueueIfEmpty(store.ID.Hex(), "purchase")

	purchase.CreateProductsPurchaseHistory()

	err = purchase.AddPayments()
	if err != nil {
		response.Status = false
		response.Errors["creating_payments"] = "Error creating payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.SetPaymentStatus()
	purchase.Update()

	err = purchase.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["add_stock"] = "Unable to add stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.UpdateProductUnitPriceInStore()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["product_unit_price"] = "Unable to update product unit price:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.CloseSalesPayment()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["closing_sales_payment"] = "error closing sales payment: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.SetProductsPurchaseStats()

	purchase.SetVendorPurchaseStats()

	if !store.Settings.DisablePurchasesOnAccounts {
		err = purchase.DoAccounting()
		if err != nil {
			response.Status = false
			response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	if purchase.VendorID != nil && !purchase.VendorID.IsZero() {
		vendor, _ := store.FindVendorByID(purchase.VendorID, bson.M{})
		if vendor != nil {
			if !store.Settings.DisablePurchasesOnAccounts {
				vendor.SetCreditBalance()
			}
		}
	}

	if !store.Settings.DisablePurchasesOnAccounts {
		go purchase.SetPostBalances()
	}

	go purchase.CreateProductsHistory()

	store.NotifyUsers("purchase_updated")
	response.Status = true
	response.Result = purchase

	json.NewEncoder(w).Encode(response)
}

// UpdatePurchase : handler function for PUT /v1/purchase call
func UpdatePurchase(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	var purchase *models.Purchase
	var purchaseOld *models.Purchase

	params := mux.Vars(r)

	purchaseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["purchase_id"] = "Invalid Purchase ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchaseOld, err = store.FindPurchaseByID(&purchaseID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_purchase"] = "Unable to find purchase:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase, err = store.FindPurchaseByID(&purchaseID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_purchase"] = "Unable to find purchase:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &purchase) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.UpdatedBy = &userID
	now := time.Now()
	purchase.UpdatedAt = &now
	purchase.FindNetTotal()

	// Validate data
	if errs := purchase.Validate(w, r, "update", purchaseOld); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.CreateNewVendorFromName()
	if err != nil {
		response.Status = false
		response.Errors["new_vendor_from_name"] = "error creating new vendor from name: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.FindTotalQuantity()
	purchase.UpdateForeignLabelFields()
	purchase.CalculatePurchaseExpectedProfit()

	err = purchase.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.ClearProductsPurchaseHistory()
	purchase.CreateProductsPurchaseHistory()

	err = purchase.UpdatePayments()
	if err != nil {
		response.Status = false
		response.Errors["updated_payments"] = "Error updating payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.SetPaymentStatus()
	purchase.Update()

	err = purchaseOld.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["remove_stock"] = "Unable to remove stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["add_stock"] = "Unable to add stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.UpdateProductUnitPriceInStore()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["product_unit_price"] = "Unable to update product unit price:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.AttributesValueChangeEvent(purchaseOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.CloseSalesPayment()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["closing_sales_payment"] = "error closing sales payment: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.SetProductsPurchaseStats()
	purchaseOld.SetProductsPurchaseStats()
	purchase.SetVendorPurchaseStats()

	if !store.Settings.DisablePurchasesOnAccounts {
		err = purchase.UndoAccounting()
		if err != nil {
			response.Status = false
			response.Errors["undo_accounting"] = "Error undo accounting: " + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}

		err = purchase.DoAccounting()
		if err != nil {
			response.Status = false
			response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	purchase, err = store.FindPurchaseByID(&purchase.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find purchase:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if purchase.VendorID != nil && !purchase.VendorID.IsZero() {
		vendor, _ := store.FindVendorByID(purchase.VendorID, bson.M{})
		if vendor != nil {
			if !store.Settings.DisablePurchasesOnAccounts {
				vendor.SetCreditBalance()
			}
		}
	}

	if purchaseOld.VendorID != nil && !purchaseOld.VendorID.IsZero() {
		vendor, _ := store.FindVendorByID(purchaseOld.VendorID, bson.M{})
		if vendor != nil {
			if !store.Settings.DisablePurchasesOnAccounts {
				vendor.SetCreditBalance()
			}
			purchaseOld.SetVendorPurchaseStats()
		}
		purchaseOld.SetProductsPurchaseStats()
	}

	if !store.Settings.DisablePurchasesOnAccounts {
		go purchase.SetPostBalances()
	}

	go func() {
		purchase.ClearProductsHistory()
		purchase.CreateProductsHistory()
	}()

	store.NotifyUsers("purchase_updated")
	response.Status = true
	response.Result = purchase
	json.NewEncoder(w).Encode(response)
}

// ViewPurchase : handler function for GET /v1/purchase/<id> call
func ViewPurchase(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)

	purchaseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid Purchase ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var purchase *models.Purchase

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	store, err := ParseStore(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase, err = store.FindPurchaseByID(&purchaseID, selectFields)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if purchase.VendorID != nil && !purchase.VendorID.IsZero() {
		vendor, err := store.FindVendorByID(purchase.VendorID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			w.WriteHeader(http.StatusBadRequest)
			response.Status = false
			response.Errors["view"] = "error fetching vendor:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
		vendor.SetSearchLabel()
		purchase.Vendor = vendor
	}

	response.Status = true
	response.Result = purchase

	json.NewEncoder(w).Encode(response)
}

// DeletePurchase : handler function for DELETE /v1/purchase/<id> call
func DeletePurchase(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)

	purchaseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["quotation_id"] = "Invalid Purchase ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase, err := store.FindPurchaseByID(&purchaseID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.DeletePurchase(tokenClaims)
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = "Deleted successfully"

	json.NewEncoder(w).Encode(response)
}

// CreateOrder : handler for POST /order
func CalculatePurchaseNetTotal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	var purchase *models.Purchase
	// Decode data
	if !utils.Decode(w, r, &purchase) {
		return
	}

	purchase.FindNetTotal()

	response.Status = true
	response.Result = purchase

	json.NewEncoder(w).Encode(response)
}

type FieldBoundingBox struct {
	Field      string                 `json:"field"`
	Value      string                 `json:"value"`
	Page       int64                  `json:"page"`
	Vertices   []*documentaipb.Vertex `json:"vertices"`
	IsLineItem bool                   `json:"is_line_item"`
	GroupID    string                 `json:"group_id,omitempty"`
}

func ParsePurchaseBill(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = r.ParseMultipartForm(100 << 20)
	if err != nil {
		http.Error(w, "Could not parse form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Could not retrieve image file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	img, format, err := image.Decode(file)
	if err != nil {
		http.Error(w, "Uploaded file is not a valid image", http.StatusUnsupportedMediaType)
		return
	}
	file.Seek(0, io.SeekStart)

	uploadDir := "./images/purchase"
	os.MkdirAll(uploadDir, os.ModePerm)

	timestamp := time.Now().UnixNano()
	filename := fmt.Sprintf("purchase_%d.%s", timestamp, format)
	filePath := filepath.Join(uploadDir, filename)

	out, err := os.Create(filePath)
	if err != nil {
		http.Error(w, "Could not create file on server", http.StatusInternalServerError)
		return
	}
	defer out.Close()

	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		err = jpeg.Encode(out, img, &jpeg.Options{Quality: 85})
	case "png":
		err = png.Encode(out, img)
	default:
		http.Error(w, "Unsupported image format", http.StatusUnsupportedMediaType)
		return
	}
	if err != nil {
		http.Error(w, "Could not encode image", http.StatusInternalServerError)
		return
	}

	// Document AI
	projectID := "startpos-464823"
	location := "us"
	processorID := "4b486929b947b5c2"

	imageBytes, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read image: %v", err)
	}

	ctx := context.Background()
	client, err := documentai.NewDocumentProcessorClient(ctx, option.WithCredentialsFile(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")))
	if err != nil {
		log.Fatalf("Failed to create Document AI client: %v", err)
	}
	defer client.Close()

	name := fmt.Sprintf("projects/%s/locations/%s/processors/%s", projectID, location, processorID)
	mimeType := "image/jpeg"
	if format == "png" {
		mimeType = "image/png"
	}

	req := &documentaipb.ProcessRequest{
		Name: name,
		Source: &documentaipb.ProcessRequest_RawDocument{
			RawDocument: &documentaipb.RawDocument{
				Content:  imageBytes,
				MimeType: mimeType,
			},
		},
	}

	resp, err := client.ProcessDocument(ctx, req)
	if err != nil {
		log.Fatalf("Failed to process image: %v", err)
	}

	// Result holders
	invoiceFields := make(map[string]string)
	var invoiceDateTime time.Time
	var lineItems []map[string]string
	var boundingBoxes []FieldBoundingBox

	// Line items: group by unique ID
	lineItemGroups := make(map[string]map[string]string)

	for _, entity := range resp.Document.GetEntities() {
		field := entity.GetType()
		value := entity.GetMentionText()
		page := int64(0)
		var vertices []*documentaipb.Vertex

		if len(entity.GetPageAnchor().GetPageRefs()) > 0 {
			pageRef := entity.GetPageAnchor().GetPageRefs()[0]
			page = pageRef.GetPage()
			vertices = pageRef.GetBoundingPoly().GetVertices()
		}

		isLineItem := strings.HasPrefix(field, "Line_")
		groupID := entity.GetId()
		if groupID == "" && isLineItem {
			groupID = fmt.Sprintf("line_%d", len(lineItemGroups)+1)
		}

		box := FieldBoundingBox{
			Field:      field,
			Value:      value,
			Page:       page,
			Vertices:   vertices,
			IsLineItem: isLineItem,
			GroupID:    groupID,
		}

		boundingBoxes = append(boundingBoxes, box)

		if isLineItem {
			if _, exists := lineItemGroups[groupID]; !exists {
				lineItemGroups[groupID] = make(map[string]string)
			}
			lineItemGroups[groupID][field] = value
		} else {
			invoiceFields[field] = value
			if field == "Invoice_Datetime" {
				t, err := time.Parse(time.RFC3339, value)
				if err == nil {
					invoiceDateTime = t
				}
			}
		}
	}

	for _, item := range lineItemGroups {
		lineItems = append(lineItems, item)
	}

	// Clean up uploaded file
	err = os.Remove(filePath)
	if err != nil {
		log.Println("Failed to delete image file:", err)
	}

	// Final Response
	response.Status = true
	response.Result = map[string]interface{}{
		"filename":         filename,
		"fields":           invoiceFields,
		"invoice_datetime": invoiceDateTime.Format(time.RFC3339),
		"line_items":       lineItems,
		"bounding_boxes":   boundingBoxes,
	}
	json.NewEncoder(w).Encode(response)
}

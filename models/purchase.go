package models

import (
	"context"
	"errors"
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

type PurchaseProduct struct {
	ProductID        primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Quantity         int                `json:"quantity,omitempty" bson:"quantity,omitempty"`
	UnitPrice        float32            `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	SellingUnitPrice float32            `bson:"selling_unit_price,omitempty" json:"selling_unit_price,omitempty"`
}

//Purchase : Purchase structure
type Purchase struct {
	ID                       primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Code                     string             `bson:"code,omitempty" json:"code,omitempty"`
	StoreID                  primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	VendorID                 primitive.ObjectID `json:"vendor_id,omitempty" bson:"vendor_id,omitempty"`
	Products                 []PurchaseProduct  `bson:"products,omitempty" json:"products,omitempty"`
	OrderPlacedBy            primitive.ObjectID `json:"order_placed_by,omitempty" bson:"order_placed,omitempty"`
	OrderPlacedBySignatureID primitive.ObjectID `json:"order_placed_by_signature_id,omitempty" bson:"order_placed_signature_id,omitempty"`
	VatPercent               *float32           `bson:"vat_percent,omitempty" json:"vat_percent,omitempty"`
	Discount                 float32            `bson:"discount,omitempty" json:"discount,omitempty"`
	Status                   string             `bson:"status,omitempty" json:"status,omitempty"`
	StockAdded               bool               `bson:"stock_added,omitempty" json:"stock_added,omitempty"`
	Deleted                  bool               `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy                primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedAt                time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt                time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt                time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy                primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy                primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
}

func SearchPurchase(w http.ResponseWriter, r *http.Request) (purchases []Purchase, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	keys, ok := r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return purchases, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[vendor_id]"]
	if ok && len(keys[0]) >= 1 {
		vendorID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return purchases, criterias, err
		}
		criterias.SearchBy["vendor_id"] = vendorID
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

	if govalidator.IsNull(purchase.Status) {
		errs["status"] = "Status is required"
	}

	if scenario == "update" {
		if purchase.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsPurchaseExists(purchase.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Purchase:" + purchase.ID.Hex()
		}

		if oldPurchase != nil {
			if oldPurchase.Status == "delivered" {

				if purchase.Status == "pending" ||
					purchase.Status == "cancelled" ||
					purchase.Status == "order_placed" ||
					purchase.Status == "dispatched" {
					errs["status"] = "Can't change the status from delivered to pending/cancelled/order_placed/dispatched"
				}
			}
		}

	}

	if purchase.StoreID.IsZero() {
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

	if purchase.VendorID.IsZero() {
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

	if purchase.OrderPlacedBy.IsZero() {
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

	if !purchase.OrderPlacedBySignatureID.IsZero() {
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
		errs["products"] = "Atleast 1 product is required for purchase"
	}

	for _, product := range purchase.Products {
		if product.ProductID.IsZero() {
			errs["product_id"] = "Product is required for purchase"
		} else {
			exists, err := IsProductExists(product.ProductID)
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

		if product.UnitPrice == 0 {
			errs["unit_price"] = "Unit Price is required"
		}

		if product.SellingUnitPrice == 0 {
			errs["selling_unit_price"] = "Selling Unit Price is required"
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
		product, err := FindProductByID(purchaseProduct.ProductID)
		if err != nil {
			return err
		}

		storeExistInProductStock := false
		for k, stock := range product.Stock {
			if stock.StoreID.Hex() == purchase.StoreID.Hex() {
				product.Stock[k].Stock += purchaseProduct.Quantity
				storeExistInProductStock = true
				break
			}
		}

		if !storeExistInProductStock {
			productStock := ProductStock{
				StoreID: purchase.StoreID,
				Stock:   purchaseProduct.Quantity,
			}
			product.Stock = append(product.Stock, productStock)
		}

		product, err = product.Update()
		if err != nil {
			return err
		}
	}
	purchase.StockAdded = true
	_, err = purchase.Update()
	if err != nil {
		return err
	}

	return nil
}

func (purchase *Purchase) RemoveStock() (err error) {
	for _, purchaseProduct := range purchase.Products {
		product, err := FindProductByID(purchaseProduct.ProductID)
		if err != nil {
			return err
		}

		for k, stock := range product.Stock {
			if stock.StoreID.Hex() == purchase.StoreID.Hex() {
				product.Stock[k].Stock -= purchaseProduct.Quantity
				break
			}
		}

		product, err = product.Update()
		if err != nil {
			return err
		}
	}
	return nil
}

func (purchase *Purchase) UpdateProductUnitPriceInStore() (err error) {

	for _, purchaseProduct := range purchase.Products {
		product, err := FindProductByID(purchaseProduct.ProductID)
		if err != nil {
			return err
		}

		storeExistInProductUnitPrice := false
		for k, unitPrice := range product.UnitPrices {
			if unitPrice.StoreID.Hex() == purchase.StoreID.Hex() {
				product.UnitPrices[k].WholeSalePrice = purchaseProduct.UnitPrice
				product.UnitPrices[k].RetailPrice = purchaseProduct.SellingUnitPrice
				storeExistInProductUnitPrice = true
				break
			}
		}

		if !storeExistInProductUnitPrice {
			productUnitPrice := ProductUnitPrice{
				StoreID:        purchase.StoreID,
				WholeSalePrice: purchaseProduct.UnitPrice,
				RetailPrice:    purchaseProduct.SellingUnitPrice,
			}
			product.UnitPrices = append(product.UnitPrices, productUnitPrice)
		}
		_, err = product.Update()
		if err != nil {
			return err
		}
	}

	return nil
}

func (purchase *Purchase) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	purchase.ID = primitive.NewObjectID()
	if len(purchase.Code) == 0 {
		for true {
			purchase.Code = strings.ToUpper(GeneratePurchaseCode(12))
			exists, err := purchase.IsCodeExists()
			if err != nil {
				return err
			}
			if !exists {
				break
			}
		}
	}
	_, err := collection.InsertOne(ctx, &purchase)
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

func GeneratePurchaseCode(n int) string {
	letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func (purchase *Purchase) Update() (*Purchase, error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()
	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": purchase.ID},
		bson.M{"$set": purchase},
		updateOptions,
	)
	if err != nil {
		return nil, err
	}

	if updateResult.MatchedCount > 0 {
		return purchase, nil
	}
	return nil, nil
}

func (purchase *Purchase) DeletePurchase(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	purchase.Deleted = true
	purchase.DeletedBy = userID
	purchase.DeletedAt = time.Now().Local()

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

func FindPurchaseByID(ID primitive.ObjectID) (purchase *Purchase, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = collection.FindOne(ctx,
		bson.M{"_id": ID, "deleted": bson.M{"$ne": true}}).
		Decode(&purchase)
	if err != nil {
		return nil, err
	}

	return purchase, err
}

func IsPurchaseExists(ID primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

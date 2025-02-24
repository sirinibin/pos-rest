package models

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

var BANK_PAYMENT_METHODS = []string{"debit_card", "credit_card", "bank_card", "bank_transfer", "cheque"}

type SearchCriterias struct {
	Page     int                    `bson:"page,omitempty" json:"page,omitempty"`
	Size     int                    `bson:"size,omitempty" json:"size,omitempty"`
	Select   map[string]interface{} `bson:"select,omitempty" json:"select,omitempty"`
	SearchBy map[string]interface{} `bson:"search_by,omitempty" json:"search_by,omitempty"`
	SortBy   map[string]interface{} `bson:"sort_by,omitempty" json:"sort_by,omitempty"`
}

func FormatFloat(num float64, prc int) string {
	var (
		zero, dot = "0", "."

		str = fmt.Sprintf("%."+strconv.Itoa(prc)+"f", num)
	)

	return strings.TrimRight(strings.TrimRight(str, zero), dot)
}

func TruncateToTwoDecimals(some float64) float64 {
	return float64(int(some*100)) / 100
}

func RoundToTwoDecimal(number float64) float64 {

	//numStr := fmt.Sprintf("%.2f", TruncateToTwoDecimals(number))
	numStr := fmt.Sprintf("%.2f", number)

	//numStr := strconv.FormatFloat(number, 'f', -1, 64)
	//numStr := FormatFloat(number, 2)
	numFloat, _ := strconv.ParseFloat(numStr, 32)
	return RoundFloat(numFloat, 2)
}

func RoundFloat(val float64, precision uint) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
	//return math.Floor(val*ratio) / ratio
}

func ToFixed(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(math.Round(num*output)) / output
}

func GenerateFileName(prefix, suffix string) string {
	randBytes := make([]byte, 16)
	rand.Read(randBytes)
	return prefix + hex.EncodeToString(randBytes) + suffix
}

func GenerateCode(n int) string {
	letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func IsStringBase64(content string) (bool, error) {
	return regexp.MatchString(`^([A-Za-z0-9+/]{4})*([A-Za-z0-9+/]{3}=|[A-Za-z0-9+/]{2}==)?$`, content)
}

func ConvertTimeZoneToUTC(timeZoneOffset float64, date time.Time) time.Time {
	hrs, mins := math.Modf(timeZoneOffset)
	mins = 60 * mins
	return date.Add(time.Hour*time.Duration(hrs) + time.Minute*time.Duration(mins))
}

func GetTotalCount(filter map[string]interface{}, collectionName string) (count int64, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection(collectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, filter)
}

func TrimLogicalOperatorPrefix(str string) string {
	if strings.HasPrefix(str, "<=") {
		str = strings.TrimPrefix(str, "<=")

	} else if strings.HasPrefix(str, "<") {
		str = strings.TrimPrefix(str, "<")

	} else if strings.HasPrefix(str, ">=") {
		str = strings.TrimPrefix(str, ">=")

	} else if strings.HasPrefix(str, ">") {
		str = strings.TrimPrefix(str, ">")
	}
	return str
}

func GetMongoLogicalOperator(str string) (operator string) {
	operator = ""
	if strings.HasPrefix(str, "<=") {
		operator = "$lte"
	} else if strings.HasPrefix(str, "<") {
		operator = "$lt"
	} else if strings.HasPrefix(str, ">=") {
		operator = "$gte"
	} else if strings.HasPrefix(str, ">") {
		operator = "$gt"
	}
	return operator
}

func ParseSelectString(selectStr string) (fields map[string]interface{}) {

	if selectStr == "" {
		return nil
	}

	fields = make(map[string]interface{})

	fieldArray := strings.Split(selectStr, ",")

	for _, field := range fieldArray {
		if strings.HasPrefix(field, "-") {
			fields[strings.TrimPrefix(field, "-")] = 0
		} else {
			fields[field] = 1
		}
	}

	return fields
}

func GetSortByFields(sortString string) (sortBy map[string]interface{}) {
	sortFieldWithOrder := strings.Fields(sortString)
	sortBy = map[string]interface{}{}

	if len(sortFieldWithOrder) == 2 {
		if sortFieldWithOrder[1] == "1" {
			sortBy[sortFieldWithOrder[0]] = 1 // Sort by Ascending order
		} else if sortFieldWithOrder[1] == "-1" {
			sortBy[sortFieldWithOrder[0]] = -1 // Sort by Descending order
		}
	} else if len(sortFieldWithOrder) == 1 {
		if strings.HasPrefix(sortFieldWithOrder[0], "-") {
			sortFieldWithOrder[0] = strings.TrimPrefix(sortFieldWithOrder[0], "-")
			sortBy[sortFieldWithOrder[0]] = -1 // Sort by Ascending order
		} else {
			sortBy[sortFieldWithOrder[0]] = 1 // Sort by Ascending order
		}

	}

	return sortBy
}

func ParseRelationalSelectString(selectFields interface{}, prefix string) (fields map[string]interface{}) {

	fields = make(map[string]interface{})

	fieldArray := []string{}

	_, ok := selectFields.(string)
	if ok {
		fieldArray = strings.Split(selectFields.(string), ",")
		for _, field := range fieldArray {
			if strings.HasPrefix(field, prefix+".") {
				splits := strings.Split(field, ".")
				if len(splits) > 1 {
					if strings.HasPrefix(splits[1], "-") {
						fields[strings.TrimPrefix(splits[1], "-")] = 0
					} else {
						fields[splits[1]] = 1
					}
				}

			}
		}
	}

	value, ok := selectFields.(map[string]interface{})
	if ok {
		for field, _ := range value {
			if strings.HasPrefix(field, prefix+".") {
				splits := strings.Split(field, ".")
				if len(splits) > 1 {
					if strings.HasPrefix(splits[1], "-") {
						fields[strings.TrimPrefix(splits[1], "-")] = 0
					} else {
						fields[splits[1]] = 1
					}
				}

			}
		}
	}

	return fields
}

func ClearSalesHistory() error {
	log.Print("Clearing Sales history")
	collection := db.Client().Database(db.GetPosDB()).Collection("product_sales_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}
	return nil
}

func ClearSalesReturnHistory() error {
	log.Print("Clearing Sales Return hsitory")
	ctx := context.Background()
	collection := db.Client().Database(db.GetPosDB()).Collection("product_sales_return_history")
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}

	return nil
}

func ClearPurchaseHistory() error {
	log.Print("Clearing Purchase hsitory")
	ctx := context.Background()
	collection := db.Client().Database(db.GetPosDB()).Collection("product_purchase_history")
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}

	return nil
}

func ClearPurchaseReturnHistory() error {
	log.Print("Clearing Purchase Return hsitory")
	ctx := context.Background()
	collection := db.Client().Database(db.GetPosDB()).Collection("product_purchase_return_history")
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}
	return nil
}

func ClearQuotationHistory() error {
	log.Print("Clearing Quotation history")
	ctx := context.Background()
	collection := db.Client().Database(db.GetPosDB()).Collection("product_quotation_history")
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}

	return nil
}

func ClearDeliveryNoteHistory() error {
	log.Print("Clearing Delivery Note history")
	ctx := context.Background()
	collection := db.Client().Database(db.GetPosDB()).Collection("product_delivery_note_history")
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}

	return nil
}

func ClearSalesReturnPayments() error {
	log.Print("Clearing Sales Return payments")
	ctx := context.Background()
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_return_payment")
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}

	return nil
}

func ClearPurchaseReturnPayments() error {
	log.Print("Clearing Purchase Return payments")
	ctx := context.Background()
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_return_payment")
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}

	return nil
}

func GetIntSearchElement(
	fieldName string,
	operator string,
	storeID *primitive.ObjectID,
	value int64,
) bson.M {
	element := bson.M{"$elemMatch": bson.M{}}

	if operator != "" {
		if storeID != nil && !storeID.IsZero() {
			element["$elemMatch"] = bson.M{
				fieldName: bson.M{
					operator: value,
				},
				"store_id": storeID,
			}
		} else {
			element["$elemMatch"] = bson.M{
				fieldName: bson.M{
					operator: value,
				},
			}
		}

	} else {
		if storeID != nil && !storeID.IsZero() {
			element["$elemMatch"] = bson.M{
				fieldName:  value,
				"store_id": storeID,
			}
		} else {
			element["$elemMatch"] = bson.M{
				fieldName: value,
			}
		}
	}

	return element
}

func GetFloatSearchElement(
	fieldName string,
	operator string,
	storeID *primitive.ObjectID,
	value float64,
) bson.M {
	element := bson.M{"$elemMatch": bson.M{}}

	if operator != "" {
		if storeID != nil && !storeID.IsZero() {
			element["$elemMatch"] = bson.M{
				fieldName: bson.M{
					operator: value,
				},
				"store_id": storeID,
			}
		} else {
			element["$elemMatch"] = bson.M{
				fieldName: bson.M{
					operator: value,
				},
			}
		}

	} else {
		if storeID != nil && !storeID.IsZero() {
			element["$elemMatch"] = bson.M{
				fieldName:  value,
				"store_id": storeID,
			}
		} else {
			element["$elemMatch"] = bson.M{
				fieldName: value,
			}
		}
	}

	return element
}

func (customer *Customer) IsStoreExistsInCustomer(storeID primitive.ObjectID) bool {
	for _, store := range customer.Stores {
		if store.StoreID.Hex() == storeID.Hex() {
			return true
		}
	}
	return false
}

func (vendor *Vendor) IsStoreExistsInVendor(storeID primitive.ObjectID) bool {
	for _, store := range vendor.Stores {
		if store.StoreID.Hex() == storeID.Hex() {
			return true
		}
	}
	return false
}

func PrettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	fmt.Println(string(s))
	return string(s)
}

func IsDateTimesEqual(time1 *time.Time, time2 *time.Time) bool {
	var timeValue1 time.Time
	timeValue1, _ = time.Parse("2006-01-02T15:04", time1.Format("2006-01-02T15:04"))

	var timeValue2 time.Time
	timeValue2, _ = time.Parse("2006-01-02T15:04", time2.Format("2006-01-02T15:04"))

	return timeValue1.Equal(timeValue2)
}

func IsAfter(time1 *time.Time, time2 *time.Time) bool {
	var timeValue1 time.Time
	timeValue1, _ = time.Parse("2006-01-02T15:04", time1.Format("2006-01-02T15:04"))

	var timeValue2 time.Time
	timeValue2, _ = time.Parse("2006-01-02T15:04", time2.Format("2006-01-02T15:04"))

	return timeValue1.After(timeValue2)
}

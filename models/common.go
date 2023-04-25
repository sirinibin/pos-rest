package models

import (
	"context"
	"encoding/hex"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"gopkg.in/mgo.v2/bson"
)

type SearchCriterias struct {
	Page     int                    `bson:"page,omitempty" json:"page,omitempty"`
	Size     int                    `bson:"size,omitempty" json:"size,omitempty"`
	Select   map[string]interface{} `bson:"select,omitempty" json:"select,omitempty"`
	SearchBy map[string]interface{} `bson:"search_by,omitempty" json:"search_by,omitempty"`
	SortBy   map[string]interface{} `bson:"sort_by,omitempty" json:"sort_by,omitempty"`
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
	return date.Add(time.Hour * time.Duration(timeZoneOffset))
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

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

	"github.com/shopspring/decimal"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

var BANK_PAYMENT_METHODS = []string{"debit_card", "credit_card", "bank_card", "bank_transfer", "bank_cheque"}

type SearchCriterias struct {
	Page     int                    `bson:"page,omitempty" json:"page,omitempty"`
	Size     int                    `bson:"size,omitempty" json:"size,omitempty"`
	Select   map[string]interface{} `bson:"select,omitempty" json:"select,omitempty"`
	SearchBy map[string]interface{} `bson:"search_by,omitempty" json:"search_by,omitempty"`
	SortBy   map[string]interface{} `bson:"sort_by,omitempty" json:"sort_by,omitempty"`
}

func RoundFloat(val float64, precision uint) float64 {
	return math.Round(val*100) / 100
	//return val
	/*
		ratio := math.Pow(10, float64(precision))
		return math.Round(val*ratio) / ratio
		//return math.Floor(val*ratio) / ratio
	*/
}

func ToFixed(num float64, precision int) float64 {
	return num
	/*
		output := math.Pow(10, float64(precision))
		return float64(math.Round(num*output)) / output
	*/
}

func RoundDecimal(val float64) float64 {
	d := decimal.NewFromFloat(val)
	result, _ := d.Round(2).Float64()
	return result
}

func RoundTo2Decimals(num float64) float64 {
	//return RoundDecimal(num)
	return math.Round(num*100) / 100
	//return math.Trunc((num+1e-9)*100) / 100
	/*strValue := fmt.Sprintf("%.2f", num)
	trimmedValue, _ := strconv.ParseFloat(strValue, 64)
	return trimmedValue*/
}

// Just trim to 2 decimal places
func ToFixed2(num float64, precision int) float64 {
	//return math.Trunc(num*100) / 100
	// Add a small epsilon (1e-9) to prevent precision loss
	return math.Trunc((num+1e-9)*100) / 100

	/*
		strValue := fmt.Sprintf("%.2f", num)
		trimmedValue, _ := strconv.ParseFloat(strValue, 64)
		return trimmedValue
	*/
	//return RoundFloat(num, uint(precision))
	/*
		output := math.Pow(10, float64(precision))
		return float64(math.Round(num*output)) / output
	*/
}

// Just trim to 2 decimal places
func RoundTo2(num float64, precision int) float64 {
	//return math.Trunc(num*100) / 100

	strValue := fmt.Sprintf("%.2f", num)
	trimmedValue, _ := strconv.ParseFloat(strValue, 64)
	return trimmedValue

	//return RoundFloat(num, uint(precision))
	/*
		output := math.Pow(10, float64(precision))
		return float64(math.Round(num*output)) / output
	*/
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

func (store *Store) GetTotalCount(filter map[string]interface{}, collectionName string) (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection(collectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, filter)
}

func GetTotalCount(filter map[string]interface{}, collectionName string) (count int64, err error) {
	collection := db.Client("").Database(db.GetPosDB()).Collection(collectionName)
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

func (store *Store) ClearSalesHistory() error {
	log.Print("Clearing Sales history")
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_sales_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}
	return nil
}

func (store *Store) ClearSalesReturnHistory() error {
	log.Print("Clearing Sales Return hsitory")
	ctx := context.Background()
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_sales_return_history")
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) ClearPurchaseHistory() error {
	log.Print("Clearing Purchase hsitory")
	ctx := context.Background()
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_purchase_history")
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) ClearPurchaseReturnHistory() error {
	log.Print("Clearing Purchase Return hsitory")
	ctx := context.Background()
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_purchase_return_history")
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}
	return nil
}

func (store *Store) ClearQuotationHistory() error {
	log.Print("Clearing Quotation history")
	ctx := context.Background()
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_quotation_history")
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) ClearDeliveryNoteHistory() error {
	log.Print("Clearing Delivery Note history")
	ctx := context.Background()
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_delivery_note_history")
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) ClearSalesReturnPayments() error {
	log.Print("Clearing Sales Return payments")
	ctx := context.Background()
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_return_payment")
	_, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) ClearPurchaseReturnPayments() error {
	log.Print("Clearing Purchase Return payments")
	ctx := context.Background()
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_return_payment")
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

func RemoveObjectID(slice []*primitive.ObjectID, idToRemove *primitive.ObjectID) []*primitive.ObjectID {
	result := make([]*primitive.ObjectID, 0, len(slice))
	for _, id := range slice {
		if id == nil || idToRemove == nil {
			continue
		}
		if *id != *idToRemove {
			result = append(result, id)
		}
	}
	return result
}

var TimezoneMap = map[string]string{
	"AF": "Asia/Kabul",
	"AL": "Europe/Tirane",
	"DZ": "Africa/Algiers",
	"AO": "Africa/Luanda",
	"AR": "America/Argentina/Buenos_Aires",
	"AM": "Asia/Yerevan",
	"AU": "Australia/Sydney",
	"AT": "Europe/Vienna",
	"AZ": "Asia/Baku",
	"BH": "Asia/Bahrain",
	"BD": "Asia/Dhaka",
	"BY": "Europe/Minsk",
	"BE": "Europe/Brussels",
	"BJ": "Africa/Porto-Novo",
	"BO": "America/La_Paz",
	"BA": "Europe/Sarajevo",
	"BW": "Africa/Gaborone",
	"BR": "America/Sao_Paulo",
	"BN": "Asia/Brunei",
	"BG": "Europe/Sofia",
	"BF": "Africa/Ouagadougou",
	"BI": "Africa/Bujumbura",
	"KH": "Asia/Phnom_Penh",
	"CM": "Africa/Douala",
	"CA": "America/Toronto",
	"CF": "Africa/Bangui",
	"TD": "Africa/Ndjamena",
	"CL": "America/Santiago",
	"CN": "Asia/Shanghai",
	"CO": "America/Bogota",
	"KM": "Indian/Comoro",
	"CD": "Africa/Kinshasa",
	"CG": "Africa/Brazzaville",
	"CR": "America/Costa_Rica",
	"HR": "Europe/Zagreb",
	"CU": "America/Havana",
	"CY": "Asia/Nicosia",
	"CZ": "Europe/Prague",
	"DK": "Europe/Copenhagen",
	"DJ": "Africa/Djibouti",
	"DO": "America/Santo_Domingo",
	"EC": "America/Guayaquil",
	"EG": "Africa/Cairo",
	"SV": "America/El_Salvador",
	"GQ": "Africa/Malabo",
	"ER": "Africa/Asmara",
	"EE": "Europe/Tallinn",
	"ET": "Africa/Addis_Ababa",
	"FI": "Europe/Helsinki",
	"FR": "Europe/Paris",
	"GA": "Africa/Libreville",
	"GM": "Africa/Banjul",
	"GE": "Asia/Tbilisi",
	"DE": "Europe/Berlin",
	"GH": "Africa/Accra",
	"GR": "Europe/Athens",
	"GT": "America/Guatemala",
	"GN": "Africa/Conakry",
	"GW": "Africa/Bissau",
	"GY": "America/Guyana",
	"HT": "America/Port-au-Prince",
	"HN": "America/Tegucigalpa",
	"HU": "Europe/Budapest",
	"IS": "Atlantic/Reykjavik",
	"IN": "Asia/Kolkata",
	"ID": "Asia/Jakarta",
	"IR": "Asia/Tehran",
	"IQ": "Asia/Baghdad",
	"IE": "Europe/Dublin",
	"IL": "Asia/Jerusalem",
	"IT": "Europe/Rome",
	"CI": "Africa/Abidjan",
	"JM": "America/Jamaica",
	"JP": "Asia/Tokyo",
	"JO": "Asia/Amman",
	"KZ": "Asia/Almaty",
	"KE": "Africa/Nairobi",
	"KR": "Asia/Seoul",
	"KW": "Asia/Kuwait",
	"KG": "Asia/Bishkek",
	"LA": "Asia/Vientiane",
	"LV": "Europe/Riga",
	"LB": "Asia/Beirut",
	"LS": "Africa/Maseru",
	"LR": "Africa/Monrovia",
	"LY": "Africa/Tripoli",
	"LT": "Europe/Vilnius",
	"LU": "Europe/Luxembourg",
	"MG": "Indian/Antananarivo",
	"MW": "Africa/Blantyre",
	"MY": "Asia/Kuala_Lumpur",
	"MV": "Indian/Maldives",
	"ML": "Africa/Bamako",
	"MT": "Europe/Malta",
	"MR": "Africa/Nouakchott",
	"MU": "Indian/Mauritius",
	"MX": "America/Mexico_City",
	"MD": "Europe/Chisinau",
	"MN": "Asia/Ulaanbaatar",
	"ME": "Europe/Podgorica",
	"MA": "Africa/Casablanca",
	"MZ": "Africa/Maputo",
	"MM": "Asia/Yangon",
	"NA": "Africa/Windhoek",
	"NP": "Asia/Kathmandu",
	"NL": "Europe/Amsterdam",
	"NZ": "Pacific/Auckland",
	"NI": "America/Managua",
	"NE": "Africa/Niamey",
	"NG": "Africa/Lagos",
	"NO": "Europe/Oslo",
	"OM": "Asia/Muscat",
	"PK": "Asia/Karachi",
	"PA": "America/Panama",
	"PY": "America/Asuncion",
	"PE": "America/Lima",
	"PH": "Asia/Manila",
	"PL": "Europe/Warsaw",
	"PT": "Europe/Lisbon",
	"QA": "Asia/Qatar",
	"RO": "Europe/Bucharest",
	"RU": "Europe/Moscow",
	"RW": "Africa/Kigali",
	"SA": "Asia/Riyadh",
	"SN": "Africa/Dakar",
	"RS": "Europe/Belgrade",
	"SG": "Asia/Singapore",
	"SK": "Europe/Bratislava",
	"SI": "Europe/Ljubljana",
	"ZA": "Africa/Johannesburg",
	"ES": "Europe/Madrid",
	"LK": "Asia/Colombo",
	"SE": "Europe/Stockholm",
	"CH": "Europe/Zurich",
	"SY": "Asia/Damascus",
	"TW": "Asia/Taipei",
	"TZ": "Africa/Dar_es_Salaam",
	"TH": "Asia/Bangkok",
	"TN": "Africa/Tunis",
	"TR": "Europe/Istanbul",
	"UG": "Africa/Kampala",
	"UA": "Europe/Kyiv",
	"AE": "Asia/Dubai",
	"GB": "Europe/London",
	"US": "America/New_York",
	"UY": "America/Montevideo",
	"UZ": "Asia/Tashkent",
	"VE": "America/Caracas",
	"VN": "Asia/Ho_Chi_Minh",
	"YE": "Asia/Aden",
	"ZM": "Africa/Lusaka",
	"ZW": "Africa/Harare",
}

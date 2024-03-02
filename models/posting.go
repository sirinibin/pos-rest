package models

import (
	"context"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// Post : Post structure
type Post struct {
	Date          *time.Time         `bson:"date,omitempty" json:"date,omitempty"`
	AccountID     primitive.ObjectID `json:"account_id,omitempty" bson:"account_id,omitempty"`
	AccountName   string             `json:"account_name,omitempty" bson:"account_name,omitempty"`
	AccountNumber string             `bson:"account_number,omitempty" json:"account_number,omitempty"`
	DebitOrCredit string             `json:"debit_or_credit,omitempty" bson:"debit_or_credit,omitempty"`
	Debit         float64            `bson:"debit,omitempty" json:"debit,omitempty"`
	Credit        float64            `bson:"credit,omitempty" json:"credit,omitempty"`
	CreatedAt     *time.Time         `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time         `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// Account : Account structure
type Posting struct {
	ID             primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date           *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	StoreID        *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	AccountID      primitive.ObjectID  `json:"account_id,omitempty" bson:"account_id,omitempty"`
	AccountName    string              `json:"account_name,omitempty" bson:"account_name,omitempty"`
	AccountNumber  string              `bson:"account_number,omitempty" json:"account_number,omitempty"`
	ReferenceID    primitive.ObjectID  `json:"reference_id,omitempty" bson:"reference_id,omitempty"`
	ReferenceModel string              `bson:"reference_model,omitempty" json:"reference_model,omitempty"`
	ReferenceCode  string              `bson:"reference_code,omitempty" json:"reference_code,omitempty"`
	Posts          []Post              `json:"posts,omitempty" bson:"posts,omitempty"`
	DebitTotal     float64             `bson:"debit_total" json:"debit_total"`
	CreditTotal    float64             `bson:"credit_total" json:"credit_total"`
	CreatedAt      *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt      *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

type PostingListStats struct {
	ID                    *primitive.ObjectID `json:"id" bson:"_id"`
	DebitTotal            float64             `json:"debit_total" bson:"debit_total"`
	CreditTotal           float64             `json:"credit_total" bson:"credit_total"`
	DebitTotalBoughtDown  float64             `json:"debit_total_bought_down" bson:"debit_total_bought_down"`
	CreditTotalBoughtDown float64             `json:"credit_total_bought_down" bson:"credit_total_bought_down"`
}

func GetPostingListStats(filter map[string]interface{}, startDate time.Time, endDate time.Time) (stats PostingListStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("posting")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	debitTotalCondition := bson.M{}
	creditTotalCondition := bson.M{}

	if !startDate.IsZero() && !endDate.IsZero() {
		debitTotalCondition = bson.M{"$sum": bson.M{"$sum": bson.M{
			"$map": bson.M{
				"input": "$posts",
				"as":    "post",
				"in": bson.M{
					"$cond": []interface{}{
						bson.M{"$and": []interface{}{
							bson.M{"$gte": []interface{}{"$$post.date", startDate}},
							bson.M{"$lte": []interface{}{"$$post.date", endDate}},
							bson.M{"$gt": []interface{}{"$$post.debit", 0}},
						}},
						"$$post.debit",
						0,
					},
				},
			},
		}}}

		creditTotalCondition = bson.M{"$sum": bson.M{"$sum": bson.M{
			"$map": bson.M{
				"input": "$posts",
				"as":    "post",
				"in": bson.M{
					"$cond": []interface{}{
						bson.M{"$and": []interface{}{
							bson.M{"$gte": []interface{}{"$$post.date", startDate}},
							bson.M{"$lte": []interface{}{"$$post.date", endDate}},
							bson.M{"$gt": []interface{}{"$$post.credit", 0}},
						}},
						"$$post.credit",
						0,
					},
				},
			},
		}}}
	} else if !startDate.IsZero() {
		debitTotalCondition = bson.M{"$sum": bson.M{"$sum": bson.M{
			"$map": bson.M{
				"input": "$posts",
				"as":    "post",
				"in": bson.M{
					"$cond": []interface{}{
						bson.M{"$and": []interface{}{
							bson.M{"$gte": []interface{}{"$$post.date", startDate}},
							bson.M{"$gt": []interface{}{"$$post.debit", 0}},
						}},
						"$$post.debit",
						0,
					},
				},
			},
		}}}

		creditTotalCondition = bson.M{"$sum": bson.M{"$sum": bson.M{
			"$map": bson.M{
				"input": "$posts",
				"as":    "post",
				"in": bson.M{
					"$cond": []interface{}{
						bson.M{"$and": []interface{}{
							bson.M{"$gte": []interface{}{"$$post.date", startDate}},
							bson.M{"$gt": []interface{}{"$$post.credit", 0}},
						}},
						"$$post.credit",
						0,
					},
				},
			},
		}}}

	} else if !endDate.IsZero() {
		debitTotalCondition = bson.M{"$sum": bson.M{"$sum": bson.M{
			"$map": bson.M{
				"input": "$posts",
				"as":    "post",
				"in": bson.M{
					"$cond": []interface{}{
						bson.M{"$and": []interface{}{
							bson.M{"$lte": []interface{}{"$$post.date", endDate}},
							bson.M{"$gt": []interface{}{"$$post.debit", 0}},
						}},
						"$$post.debit",
						0,
					},
				},
			},
		}}}

		creditTotalCondition = bson.M{"$sum": bson.M{"$sum": bson.M{
			"$map": bson.M{
				"input": "$posts",
				"as":    "post",
				"in": bson.M{
					"$cond": []interface{}{
						bson.M{"$and": []interface{}{
							bson.M{"$lte": []interface{}{"$$post.date", endDate}},
							bson.M{"$gt": []interface{}{"$$post.credit", 0}},
						}},
						"$$post.credit",
						0,
					},
				},
			},
		}}}

	}

	if startDate.IsZero() && endDate.IsZero() {
		debitTotalCondition = bson.M{"$sum": "$debit_total"}
		creditTotalCondition = bson.M{"$sum": "$credit_total"}
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":          nil,
				"debit_total":  debitTotalCondition,
				"credit_total": creditTotalCondition,
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
		stats.DebitTotal = math.Ceil(stats.DebitTotal*100) / 100
		stats.CreditTotal = math.Ceil(stats.CreditTotal*100) / 100
	}

	if startDate.IsZero() {
		return stats, nil
	}

	delete(filter, "posts.date")

	pipeline = []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id": nil,
				"debit_total_bought_down": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$posts",
						"as":    "post",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$lt": []interface{}{"$$post.date", startDate}},
									bson.M{"$gt": []interface{}{"$$post.debit", 0}},
								}},
								"$$post.debit",
								0,
							},
						},
					},
				}}},
				"credit_total_bought_down": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$posts",
						"as":    "post",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$lt": []interface{}{"$$post.date", startDate}},
									bson.M{"$gt": []interface{}{"$$post.credit", 0}},
								}},
								"$$post.credit",
								0,
							},
						},
					},
				}}},

				/*
					"debit_total_bought_down": bson.M{"$sum": bson.M{"$cond": []interface{}{
						bson.M{"$lt": []interface{}{"$posts.date", startDate}},
						"$posts.debit",
						0,
					}}},
					"credit_total_bought_down": bson.M{"$sum": bson.M{"$cond": []interface{}{
						bson.M{"$lt": []interface{}{"$posts.date", startDate}},
						"$posts.credit",
						0,
					}}},
				*/
			},
		},
	}

	cur, err = collection.Aggregate(ctx, pipeline)
	if err != nil {
		return stats, err
	}

	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return stats, err
		}
		stats.DebitTotalBoughtDown = math.Ceil(stats.DebitTotalBoughtDown*100) / 100
		stats.CreditTotalBoughtDown = math.Ceil(stats.CreditTotalBoughtDown*100) / 100
	}

	return stats, nil
}

func SearchPosting(w http.ResponseWriter, r *http.Request) (
	models []Posting,
	criterias SearchCriterias,
	err error,
	startDate time.Time,
	endDate time.Time,
) {
	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	var storeID primitive.ObjectID
	keys, ok := r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err, startDate, endDate
		}
		criterias.SearchBy["store_id"] = storeID
	}

	var accountID primitive.ObjectID
	keys, ok = r.URL.Query()["search[account_id]"]
	if ok && len(keys[0]) >= 1 {
		accountID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err, startDate, endDate
		}
		criterias.SearchBy["account_id"] = accountID
	}

	debitObjectIds := []primitive.ObjectID{}
	creditObjectIds := []primitive.ObjectID{}

	keys, ok = r.URL.Query()["search[debit_account_id]"]
	if ok && len(keys[0]) >= 1 {

		accountIds := strings.Split(keys[0], ",")

		//objecIds := []primitive.ObjectID{}

		for _, id := range accountIds {
			accountID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return models, criterias, err, startDate, endDate
			}
			debitObjectIds = append(debitObjectIds, accountID)
		}

		/*
			if len(objecIds) > 0 {
				criterias.SearchBy["posts"] = bson.M{"$elemMatch": bson.M{
					"account_id":      bson.M{"$in": objecIds},
					"debit_or_credit": bson.M{"$eq": "debit"},
				}}
			}
		*/
	}

	keys, ok = r.URL.Query()["search[credit_account_id]"]
	if ok && len(keys[0]) >= 1 {

		accountIds := strings.Split(keys[0], ",")

		//objecIds := []primitive.ObjectID{}

		for _, id := range accountIds {
			accountID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return models, criterias, err, startDate, endDate
			}
			creditObjectIds = append(creditObjectIds, accountID)
		}

		/*
			if len(objecIds) > 0 {
				//criterias.SearchBy["posts.account_id"] = bson.M{"$in": objecIds}
				criterias.SearchBy["posts"] = bson.M{"$elemMatch": bson.M{
					"account_id":      bson.M{"$in": objecIds},
					"debit_or_credit": bson.M{"$eq": "credit"},
				}}

			}
		*/
	}

	if len(debitObjectIds) > 0 && len(creditObjectIds) > 0 {
		criterias.SearchBy["$and"] = []bson.M{
			{"posts": bson.M{"$elemMatch": bson.M{
				"account_id":      bson.M{"$in": debitObjectIds},
				"debit_or_credit": bson.M{"$eq": "debit"},
			}}},
			{"posts": bson.M{"$elemMatch": bson.M{
				"account_id":      bson.M{"$in": creditObjectIds},
				"debit_or_credit": bson.M{"$eq": "credit"},
			}}},
		}
	} else if len(debitObjectIds) > 0 {
		criterias.SearchBy["posts"] = bson.M{"$elemMatch": bson.M{
			"account_id":      bson.M{"$in": debitObjectIds},
			"debit_or_credit": bson.M{"$eq": "debit"},
		}}
	} else if len(creditObjectIds) > 0 {
		criterias.SearchBy["posts"] = bson.M{"$elemMatch": bson.M{
			"account_id":      bson.M{"$in": creditObjectIds},
			"debit_or_credit": bson.M{"$eq": "credit"},
		}}
	}

	keys, ok = r.URL.Query()["search[account_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["account_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[account_number]"]
	if ok && len(keys[0]) >= 1 {
		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return models, criterias, err, startDate, endDate
		}
		criterias.SearchBy["account_number"] = value
	}

	keys, ok = r.URL.Query()["search[reference_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err, startDate, endDate
		}
		criterias.SearchBy["reference_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[reference_model]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["reference_model"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[reference_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["reference_code"] = keys[0]
	}

	keys, ok = r.URL.Query()["sort"]
	if ok && len(keys[0]) >= 1 {
		keys[0] = strings.Replace(keys[0], "stores.", "stores."+storeID.Hex()+".", -1)
		criterias.SortBy = GetSortByFields(keys[0])
	}

	timeZoneOffset := 0.0
	keys, ok = r.URL.Query()["search[timezone_offset]"]
	if ok && len(keys[0]) >= 1 {
		if s, err := strconv.ParseFloat(keys[0], 64); err == nil {
			timeZoneOffset = s
		}
	}

	//var startDate time.Time
	//var endDate time.Time

	keys, ok = r.URL.Query()["search[date_str]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err, startDate, endDate
		}

		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}

		endDate = startDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
		//criterias.SearchBy["posts.date"] = bson.M{"$gte": startDate, "$lte": endDate}
		/*criterias.SearchBy["posts"] = bson.M{"$elemMatch": bson.M{
			"date": bson.M{"$gte": startDate, "$lte": endDate},
		}}*/
		log.Print("Okk1")
	}

	keys, ok = r.URL.Query()["search[from_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err, startDate, endDate
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
			return models, criterias, err, startDate, endDate
		}

		if timeZoneOffset != 0 {
			endDate = ConvertTimeZoneToUTC(timeZoneOffset, endDate)
		}

		endDate = endDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
	}

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		//Relational Select Fields
	}

	if !startDate.IsZero() && !endDate.IsZero() {
		criterias.SearchBy["posts.date"] = bson.M{"$gte": startDate, "$lte": endDate}
		/*
			criterias.SearchBy["posts"] = bson.M{"$elemMatch": bson.M{
				"date": bson.M{"$gte": startDate, "$lte": endDate},
			}}
		*/

		criterias.Select["posts"] = bson.M{"$filter": bson.M{
			"input": "$posts",
			"as":    "post",
			"cond": bson.M{
				"$and": []interface{}{
					bson.M{"$gte": []interface{}{"$$post.date", startDate}},
					bson.M{"$lte": []interface{}{"$$post.date", endDate}},
				},
			},
		}}

		/*
			criterias.SearchBy["posts"] = bson.M{"$elemMatch": bson.M{
				"date": bson.M{"$gte": startDate, "$lte": endDate},
			}}
			8/

			criterias.SearchBy["posts"] = bson.M{"$filter": bson.M{
				"input": "$posts",
				"as":    "post",
				"cond": bson.M{
					"$and": []interface{}{
						bson.M{"$gte": []interface{}{"$$post.date", startDate}},
						bson.M{"$lte": []interface{}{"$$post.date", endDate}},
					},
				},
			}}

			criterias.Select["posts"] = bson.M{"$filter": bson.M{
				"input": "$posts",
				"as":    "post",
				"cond": bson.M{
					"$and": []interface{}{
						bson.M{"$gte": []interface{}{"$$post.date", startDate}},
						bson.M{"$lte": []interface{}{"$$post.date", endDate}},
					},
				},
			}}
			/*
				criterias.Select["posts"] = bson.M{"$filter": bson.M{
					"input": "$posts",
					"as":    "post",
					"cond": bson.M{
						"$and": []interface{}{
							bson.M{ "$gt": []interface{}{ "$$item.a", 0  },
							bson.M{ "$gt": []interface{}{ "$$item.a", 0  },
					},
				},},
		*/
		/*
			"cond": bson.M{
				"$gte": []interface{}{"$$post.date", startDate},
				"$lte": []interface{}{"$$post.date", endDate},
			},*/
		/*
			"cond": bson.M{"$elemMatch": bson.M{
				"$$post.date": bson.M{"$gte": startDate, "$lte": endDate},
			}},*/

		//log.Print("Okk2")
	} else if !startDate.IsZero() {
		criterias.SearchBy["posts.date"] = bson.M{"$gte": startDate}
		criterias.Select["posts"] = bson.M{"$filter": bson.M{
			"input": "$posts",
			"as":    "post",
			"cond":  bson.M{"$gte": []interface{}{"$$post.date", startDate}},
		}}
	} else if !endDate.IsZero() {
		criterias.SearchBy["posts.date"] = bson.M{"$lte": endDate}
		criterias.Select["posts"] = bson.M{"$filter": bson.M{
			"input": "$posts",
			"as":    "post",
			"cond":  bson.M{"$lte": []interface{}{"$$post.date", endDate}},
		}}
	}

	var createdAtStartDate time.Time
	var createdAtEndDate time.Time

	keys, ok = r.URL.Query()["search[created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err, startDate, endDate
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
			return models, criterias, err, startDate, endDate
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
			return models, criterias, err, startDate, endDate
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

	keys, ok = r.URL.Query()["search[debit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err, startDate, endDate
		}

		if operator != "" {
			criterias.SearchBy["posts.debit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["posts.debit"] = value
		}
	}

	keys, ok = r.URL.Query()["search[credit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err, startDate, endDate
		}

		if operator != "" {
			criterias.SearchBy["posts.credit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["posts.credit"] = value
		}
	}

	keys, ok = r.URL.Query()["limit"]
	if ok && len(keys[0]) >= 1 {
		criterias.Size, _ = strconv.Atoi(keys[0])
	}
	keys, ok = r.URL.Query()["page"]
	if ok && len(keys[0]) >= 1 {
		criterias.Page, _ = strconv.Atoi(keys[0])
	}

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.Client().Database(db.GetPosDB()).Collection("posting")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

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
		return models, criterias, errors.New("Error fetching Customers:" + err.Error()), startDate, endDate
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error()), startDate, endDate
		}
		model := Posting{}
		err = cur.Decode(&model)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error()), startDate, endDate
		}

		models = append(models, model)
	} //end for loop

	return models, criterias, nil, startDate, endDate

}

func RemovePostingsByReferenceID(referenceID primitive.ObjectID) error {
	ctx := context.Background()
	collection := db.Client().Database(db.GetPosDB()).Collection("posting")
	_, err := collection.DeleteMany(ctx, bson.M{
		"reference_id": referenceID,
	})
	if err != nil {
		return err
	}

	return nil
}

func (ledger *Ledger) GetRelatedAccounts() (map[string]Account, error) {
	accounts := map[string]Account{}
	for _, journal := range ledger.Journals {
		if journal.AccountID.IsZero() {
			continue
		}

		account, err := FindAccountByID(journal.AccountID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}

		if account != nil && !account.ID.IsZero() {
			accounts[account.ID.Hex()] = *account
		}
	}
	return accounts, nil
}

func IsAccountExistsInPosts(accountID primitive.ObjectID, posts []Post) bool {
	for _, post := range posts {
		if post.AccountID.Hex() == accountID.Hex() {
			return true
		}
	}
	return false
}

func IsAccountExistsInPostings(accountID primitive.ObjectID, postings []Posting) bool {
	for _, posting := range postings {
		if posting.AccountID.Hex() == accountID.Hex() {
			return true
		}
	}
	return false
}

func IsAccountExistsInGroup(accountNumber string, groupAccountNumbers []string) bool {
	for _, groupAccountNumber := range groupAccountNumbers {
		if groupAccountNumber == accountNumber {
			return true
		}
	}
	return false
}

func (ledger *Ledger) CreatePostings() (postings []Posting, err error) {
	now := time.Now()

	for k1, journal := range ledger.Journals {
		if journal.AccountID.IsZero() {
			log.Print("Account not set in ledger: " + ledger.ID.Hex())
			continue
		}

		account, err := FindAccountByID(journal.AccountID, bson.M{})
		if err != nil {
			return nil, errors.New("error finding account: " + journal.AccountName)
		}

		if IsAccountExistsInPostings(journal.AccountID, postings) {
			continue
		}

		posts := []Post{} // Reset posts
		debitTotal := float64(0.00)
		creditTotal := float64(0.00)
		for k2, journal2 := range ledger.Journals {
			if k2 == k1 || account.Number == journal2.AccountNumber {
				continue
			}

			if !IsAccountExistsInGroup(account.Number, journal2.GroupAccounts) {
				continue
			}

			if account.Number == journal2.AccountNumber && journal.DebitOrCredit != journal2.DebitOrCredit {
				//continue
			}
			//if journal2.AccountID.Hex() == account.ID.Hex() || !journal.Date.Equal(*journal2.Date) {
			/*if !journal.Date.Equal(*journal2.Date) || IsAccountExistsInPosts(journal.AccountID, posts) {
				continue
			}*/

			/*
				if !journal.Date.Equal(*journal2.Date) {
					continue
				}
			*/

			if journal.DebitOrCredit == "debit" && journal2.DebitOrCredit == "credit" {
				amount := journal2.Credit

				/*
					amount := float64(0.00)
					if journal.Debit < journal2.Credit {
						amount = journal.Debit
					} else {
						amount = journal2.Credit
					}
				*/

				posts = append(posts, Post{
					Date:          journal2.Date,
					AccountID:     journal2.AccountID,
					AccountName:   journal2.AccountName,
					AccountNumber: journal2.AccountNumber,
					DebitOrCredit: "debit",
					Debit:         amount,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				})
				debitTotal += amount
			} else if journal.DebitOrCredit == "credit" && journal2.DebitOrCredit == "debit" {
				amount := journal2.Debit

				/*
					amount := float64(0.00)
					if journal.Credit < journal2.Debit {
						amount = journal.Credit
					} else {
						amount = journal2.Debit
					}
				*/

				posts = append(posts, Post{
					Date:          journal2.Date,
					AccountID:     journal2.AccountID,
					AccountName:   journal2.AccountName,
					AccountNumber: journal2.AccountNumber,
					DebitOrCredit: "credit",
					Credit:        amount,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				})
				creditTotal += amount
			} else if journal.DebitOrCredit == "debit" && journal2.DebitOrCredit == "debit" {
				amount := journal2.Debit

				/*
					amount := float64(0.00)
					if journal.Debit < journal2.Debit {
						amount = journal.Debit
					} else {
						amount = journal2.Debit
					}
				*/

				posts = append(posts, Post{
					Date:          journal2.Date,
					AccountID:     journal2.AccountID,
					AccountName:   journal2.AccountName,
					AccountNumber: journal2.AccountNumber,
					DebitOrCredit: "credit",
					Credit:        amount,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				})
				creditTotal += amount
			} else if journal.DebitOrCredit == "credit" && journal2.DebitOrCredit == "credit" {
				amount := journal2.Credit
				/*
					amount := float64(0.00)
					if journal.Credit < journal2.Credit {
						amount = journal.Credit
					} else {
						amount = journal2.Credit
					}
				*/

				posts = append(posts, Post{
					Date:          journal2.Date,
					AccountID:     journal2.AccountID,
					AccountName:   journal2.AccountName,
					AccountNumber: journal2.AccountNumber,
					DebitOrCredit: "debit",
					Debit:         amount,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				})
				debitTotal += amount
			}
		}

		posting := &Posting{
			Date:           journal.Date,
			StoreID:        ledger.StoreID,
			AccountID:      account.ID,
			AccountName:    account.Name,
			AccountNumber:  account.Number,
			ReferenceID:    ledger.ReferenceID,
			ReferenceModel: ledger.ReferenceModel,
			ReferenceCode:  ledger.ReferenceCode,
			Posts:          posts,
			DebitTotal:     debitTotal,
			CreditTotal:    creditTotal,
			CreatedAt:      &now,
			UpdatedAt:      &now,
		}

		err = posting.Insert()
		if err != nil {
			return nil, errors.New("error inserting post: " + err.Error())
		}

		postings = append(postings, *posting)

		err = account.CalculateBalance()
		if err != nil {
			return nil, errors.New("error calculating account balance: " + err.Error())
		}

	} // end for

	return postings, nil
}

func (posting *Posting) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("posting")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	posting.ID = primitive.NewObjectID()

	_, err := collection.InsertOne(ctx, &posting)
	if err != nil {
		return err
	}

	return nil
}

func (posting *Posting) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("posting")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": posting.ID},
		bson.M{"$set": posting},
		updateOptions,
	)
	if err != nil {
		return err
	}

	if updateResult.MatchedCount > 0 {
		return nil
	}

	return nil
}

func FindPostingByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (posting *Posting, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("posting")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&posting)
	if err != nil {
		return nil, err
	}

	return posting, err
}

func IsPostingExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("posting")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

func ProcessPostings() error {
	log.Print("Processing postings")
	//postingCount := 0

	//counts := map[string]int{}

	collection := db.Client().Database(db.GetPosDB()).Collection("posting")
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

	//productCount := 1
	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		posting := Posting{}
		err = cur.Decode(&posting)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		/*
			if posting.StoreID.Hex() != "61cf42e580e87d715a4cb9e6" {
				continue
			}

			if posting.ReferenceModel != "sales" {
				continue
			}

			if posting.AccountNumber != "1001" {
				continue
			}

			postingCount++

			if _, ok := counts[posting.ReferenceCode+"_"+posting.StoreID.Hex()]; ok {
				counts[posting.ReferenceCode+"_"+posting.StoreID.Hex()]++
				log.Print("Increasing")
			} else {
				counts[posting.ReferenceCode+"_"+posting.StoreID.Hex()] = 1
			}

			if counts[posting.ReferenceCode+"_"+posting.StoreID.Hex()] > 1 {
				log.Print("more than one found")
				log.Print(counts[posting.ReferenceCode+"_"+posting.StoreID.Hex()])
				log.Print(posting.ReferenceCode)
				log.Print("Store_id" + posting.StoreID.Hex())
			}

			order, _ := FindOrderByID(&posting.ReferenceID, bson.M{})
			cashMethodFound := false
			for _, method := range order.PaymentMethods {
				if method == "cash" {
					cashMethodFound = true
				}
			}

			if !cashMethodFound {
				log.Print("Cash method not found")
				log.Print(order.Code)

			}
		*/

		/*
			if _, ok := counts[posting.ReferenceCode]; ok {

			}
			*
			/*
				if order.StoreID.Hex() != "61cf42e580e87d715a4cb9e6" {
					continue
				}

				for _, method := range order.PaymentMethods {
					if method == "cash" {
						cashOrdersCount++
						ledgerCount, _ := GetTotalCount(bson.M{"reference_id": order.ID}, "ledger")
						if ledgerCount > 1 {
							log.Print("More than 1")
							log.Print(ledgerCount)
							log.Print(order.Code)
						}

						if ledgerCount == 0 {
							log.Print("No ledger found")
							log.Print(ledgerCount)
							log.Print(order.Code)
						}

						if ledgerCount > 0 {
							ledgersCount++
						}

						postingCount, _ := GetTotalCount(bson.M{"reference_id": order.ID, "account_number": "1001"}, "posting")
						if postingCount > 0 {
							postingsCount++
						}

						if postingCount == 0 {
							log.Print("No posting found")
							log.Print(postingCount)
							log.Print(order.Code)
						}

						if postingCount > 1 {
							log.Print("More than 1")
							log.Print(postingCount)
							log.Print(order.Code)
						}
					}
				}
		*/

	}

	//log.Print("postings count: ")
	//log.Print(postingCount)
	/*
		log.Print("Ledger count: ")
		log.Print(ledgersCount)
		log.Print("Cash orders: ")
		log.Print(cashOrdersCount)

		log.Print("postings count: ")
		log.Print(postingsCount)
	*/

	log.Print("DONE!")
	return nil
}

package controller

import (
	"encoding/json"
	"math"
	"net/http"

	"github.com/sirinibin/pos-rest/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

// ListLedger : handler for GET /ledger
func ListPostings(w http.ResponseWriter, r *http.Request) {
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

	postings := []models.Posting{}

	postings, criterias, err, startDate, endDate := models.SearchPosting(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find postings:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var postingListStats models.PostingListStats
	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] == "1" {
			response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "posting")
			if err != nil {
				response.Status = false
				response.Errors["total_count"] = "Unable to find total count of accounts:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}

			postingListStats, err = models.GetPostingListStats(criterias.SearchBy, startDate, endDate)
			if err != nil {
				response.Status = false
				response.Errors["posting_list_stats"] = "Unable to find posting list stats:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	var accountID primitive.ObjectID
	var account *models.Account
	keys, ok = r.URL.Query()["search[account_id]"]
	if ok && len(keys[0]) >= 1 {
		accountID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			response.Status = false
			response.Errors["account_id"] = "Invalid account id:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}

		account, err = models.FindAccountByID(accountID, bson.M{})
		if err != nil {
			response.Status = false
			response.Errors["account_id"] = "Invalid account id:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	response.Meta = map[string]interface{}{}

	debitTotal := postingListStats.DebitTotal
	creditTotal := postingListStats.CreditTotal

	if account != nil {
		debitTotalBoughtDown := postingListStats.DebitTotalBoughtDown
		creditTotalBoughtDown := postingListStats.CreditTotalBoughtDown
		balanceBoughtDown := 0.0
		/*
			if account.DebitTotal > postingListStats.DebitTotal {
				debitTotalBoughtDown = account.DebitTotal - postingListStats.DebitTotal
			}

			if account.CreditTotal > postingListStats.CreditTotal {
				creditTotalBoughtDown = account.CreditTotal - postingListStats.CreditTotal
			}
		*/

		if debitTotalBoughtDown > creditTotalBoughtDown {
			balanceBoughtDown = math.Round((debitTotalBoughtDown-creditTotalBoughtDown)*100) / 100
		} else if creditTotalBoughtDown > debitTotalBoughtDown {
			balanceBoughtDown = math.Round((creditTotalBoughtDown-debitTotalBoughtDown)*100) / 100
		}

		if account.ReferenceModel != nil && *account.ReferenceModel == "customer" {
			if creditTotalBoughtDown > debitTotalBoughtDown {
				account.Type = "liability" //creditor
			} else if creditTotalBoughtDown < debitTotalBoughtDown {
				account.Type = "asset" //debtor
			}
		}

		balanceBoughtDownType := ""
		if account.Type == "divident" || account.Type == "expense" || account.Type == "asset" {
			balanceBoughtDownType = "debit"
			debitTotal += balanceBoughtDown
		} else if account.Type == "liability" || account.Type == "equity" || account.Type == "revenue" {
			balanceBoughtDownType = "credit"
			creditTotal += balanceBoughtDown
		}

		response.Meta[balanceBoughtDownType+"_balance_bought_down"] = balanceBoughtDown
	}

	response.Meta["debit_total"] = math.Round(debitTotal*100) / 100
	response.Meta["credit_total"] = math.Round(creditTotal*100) / 100

	if debitTotal < creditTotal {
		response.Meta["debit_balance"] = math.Round((creditTotal-debitTotal)*100) / 100
	} else if postingListStats.CreditTotal < postingListStats.DebitTotal {
		response.Meta["credit_balance"] = math.Round((debitTotal-creditTotal)*100) / 100
	}

	response.Status = true
	response.Criterias = criterias
	/*
		response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "posting")
		if err != nil {
			response.Status = false
			response.Errors["total_count"] = "Unable to find total count of ledgers:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	*/

	if len(postings) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = postings
	}

	json.NewEncoder(w).Encode(response)

}

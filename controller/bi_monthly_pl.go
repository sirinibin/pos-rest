package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// BIMonthlyPL : GET /v1/bi/monthly-pl
// Returns monthly revenue and expense using the exact P&L Statement formula.
// Query params:
//   search[store_id] — required
//   months           — how many months back (default 12, max 60)
func BIMonthlyPL(w http.ResponseWriter, r *http.Request) {
	store, ok := biAuthAndStore(w, r)
	if !ok {
		return
	}

	months := 56
	if v := r.URL.Query().Get("months"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			months = n
		}
	}

	records, err := store.GetMonthlyPL(months)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": false,
			"errors": map[string]string{"monthly_pl": err.Error()},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": true,
		"result": records,
	})
}

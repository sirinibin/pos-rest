package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sirinibin/startpos/backend/models"
)

// GetAutoMobileDashboard handles GET /v1/automobile/dashboard.
func GetAutoMobileDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response := models.Response{Errors: map[string]string{}}

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
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

	fromDate, toDate := parseWorkshopDashboardDateRange(r, models.CountryTimezoneOffset(store.CountryCode))
	dashboard, err := store.GetAutoMobileDashboard(fromDate, toDate)
	if err != nil {
		response.Status = false
		response.Errors["dashboard"] = "Unable to compute dashboard:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = dashboard
	json.NewEncoder(w).Encode(response)
}

func parseWorkshopDashboardDateRange(r *http.Request, tzOffset float64) (*time.Time, *time.Time) {
	var fromDate, toDate *time.Time

	if raw := r.URL.Query().Get("from_date"); raw != "" {
		if localStart, err := time.Parse("2006-01-02", raw); err == nil {
			utcStart := models.ConvertTimeZoneToUTC(tzOffset, localStart)
			fromDate = &utcStart
		}
	}

	if raw := r.URL.Query().Get("to_date"); raw != "" {
		if localEnd, err := time.Parse("2006-01-02", raw); err == nil {
			nextDayUTC := models.ConvertTimeZoneToUTC(tzOffset, localEnd.AddDate(0, 0, 1))
			toDate = &nextDayUTC
		}
	}

	return fromDate, toDate
}

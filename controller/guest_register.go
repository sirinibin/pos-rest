package controller

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type GuestRegisterRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Mob      string `json:"mob"`
	Password string `json:"password"`

	StoreName          string `json:"store_name"`
	StoreNameInArabic  string `json:"store_name_in_arabic"`
	BusinessCategory   string `json:"business_category"`
	RegistrationNumber string `json:"registration_number"`
	VATNo              string `json:"vat_no"`
	Phone              string `json:"phone"`
	CountryCode        string `json:"country_code"`
	CountryName        string `json:"country_name"`
	ZatcaPhase         string `json:"zatca_phase"`

	NationalAddress models.NationalAddress `json:"national_address"`
}

// validateGuestRegisterRequest checks all required fields except email-exists
// (that check requires a DB call and is done separately in the handler).
// Returns a map of field → error message; empty means no errors.
func validateGuestRegisterRequest(req GuestRegisterRequest) map[string]string {
	errs := make(map[string]string)

	// Length caps — reject oversized payloads before any further processing.
	if len(req.Name) > 200 {
		errs["name"] = "Name is too long"
	} else if govalidator.IsNull(req.Name) {
		errs["name"] = "Name is required"
	}
	if len(req.Email) > 254 {
		errs["email"] = "Email is too long"
	} else if govalidator.IsNull(req.Email) {
		errs["email"] = "Email is required"
	} else if !govalidator.IsEmail(req.Email) {
		errs["email"] = "Invalid email address"
	}
	if len(req.Mob) > 20 {
		errs["mob"] = "Mobile number is too long"
	} else if govalidator.IsNull(req.Mob) {
		errs["mob"] = "Mobile number is required"
	}
	if len(req.Password) > 128 {
		errs["password"] = "Password is too long"
	} else if govalidator.IsNull(req.Password) {
		errs["password"] = "Password is required"
	} else if len(req.Password) < 6 {
		errs["password"] = "Password must be at least 6 characters"
	}
	if req.ZatcaPhase != "1" && req.ZatcaPhase != "2" {
		errs["zatca_phase"] = "ZATCA phase must be 1 or 2"
	}
	if govalidator.IsNull(req.BusinessCategory) {
		errs["business_category"] = "Business category is required"
	}
	// country rules (models/country_profile.go): an empty country is Saudi
	// Arabia; other GCC countries have an optional tax number and a simpler
	// address.
	cp := models.CountryProfileOrSaudi(req.CountryCode)
	if c := strings.TrimSpace(req.CountryCode); c != "" && models.CountryProfileFor(c) == nil {
		errs["country_code"] = "Choose a supported country"
	}
	// the CR / licence number is required in the GCC; India's PAN is optional
	if cp.CRRequired && govalidator.IsNull(req.RegistrationNumber) {
		errs["registration_number"] = "Registration number (CRN) is required"
	}
	if cp.TaxIDRequired {
		if govalidator.IsNull(req.VATNo) {
			errs["vat_no"] = "VAT number is required"
		} else if len(req.VATNo) != 15 {
			errs["vat_no"] = "VAT No. should be 15 digits"
		}
	} else if !govalidator.IsNull(req.VATNo) && !cp.ValidTaxID(req.VATNo) {
		errs["vat_no"] = cp.TaxIDLabelEn + ": " + cp.TaxIDHint
	}
	if cp.NationalAddress {
		if govalidator.IsNull(req.NationalAddress.BuildingNo) {
			errs["national_address_building_no"] = "Building number is required"
		}
		if govalidator.IsNull(req.NationalAddress.DistrictName) {
			errs["national_address_district_name"] = "District name is required"
		}
		if govalidator.IsNull(req.NationalAddress.ZipCode) {
			errs["national_address_zipcode"] = "Zip code is required"
		}
	}
	if govalidator.IsNull(req.NationalAddress.StreetName) {
		errs["national_address_street_name"] = "Street name is required"
	}
	if govalidator.IsNull(req.NationalAddress.CityName) {
		errs["national_address_city_name"] = "City name is required"
	}

	return errs
}

// buildGuestStore constructs a Store value from a validated GuestRegisterRequest.
// branchCode is a pre-generated short code (e.g. first 8 hex chars of a new ObjectID).
// now is injected so callers (and tests) can control the timestamp.
func buildGuestStore(req GuestRegisterRequest, branchCode string, now time.Time) *models.Store {
	storeName := strings.TrimSpace(req.StoreName)
	if storeName == "" {
		storeName = req.Name
	}
	storeNameAr := strings.TrimSpace(req.StoreNameInArabic)
	if storeNameAr == "" {
		storeNameAr = storeName
	}
	phone := strings.TrimSpace(req.Phone)
	if phone == "" {
		phone = req.Mob
	}
	countryCode := strings.ToUpper(strings.TrimSpace(req.CountryCode))
	if countryCode == "" {
		countryCode = "SA"
	}
	countryName := strings.TrimSpace(req.CountryName)
	if countryName == "" {
		countryName = models.CountryProfileOrSaudi(countryCode).NameEn
	}

	na := req.NationalAddress
	if na.StreetNameArabic == "" {
		na.StreetNameArabic = na.StreetName
	}
	if na.DistrictNameArabic == "" {
		na.DistrictNameArabic = na.DistrictName
	}
	if na.CityNameArabic == "" {
		na.CityNameArabic = na.CityName
	}

	sn := func(prefix string, padding int64, start int64) models.SerialNumber {
		return models.SerialNumber{Prefix: prefix, PaddingCount: padding, StartFromCount: start}
	}

	cp := models.CountryProfileOrSaudi(countryCode)
	zatcaPhase := req.ZatcaPhase
	zatcaEnv := "NonProduction"
	if !models.ZatcaApplies(countryCode) {
		// ZATCA is Saudi-only: other GCC stores stay on plain Phase 1
		zatcaPhase = "1"
	} else if zatcaPhase == "2" {
		zatcaEnv = "Production"
	}

	return &models.Store{
		Name:                       storeName,
		NameInArabic:               storeNameAr,
		Code:                       branchCode,
		BranchName:                 "Main Branch",
		RegistrationNumber:         req.RegistrationNumber,
		RegistrationNumberInArabic: req.RegistrationNumber,
		VATNo:                      req.VATNo,
		VATNoInArabic:              req.VATNo,
		VatPercent:                 cp.VatPercent,
		BusinessCategory:           req.BusinessCategory,
		Email:                      req.Email,
		Phone:                      phone,
		PhoneInArabic:              phone,
		CountryCode:                countryCode,
		CountryName:                countryName,
		NationalAddress:            na,

		SalesSerialNumber:                sn("S-INV", 3, 1),
		SalesReturnSerialNumber:          sn("SR-INV", 3, 1),
		PurchaseSerialNumber:             sn("P-INV", 3, 1),
		PurchaseReturnSerialNumber:       sn("PR-INV", 3, 1),
		PurchaseOrderSerialNumber:        sn("PO", 4, 1),
		PurchaseRequestSerialNumber:      sn("PR", 4, 1),
		QuotationSerialNumber:            sn("QTN", 3, 1),
		QuotationSalesReturnSerialNumber: sn("QTN-SR-INV", 3, 1),
		CustomerSerialNumber:             sn("CUST", 4, 1),
		VendorSerialNumber:               sn("VND", 4, 1),
		ExpenseSerialNumber:              sn("EXP", 4, 1),
		DeliveryNoteSerialNumber:         sn("DEL-NOTE", 6, 1),
		CustomerDepositSerialNumber:      sn("CUST-RCVBLE", 4, 1),
		CustomerWithdrawalSerialNumber:   sn("CUST-PAYBLE", 4, 1),
		CapitalDepositSerialNumber:       sn("CAP-DPST", 4, 1),
		DividentSerialNumber:             sn("CAP-DRWNG", 4, 1),
		StockTransferSerialNumber:        sn("ST-TR", 3, 1),
		NonVATSalesSerialNumber:          sn("NVS", 3, 1),
		NonVATSalesReturnSerialNumber:    sn("NVS-R", 3, 1),

		Zatca: models.Zatca{
			Phase: zatcaPhase,
			Env:   zatcaEnv,
		},

		Settings: models.StoreSettings{
			EnableAutomobileModule:    true,
			EnableAutomobileDashboard: true,
		},

		CreatedAt: &now,
		UpdatedAt: &now,
	}
}

// GuestRegister handles POST /v1/guest-register.
// No auth required. Creates a store + Manager user in one call.
func GuestRegister(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	var req GuestRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["request"] = "Invalid request body: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	for k, v := range validateGuestRegisterRequest(req) {
		response.Errors[k] = v
	}

	if _, ok := response.Errors["email"]; !ok {
		tempUser := &models.User{Email: req.Email}
		exists, err := tempUser.IsEmailExists()
		if err != nil {
			response.Errors["email"] = err.Error()
		} else if exists {
			response.Errors["email"] = "Email is already in use"
		}
	}

	if len(response.Errors) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		json.NewEncoder(w).Encode(response)
		return
	}

	branchCode := primitive.NewObjectID().Hex()[:8]
	now := time.Now()
	store := buildGuestStore(req, branchCode, now)

	if err := store.Insert(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["store"] = "Failed to create store: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if _, err := store.CreateDB(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["store_db"] = "Failed to create store database: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := store.CreateAllIndexes(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["store_indexes"] = "Failed to create indexes: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	storeID := store.ID
	user := &models.User{
		Name:       req.Name,
		Email:      req.Email,
		Mob:        req.Mob,
		Password:   models.HashPassword(req.Password),
		Role:       "Manager",
		StoreIDs:   []*primitive.ObjectID{&storeID},
		StoreNames: []string{store.Name},
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}

	if err := user.Insert(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["user"] = "Failed to create user: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	user.Password = ""
	response.Status = true
	response.Result = map[string]interface{}{
		"user":  user,
		"store": store,
	}
	json.NewEncoder(w).Encode(response)
}

// NewRegistrationStore builds a new store exactly as guest registration does
// (serials, ZATCA phase, defaults). The StartERP adapter uses it when a
// platform admin adds a store.
func NewRegistrationStore(req GuestRegisterRequest, now time.Time) *models.Store {
	return buildGuestStore(req, primitive.NewObjectID().Hex()[:8], now)
}

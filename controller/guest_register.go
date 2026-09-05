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

	if govalidator.IsNull(req.Name) {
		response.Errors["name"] = "Name is required"
	}
	if govalidator.IsNull(req.Email) {
		response.Errors["email"] = "Email is required"
	} else if !govalidator.IsEmail(req.Email) {
		response.Errors["email"] = "Invalid email address"
	}
	if govalidator.IsNull(req.Mob) {
		response.Errors["mob"] = "Mobile number is required"
	}
	if govalidator.IsNull(req.Password) {
		response.Errors["password"] = "Password is required"
	} else if len(req.Password) < 6 {
		response.Errors["password"] = "Password must be at least 6 characters"
	}
	if req.ZatcaPhase != "1" && req.ZatcaPhase != "2" {
		response.Errors["zatca_phase"] = "ZATCA phase must be 1 or 2"
	}
	if govalidator.IsNull(req.BusinessCategory) {
		response.Errors["business_category"] = "Business category is required"
	}
	if govalidator.IsNull(req.RegistrationNumber) {
		response.Errors["registration_number"] = "Registration number (CRN) is required"
	}
	if govalidator.IsNull(req.VATNo) {
		response.Errors["vat_no"] = "VAT number is required"
	} else if len(req.VATNo) != 15 {
		response.Errors["vat_no"] = "VAT No. should be 15 digits"
	}
	if govalidator.IsNull(req.NationalAddress.BuildingNo) {
		response.Errors["national_address_building_no"] = "Building number is required"
	}
	if govalidator.IsNull(req.NationalAddress.StreetName) {
		response.Errors["national_address_street_name"] = "Street name is required"
	}
	if govalidator.IsNull(req.NationalAddress.DistrictName) {
		response.Errors["national_address_district_name"] = "District name is required"
	}
	if govalidator.IsNull(req.NationalAddress.CityName) {
		response.Errors["national_address_city_name"] = "City name is required"
	}
	if govalidator.IsNull(req.NationalAddress.ZipCode) {
		response.Errors["national_address_zipcode"] = "Zip code is required"
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
	countryCode := strings.TrimSpace(req.CountryCode)
	if countryCode == "" {
		countryCode = "SA"
	}
	countryName := strings.TrimSpace(req.CountryName)
	if countryName == "" {
		countryName = "Saudi Arabia"
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

	branchCode := primitive.NewObjectID().Hex()[:8]

	sn := func(prefix string) models.SerialNumber {
		return models.SerialNumber{Prefix: prefix, PaddingCount: 4, StartFromCount: 0}
	}

	now := time.Now()

	store := &models.Store{
		Name:                        storeName,
		NameInArabic:                storeNameAr,
		Code:                        branchCode,
		BranchName:                  "Main Branch",
		RegistrationNumber:          req.RegistrationNumber,
		RegistrationNumberInArabic:  req.RegistrationNumber,
		VATNo:                       req.VATNo,
		VATNoInArabic:               req.VATNo,
		VatPercent:                  15,
		BusinessCategory:            req.BusinessCategory,
		Email:                       req.Email,
		Phone:                       phone,
		PhoneInArabic:               phone,
		CountryCode:                 countryCode,
		CountryName:                 countryName,
		NationalAddress:             na,

		SalesSerialNumber:               sn("INV"),
		SalesReturnSerialNumber:         sn("RINV"),
		PurchaseSerialNumber:            sn("PO"),
		PurchaseReturnSerialNumber:      sn("RPO"),
		PurchaseOrderSerialNumber:       sn("POO"),
		PurchaseRequestSerialNumber:     sn("PRQ"),
		QuotationSerialNumber:           sn("QT"),
		QuotationSalesReturnSerialNumber: sn("RQT"),
		CustomerSerialNumber:            sn("CUS"),
		VendorSerialNumber:              sn("VEN"),
		ExpenseSerialNumber:             sn("EXP"),
		DeliveryNoteSerialNumber:        sn("DN"),
		CustomerDepositSerialNumber:     sn("CD"),
		CustomerWithdrawalSerialNumber:  sn("CW"),
		CapitalDepositSerialNumber:      sn("CAP"),
		DividentSerialNumber:            sn("DIV"),
		StockTransferSerialNumber:       sn("ST"),
		NonVATSalesSerialNumber:         sn("NVI"),
		NonVATSalesReturnSerialNumber:   sn("RNVI"),

		Zatca: models.Zatca{
			Phase: req.ZatcaPhase,
		},

		CreatedAt: &now,
		UpdatedAt: &now,
	}

	if req.ZatcaPhase == "2" {
		store.Zatca.Env = "Production"
	}

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
		Password:   req.Password,
		Role:       "Manager",
		StoreIDs:   []*primitive.ObjectID{&storeID},
		StoreNames: []string{storeName},
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

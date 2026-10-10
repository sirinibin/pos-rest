package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// validReq returns a fully-populated GuestRegisterRequest that passes all
// field-level validation (no DB calls required).
func validReq() GuestRegisterRequest {
	return GuestRegisterRequest{
		Name:               "Ali Hassan",
		Email:              "ali@example.com",
		Mob:                "+966501234567",
		Password:           "secret99",
		StoreName:          "Gulf Workshop",
		StoreNameInArabic:  "ورشة الخليج",
		BusinessCategory:   "Automobile",
		RegistrationNumber: "1234567890",
		VATNo:              "300000000000003",
		ZatcaPhase:         "1",
		NationalAddress: models.NationalAddress{
			BuildingNo:   "1234",
			StreetName:   "King Fahd Road",
			DistrictName: "Al Olaya",
			CityName:     "Riyadh",
			ZipCode:      "12345",
		},
	}
}

// ---------------------------------------------------------------------------
// Pure-function tests: validateGuestRegisterRequest
// ---------------------------------------------------------------------------

func TestValidateGuestRegisterRequest(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*GuestRegisterRequest)
		wantErr   string // expected key in returned errors map
		wantNoErr string // key that must NOT be in errors (sanity)
	}{
		{
			name:      "valid request has no errors",
			mutate:    func(r *GuestRegisterRequest) {},
			wantNoErr: "name",
		},
		{
			name:    "missing name",
			mutate:  func(r *GuestRegisterRequest) { r.Name = "" },
			wantErr: "name",
		},
		{
			name:    "missing email",
			mutate:  func(r *GuestRegisterRequest) { r.Email = "" },
			wantErr: "email",
		},
		{
			name:    "invalid email — no @",
			mutate:  func(r *GuestRegisterRequest) { r.Email = "notanemail" },
			wantErr: "email",
		},
		{
			name:    "invalid email — no TLD",
			mutate:  func(r *GuestRegisterRequest) { r.Email = "user@" },
			wantErr: "email",
		},
		{
			name:    "missing mob",
			mutate:  func(r *GuestRegisterRequest) { r.Mob = "" },
			wantErr: "mob",
		},
		{
			name:    "missing password",
			mutate:  func(r *GuestRegisterRequest) { r.Password = "" },
			wantErr: "password",
		},
		{
			name:    "password too short (5 chars)",
			mutate:  func(r *GuestRegisterRequest) { r.Password = "abc12" },
			wantErr: "password",
		},
		{
			name:      "password exactly 6 chars is valid",
			mutate:    func(r *GuestRegisterRequest) { r.Password = "abc123" },
			wantNoErr: "password",
		},
		{
			name:    "zatca_phase empty",
			mutate:  func(r *GuestRegisterRequest) { r.ZatcaPhase = "" },
			wantErr: "zatca_phase",
		},
		{
			name:    "zatca_phase invalid value",
			mutate:  func(r *GuestRegisterRequest) { r.ZatcaPhase = "3" },
			wantErr: "zatca_phase",
		},
		{
			name:      "zatca_phase 1 is valid",
			mutate:    func(r *GuestRegisterRequest) { r.ZatcaPhase = "1" },
			wantNoErr: "zatca_phase",
		},
		{
			name:      "zatca_phase 2 is valid",
			mutate:    func(r *GuestRegisterRequest) { r.ZatcaPhase = "2" },
			wantNoErr: "zatca_phase",
		},
		{
			name:    "missing business_category",
			mutate:  func(r *GuestRegisterRequest) { r.BusinessCategory = "" },
			wantErr: "business_category",
		},
		{
			name:    "missing registration_number",
			mutate:  func(r *GuestRegisterRequest) { r.RegistrationNumber = "" },
			wantErr: "registration_number",
		},
		{
			name:    "missing vat_no",
			mutate:  func(r *GuestRegisterRequest) { r.VATNo = "" },
			wantErr: "vat_no",
		},
		{
			name:    "vat_no too short (14 digits)",
			mutate:  func(r *GuestRegisterRequest) { r.VATNo = "30000000000000" },
			wantErr: "vat_no",
		},
		{
			name:    "vat_no too long (16 digits)",
			mutate:  func(r *GuestRegisterRequest) { r.VATNo = "3000000000000031" },
			wantErr: "vat_no",
		},
		{
			name:      "vat_no exactly 15 chars is valid",
			mutate:    func(r *GuestRegisterRequest) { r.VATNo = "300000000000003" },
			wantNoErr: "vat_no",
		},
		{
			name:    "missing national_address building_no",
			mutate:  func(r *GuestRegisterRequest) { r.NationalAddress.BuildingNo = "" },
			wantErr: "national_address_building_no",
		},
		{
			name:    "missing national_address street_name",
			mutate:  func(r *GuestRegisterRequest) { r.NationalAddress.StreetName = "" },
			wantErr: "national_address_street_name",
		},
		{
			name:    "missing national_address district_name",
			mutate:  func(r *GuestRegisterRequest) { r.NationalAddress.DistrictName = "" },
			wantErr: "national_address_district_name",
		},
		{
			name:    "missing national_address city_name",
			mutate:  func(r *GuestRegisterRequest) { r.NationalAddress.CityName = "" },
			wantErr: "national_address_city_name",
		},
		{
			name:    "missing national_address zipcode",
			mutate:  func(r *GuestRegisterRequest) { r.NationalAddress.ZipCode = "" },
			wantErr: "national_address_zipcode",
		},
		{
			name: "multiple errors returned together",
			mutate: func(r *GuestRegisterRequest) {
				r.Name = ""
				r.Email = ""
				r.Mob = ""
			},
			wantErr: "name",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := validReq()
			tc.mutate(&req)
			errs := validateGuestRegisterRequest(req)

			if tc.wantErr != "" {
				if _, ok := errs[tc.wantErr]; !ok {
					t.Errorf("expected error key %q, got errors=%v", tc.wantErr, errs)
				}
			}
			if tc.wantNoErr != "" {
				if msg, ok := errs[tc.wantNoErr]; ok {
					t.Errorf("unexpected error for key %q: %q", tc.wantNoErr, msg)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure-function tests: buildGuestStore — default-filling
// ---------------------------------------------------------------------------

func TestBuildGuestStore_Defaults(t *testing.T) {
	now := time.Now()

	t.Run("store_name defaults to user name when blank", func(t *testing.T) {
		req := validReq()
		req.StoreName = ""
		s := buildGuestStore(req, "abc12345", now)
		if s.Name != req.Name {
			t.Errorf("want Name=%q, got %q", req.Name, s.Name)
		}
	})

	t.Run("store_name defaults to user name when whitespace only", func(t *testing.T) {
		req := validReq()
		req.StoreName = "   "
		s := buildGuestStore(req, "abc12345", now)
		if s.Name != req.Name {
			t.Errorf("want Name=%q, got %q", req.Name, s.Name)
		}
	})

	t.Run("store_name is used when provided", func(t *testing.T) {
		req := validReq()
		req.StoreName = "My Shop"
		s := buildGuestStore(req, "abc12345", now)
		if s.Name != "My Shop" {
			t.Errorf("want Name=%q, got %q", "My Shop", s.Name)
		}
	})

	t.Run("arabic name defaults to english store name when blank", func(t *testing.T) {
		req := validReq()
		req.StoreNameInArabic = ""
		req.StoreName = "Gulf Workshop"
		s := buildGuestStore(req, "abc12345", now)
		if s.NameInArabic != "Gulf Workshop" {
			t.Errorf("want NameInArabic=%q, got %q", "Gulf Workshop", s.NameInArabic)
		}
	})

	t.Run("arabic name used when provided", func(t *testing.T) {
		req := validReq()
		req.StoreNameInArabic = "ورشة الخليج"
		s := buildGuestStore(req, "abc12345", now)
		if s.NameInArabic != "ورشة الخليج" {
			t.Errorf("want NameInArabic=%q, got %q", "ورشة الخليج", s.NameInArabic)
		}
	})

	t.Run("phone defaults to mob when blank", func(t *testing.T) {
		req := validReq()
		req.Phone = ""
		s := buildGuestStore(req, "abc12345", now)
		if s.Phone != req.Mob {
			t.Errorf("want Phone=%q, got %q", req.Mob, s.Phone)
		}
	})

	t.Run("phone used when provided", func(t *testing.T) {
		req := validReq()
		req.Phone = "+966501111111"
		s := buildGuestStore(req, "abc12345", now)
		if s.Phone != "+966501111111" {
			t.Errorf("want Phone=%q, got %q", "+966501111111", s.Phone)
		}
	})

	t.Run("country_code defaults to SA", func(t *testing.T) {
		req := validReq()
		req.CountryCode = ""
		s := buildGuestStore(req, "abc12345", now)
		if s.CountryCode != "SA" {
			t.Errorf("want CountryCode=%q, got %q", "SA", s.CountryCode)
		}
	})

	t.Run("country_code used when provided", func(t *testing.T) {
		req := validReq()
		req.CountryCode = "AE"
		s := buildGuestStore(req, "abc12345", now)
		if s.CountryCode != "AE" {
			t.Errorf("want CountryCode=%q, got %q", "AE", s.CountryCode)
		}
	})

	t.Run("country_name defaults to Saudi Arabia", func(t *testing.T) {
		req := validReq()
		req.CountryName = ""
		s := buildGuestStore(req, "abc12345", now)
		if s.CountryName != "Saudi Arabia" {
			t.Errorf("want CountryName=%q, got %q", "Saudi Arabia", s.CountryName)
		}
	})

	t.Run("national address arabic fields default to english when blank", func(t *testing.T) {
		req := validReq()
		req.NationalAddress.StreetNameArabic = ""
		req.NationalAddress.DistrictNameArabic = ""
		req.NationalAddress.CityNameArabic = ""
		s := buildGuestStore(req, "abc12345", now)
		if s.NationalAddress.StreetNameArabic != req.NationalAddress.StreetName {
			t.Errorf("StreetNameArabic: want %q, got %q",
				req.NationalAddress.StreetName, s.NationalAddress.StreetNameArabic)
		}
		if s.NationalAddress.DistrictNameArabic != req.NationalAddress.DistrictName {
			t.Errorf("DistrictNameArabic: want %q, got %q",
				req.NationalAddress.DistrictName, s.NationalAddress.DistrictNameArabic)
		}
		if s.NationalAddress.CityNameArabic != req.NationalAddress.CityName {
			t.Errorf("CityNameArabic: want %q, got %q",
				req.NationalAddress.CityName, s.NationalAddress.CityNameArabic)
		}
	})

	t.Run("national address arabic fields kept when provided", func(t *testing.T) {
		req := validReq()
		req.NationalAddress.StreetNameArabic = "شارع الملك"
		s := buildGuestStore(req, "abc12345", now)
		if s.NationalAddress.StreetNameArabic != "شارع الملك" {
			t.Errorf("StreetNameArabic: want %q, got %q", "شارع الملك", s.NationalAddress.StreetNameArabic)
		}
	})

	t.Run("branch_code is set from argument", func(t *testing.T) {
		req := validReq()
		s := buildGuestStore(req, "deadbeef", now)
		if s.Code != "deadbeef" {
			t.Errorf("want Code=%q, got %q", "deadbeef", s.Code)
		}
	})

	t.Run("branch_name is always Main Branch", func(t *testing.T) {
		s := buildGuestStore(validReq(), "x", now)
		if s.BranchName != "Main Branch" {
			t.Errorf("want BranchName=%q, got %q", "Main Branch", s.BranchName)
		}
	})

	t.Run("vat_percent is 15", func(t *testing.T) {
		s := buildGuestStore(validReq(), "x", now)
		if s.VatPercent != 15 {
			t.Errorf("want VatPercent=15, got %v", s.VatPercent)
		}
	})

	t.Run("timestamps are set from injected now", func(t *testing.T) {
		fixed := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		s := buildGuestStore(validReq(), "x", fixed)
		if s.CreatedAt == nil || !s.CreatedAt.Equal(fixed) {
			t.Errorf("want CreatedAt=%v, got %v", fixed, s.CreatedAt)
		}
		if s.UpdatedAt == nil || !s.UpdatedAt.Equal(fixed) {
			t.Errorf("want UpdatedAt=%v, got %v", fixed, s.UpdatedAt)
		}
	})
}

// ---------------------------------------------------------------------------
// Pure-function tests: buildGuestStore — ZATCA environment
// ---------------------------------------------------------------------------

func TestBuildGuestStore_ZatcaEnv(t *testing.T) {
	now := time.Now()

	tests := []struct {
		phase   string
		wantEnv string
	}{
		{"1", "NonProduction"},
		{"2", "Production"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run("phase "+tc.phase+" → "+tc.wantEnv, func(t *testing.T) {
			req := validReq()
			req.ZatcaPhase = tc.phase
			s := buildGuestStore(req, "x", now)
			if s.Zatca.Env != tc.wantEnv {
				t.Errorf("want Zatca.Env=%q, got %q", tc.wantEnv, s.Zatca.Env)
			}
			if s.Zatca.Phase != tc.phase {
				t.Errorf("want Zatca.Phase=%q, got %q", tc.phase, s.Zatca.Phase)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure-function tests: buildGuestStore — automobile module settings
// ---------------------------------------------------------------------------

func TestBuildGuestStore_AutomobileSettings(t *testing.T) {
	s := buildGuestStore(validReq(), "x", time.Now())

	if !s.Settings.EnableAutomobileModule {
		t.Error("want EnableAutomobileModule=true")
	}
	if !s.Settings.EnableAutomobileDashboard {
		t.Error("want EnableAutomobileDashboard=true")
	}
}

// ---------------------------------------------------------------------------
// Pure-function tests: buildGuestStore — serial number prefixes/padding/start
// ---------------------------------------------------------------------------

func TestBuildGuestStore_SerialNumbers(t *testing.T) {
	s := buildGuestStore(validReq(), "x", time.Now())

	type snWant struct {
		label  string
		got    models.SerialNumber
		prefix string
		pad    int64
		start  int64
	}

	cases := []snWant{
		{"Sales", s.SalesSerialNumber, "S-INV", 3, 1},
		{"SalesReturn", s.SalesReturnSerialNumber, "SR-INV", 3, 1},
		{"Purchase", s.PurchaseSerialNumber, "P-INV", 3, 1},
		{"PurchaseReturn", s.PurchaseReturnSerialNumber, "PR-INV", 3, 1},
		{"PurchaseOrder", s.PurchaseOrderSerialNumber, "PO", 4, 1},
		{"PurchaseRequest", s.PurchaseRequestSerialNumber, "PR", 4, 1},
		{"Quotation", s.QuotationSerialNumber, "QTN", 3, 1},
		{"QuotationSalesReturn", s.QuotationSalesReturnSerialNumber, "QTN-SR-INV", 3, 1},
		{"Customer", s.CustomerSerialNumber, "CUST", 4, 1},
		{"Vendor", s.VendorSerialNumber, "VND", 4, 1},
		{"Expense", s.ExpenseSerialNumber, "EXP", 4, 1},
		{"DeliveryNote", s.DeliveryNoteSerialNumber, "DEL-NOTE", 6, 1},
		{"CustomerDeposit", s.CustomerDepositSerialNumber, "CUST-RCVBLE", 4, 1},
		{"CustomerWithdrawal", s.CustomerWithdrawalSerialNumber, "CUST-PAYBLE", 4, 1},
		{"CapitalDeposit", s.CapitalDepositSerialNumber, "CAP-DPST", 4, 1},
		{"Divident", s.DividentSerialNumber, "CAP-DRWNG", 4, 1},
		{"StockTransfer", s.StockTransferSerialNumber, "ST-TR", 3, 1},
		{"NonVATSales", s.NonVATSalesSerialNumber, "NVS", 3, 1},
		{"NonVATSalesReturn", s.NonVATSalesReturnSerialNumber, "NVS-R", 3, 1},
	}

	for _, c := range cases {
		c := c
		t.Run(c.label, func(t *testing.T) {
			if c.got.Prefix != c.prefix {
				t.Errorf("Prefix: want %q, got %q", c.prefix, c.got.Prefix)
			}
			if c.got.PaddingCount != c.pad {
				t.Errorf("PaddingCount: want %d, got %d", c.pad, c.got.PaddingCount)
			}
			if c.got.StartFromCount != c.start {
				t.Errorf("StartFromCount: want %d, got %d", c.start, c.got.StartFromCount)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HTTP handler tests: GuestRegister (no DB required)
// ---------------------------------------------------------------------------

// postGuestRegister fires the handler with the given JSON body and returns the
// recorded response.
func postGuestRegister(body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/v1/guest-register", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	GuestRegister(w, r)
	return w
}

// TestGuestRegister_PasswordIsHashed verifies that the handler stores a bcrypt
// hash, not plaintext — regression test for the bug fixed in this session.
func TestGuestRegister_PasswordIsHashed(t *testing.T) {
	src, err := os.ReadFile("guest_register.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if !strings.Contains(string(src), "models.HashPassword(req.Password)") {
		t.Error("guest_register.go must store models.HashPassword(req.Password) — storing plaintext breaks login")
	}
	if strings.Contains(string(src), "Password:   req.Password") || strings.Contains(string(src), "Password: req.Password") {
		t.Error("guest_register.go must NOT store plaintext: Password: req.Password")
	}
}

// TestHashPassword_NotPlaintext verifies that models.HashPassword returns a
// bcrypt-style hash that differs from the input and is non-empty.
func TestHashPassword_NotPlaintext(t *testing.T) {
	plain := "secret99"
	hashed := models.HashPassword(plain)
	if hashed == "" {
		t.Fatal("HashPassword returned empty string")
	}
	if hashed == plain {
		t.Errorf("HashPassword returned plaintext (%q); expected a bcrypt hash", hashed)
	}
	if len(hashed) < 20 {
		t.Errorf("HashPassword result too short (%d chars); does not look like a bcrypt hash", len(hashed))
	}
}

func TestGuestRegister_InvalidJSON(t *testing.T) {
	w := postGuestRegister("{not valid json")
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
	var resp struct {
		Status bool              `json:"status"`
		Errors map[string]string `json:"errors"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status {
		t.Error("want status=false")
	}
	if _, ok := resp.Errors["request"]; !ok {
		t.Errorf("want errors.request, got %v", resp.Errors)
	}
}

// TestGuestRegister_FieldValidation covers HTTP-level validation errors.
// All test cases use an invalid email so that the email-exists DB call is
// skipped — the response body is checked for at least the expected error key.
func TestGuestRegister_FieldValidation(t *testing.T) {
	base := validReq()
	// Use an invalid email so IsEmailExists is never called.
	base.Email = "INVALID"

	tests := []struct {
		name    string
		mutate  func(r *GuestRegisterRequest)
		wantKey string
	}{
		{
			name:    "missing name",
			mutate:  func(r *GuestRegisterRequest) { r.Name = "" },
			wantKey: "name",
		},
		{
			name:    "missing email",
			mutate:  func(r *GuestRegisterRequest) { r.Email = "" },
			wantKey: "email",
		},
		{
			name:    "invalid email",
			mutate:  func(r *GuestRegisterRequest) { r.Email = "bad-email" },
			wantKey: "email",
		},
		{
			name:    "missing mob",
			mutate:  func(r *GuestRegisterRequest) { r.Mob = "" },
			wantKey: "mob",
		},
		{
			name:    "missing password",
			mutate:  func(r *GuestRegisterRequest) { r.Password = "" },
			wantKey: "password",
		},
		{
			name:    "password too short",
			mutate:  func(r *GuestRegisterRequest) { r.Password = "12345" },
			wantKey: "password",
		},
		{
			name:    "invalid zatca_phase",
			mutate:  func(r *GuestRegisterRequest) { r.ZatcaPhase = "9" },
			wantKey: "zatca_phase",
		},
		{
			name:    "missing business_category",
			mutate:  func(r *GuestRegisterRequest) { r.BusinessCategory = "" },
			wantKey: "business_category",
		},
		{
			name:    "missing registration_number",
			mutate:  func(r *GuestRegisterRequest) { r.RegistrationNumber = "" },
			wantKey: "registration_number",
		},
		{
			name:    "missing vat_no",
			mutate:  func(r *GuestRegisterRequest) { r.VATNo = "" },
			wantKey: "vat_no",
		},
		{
			name:    "vat_no wrong length",
			mutate:  func(r *GuestRegisterRequest) { r.VATNo = "123" },
			wantKey: "vat_no",
		},
		{
			name:    "missing building_no",
			mutate:  func(r *GuestRegisterRequest) { r.NationalAddress.BuildingNo = "" },
			wantKey: "national_address_building_no",
		},
		{
			name:    "missing street_name",
			mutate:  func(r *GuestRegisterRequest) { r.NationalAddress.StreetName = "" },
			wantKey: "national_address_street_name",
		},
		{
			name:    "missing district_name",
			mutate:  func(r *GuestRegisterRequest) { r.NationalAddress.DistrictName = "" },
			wantKey: "national_address_district_name",
		},
		{
			name:    "missing city_name",
			mutate:  func(r *GuestRegisterRequest) { r.NationalAddress.CityName = "" },
			wantKey: "national_address_city_name",
		},
		{
			name:    "missing zipcode",
			mutate:  func(r *GuestRegisterRequest) { r.NationalAddress.ZipCode = "" },
			wantKey: "national_address_zipcode",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := base
			tc.mutate(&req)
			body, _ := json.Marshal(req)
			w := postGuestRegister(string(body))

			if w.Code != http.StatusBadRequest {
				t.Errorf("want 400, got %d", w.Code)
			}

			ct := w.Header().Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("want JSON Content-Type, got %q", ct)
			}

			var resp struct {
				Status bool              `json:"status"`
				Errors map[string]string `json:"errors"`
			}
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Status {
				t.Error("want status=false")
			}
			if _, ok := resp.Errors[tc.wantKey]; !ok {
				t.Errorf("want errors[%q], got %v", tc.wantKey, resp.Errors)
			}
		})
	}
}

// TestGuestRegister_EmailGuard verifies that when the email field has a
// validation error (missing or invalid), the email-exists DB check is skipped
// and the handler returns 400 without panicking or connecting to MongoDB.
func TestGuestRegister_EmailGuard(t *testing.T) {
	for _, email := range []string{"", "not-an-email"} {
		email := email
		t.Run("email="+email, func(t *testing.T) {
			req := validReq()
			req.Email = email
			body, _ := json.Marshal(req)
			// If IsEmailExists were called it would panic (no DB in unit tests).
			// Getting a clean 400 proves the guard works.
			w := postGuestRegister(string(body))
			if w.Code != http.StatusBadRequest {
				t.Errorf("want 400, got %d", w.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration tests (skipped unless -run Integration or -count 1 with DB up)
// ---------------------------------------------------------------------------

// guestRegisterCleanup removes everything a successful guest registration
// creates: the user, the store document and the store_<id> database.
func guestRegisterCleanup(t *testing.T, email string) {
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		main := db.Client("").Database(db.GetPosDB())
		var u struct {
			StoreIDs []primitive.ObjectID `bson:"store_ids"`
		}
		if main.Collection("user").FindOne(ctx, bson.M{"email": email}).Decode(&u) == nil {
			for _, sid := range u.StoreIDs {
				_, _ = main.Collection("store").DeleteOne(ctx, bson.M{"_id": sid})
				_ = db.GetDB("store_" + sid.Hex()).Drop(ctx)
			}
		}
		_, _ = main.Collection("user").DeleteMany(ctx, bson.M{"email": email})
	})
}

// countStoresNamed counts store documents with the given name.
func countStoresNamed(t *testing.T, name string) int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n, err := db.Client("").Database(db.GetPosDB()).Collection("store").CountDocuments(ctx, bson.M{"name": name})
	if err != nil {
		t.Fatalf("count stores: %v", err)
	}
	return n
}

// TestGuestRegister_Integration_DuplicateEmail checks that registering with
// an e-mail that already belongs to a user is rejected with 400 and
// errors.email, and that no store is created for the rejected request.
func TestGuestRegister_Integration_DuplicateEmail(t *testing.T) {
	fx := requireDB(t)
	req := validReq()
	req.Email = fx.AdminEmail
	req.StoreName = uniqName("Dup Email Store")
	body, _ := json.Marshal(req)

	w := postGuestRegister(string(body))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Status bool              `json:"status"`
		Errors map[string]string `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	if resp.Status || resp.Errors["email"] != "Email is already in use" {
		t.Fatalf("want status=false errors.email=\"Email is already in use\", got %+v", resp)
	}
	// the duplicate check is combined with the other validation errors
	req.Mob = ""
	body, _ = json.Marshal(req)
	w = postGuestRegister(string(body))
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if w.Code != http.StatusBadRequest || resp.Errors["email"] == "" || resp.Errors["mob"] == "" {
		t.Fatalf("want both email and mob errors, got %d %s", w.Code, w.Body.String())
	}
	if n := countStoresNamed(t, req.StoreName); n != 0 {
		t.Fatalf("rejected registration must not create a store, found %d", n)
	}
	// the existing user is untouched
	u, err := models.FindUserByEmail(fx.AdminEmail)
	if err != nil || u.ID != fx.Admin || u.Role != "Admin" {
		t.Fatalf("existing admin changed: %+v %v", u, err)
	}
}

// TestGuestRegister_Integration_Success verifies end-to-end store+user
// creation: response shape, the persisted store (serials, ZATCA, VAT), the
// Manager user (hashed password, linked store), the store database's
// indexes, a working access token, and that the e-mail is then taken.
func TestGuestRegister_Integration_Success(t *testing.T) {
	requireDB(t)
	req := validReq()
	req.Email = fmt.Sprintf("guest+%d@t1.example", time.Now().UnixNano())
	req.StoreName = uniqName("Guest Workshop")
	guestRegisterCleanup(t, req.Email)
	body, _ := json.Marshal(req)

	w := postGuestRegister(string(body))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Status bool              `json:"status"`
		Errors map[string]string `json:"errors"`
		Result struct {
			User  map[string]interface{} `json:"user"`
			Store map[string]interface{} `json:"store"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Status || len(resp.Errors) != 0 {
		t.Fatalf("want status=true and no errors, got %s", w.Body.String())
	}
	if pw, _ := resp.Result.User["password"].(string); pw != "" {
		t.Fatalf("response must not leak the password hash, got %q", pw)
	}
	storeID, err := primitive.ObjectIDFromHex(fmt.Sprint(resp.Result.Store["id"]))
	if err != nil {
		t.Fatalf("store id in response: %s", w.Body.String())
	}

	// store document
	store, err := models.FindStoreByID(&storeID, bson.M{})
	if err != nil {
		t.Fatalf("store not persisted: %v", err)
	}
	if store.Name != req.StoreName || store.VATNo != req.VATNo || store.CountryCode != "SA" || store.BranchName != "Main Branch" ||
		store.Email != req.Email || len(store.Code) != 8 || store.Zatca.Phase != "1" || store.Zatca.Env != "NonProduction" ||
		store.SalesSerialNumber.Prefix != "S-INV" || store.CustomerSerialNumber.Prefix != "CUST" || store.VatPercent != 15 ||
		store.NationalAddress.CityName != "Riyadh" || store.NationalAddress.CityNameArabic != "Riyadh" {
		t.Fatalf("persisted store mismatch: %+v", store)
	}

	// user document
	u, err := models.FindUserByEmail(req.Email)
	if err != nil {
		t.Fatalf("user not persisted: %v", err)
	}
	if u.Name != req.Name || u.Mob != req.Mob || u.Role != "Manager" || u.Admin ||
		len(u.StoreIDs) != 1 || *u.StoreIDs[0] != storeID {
		t.Fatalf("persisted user mismatch: %+v", u)
	}
	if u.Password == req.Password || !u.VerifyPassword(req.Password) || u.VerifyPassword("wrong-pass") {
		t.Fatalf("password must be stored as a hash of the submitted password")
	}
	// NOTE: result.user.id is currently the zero ObjectID because
	// models.User.Insert does not assign the generated _id back to the struct
	// (reported); identify the returned user by e-mail and role instead.
	if resp.Result.User["email"] != req.Email || resp.Result.User["role"] != "Manager" {
		t.Fatalf("response user mismatch: %v", resp.Result.User)
	}

	// store database exists with its indexes
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	colls, err := db.GetDB("store_"+storeID.Hex()).ListCollectionNames(ctx, bson.M{})
	if err != nil || len(colls) == 0 {
		t.Fatalf("store database has no collections (indexes not created): %v %v", colls, err)
	}

	// the new manager can obtain a working access token
	tok, err := models.GenerateAccesstoken(req.Email)
	if err != nil {
		t.Fatalf("token for new user: %v", err)
	}
	hr := httptest.NewRequest("GET", "/", nil)
	hr.Header.Set("Authorization", tok.Token)
	if claims, err := models.AuthenticateByAccessToken(hr); err != nil || claims.UserID != u.ID.Hex() {
		t.Fatalf("auth as new user: %+v %v", claims, err)
	}

	// registering again with the same e-mail is now rejected
	req.StoreName = uniqName("Guest Workshop Again")
	body, _ = json.Marshal(req)
	if w := postGuestRegister(string(body)); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Email is already in use") {
		t.Fatalf("second registration: want 400 email in use, got %d %s", w.Code, w.Body.String())
	}
	if n := countStoresNamed(t, req.StoreName); n != 0 {
		t.Fatalf("second registration must not create a store, found %d", n)
	}
}

// ---------------------------------------------------------------------------
// GCC countries (models/country_profile.go)
// ---------------------------------------------------------------------------

func gccReq(code string) GuestRegisterRequest {
	r := validReq()
	r.CountryCode = code
	r.VATNo = ""
	r.NationalAddress = models.NationalAddress{StreetName: "Sheikh Zayed Rd", CityName: "Dubai"}
	return r
}

func TestValidateGuestRegisterRequest_GCC(t *testing.T) {
	for _, code := range []string{"AE", "OM", "QA", "BH", "KW", "ae"} {
		if errs := validateGuestRegisterRequest(gccReq(code)); len(errs) != 0 {
			t.Errorf("%s: unexpected %v", code, errs)
		}
	}
	tests := []struct {
		name string
		req  func() GuestRegisterRequest
		key  string
	}{
		{"unsupported country", func() GuestRegisterRequest { return gccReq("GB") }, "country_code"},
		{"bad GSTIN checksum", func() GuestRegisterRequest { r := gccReq("IN"); r.VATNo = "27AAPFU0939F1ZA"; return r }, "vat_no"},
		{"GCC still needs the CR", func() GuestRegisterRequest { r := gccReq("AE"); r.RegistrationNumber = ""; return r }, "registration_number"},
		{"bad UAE TRN", func() GuestRegisterRequest { r := gccReq("AE"); r.VATNo = "12"; return r }, "vat_no"},
		{"street required", func() GuestRegisterRequest { r := gccReq("BH"); r.NationalAddress.StreetName = ""; return r }, "national_address_street_name"},
		{"city required", func() GuestRegisterRequest { r := gccReq("KW"); r.NationalAddress.CityName = ""; return r }, "national_address_city_name"},
		{"Saudi still needs VAT", func() GuestRegisterRequest { r := validReq(); r.VATNo = ""; return r }, "vat_no"},
		{"Saudi still needs building no.", func() GuestRegisterRequest {
			r := validReq()
			r.NationalAddress.BuildingNo = ""
			return r
		}, "national_address_building_no"},
	}
	for _, tc := range tests {
		if errs := validateGuestRegisterRequest(tc.req()); errs[tc.key] == "" {
			t.Errorf("%s: expected %s, got %v", tc.name, tc.key, errs)
		}
	}
}

func TestValidateGuestRegisterRequest_India(t *testing.T) {
	r := gccReq("IN")
	r.RegistrationNumber = "" // PAN is optional in India
	r.VATNo = "27AAPFU0939F1ZV"
	if errs := validateGuestRegisterRequest(r); len(errs) != 0 {
		t.Fatalf("India: unexpected %v", errs)
	}
	r.VATNo = ""
	if errs := validateGuestRegisterRequest(r); len(errs) != 0 {
		t.Fatalf("India without GSTIN: unexpected %v", errs)
	}
	if errs := validateGuestRegisterRequest(gccReq("GB")); errs["country_code"] != "Choose a supported country" {
		t.Errorf("country message: %v", errs)
	}
}

func TestBuildGuestStore_GCCCountry(t *testing.T) {
	now := time.Now()
	tests := []struct {
		code, name, phase string
		vat               float64
	}{
		{"", "Saudi Arabia", "2", 15},
		{"SA", "Saudi Arabia", "2", 15},
		{"ae", "United Arab Emirates", "1", 5},
		{"OM", "Oman", "1", 5},
		{"QA", "Qatar", "1", 0},
		{"BH", "Bahrain", "1", 10},
		{"KW", "Kuwait", "1", 0},
		{"in", "India", "1", 18},
	}
	for _, tc := range tests {
		r := validReq()
		r.CountryCode, r.ZatcaPhase = tc.code, "2"
		s := buildGuestStore(r, "abc12345", now)
		want := strings.ToUpper(tc.code)
		if want == "" {
			want = "SA"
		}
		if s.CountryCode != want || s.CountryName != tc.name || s.VatPercent != tc.vat || s.Zatca.Phase != tc.phase {
			t.Errorf("%q: country=%s/%s vat=%v phase=%s", tc.code, s.CountryCode, s.CountryName, s.VatPercent, s.Zatca.Phase)
		}
		if tc.phase == "1" && s.Zatca.Env != "NonProduction" {
			t.Errorf("%q: non-Saudi store must not get a ZATCA production env", tc.code)
		}
	}
}

package controller

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── zatcaSensitiveFieldsChanged ───────────────────────────────────────────────
//
// Table-driven tests that exercise every branch of the function without
// touching a real database.

func TestZatcaSensitiveFieldsChanged(t *testing.T) {
	// base is a fully-populated store used as a convenient starting point.
	// Tests clone it and mutate only the field(s) under test.
	base := models.Store{
		Name:               "Test Store",
		NameInArabic:       "مخزن تجريبي",
		Code:               "TST",
		BranchName:         "Main",
		RegistrationNumber: "REG123",
		VATNo:              "VAT123456",
		NationalAddress: models.NationalAddress{
			ShortCode:    "SC1",
			BuildingNo:   "1234",
			StreetName:   "King St",
			DistrictName: "Central",
			CityName:     "Riyadh",
			ZipCode:      "12345",
			AdditionalNo: "6789",
			UnitNo:       "01",
		},
		SalesSerialNumber: models.SerialNumber{
			Prefix:         "INV",
			PaddingCount:   5,
			StartFromCount: 1,
		},
		SalesReturnSerialNumber: models.SerialNumber{
			Prefix:         "RET",
			PaddingCount:   5,
			StartFromCount: 1,
		},
		CustomerDepositSerialNumber: models.SerialNumber{
			Prefix:         "DEP",
			PaddingCount:   5,
			StartFromCount: 1,
		},
		CustomerWithdrawalSerialNumber: models.SerialNumber{
			Prefix:         "WDR",
			PaddingCount:   5,
			StartFromCount: 1,
		},
		Settings: models.StoreSettings{
			EnableZatcaReportingForReceivables: false,
			EnableZatcaReportingForPayables:    false,
		},
	}

	cases := []struct {
		name     string
		old      models.Store
		new_     models.Store
		isAdmin  bool
		wantTrue bool
	}{
		// ── Identity cases ────────────────────────────────────────────────────
		{
			name:     "identical stores no change",
			old:      base,
			new_:     base,
			isAdmin:  false,
			wantTrue: false,
		},
		{
			name:     "empty structs both sides",
			old:      models.Store{},
			new_:     models.Store{},
			isAdmin:  false,
			wantTrue: false,
		},

		// ── Core identity fields ──────────────────────────────────────────────
		{
			name: "Name changed",
			old:  base,
			new_: func() models.Store { s := base; s.Name = "Changed Store"; return s }(),
			wantTrue: true,
		},
		{
			name: "NameInArabic changed",
			old:  base,
			new_: func() models.Store { s := base; s.NameInArabic = "تغيير"; return s }(),
			wantTrue: true,
		},
		{
			name:     "Code changed",
			old:      base,
			new_:     func() models.Store { s := base; s.Code = "NEW"; return s }(),
			wantTrue: true,
		},
		{
			name: "BranchName changed",
			old:  base,
			new_: func() models.Store { s := base; s.BranchName = "Branch2"; return s }(),
			wantTrue: true,
		},
		{
			name: "RegistrationNumber changed",
			old:  base,
			new_: func() models.Store { s := base; s.RegistrationNumber = "REG999"; return s }(),
			wantTrue: true,
		},
		{
			name: "VATNo changed",
			old:  base,
			new_: func() models.Store { s := base; s.VATNo = "VAT999999"; return s }(),
			wantTrue: true,
		},

		// ── Business Category ─────────────────────────────────────────────────
		{
			name:     "BusinessCategory changed",
			old:      base,
			new_:     func() models.Store { s := base; s.BusinessCategory = "Retail"; return s }(),
			wantTrue: true,
		},
		{
			name:     "BusinessCategory unchanged",
			old:      base,
			new_:     base,
			wantTrue: false,
		},

		// ── National Address fields ───────────────────────────────────────────
		{
			name: "NationalAddress.ShortCode changed",
			old:  base,
			new_: func() models.Store {
				s := base
				s.NationalAddress.ShortCode = "SC2"
				return s
			}(),
			wantTrue: true,
		},
		{
			name: "NationalAddress.BuildingNo changed",
			old:  base,
			new_: func() models.Store {
				s := base
				s.NationalAddress.BuildingNo = "9999"
				return s
			}(),
			wantTrue: true,
		},
		{
			name: "NationalAddress.StreetName changed",
			old:  base,
			new_: func() models.Store {
				s := base
				s.NationalAddress.StreetName = "New Street"
				return s
			}(),
			wantTrue: true,
		},
		{
			name: "NationalAddress.DistrictName changed",
			old:  base,
			new_: func() models.Store {
				s := base
				s.NationalAddress.DistrictName = "North"
				return s
			}(),
			wantTrue: true,
		},
		{
			name: "NationalAddress.CityName changed",
			old:  base,
			new_: func() models.Store {
				s := base
				s.NationalAddress.CityName = "Jeddah"
				return s
			}(),
			wantTrue: true,
		},
		{
			name: "NationalAddress.ZipCode changed",
			old:  base,
			new_: func() models.Store {
				s := base
				s.NationalAddress.ZipCode = "54321"
				return s
			}(),
			wantTrue: true,
		},
		{
			name: "NationalAddress.AdditionalNo changed",
			old:  base,
			new_: func() models.Store {
				s := base
				s.NationalAddress.AdditionalNo = "0001"
				return s
			}(),
			wantTrue: true,
		},
		{
			name: "NationalAddress.UnitNo changed",
			old:  base,
			new_: func() models.Store {
				s := base
				s.NationalAddress.UnitNo = "02"
				return s
			}(),
			wantTrue: true,
		},
		{
			name: "NationalAddress.StreetNameArabic changed",
			old:  base,
			new_: func() models.Store {
				s := base
				s.NationalAddress.StreetNameArabic = "شارع الملك"
				return s
			}(),
			wantTrue: true,
		},
		{
			name: "NationalAddress.DistrictNameArabic changed",
			old:  base,
			new_: func() models.Store {
				s := base
				s.NationalAddress.DistrictNameArabic = "العليا"
				return s
			}(),
			wantTrue: true,
		},
		{
			name: "NationalAddress.CityNameArabic changed",
			old:  base,
			new_: func() models.Store {
				s := base
				s.NationalAddress.CityNameArabic = "جدة"
				return s
			}(),
			wantTrue: true,
		},
		{
			name: "Non-ZATCA fields (phone, email, title) changed",
			old:  base,
			new_: func() models.Store {
				s := base
				s.Phone = "0551234567"
				s.Email = "new@example.com"
				s.Title = "Tax Invoice"
				return s
			}(),
			wantTrue: false,
		},

		// ── Serial numbers — admin-only gate ──────────────────────────────────
		{
			name: "SalesSerialNumber.Prefix changed isAdmin=true",
			old:  base,
			new_: func() models.Store {
				s := base
				s.SalesSerialNumber.Prefix = "SI"
				return s
			}(),
			isAdmin:  true,
			wantTrue: true,
		},
		{
			name: "SalesSerialNumber.Prefix changed isAdmin=false",
			old:  base,
			new_: func() models.Store {
				s := base
				s.SalesSerialNumber.Prefix = "SI"
				return s
			}(),
			isAdmin:  false,
			wantTrue: false,
		},
		{
			name: "SalesSerialNumber.PaddingCount changed isAdmin=true",
			old:  base,
			new_: func() models.Store {
				s := base
				s.SalesSerialNumber.PaddingCount = 8
				return s
			}(),
			isAdmin:  true,
			wantTrue: true,
		},
		{
			name: "SalesSerialNumber.StartFromCount changed isAdmin=true",
			old:  base,
			new_: func() models.Store {
				s := base
				s.SalesSerialNumber.StartFromCount = 100
				return s
			}(),
			isAdmin:  true,
			wantTrue: true,
		},
		{
			name: "SalesReturnSerialNumber.Prefix changed isAdmin=true",
			old:  base,
			new_: func() models.Store {
				s := base
				s.SalesReturnSerialNumber.Prefix = "SR"
				return s
			}(),
			isAdmin:  true,
			wantTrue: true,
		},
		{
			name: "SalesReturnSerialNumber.PaddingCount changed isAdmin=true",
			old:  base,
			new_: func() models.Store {
				s := base
				s.SalesReturnSerialNumber.PaddingCount = 7
				return s
			}(),
			isAdmin:  true,
			wantTrue: true,
		},
		{
			name: "SalesReturnSerialNumber.StartFromCount changed isAdmin=true",
			old:  base,
			new_: func() models.Store {
				s := base
				s.SalesReturnSerialNumber.StartFromCount = 50
				return s
			}(),
			isAdmin:  true,
			wantTrue: true,
		},

		// ── Customer deposit serial — EnableZatcaReportingForReceivables gate ─
		{
			name: "Deposit prefix changed isAdmin=true EnableReceivables=true",
			old:  base,
			new_: func() models.Store {
				s := base
				s.CustomerDepositSerialNumber.Prefix = "DP"
				s.Settings.EnableZatcaReportingForReceivables = true
				return s
			}(),
			isAdmin:  true,
			wantTrue: true,
		},
		{
			name: "Deposit prefix changed isAdmin=true EnableReceivables=false",
			old:  base,
			new_: func() models.Store {
				s := base
				s.CustomerDepositSerialNumber.Prefix = "DP"
				s.Settings.EnableZatcaReportingForReceivables = false
				return s
			}(),
			isAdmin:  true,
			wantTrue: false,
		},
		{
			name: "Deposit prefix changed isAdmin=false EnableReceivables=true",
			old:  base,
			new_: func() models.Store {
				s := base
				s.CustomerDepositSerialNumber.Prefix = "DP"
				s.Settings.EnableZatcaReportingForReceivables = true
				return s
			}(),
			isAdmin:  false,
			wantTrue: false,
		},

		// ── Customer withdrawal serial — EnableZatcaReportingForPayables gate ─
		{
			name: "Withdrawal prefix changed isAdmin=true EnablePayables=true",
			old:  base,
			new_: func() models.Store {
				s := base
				s.CustomerWithdrawalSerialNumber.Prefix = "WD"
				s.Settings.EnableZatcaReportingForPayables = true
				return s
			}(),
			isAdmin:  true,
			wantTrue: true,
		},
		{
			name: "Withdrawal prefix changed isAdmin=true EnablePayables=false",
			old:  base,
			new_: func() models.Store {
				s := base
				s.CustomerWithdrawalSerialNumber.Prefix = "WD"
				s.Settings.EnableZatcaReportingForPayables = false
				return s
			}(),
			isAdmin:  true,
			wantTrue: false,
		},
		{
			name: "Withdrawal prefix changed isAdmin=false EnablePayables=true",
			old:  base,
			new_: func() models.Store {
				s := base
				s.CustomerWithdrawalSerialNumber.Prefix = "WD"
				s.Settings.EnableZatcaReportingForPayables = true
				return s
			}(),
			isAdmin:  false,
			wantTrue: false,
		},

		// ── Combination cases ─────────────────────────────────────────────────
		{
			name: "Multiple core fields changed simultaneously",
			old:  base,
			new_: func() models.Store {
				s := base
				s.Name = "A"
				s.VATNo = "V999"
				s.NationalAddress.CityName = "Dammam"
				return s
			}(),
			isAdmin:  false,
			wantTrue: true,
		},
		{
			name: "Core field changed serial unchanged",
			old:  base,
			new_: func() models.Store {
				s := base
				s.Name = "Different"
				// serials identical to base
				return s
			}(),
			isAdmin:  true,
			wantTrue: true,
		},
		{
			name: "No core change admin serial change",
			old:  base,
			new_: func() models.Store {
				s := base
				s.SalesSerialNumber.Prefix = "XX"
				return s
			}(),
			isAdmin:  true,
			wantTrue: true,
		},
		{
			name: "No core change non-admin serial change",
			old:  base,
			new_: func() models.Store {
				s := base
				s.SalesSerialNumber.Prefix = "XX"
				return s
			}(),
			isAdmin:  false,
			wantTrue: false,
		},
	}

	for _, c := range cases {
		c := c // capture range variable
		t.Run(c.name, func(t *testing.T) {
			got := zatcaSensitiveFieldsChanged(c.old, c.new_, c.isAdmin)
			if got != c.wantTrue {
				t.Errorf("zatcaSensitiveFieldsChanged() = %v, want %v", got, c.wantTrue)
			}
		})
	}
}

// ── preserveZatcaCredentials ─────────────────────────────────────────────────

func TestPreserveZatcaCredentials(t *testing.T) {
	now := time.Now()
	uid := primitive.NewObjectID()
	old := models.Zatca{
		Phase: "2", Env: "Production", Otp: "123456", PrivateKey: "PK", Csr: "CSR",
		ComplianceRequestID: 11, BinarySecurityToken: "BST", Secret: "S",
		ProductionRequestID: 22, ProductionBinarySecurityToken: "PBST", ProductionSecret: "PS",
		Connected: true, LastConnectedAt: &now, ConnectedBy: &uid, ZatcaReconnectRequired: true,
	}
	cases := []struct {
		name string
		in   models.Zatca
	}{
		{"switch to phase 1 keeps credentials", models.Zatca{Phase: "1", Env: "Production"}},
		{"client tries to wipe credentials", models.Zatca{Phase: "2", Env: "Production", PrivateKey: "", Connected: false}},
		{"client tries to forge credentials", models.Zatca{Phase: "2", Env: "Production", PrivateKey: "EVIL", ProductionSecret: "EVIL", Connected: true, ProductionRequestID: 999}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			z := c.in
			preserveZatcaCredentials(&z, old)
			if z.Phase != c.in.Phase || z.Env != c.in.Env {
				t.Fatalf("phase/env must stay client-editable: %q %q", z.Phase, z.Env)
			}
			if z.PrivateKey != "PK" || z.Csr != "CSR" || z.Secret != "S" || z.BinarySecurityToken != "BST" ||
				z.ProductionSecret != "PS" || z.ProductionBinarySecurityToken != "PBST" ||
				z.ComplianceRequestID != 11 || z.ProductionRequestID != 22 || !z.Connected ||
				z.LastConnectedAt != &now || z.ConnectedBy != &uid || z.Otp != "123456" {
				t.Fatalf("credentials not preserved: %+v", z)
			}
		})
	}
}

// ── ZATCA environment changes ────────────────────────────────────────────────

func TestZatcaEnvChangeError(t *testing.T) {
	docs := func(n int64, err error) func() (int64, error) {
		return func() (int64, error) { return n, err }
	}
	never := func() (int64, error) { t.Fatal("documents must not be counted"); return 0, nil }
	cases := []struct {
		name           string
		oldEnv, newEnv string
		admin          bool
		count          func() (int64, error)
		wantStatus     int
		wantMsg        string
	}{
		{"unchanged", "Production", "Production", false, never, 0, ""},
		{"unchanged with spaces", "Production", " Production ", false, never, 0, ""},
		{"first time set by non-admin", "", "NonProduction", false, never, 0, ""},
		{"first time set by admin", "", "Simulation", true, never, 0, ""},
		{"non-admin change", "NonProduction", "Production", false, never, 403, "Only admins"},
		{"admin change, no documents", "NonProduction", "Production", true, docs(0, nil), 0, ""},
		{"admin change, documents exist", "Simulation", "Production", true, docs(3, nil), 400, "can't be changed"},
		{"admin change, count fails", "Simulation", "Production", true, docs(0, errors.New("db down")), 500, "db down"},
		{"admin clears env with documents", "Production", "", true, docs(1, nil), 400, "can't be changed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			admin := c.admin
			st, field, msg := zatcaEnvChangeError(c.oldEnv, c.newEnv, func() bool { return admin }, c.count)
			if st != c.wantStatus || !strings.Contains(msg, c.wantMsg) || (c.wantMsg == "" && msg != "") {
				t.Fatalf("got %d %q %q, want %d %q", st, field, msg, c.wantStatus, c.wantMsg)
			}
			if msg != "" && field != "zatca_env" {
				t.Fatalf("field = %q", field)
			}
		})
	}
}

func TestCanChangeZatcaEnv(t *testing.T) {
	role := func(r string) func() string { return func() string { return r } }
	noLookup := func() string { t.Fatal("erp role must not be looked up for platform admins"); return "" }
	for _, c := range []struct {
		name string
		u    *models.User
		erp  func() string
		want bool
	}{
		{"nil user", nil, role("r_admin"), false},
		{"platform admin role", &models.User{Role: "Admin"}, noLookup, true},
		{"platform admin role lowercase", &models.User{Role: "admin"}, noLookup, true},
		{"platform admin flag", &models.User{Admin: true, Role: "Manager"}, noLookup, true},
		{"store Administrator", &models.User{Role: "Manager"}, role("r_admin"), true},
		{"store manager", &models.User{Role: "Manager"}, role("r_manager"), false},
		{"no erp role", &models.User{Role: "Manager"}, role(""), false},
		{"salesman", &models.User{Role: "SalesMan"}, role("r_salesman"), false},
		{"no lookup available", &models.User{Role: "Manager"}, nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := canChangeZatcaEnv(c.u, c.erp); got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

package controller

import (
	"testing"

	"github.com/sirinibin/startpos/backend/models"
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

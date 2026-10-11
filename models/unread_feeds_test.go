package models

import "testing"

func TestUnreadFeedsEnabled(t *testing.T) {
	cases := []struct {
		name         string
		s            StoreSettings
		wantWhatsApp bool
		wantEmail    bool
	}{
		{"zero settings", StoreSettings{}, false, false},
		{"module off, everything connected", StoreSettings{
			StoreRFQWABAPhoneNumberID: "111", BotWABAPhoneNumberID: "222",
			RFQEmailConnected: true, RFQEmailAccounts: []RFQEmailAccount{{Email: "a@b.c"}},
		}, false, false},
		{"module on, nothing connected", StoreSettings{EnableRFQModule: true}, false, false},
		{"module on, whitespace phone id", StoreSettings{EnableRFQModule: true, StoreRFQWABAPhoneNumberID: "  "}, false, false},
		{"module on, store RFQ number", StoreSettings{EnableRFQModule: true, StoreRFQWABAPhoneNumberID: "111"}, true, false},
		{"module on, bot number", StoreSettings{EnableRFQModule: true, BotWABAPhoneNumberID: "222"}, true, false},
		{"module on, legacy Evolution number only", StoreSettings{EnableRFQModule: true, StoreRFQWhatsAppPhone: "9665"}, false, false},
		{"module on, legacy email flag", StoreSettings{EnableRFQModule: true, RFQEmailConnected: true}, false, true},
		{"module on, email account", StoreSettings{EnableRFQModule: true, RFQEmailAccounts: []RFQEmailAccount{{Email: "a@b.c"}}}, false, true},
		{"module on, empty account list", StoreSettings{EnableRFQModule: true, RFQEmailAccounts: []RFQEmailAccount{}}, false, false},
		{"module on, both", StoreSettings{EnableRFQModule: true, BotWABAPhoneNumberID: "222", RFQEmailConnected: true}, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wa, em := UnreadFeedsEnabled(c.s)
			if wa != c.wantWhatsApp || em != c.wantEmail {
				t.Fatalf("UnreadFeedsEnabled = (%v, %v), want (%v, %v)", wa, em, c.wantWhatsApp, c.wantEmail)
			}
		})
	}
}

func TestUnreadFeedSettingsProjection_CoversEveryFieldRead(t *testing.T) {
	for _, k := range []string{
		"settings.enable_rfq_module",
		"settings.store_rfq_waba_phone_number_id",
		"settings.bot_waba_phone_number_id",
		"settings.rfq_email_connected",
		"settings.rfq_email_accounts.email",
	} {
		if _, ok := unreadFeedSettingsProjection[k]; !ok {
			t.Errorf("projection is missing %s", k)
		}
	}
}

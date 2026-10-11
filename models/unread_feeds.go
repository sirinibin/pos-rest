package models

import (
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// unreadFeedSettingsProjection loads only the settings UnreadFeedsEnabled reads.
var unreadFeedSettingsProjection = bson.M{
	"settings.enable_rfq_module":              1,
	"settings.store_rfq_waba_phone_number_id": 1,
	"settings.bot_waba_phone_number_id":       1,
	"settings.rfq_email_connected":            1,
	"settings.rfq_email_accounts.email":       1,
}

// UnreadFeedsEnabled reports whether the header's WhatsApp and email unread
// feeds can have anything to show for a store: both need the RFQ module on,
// WhatsApp needs a Meta Cloud API number, email a connected account.
// reactjs-pos src/utils/unreadFeeds.js applies the same rule before polling.
func UnreadFeedsEnabled(s StoreSettings) (whatsapp, email bool) {
	if !s.EnableRFQModule {
		return false, false
	}
	whatsapp = strings.TrimSpace(s.StoreRFQWABAPhoneNumberID) != "" ||
		strings.TrimSpace(s.BotWABAPhoneNumberID) != ""
	email = s.RFQEmailConnected || len(s.RFQEmailAccounts) > 0
	return whatsapp, email
}

// FindStoreUnreadFeeds loads a store's settings (only the fields needed) and
// applies UnreadFeedsEnabled.
func FindStoreUnreadFeeds(storeID primitive.ObjectID) (whatsapp, email bool, err error) {
	store, err := FindStoreByID(&storeID, unreadFeedSettingsProjection)
	if err != nil {
		return false, false, err
	}
	whatsapp, email = UnreadFeedsEnabled(store.Settings)
	return whatsapp, email, nil
}

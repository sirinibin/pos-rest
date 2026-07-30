package models

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ValidateOpeningBalanceDateForAccount checks that the proposed opening balance
// date is at least 1 second before the earliest non-opening-balance posting
// already recorded for the account identified by accountReferenceID. Returns
// a non-nil error with a human-readable message if the constraint is violated.
// If the account has no such postings, or does not exist yet, it returns nil.
// ValidateOpeningBalanceDateForAccountID checks that openingBalanceDate is before
// the earliest non-opening-balance posting for the given account ObjectID.
func (store *Store) ValidateOpeningBalanceDateForAccountID(
	accountID primitive.ObjectID,
	openingBalanceDate *time.Time,
	excludeReferenceModels []string,
) error {
	if openingBalanceDate == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	excludeFilter := bson.A{}
	for _, m := range excludeReferenceModels {
		excludeFilter = append(excludeFilter, m)
	}

	filter := bson.M{
		"account_id": accountID,
		"store_id":   store.ID,
	}
	if len(excludeFilter) > 0 {
		filter["reference_model"] = bson.M{"$nin": excludeFilter}
	}

	findOpts := options.FindOne().SetSort(bson.D{bson.E{Key: "date", Value: 1}})
	var earliest struct {
		Date *time.Time `bson:"date"`
	}
	err := db.GetDB("store_"+store.ID.Hex()).Collection("posting").
		FindOne(ctx, filter, findOpts).Decode(&earliest)
	if err != nil || earliest.Date == nil {
		return nil
	}

	if !openingBalanceDate.Before(*earliest.Date) {
		return fmt.Errorf(
			"opening balance date must be before the earliest existing transaction (%s)",
			earliest.Date.Format("2006-01-02 15:04:05"),
		)
	}
	return nil
}

func (store *Store) ValidateOpeningBalanceDateForAccount(
	accountReferenceID primitive.ObjectID,
	openingBalanceDate *time.Time,
	excludeReferenceModels []string,
) error {
	if openingBalanceDate == nil {
		return nil
	}

	account, err := store.FindAccountByReferenceID(accountReferenceID, store.ID, bson.M{})
	if err != nil || account == nil {
		return nil
	}

	return store.ValidateOpeningBalanceDateForAccountID(account.ID, openingBalanceDate, excludeReferenceModels)
}

// ValidateOpeningBalanceDateByAccountName is like ValidateOpeningBalanceDateForAccount
// but looks up the account by name instead of reference_id (used for Cash/Bank store accounts).
func (store *Store) ValidateOpeningBalanceDateByAccountName(
	accountName string,
	openingBalanceDate *time.Time,
	excludeReferenceModels []string,
) error {
	if openingBalanceDate == nil {
		return nil
	}

	account, err := store.FindAccountByName(accountName, &store.ID, nil, bson.M{})
	if err != nil || account == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	excludeFilter := bson.A{}
	for _, m := range excludeReferenceModels {
		excludeFilter = append(excludeFilter, m)
	}

	filter := bson.M{
		"account_id": account.ID,
		"store_id":   store.ID,
	}
	if len(excludeFilter) > 0 {
		filter["reference_model"] = bson.M{"$nin": excludeFilter}
	}

	findOpts := options.FindOne().SetSort(bson.D{bson.E{Key: "date", Value: 1}})
	var earliest struct {
		Date *time.Time `bson:"date"`
	}
	err = db.GetDB("store_"+store.ID.Hex()).Collection("posting").
		FindOne(ctx, filter, findOpts).Decode(&earliest)
	if err != nil || earliest.Date == nil {
		return nil
	}

	if !openingBalanceDate.Before(*earliest.Date) {
		return fmt.Errorf(
			"opening balance date must be before the earliest existing transaction (%s)",
			earliest.Date.Format("2006-01-02 15:04:05"),
		)
	}
	return nil
}

const customerOpeningBalanceReferenceModel = "customer_opening_balance"
const vendorOpeningBalanceReferenceModel = "vendor_opening_balance"
const userOpeningBalanceReferenceModel = "user_opening_balance"
const entityOpeningBalanceReferenceCode = "OPENING-BALANCE"

// removeEntityOpeningBalanceLedger removes ledger + posting docs for an entity's
// opening balance and rebuilds that account's running balances.
func (store *Store) removeEntityOpeningBalanceLedger(
	refModel string,
	entityID primitive.ObjectID,
	accountReferenceID *primitive.ObjectID,
) error {
	ctx := context.Background()
	storeDB := db.GetDB("store_" + store.ID.Hex())

	if _, err := storeDB.Collection("ledger").DeleteMany(ctx, bson.M{
		"reference_id":    entityID,
		"reference_model": refModel,
		"store_id":        store.ID,
	}); err != nil {
		return err
	}

	if _, err := storeDB.Collection("posting").DeleteMany(ctx, bson.M{
		"reference_id":    entityID,
		"reference_model": refModel,
		"store_id":        store.ID,
	}); err != nil {
		return err
	}

	if accountReferenceID == nil {
		return nil
	}

	// Recalculate balance only if the account already exists.
	account, err := store.FindAccountByReferenceID(*accountReferenceID, store.ID, bson.M{})
	if err != nil || account == nil {
		return nil
	}
	account.CalculateBalance(nil, nil)
	account.Update()
	return RebuildAccountPostingBalances(store, account.ID)
}

// createEntityOpeningBalanceLedger inserts the double-entry opening-balance
// ledger for a customer, vendor, or user account.
//
//	normal  (receivable / store-owes-vendor): DR entityAccount / CR OPENING BALANCE EQUITY
//	flipped (payable   / store-owes-customer): DR OPENING BALANCE EQUITY / CR entityAccount
//
// drEntity == true means the entity account is debited (DR entity / CR equity).
func (store *Store) createEntityOpeningBalanceLedger(
	refModel string,
	entityID primitive.ObjectID,
	entityAccountName string,
	entityAccountReferenceID *primitive.ObjectID,
	entityAccountReferenceModel *string,
	entityPhone *string,
	entityVATNo *string,
	amount float64,
	date *time.Time,
	drEntity bool,
) error {
	storeIDCopy := store.ID

	entityAccount, err := store.CreateAccountIfNotExists(
		&storeIDCopy,
		entityAccountReferenceID,
		entityAccountReferenceModel,
		entityAccountName,
		entityPhone,
		entityVATNo,
	)
	if err != nil {
		return errors.New("error getting entity account: " + err.Error())
	}

	equityAccount, err := store.CreateAccountIfNotExists(&storeIDCopy, nil, nil, "OPENING BALANCE EQUITY", nil, nil)
	if err != nil {
		return errors.New("error getting OPENING BALANCE EQUITY account: " + err.Error())
	}

	now := time.Now()
	entryDate := date
	if entryDate == nil {
		entryDate = &now
	}

	groupID := primitive.NewObjectID()
	var journals []Journal
	if drEntity {
		journals = []Journal{
			{
				Date:          entryDate,
				AccountID:     entityAccount.ID,
				AccountName:   entityAccount.Name,
				AccountNumber: entityAccount.Number,
				DebitOrCredit: "debit",
				Debit:         amount,
				GroupID:       groupID,
			},
			{
				Date:          entryDate,
				AccountID:     equityAccount.ID,
				AccountName:   equityAccount.Name,
				AccountNumber: equityAccount.Number,
				DebitOrCredit: "credit",
				Credit:        amount,
				GroupID:       groupID,
			},
		}
	} else {
		journals = []Journal{
			{
				Date:          entryDate,
				AccountID:     equityAccount.ID,
				AccountName:   equityAccount.Name,
				AccountNumber: equityAccount.Number,
				DebitOrCredit: "debit",
				Debit:         amount,
				GroupID:       groupID,
			},
			{
				Date:          entryDate,
				AccountID:     entityAccount.ID,
				AccountName:   entityAccount.Name,
				AccountNumber: entityAccount.Number,
				DebitOrCredit: "credit",
				Credit:        amount,
				GroupID:       groupID,
			},
		}
	}

	ledger := &Ledger{
		StoreID:        &storeIDCopy,
		ReferenceID:    entityID,
		ReferenceModel: refModel,
		ReferenceCode:  entityOpeningBalanceReferenceCode,
		Journals:       journals,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}

	if err := ledger.Insert(); err != nil {
		return errors.New("error inserting opening balance ledger: " + err.Error())
	}
	if _, err := ledger.CreatePostings(); err != nil {
		return errors.New("error creating opening balance postings: " + err.Error())
	}

	entityAccount.CalculateBalance(nil, nil)
	entityAccount.Update()
	if err := RebuildAccountPostingBalances(store, entityAccount.ID); err != nil {
		return errors.New("error rebuilding posting balances: " + err.Error())
	}

	return nil
}

// ── Customer ─────────────────────────────────────────────────────────────────

// PostCustomerOpeningBalanceIfNeeded (re-)posts the one-time opening-balance
// ledger for the customer. Safe to call on every create/update (idempotent).
//
// "receivable" (default) — customer owes store: DR Customer / CR Equity
// "payable"              — store owes customer:  DR Equity  / CR Customer
func (customer *Customer) PostCustomerOpeningBalanceIfNeeded(store *Store) error {
	if err := store.removeEntityOpeningBalanceLedger(
		customerOpeningBalanceReferenceModel,
		customer.ID,
		&customer.ID,
	); err != nil {
		return errors.New("error removing customer opening balance: " + err.Error())
	}

	if customer.OpeningBalance <= 0 {
		customer.OpeningBalancePosted = false
		return nil
	}

	referenceModel := "customer"
	// "receivable" = customer owes store → DR customer account (asset)
	drEntity := customer.OpeningBalanceType != "payable"

	if err := store.createEntityOpeningBalanceLedger(
		customerOpeningBalanceReferenceModel,
		customer.ID,
		customer.Name,
		&customer.ID,
		&referenceModel,
		&customer.Phone,
		&customer.VATNo,
		customer.OpeningBalance,
		customer.OpeningBalanceDate,
		drEntity,
	); err != nil {
		return err
	}

	customer.OpeningBalancePosted = true
	return nil
}

// ── Vendor ────────────────────────────────────────────────────────────────────

// PostVendorOpeningBalanceIfNeeded (re-)posts the one-time opening-balance
// ledger for the vendor. Safe to call on every create/update (idempotent).
//
// "payable" (default)  — store owes vendor:  DR Equity  / CR Vendor
// "receivable"         — vendor owes store:  DR Vendor  / CR Equity
func (vendor *Vendor) PostVendorOpeningBalanceIfNeeded(store *Store) error {
	if err := store.removeEntityOpeningBalanceLedger(
		vendorOpeningBalanceReferenceModel,
		vendor.ID,
		&vendor.ID,
	); err != nil {
		return errors.New("error removing vendor opening balance: " + err.Error())
	}

	if vendor.OpeningBalance <= 0 {
		vendor.OpeningBalancePosted = false
		return nil
	}

	referenceModel := "vendor"
	// "receivable" = vendor owes store → DR vendor account
	drEntity := vendor.OpeningBalanceType == "receivable"

	if err := store.createEntityOpeningBalanceLedger(
		vendorOpeningBalanceReferenceModel,
		vendor.ID,
		vendor.Name,
		&vendor.ID,
		&referenceModel,
		&vendor.Phone,
		&vendor.VATNo,
		vendor.OpeningBalance,
		vendor.OpeningBalanceDate,
		drEntity,
	); err != nil {
		return err
	}

	vendor.OpeningBalancePosted = true
	return nil
}

// ── User ──────────────────────────────────────────────────────────────────────

func (user *User) userAccountName() string {
	return "USER: " + user.Name
}

// PostUserOpeningBalanceIfNeeded (re-)posts the one-time opening-balance
// ledger for the user. Requires user.StoreID to be set.
//
// "payable" (default)  — store owes user:  DR Equity / CR User
// "receivable"         — user owes store:  DR User   / CR Equity
func (user *User) PostUserOpeningBalanceIfNeeded(store *Store) error {
	if err := store.removeEntityOpeningBalanceLedger(
		userOpeningBalanceReferenceModel,
		user.ID,
		&user.ID,
	); err != nil {
		return errors.New("error removing user opening balance: " + err.Error())
	}

	if user.OpeningBalance <= 0 {
		user.OpeningBalancePosted = false
		if acc, err := store.FindAccountByReferenceID(user.ID, store.ID, bson.M{}); err == nil && acc != nil {
			acc.CalculateBalance(nil, nil)
			acc.Update()
			user.Account = acc
		}
		return nil
	}

	referenceModel := "user"
	// "receivable" = user owes store → DR user account
	drEntity := user.OpeningBalanceType == "receivable"

	if err := store.createEntityOpeningBalanceLedger(
		userOpeningBalanceReferenceModel,
		user.ID,
		user.userAccountName(),
		&user.ID,
		&referenceModel,
		nil,
		nil,
		user.OpeningBalance,
		user.OpeningBalanceDate,
		drEntity,
	); err != nil {
		return err
	}

	user.OpeningBalancePosted = true

	// Refresh account snapshot on the user
	if acc, err := store.FindAccountByReferenceID(user.ID, store.ID, bson.M{}); err == nil && acc != nil {
		user.Account = acc
	}

	return nil
}

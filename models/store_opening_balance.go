package models

import (
	"context"
	"errors"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const cashOpeningBalanceReferenceModel = "cash_opening_balance"
const bankOpeningBalanceReferenceModel = "bank_opening_balance"
const openingBalanceReferenceCode = "OPENING-BALANCE"

// TimesEqual returns true when both pointers are nil or both point to equal times.
func TimesEqual(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

// removeOpeningBalanceLedger deletes the ledger + posting docs for a given
// reference model (cash_opening_balance or bank_opening_balance), then
// recalculates the affected account's balance and rebuilds running totals.
// If the account does not exist yet, it skips balance recalculation rather
// than creating an unnecessary account record.
func (store *Store) removeOpeningBalanceLedger(refModel string, accountName string) error {
	ctx := context.Background()
	storeDB := db.GetDB("store_" + store.ID.Hex())

	if _, err := storeDB.Collection("ledger").DeleteMany(ctx, bson.M{
		"reference_model": refModel,
		"store_id":        store.ID,
	}); err != nil {
		return err
	}

	if _, err := storeDB.Collection("posting").DeleteMany(ctx, bson.M{
		"reference_model": refModel,
		"store_id":        store.ID,
	}); err != nil {
		return err
	}

	// Only recalculate if the account already exists — do not create it
	// just because we are removing an opening balance entry.
	account, err := store.FindAccountByName(accountName, &store.ID, nil, bson.M{})
	if err != nil || account == nil {
		return nil
	}
	account.CalculateBalance(nil, nil)
	account.Update()
	return RebuildAccountPostingBalances(store, account.ID)
}

// createOpeningBalanceLedger inserts the double-entry opening-balance ledger:
//
//	DR <assetAccountName>  /  CR OPENING BALANCE EQUITY
//
// The ReferenceID is the store's own ID so it is unique per store.
func (store *Store) createOpeningBalanceLedger(
	refModel string,
	assetAccountName string,
	amount float64,
	date *time.Time,
) error {
	storeIDCopy := store.ID

	assetAccount, err := store.CreateAccountIfNotExists(&storeIDCopy, nil, nil, assetAccountName, nil, nil)
	if err != nil {
		return errors.New("error getting " + assetAccountName + " account: " + err.Error())
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
	journals := []Journal{
		{
			Date:          entryDate,
			AccountID:     assetAccount.ID,
			AccountName:   assetAccount.Name,
			AccountNumber: assetAccount.Number,
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

	ledger := &Ledger{
		StoreID:        &storeIDCopy,
		ReferenceID:    store.ID,
		ReferenceModel: refModel,
		ReferenceCode:  openingBalanceReferenceCode,
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

	assetAccount.CalculateBalance(nil, nil)
	assetAccount.Update()

	if err := RebuildAccountPostingBalances(store, assetAccount.ID); err != nil {
		return errors.New("error rebuilding posting balances: " + err.Error())
	}

	return nil
}

// PostCashOpeningBalanceIfNeeded removes any existing cash opening-balance
// ledger entry and re-posts it if CashOpeningBalance > 0. Safe to call on
// every create/update (idempotent).
func (store *Store) PostCashOpeningBalanceIfNeeded() error {
	if err := store.removeOpeningBalanceLedger(cashOpeningBalanceReferenceModel, "Cash"); err != nil {
		return errors.New("error removing cash opening balance: " + err.Error())
	}

	if store.Settings.CashOpeningBalance <= 0 {
		return nil
	}

	return store.createOpeningBalanceLedger(
		cashOpeningBalanceReferenceModel,
		"Cash",
		store.Settings.CashOpeningBalance,
		store.Settings.CashOpeningBalanceDate,
	)
}

// PostBankOpeningBalanceIfNeeded removes any existing bank opening-balance
// ledger entry and re-posts it if BankOpeningBalance > 0. Safe to call on
// every create/update (idempotent).
func (store *Store) PostBankOpeningBalanceIfNeeded() error {
	if err := store.removeOpeningBalanceLedger(bankOpeningBalanceReferenceModel, "Bank"); err != nil {
		return errors.New("error removing bank opening balance: " + err.Error())
	}

	if store.Settings.BankOpeningBalance <= 0 {
		return nil
	}

	return store.createOpeningBalanceLedger(
		bankOpeningBalanceReferenceModel,
		"Bank",
		store.Settings.BankOpeningBalance,
		store.Settings.BankOpeningBalanceDate,
	)
}

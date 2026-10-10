package models

import (
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Capital withdrawals used to be saved without any ledger entry, so the
// owner's capital account and the cash/bank balances ignored them. A
// withdrawal is the reverse of a capital investment: debit the owner's
// Capital account, credit Cash or Bank.

func (capitalwithdrawal *CapitalWithdrawal) DoAccounting() error {
	ledger, err := capitalwithdrawal.CreateLedger()
	if err != nil {
		return err
	}
	_, err = ledger.CreatePostings()
	return err
}

func (capitalwithdrawal *CapitalWithdrawal) UndoAccounting() error {
	store, err := FindStoreByID(capitalwithdrawal.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledger, err := store.FindLedgerByReferenceID(capitalwithdrawal.ID, *capitalwithdrawal.StoreID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	ledgerAccounts := map[string]Account{}
	if ledger != nil {
		ledgerAccounts, err = ledger.GetRelatedAccounts()
		if err != nil {
			return err
		}
	}

	if err := store.RemoveLedgerByReferenceID(capitalwithdrawal.ID); err != nil {
		return err
	}
	if err := store.RemovePostingsByReferenceID(capitalwithdrawal.ID); err != nil {
		return err
	}
	return SetAccountBalances(ledgerAccounts)
}

func (capitalwithdrawal *CapitalWithdrawal) CreateLedger() (ledger *Ledger, err error) {
	store, err := FindStoreByID(capitalwithdrawal.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()

	owner, err := FindUserByID(capitalwithdrawal.WithdrawnByUserID, bson.M{})
	if err != nil {
		return nil, err
	}

	// Same account a capital investment by this user credits.
	referenceModel := "investor"
	capitalAccount, err := store.CreateAccountIfNotExists(
		capitalwithdrawal.StoreID,
		&owner.ID,
		&referenceModel,
		owner.Name+" Capital",
		&owner.Mob,
		nil,
	)
	if err != nil {
		return nil, err
	}

	cashAccount, err := store.CreateAccountIfNotExists(capitalwithdrawal.StoreID, nil, nil, "Cash", nil, nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := store.CreateAccountIfNotExists(capitalwithdrawal.StoreID, nil, nil, "Bank", nil, nil)
	if err != nil {
		return nil, err
	}

	var payingAccount Account
	if capitalwithdrawal.PaymentMethod == "cash" {
		payingAccount = *cashAccount
	} else if slices.Contains(BANK_PAYMENT_METHODS, capitalwithdrawal.PaymentMethod) {
		payingAccount = *bankAccount
	}

	groupID := primitive.NewObjectID()
	journals := []Journal{
		{
			Date:          capitalwithdrawal.Date,
			AccountID:     capitalAccount.ID,
			AccountNumber: capitalAccount.Number,
			AccountName:   capitalAccount.Name,
			DebitOrCredit: "debit",
			Debit:         capitalwithdrawal.Amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		},
		{
			Date:          capitalwithdrawal.Date,
			AccountID:     payingAccount.ID,
			AccountNumber: payingAccount.Number,
			AccountName:   payingAccount.Name,
			DebitOrCredit: "credit",
			Credit:        capitalwithdrawal.Amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		},
	}

	ledger = &Ledger{
		StoreID:        capitalwithdrawal.StoreID,
		ReferenceID:    capitalwithdrawal.ID,
		ReferenceModel: "capital_withdrawal",
		ReferenceCode:  capitalwithdrawal.Code,
		Journals:       journals,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}

	if err := ledger.Insert(); err != nil {
		return nil, err
	}
	return ledger, nil
}

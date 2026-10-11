package models

import (
	"bytes"
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/exp/slices"
)

// One-off ledger repair (2026-10-11).
//
// Deleting a customer deposit, customer withdrawal, capital or dividend used
// to leave its ledger and postings behind, and capital withdrawals were never
// posted at all. The code paths are fixed; this repairs the data they left:
//
//   - a deleted document of those types (and a deleted capital withdrawal)
//     that still has a ledger or postings gets exactly what a delete does
//     today: UndoAccounting, then for deposits / withdrawals SetPartyBalances;
//   - a capital withdrawal that is not deleted and has no ledger gets exactly
//     what a create does today: DoAccounting (after UndoAccounting, as an
//     edit does, so stray postings without a ledger are not doubled).
//
// apply=false (dry run) only reads. apply=true holds the store's write lock
// (HoldStore, as the delete / create handlers do) and, BEFORE changing
// anything, copies every ledger, posting, account and party balance it is
// going to remove or modify into the store's erp_repair_backup collection.
// What it adds (the new capital withdrawal ledgers, postings and accounts) is
// recorded there too, so RestoreLedgerRepair can put the store back.
//
// It is idempotent by construction: a repaired document no longer matches
// either category, so a second run finds nothing to do and writes nothing.
// The marker document only records the runs.

// LedgerRepairTag names this repair in erp_repair_backup.
const LedgerRepairTag = "ledger-2026-10-11"

const ledgerRepairBackupCollection = "erp_repair_backup"

// ledgerRepairMarkerID is the per-store marker document's _id.
const ledgerRepairMarkerID = LedgerRepairTag + ":marker"

// Backup record kinds.
const (
	repairKindRemoved  = "removed"  // a document the repair deleted (ledger, posting)
	repairKindModified = "modified" // the pre-image of a document the repair changed (account)
	repairKindParty    = "party"    // the balance fields of a customer / vendor the repair changed
	repairKindAdded    = "added"    // a document the repair created (ledger, posting, account)
)

// ledgerRepairTypes are the document types whose delete now undoes the
// accounting: report key → store collection.
var ledgerRepairTypes = []struct{ Key, Collection string }{
	{"customer_deposit", "customerdeposit"},
	{"customer_withdrawal", "customerwithdrawal"},
	{"capital", "capital"},
	{"dividend", "divident"},
	{"capital_withdrawal", "capitalwithdrawal"},
}

// partyBalanceFields are the customer / vendor fields SetCreditBalance writes.
var partyBalanceFields = []string{"credit_balance", "account"}

// RepairDocRef is one document of the report.
type RepairDocRef struct {
	Type   string  `json:"type"`
	ID     string  `json:"id"`
	Code   string  `json:"code"`
	Amount float64 `json:"amount"`
	Reason string  `json:"reason,omitempty"`
}

// RepairAccountChange is an account the repair changes, with its balance
// before and after (after = predicted on a dry run, read back on apply).
type RepairAccountChange struct {
	ID                string  `json:"id,omitempty"`
	Number            string  `json:"number,omitempty"`
	Name              string  `json:"name"`
	New               bool    `json:"new"`
	BalanceBefore     float64 `json:"balanceBefore"`
	BalanceAfter      float64 `json:"balanceAfter"`
	DebitTotalBefore  float64 `json:"debitTotalBefore"`
	DebitTotalAfter   float64 `json:"debitTotalAfter"`
	CreditTotalBefore float64 `json:"creditTotalBefore"`
	CreditTotalAfter  float64 `json:"creditTotalAfter"`
}

// LedgerRepairReport is the result for one store.
type LedgerRepairReport struct {
	Repair    string `json:"repair"`
	StoreID   string `json:"storeId"`
	StoreName string `json:"storeName"`
	Apply     bool   `json:"apply"`
	// deleted documents that still have a ledger / postings (category a)
	DeletedWithLedger []RepairDocRef `json:"deletedWithLedger"`
	// capital withdrawals that are not deleted and have no ledger (category b)
	MissingLedger []RepairDocRef `json:"missingLedger"`
	// capital withdrawals that cannot be posted (no investor): left alone
	Skipped  []RepairDocRef        `json:"skipped"`
	Counts   map[string]int        `json:"counts"`
	Accounts []RepairAccountChange `json:"accounts"`
	// apply only
	RunID      string   `json:"runId,omitempty"`
	BackupDocs int      `json:"backupDocs"`
	Errors     []string `json:"errors"`
}

// NothingToDo reports whether the store needs no repair.
func (r *LedgerRepairReport) NothingToDo() bool {
	return len(r.DeletedWithLedger) == 0 && len(r.MissingLedger) == 0
}

// LedgerRepairRun is the result of a run over one or all stores.
type LedgerRepairRun struct {
	Repair string               `json:"repair"`
	Apply  bool                 `json:"apply"`
	Stores []LedgerRepairReport `json:"stores"`
	Totals map[string]int       `json:"totals"`
}

// ledgerRepairItem is one document to repair.
type ledgerRepairItem struct {
	ref     RepairDocRef
	id      primitive.ObjectID
	missing bool // capital withdrawal to post (else: deleted document to undo)
}

func repairCtx(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

func (store *Store) repairDB() *mongo.Database {
	return db.GetDB("store_" + store.ID.Hex())
}

// referencedIDs returns which of ids have a document in collection with that
// reference_id.
func (store *Store) referencedIDs(collection string, ids []primitive.ObjectID) (map[primitive.ObjectID]bool, error) {
	out := map[primitive.ObjectID]bool{}
	for start := 0; start < len(ids); start += 1000 {
		end := start + 1000
		if end > len(ids) {
			end = len(ids)
		}
		ctx, cancel := repairCtx(60 * time.Second)
		vals, err := store.repairDB().Collection(collection).Distinct(ctx, "reference_id", bson.M{"reference_id": bson.M{"$in": ids[start:end]}})
		cancel()
		if err != nil {
			return nil, err
		}
		for _, v := range vals {
			if oid, ok := v.(primitive.ObjectID); ok {
				out[oid] = true
			}
		}
	}
	return out, nil
}

type repairDocHead struct {
	ID     primitive.ObjectID `bson:"_id"`
	Code   string             `bson:"code"`
	Amount float64            `bson:"amount"`
}

func (store *Store) repairDocHeads(collection string, filter bson.M) ([]repairDocHead, error) {
	ctx, cancel := repairCtx(120 * time.Second)
	defer cancel()
	cur, err := store.repairDB().Collection(collection).Find(ctx, filter,
		options.Find().SetProjection(bson.M{"_id": 1, "code": 1, "amount": 1, "net_total": 1}).SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []repairDocHead
	for cur.Next(ctx) {
		var h struct {
			ID     primitive.ObjectID `bson:"_id"`
			Code   string             `bson:"code"`
			Amount interface{}        `bson:"amount"`
			Net    interface{}        `bson:"net_total"` // deposits / withdrawals
		}
		if err := cur.Decode(&h); err != nil {
			return nil, err
		}
		amount := repairNum(h.Amount)
		if amount == 0 {
			amount = repairNum(h.Net)
		}
		out = append(out, repairDocHead{ID: h.ID, Code: h.Code, Amount: amount})
	}
	return out, cur.Err()
}

func repairNum(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case primitive.Decimal128:
		x, _ := strconv.ParseFloat(t.String(), 64)
		return x
	}
	return 0
}

// findLedgerRepairItems scans the store (read only).
func (store *Store) findLedgerRepairItems() (items []ledgerRepairItem, skipped []RepairDocRef, err error) {
	// (a) deleted documents that still have a ledger or postings
	for _, t := range ledgerRepairTypes {
		heads, err := store.repairDocHeads(t.Collection, bson.M{"deleted": true})
		if err != nil {
			return nil, nil, err
		}
		if len(heads) == 0 {
			continue
		}
		ids := make([]primitive.ObjectID, len(heads))
		for i, h := range heads {
			ids[i] = h.ID
		}
		withLedger, err := store.referencedIDs("ledger", ids)
		if err != nil {
			return nil, nil, err
		}
		withPostings, err := store.referencedIDs("posting", ids)
		if err != nil {
			return nil, nil, err
		}
		for _, h := range heads {
			if withLedger[h.ID] || withPostings[h.ID] {
				items = append(items, ledgerRepairItem{id: h.ID,
					ref: RepairDocRef{Type: t.Key, ID: h.ID.Hex(), Code: h.Code, Amount: h.Amount}})
			}
		}
	}

	// (b) capital withdrawals that are not deleted and have no ledger
	heads, err := store.repairDocHeads("capitalwithdrawal", bson.M{"deleted": bson.M{"$ne": true}})
	if err != nil {
		return nil, nil, err
	}
	if len(heads) > 0 {
		ids := make([]primitive.ObjectID, len(heads))
		for i, h := range heads {
			ids[i] = h.ID
		}
		withLedger, err := store.referencedIDs("ledger", ids)
		if err != nil {
			return nil, nil, err
		}
		for _, h := range heads {
			if withLedger[h.ID] {
				continue
			}
			ref := RepairDocRef{Type: "capital_withdrawal", ID: h.ID.Hex(), Code: h.Code, Amount: h.Amount}
			if reason := store.capitalWithdrawalUnpostable(h.ID); reason != "" {
				ref.Reason = reason
				skipped = append(skipped, ref)
				continue
			}
			items = append(items, ledgerRepairItem{id: h.ID, missing: true, ref: ref})
		}
	}
	return items, skipped, nil
}

// capitalWithdrawalUnpostable is why CreateLedger would refuse the capital
// withdrawal ("" when it can be posted).
func (store *Store) capitalWithdrawalUnpostable(id primitive.ObjectID) string {
	cw, err := store.FindCapitalWithdrawalByID(&id, bson.M{})
	if err != nil || cw == nil {
		return "capital withdrawal not readable"
	}
	if cw.WithdrawnByUserID == nil || cw.WithdrawnByUserID.IsZero() {
		return "capital withdrawal has no investor"
	}
	if _, err := FindUserByID(cw.WithdrawnByUserID, bson.M{"_id": 1}); err != nil {
		return "investor " + cw.WithdrawnByUserID.Hex() + " not found"
	}
	return ""
}

// ---- prediction (dry run) ----

type repairTotals struct{ debit, credit float64 }

func repairBalance(t repairTotals) float64 {
	d, c := RoundFloat(t.debit, 2), RoundFloat(t.credit, 2)
	return RoundFloat(math.Abs(c-d), 2)
}

// capitalWithdrawalAccounts resolves the accounts CreateLedger would use
// without creating any: drawing, cash and bank (nil = would be created).
type cwAccounts struct {
	drawingName         string
	drawing, cash, bank *Account
	paying              string // "cash", "bank" or ""
	amount              float64
	cashName, bankName  string
}

func (store *Store) resolveCapitalWithdrawalAccounts(id primitive.ObjectID) (*cwAccounts, error) {
	cw, err := store.FindCapitalWithdrawalByID(&id, bson.M{})
	if err != nil {
		return nil, err
	}
	investor, err := FindUserByID(cw.WithdrawnByUserID, bson.M{})
	if err != nil {
		return nil, err
	}
	r := &cwAccounts{amount: cw.Amount, cashName: "CASH", bankName: "BANK"}
	r.drawingName = strings.ToUpper(strings.TrimSpace(investor.Name + " Drawings"))
	if a, err := store.FindAccountByReferenceIDByName(investor.ID, r.drawingName, *store.StoreIDPtr(), bson.M{}); err == nil {
		r.drawing = a
	}
	if a, err := store.FindAccountByName("CASH", store.StoreIDPtr(), nil, bson.M{}); err == nil {
		r.cash = a
	}
	if a, err := store.FindAccountByName("BANK", store.StoreIDPtr(), nil, bson.M{}); err == nil {
		r.bank = a
	}
	if cw.PaymentMethod == "cash" {
		r.paying = "cash"
	} else if slices.Contains(BANK_PAYMENT_METHODS, cw.PaymentMethod) {
		r.paying = "bank"
	}
	return r, nil
}

// StoreIDPtr is the store's id as a pointer.
func (store *Store) StoreIDPtr() *primitive.ObjectID {
	id := store.ID
	return &id
}

func accountKey(a *Account, newName string) string {
	if a != nil && !a.ID.IsZero() {
		return a.ID.Hex()
	}
	return "new:" + newName
}

// ledgerRepairPlan is what the repair would change in the store.
type ledgerRepairPlan struct {
	items        []ledgerRepairItem
	removeRefs   []primitive.ObjectID          // reference ids whose ledgers / postings go
	accounts     map[string]*Account           // existing accounts touched, by id hex
	newAccounts  map[string]bool               // names of accounts that would be created
	added        map[string]repairTotals       // posting totals the capital withdrawals add, by account key
	parties      map[string]primitive.ObjectID // "customer:<id>" / "vendor:<id>" whose balance is refreshed
	partyAccount map[primitive.ObjectID]bool   // party ids whose own account is refreshed
}

func (store *Store) planLedgerRepair(items []ledgerRepairItem) (*ledgerRepairPlan, error) {
	p := &ledgerRepairPlan{items: items, accounts: map[string]*Account{}, newAccounts: map[string]bool{},
		added: map[string]repairTotals{}, parties: map[string]primitive.ObjectID{}, partyAccount: map[primitive.ObjectID]bool{}}
	for _, it := range items {
		p.removeRefs = append(p.removeRefs, it.id)
	}
	addAccount := func(id primitive.ObjectID) error {
		if id.IsZero() || p.accounts[id.Hex()] != nil {
			return nil
		}
		a, err := store.FindAccountByID(id, bson.M{})
		if err == mongo.ErrNoDocuments || a == nil {
			return nil
		}
		if err != nil {
			return err
		}
		p.accounts[id.Hex()] = a
		return nil
	}
	// accounts of the postings / ledgers that go
	if len(p.removeRefs) > 0 {
		ctx, cancel := repairCtx(120 * time.Second)
		ids, err := store.repairDB().Collection("posting").Distinct(ctx, "account_id", bson.M{"reference_id": bson.M{"$in": p.removeRefs}})
		cancel()
		if err != nil {
			return nil, err
		}
		ctx, cancel = repairCtx(120 * time.Second)
		jids, err := store.repairDB().Collection("ledger").Distinct(ctx, "journals.account_id", bson.M{"reference_id": bson.M{"$in": p.removeRefs}})
		cancel()
		if err != nil {
			return nil, err
		}
		for _, v := range append(ids, jids...) {
			if oid, ok := v.(primitive.ObjectID); ok {
				if err := addAccount(oid); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, it := range items {
		switch {
		case it.missing:
			acc, err := store.resolveCapitalWithdrawalAccounts(it.id)
			if err != nil {
				return nil, err
			}
			dk := accountKey(acc.drawing, acc.drawingName)
			for _, a := range []*Account{acc.drawing, acc.cash, acc.bank} {
				if a != nil {
					p.accounts[a.ID.Hex()] = a
				}
			}
			if acc.drawing == nil {
				p.newAccounts[acc.drawingName] = true
			}
			if acc.cash == nil {
				p.newAccounts[acc.cashName] = true
			}
			if acc.bank == nil {
				p.newAccounts[acc.bankName] = true
			}
			t := p.added[dk]
			t.debit += acc.amount
			p.added[dk] = t
			var payKey string
			switch acc.paying {
			case "cash":
				payKey = accountKey(acc.cash, acc.cashName)
			case "bank":
				payKey = accountKey(acc.bank, acc.bankName)
			}
			if payKey != "" {
				t := p.added[payKey]
				t.credit += acc.amount
				p.added[payKey] = t
			}
		case it.ref.Type == "customer_deposit" || it.ref.Type == "customer_withdrawal":
			cust, vend, emp, err := store.repairNoteParties(it)
			if err != nil {
				return nil, err
			}
			if cust != nil {
				p.parties["customer:"+cust.Hex()] = *cust
				p.partyAccount[*cust] = true
			}
			if vend != nil {
				p.parties["vendor:"+vend.Hex()] = *vend
				p.partyAccount[*vend] = true
			}
			if emp != nil {
				p.partyAccount[*emp] = true
			}
		}
	}
	// the parties' own accounts, and the namesakes SetCreditBalance also sets
	for key, id := range p.parties {
		if strings.HasPrefix(key, "customer:") {
			if c, _ := store.FindCustomerByID(&id, bson.M{}); c != nil {
				if v, _ := store.FindVendorByNameByVatNo(c.Name, c.VATNo, bson.M{}); v != nil {
					p.parties["vendor:"+v.ID.Hex()] = v.ID
				}
			}
		} else {
			if v, _ := store.FindVendorByID(&id, bson.M{}); v != nil {
				if c, _ := store.FindCustomerByNameByVatNo(v.Name, v.VATNo, bson.M{}); c != nil {
					p.parties["customer:"+c.ID.Hex()] = c.ID
				}
			}
		}
	}
	for id := range p.partyAccount {
		ctx, cancel := repairCtx(30 * time.Second)
		ids, err := store.repairDB().Collection("account").Distinct(ctx, "_id", bson.M{"reference_id": id})
		cancel()
		if err != nil {
			return nil, err
		}
		for _, v := range ids {
			if oid, ok := v.(primitive.ObjectID); ok {
				if err := addAccount(oid); err != nil {
					return nil, err
				}
			}
		}
	}
	return p, nil
}

// repairNoteParties are the customer, vendor and employee of a deposit /
// withdrawal.
func (store *Store) repairNoteParties(it ledgerRepairItem) (cust, vend, emp *primitive.ObjectID, err error) {
	nz := func(id *primitive.ObjectID) *primitive.ObjectID {
		if id == nil || id.IsZero() {
			return nil
		}
		return id
	}
	if it.ref.Type == "customer_deposit" {
		d, err := store.FindCustomerDepositByID(&it.id, bson.M{})
		if err != nil {
			return nil, nil, nil, err
		}
		return nz(d.CustomerID), nz(d.VendorID), nz(d.EmployeeID), nil
	}
	w, err := store.FindCustomerWithdrawalByID(&it.id, bson.M{})
	if err != nil {
		return nil, nil, nil, err
	}
	return nz(w.CustomerID), nz(w.VendorID), nz(w.EmployeeID), nil
}

// postingTotals sums the postings of the accounts, leaving out the postings
// of the reference ids that go (what CalculateBalance will see afterwards).
func (store *Store) postingTotals(accountIDs []primitive.ObjectID, without []primitive.ObjectID) (map[string]repairTotals, error) {
	out := map[string]repairTotals{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	match := bson.M{"account_id": bson.M{"$in": accountIDs}, "store_id": store.ID}
	if len(without) > 0 {
		match["reference_id"] = bson.M{"$nin": without}
	}
	ctx, cancel := repairCtx(120 * time.Second)
	defer cancel()
	cur, err := store.repairDB().Collection("posting").Aggregate(ctx, []bson.M{
		{"$match": match},
		{"$group": bson.M{"_id": "$account_id", "debit_total": bson.M{"$sum": "$debit_total"}, "credit_total": bson.M{"$sum": "$credit_total"}}},
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var r struct {
			ID     primitive.ObjectID `bson:"_id"`
			Debit  float64            `bson:"debit_total"`
			Credit float64            `bson:"credit_total"`
		}
		if err := cur.Decode(&r); err != nil {
			return nil, err
		}
		out[r.ID.Hex()] = repairTotals{debit: r.Debit, credit: r.Credit}
	}
	return out, cur.Err()
}

// predictAccounts is the account part of a dry-run report.
func (store *Store) predictAccounts(p *ledgerRepairPlan) ([]RepairAccountChange, error) {
	ids := []primitive.ObjectID{}
	for _, a := range p.accounts {
		ids = append(ids, a.ID)
	}
	totals, err := store.postingTotals(ids, p.removeRefs)
	if err != nil {
		return nil, err
	}
	out := []RepairAccountChange{}
	for key, a := range p.accounts {
		t := totals[key]
		add := p.added[key]
		t.debit += add.debit
		t.credit += add.credit
		out = append(out, RepairAccountChange{ID: a.ID.Hex(), Number: a.Number, Name: a.Name,
			BalanceBefore: a.Balance, DebitTotalBefore: a.DebitTotal, CreditTotalBefore: a.CreditTotal,
			BalanceAfter: repairBalance(t), DebitTotalAfter: RoundFloat(t.debit, 2), CreditTotalAfter: RoundFloat(t.credit, 2)})
	}
	for name := range p.newAccounts {
		t := p.added["new:"+name]
		out = append(out, RepairAccountChange{Name: name, New: true,
			BalanceAfter: repairBalance(t), DebitTotalAfter: RoundFloat(t.debit, 2), CreditTotalAfter: RoundFloat(t.credit, 2)})
	}
	sortRepairAccounts(out)
	return out, nil
}

func sortRepairAccounts(a []RepairAccountChange) {
	sort.SliceStable(a, func(i, j int) bool {
		if a[i].Name != a[j].Name {
			return a[i].Name < a[j].Name
		}
		return a[i].ID < a[j].ID
	})
}

// ---- report ----

func newLedgerRepairReport(store *Store, apply bool) LedgerRepairReport {
	return LedgerRepairReport{Repair: LedgerRepairTag, StoreID: store.ID.Hex(), StoreName: store.Name, Apply: apply,
		DeletedWithLedger: []RepairDocRef{}, MissingLedger: []RepairDocRef{}, Skipped: []RepairDocRef{},
		Counts: map[string]int{}, Accounts: []RepairAccountChange{}, Errors: []string{}}
}

func (r *LedgerRepairReport) fill(items []ledgerRepairItem, skipped []RepairDocRef) {
	for _, it := range items {
		if it.missing {
			r.MissingLedger = append(r.MissingLedger, it.ref)
			r.Counts["capital_withdrawal_missing_ledger"]++
		} else {
			r.DeletedWithLedger = append(r.DeletedWithLedger, it.ref)
			r.Counts[it.ref.Type+"_deleted_with_ledger"]++
		}
	}
	r.Skipped = append(r.Skipped, skipped...)
	r.Counts["skipped"] = len(skipped)
}

// RepairLedgers finds (and with apply repairs) the store's ledgers left by
// deletes made before the delete paths undid the accounting, and posts the
// capital withdrawals that were never posted. See the top of this file.
func (store *Store) RepairLedgers(apply bool) (LedgerRepairReport, error) {
	report := newLedgerRepairReport(store, apply)
	if apply {
		// the same write lock the delete / create handlers hold
		hold := HoldStore(store.ID)
		defer hold.Release()
	}
	items, skipped, err := store.findLedgerRepairItems()
	if err != nil {
		return report, err
	}
	report.fill(items, skipped)
	if len(items) == 0 {
		return report, nil
	}
	plan, err := store.planLedgerRepair(items)
	if err != nil {
		return report, err
	}
	if !apply {
		report.Accounts, err = store.predictAccounts(plan)
		return report, err
	}
	return report, store.applyLedgerRepair(plan, &report)
}

// ---- apply ----

type repairBackup struct {
	store *Store
	run   primitive.ObjectID
	at    time.Time
	seq   int
	count int
	saved map[string]bool // collection:_id already backed up in this run
}

func (b *repairBackup) write(kind, collection string, id interface{}, doc interface{}, extra bson.M) error {
	key := collection + ":" + kind + ":" + idString(id)
	if b.saved[key] {
		return nil
	}
	b.seq++
	rec := bson.D{
		{Key: "repair", Value: LedgerRepairTag},
		{Key: "run", Value: b.run},
		{Key: "seq", Value: b.seq},
		{Key: "kind", Value: kind},
		{Key: "collection", Value: collection},
		{Key: "doc_id", Value: id},
		{Key: "doc", Value: doc},
		{Key: "at", Value: b.at},
	}
	for k, v := range extra {
		rec = append(rec, bson.E{Key: k, Value: v})
	}
	ctx, cancel := repairCtx(30 * time.Second)
	defer cancel()
	if _, err := b.store.repairDB().Collection(ledgerRepairBackupCollection).InsertOne(ctx, rec); err != nil {
		return errors.New("backup: " + err.Error())
	}
	b.saved[key] = true
	b.count++
	return nil
}

func idString(id interface{}) string {
	if oid, ok := id.(primitive.ObjectID); ok {
		return oid.Hex()
	}
	if s, ok := id.(string); ok {
		return s
	}
	return ""
}

// rawDocs reads the raw documents of a collection matching filter, by _id.
func (store *Store) rawDocs(collection string, filter bson.M, projection bson.M) (map[primitive.ObjectID]bson.Raw, []primitive.ObjectID, error) {
	ctx, cancel := repairCtx(300 * time.Second)
	defer cancel()
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})
	if projection != nil {
		opts.SetProjection(projection)
	}
	cur, err := store.repairDB().Collection(collection).Find(ctx, filter, opts)
	if err != nil {
		return nil, nil, err
	}
	defer cur.Close(ctx)
	out := map[primitive.ObjectID]bson.Raw{}
	var order []primitive.ObjectID
	for cur.Next(ctx) {
		raw := make(bson.Raw, len(cur.Current))
		copy(raw, cur.Current)
		oid, ok := raw.Lookup("_id").ObjectIDOK()
		if !ok {
			continue
		}
		out[oid] = raw
		order = append(order, oid)
	}
	return out, order, cur.Err()
}

func (store *Store) applyLedgerRepair(plan *ledgerRepairPlan, report *LedgerRepairReport) error {
	b := &repairBackup{store: store, run: primitive.NewObjectID(), at: time.Now().UTC(), saved: map[string]bool{}}
	report.RunID = b.run.Hex()

	// in-memory pre-images: every account, and the balance fields of every
	// customer / vendor (to catch any change the plan did not foresee)
	accountsBefore, _, err := store.rawDocs("account", bson.M{}, nil)
	if err != nil {
		return err
	}
	partyProj := bson.M{"credit_balance": 1, "account": 1}
	customersBefore, _, err := store.rawDocs("customer", bson.M{}, partyProj)
	if err != nil {
		return err
	}
	vendorsBefore, _, err := store.rawDocs("vendor", bson.M{}, partyProj)
	if err != nil {
		return err
	}

	// 1. back up everything the plan removes or changes, before any change
	ledgers, ledgerOrder, err := store.rawDocs("ledger", bson.M{"reference_id": bson.M{"$in": plan.removeRefs}}, nil)
	if err != nil {
		return err
	}
	for _, id := range ledgerOrder {
		if err := b.write(repairKindRemoved, "ledger", id, ledgers[id], nil); err != nil {
			return err
		}
	}
	postings, postingOrder, err := store.rawDocs("posting", bson.M{"reference_id": bson.M{"$in": plan.removeRefs}}, nil)
	if err != nil {
		return err
	}
	for _, id := range postingOrder {
		if err := b.write(repairKindRemoved, "posting", id, postings[id], nil); err != nil {
			return err
		}
	}
	accountIDs := []string{}
	for k := range plan.accounts {
		accountIDs = append(accountIDs, k)
	}
	sort.Strings(accountIDs)
	for _, k := range accountIDs {
		oid, _ := primitive.ObjectIDFromHex(k)
		if raw, ok := accountsBefore[oid]; ok {
			if err := b.write(repairKindModified, "account", oid, raw, nil); err != nil {
				return err
			}
		}
	}
	backupParty := func(collection string, id primitive.ObjectID, raw bson.Raw) error {
		fields := bson.A{}
		for _, f := range partyBalanceFields {
			if _, err := raw.LookupErr(f); err == nil {
				fields = append(fields, f)
			}
		}
		return b.write(repairKindParty, collection, id, raw, bson.M{"fields": fields})
	}
	partyKeys := []string{}
	for k := range plan.parties {
		partyKeys = append(partyKeys, k)
	}
	sort.Strings(partyKeys)
	for _, k := range partyKeys {
		id := plan.parties[k]
		coll, before := "customer", customersBefore
		if strings.HasPrefix(k, "vendor:") {
			coll, before = "vendor", vendorsBefore
		}
		if raw, ok := before[id]; ok {
			if err := backupParty(coll, id, raw); err != nil {
				return err
			}
		}
	}

	// 2. repair, with exactly the calls a delete / create makes today
	for _, it := range plan.items {
		if err := store.repairOne(it); err != nil {
			report.Errors = append(report.Errors, it.ref.Type+" "+it.ref.Code+" ("+it.ref.ID+"): "+err.Error())
		}
		if it.missing {
			// what the posting added
			for _, coll := range []string{"ledger", "posting"} {
				docs, order, err := store.rawDocs(coll, bson.M{"reference_id": it.id}, nil)
				if err != nil {
					return err
				}
				for _, id := range order {
					if err := b.write(repairKindAdded, coll, id, docs[id], bson.M{"reference_id": it.id}); err != nil {
						return err
					}
				}
			}
		}
	}

	// 3. anything else that changed (pre-image) or was created
	accountsAfter, accountOrder, err := store.rawDocs("account", bson.M{}, nil)
	if err != nil {
		return err
	}
	beforeIDs := make([]primitive.ObjectID, 0, len(accountsBefore))
	for id := range accountsBefore {
		beforeIDs = append(beforeIDs, id)
	}
	sort.Slice(beforeIDs, func(i, j int) bool { return beforeIDs[i].Hex() < beforeIDs[j].Hex() })
	changed := map[primitive.ObjectID]bool{}
	for _, id := range beforeIDs {
		after, ok := accountsAfter[id]
		if !ok || !bytes.Equal(after, accountsBefore[id]) {
			changed[id] = true
			if err := b.write(repairKindModified, "account", id, accountsBefore[id], bson.M{"late": true}); err != nil {
				return err
			}
		}
	}
	created := []primitive.ObjectID{}
	for _, id := range accountOrder {
		if _, ok := accountsBefore[id]; !ok {
			created = append(created, id)
			if err := b.write(repairKindAdded, "account", id, accountsAfter[id], nil); err != nil {
				return err
			}
		}
	}
	for _, pc := range []struct {
		coll   string
		before map[primitive.ObjectID]bson.Raw
	}{{"customer", customersBefore}, {"vendor", vendorsBefore}} {
		after, _, err := store.rawDocs(pc.coll, bson.M{}, partyProj)
		if err != nil {
			return err
		}
		for id, raw := range pc.before {
			if a, ok := after[id]; ok && !bytes.Equal(a, raw) {
				if err := backupParty(pc.coll, id, raw); err != nil {
					return err
				}
			}
		}
	}
	report.BackupDocs = b.count

	// 4. accounts before / after (read back)
	out := []RepairAccountChange{}
	seen := map[primitive.ObjectID]bool{}
	addRow := func(id primitive.ObjectID, isNew bool) {
		if seen[id] {
			return
		}
		seen[id] = true
		var before, after Account
		if raw, ok := accountsBefore[id]; ok {
			_ = bson.Unmarshal(raw, &before)
		}
		if raw, ok := accountsAfter[id]; ok {
			_ = bson.Unmarshal(raw, &after)
		}
		out = append(out, RepairAccountChange{ID: id.Hex(), Number: after.Number, Name: after.Name, New: isNew,
			BalanceBefore: before.Balance, DebitTotalBefore: before.DebitTotal, CreditTotalBefore: before.CreditTotal,
			BalanceAfter: after.Balance, DebitTotalAfter: after.DebitTotal, CreditTotalAfter: after.CreditTotal})
	}
	for _, k := range accountIDs {
		oid, _ := primitive.ObjectIDFromHex(k)
		addRow(oid, false)
	}
	for _, id := range beforeIDs {
		if changed[id] {
			addRow(id, false)
		}
	}
	for _, id := range created {
		addRow(id, true)
	}
	sortRepairAccounts(out)
	report.Accounts = out

	// 5. the marker (a record of the run; correctness never depends on it)
	ctx, cancel := repairCtx(30 * time.Second)
	defer cancel()
	_, err = store.repairDB().Collection(ledgerRepairBackupCollection).UpdateOne(ctx, bson.M{"_id": ledgerRepairMarkerID},
		bson.M{
			"$set": bson.M{"repair": LedgerRepairTag, "kind": "marker", "last_run_at": b.at},
			"$push": bson.M{"runs": bson.M{"run": b.run, "at": b.at, "counts": report.Counts,
				"deleted_with_ledger": len(report.DeletedWithLedger), "missing_ledger": len(report.MissingLedger),
				"backup_docs": b.count, "errors": report.Errors}},
		}, options.Update().SetUpsert(true))
	return err
}

// repairOne repairs one document with the calls today's delete / create
// handlers make.
func (store *Store) repairOne(it ledgerRepairItem) error {
	switch it.ref.Type {
	case "customer_deposit":
		d, err := store.FindCustomerDepositByID(&it.id, bson.M{})
		if err != nil {
			return err
		}
		if err := d.UndoAccounting(); err != nil {
			return err
		}
		d.SetPartyBalances()
		if d.StoreID != nil {
			MarkDashboardDirty(*d.StoreID, d.Date)
		}
	case "customer_withdrawal":
		w, err := store.FindCustomerWithdrawalByID(&it.id, bson.M{})
		if err != nil {
			return err
		}
		if err := w.UndoAccounting(); err != nil {
			return err
		}
		w.SetPartyBalances()
	case "capital":
		c, err := store.FindCapitalByID(&it.id, bson.M{})
		if err != nil {
			return err
		}
		return c.UndoAccounting()
	case "dividend":
		d, err := store.FindDividentByID(&it.id, bson.M{})
		if err != nil {
			return err
		}
		return d.UndoAccounting()
	case "capital_withdrawal":
		cw, err := store.FindCapitalWithdrawalByID(&it.id, bson.M{})
		if err != nil {
			return err
		}
		// as an edit: off the ledger first, then posted unless deleted
		if err := cw.UndoAccounting(); err != nil {
			return err
		}
		if it.missing && !cw.Deleted {
			return cw.DoAccounting()
		}
	default:
		return errors.New("unknown type " + it.ref.Type)
	}
	return nil
}

// ---- all stores ----

// LedgerRepairStoreIDs lists every store id (or just the one given).
func LedgerRepairStoreIDs(only *primitive.ObjectID) ([]primitive.ObjectID, error) {
	if only != nil {
		return []primitive.ObjectID{*only}, nil
	}
	collection := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx, cancel := repairCtx(60 * time.Second)
	defer cancel()
	cur, err := collection.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"_id": 1}).SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var ids []primitive.ObjectID
	for cur.Next(ctx) {
		var d struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if cur.Decode(&d) == nil {
			ids = append(ids, d.ID)
		}
	}
	return ids, cur.Err()
}

// RunLedgerRepair runs RepairLedgers over one store (storeID) or all stores.
// A store that fails is reported with its error; the others still run.
func RunLedgerRepair(storeID *primitive.ObjectID, apply bool) (LedgerRepairRun, error) {
	run := LedgerRepairRun{Repair: LedgerRepairTag, Apply: apply, Stores: []LedgerRepairReport{}, Totals: map[string]int{
		"stores": 0, "deletedWithLedger": 0, "missingLedger": 0, "skipped": 0, "errors": 0, "backupDocs": 0}}
	ids, err := LedgerRepairStoreIDs(storeID)
	if err != nil {
		return run, err
	}
	for _, id := range ids {
		id := id
		store, err := FindStoreByID(&id, bson.M{})
		if err != nil {
			if storeID != nil {
				return run, err
			}
			r := LedgerRepairReport{Repair: LedgerRepairTag, StoreID: id.Hex(), Apply: apply, Errors: []string{"store: " + err.Error()}}
			run.Stores = append(run.Stores, r)
			run.Totals["errors"]++
			continue
		}
		r, err := store.RepairLedgers(apply)
		if err != nil {
			r.Errors = append(r.Errors, err.Error())
		}
		run.Totals["stores"]++
		run.Totals["deletedWithLedger"] += len(r.DeletedWithLedger)
		run.Totals["missingLedger"] += len(r.MissingLedger)
		run.Totals["skipped"] += len(r.Skipped)
		run.Totals["errors"] += len(r.Errors)
		run.Totals["backupDocs"] += r.BackupDocs
		// stores with nothing to report are left out of a run over all stores
		if storeID != nil || !r.NothingToDo() || len(r.Errors) > 0 || len(r.Skipped) > 0 {
			run.Stores = append(run.Stores, r)
		}
	}
	return run, nil
}

// ---- restore ----

// LedgerRepairRestoreReport is what RestoreLedgerRepair put back.
type LedgerRepairRestoreReport struct {
	Repair           string   `json:"repair"`
	StoreID          string   `json:"storeId"`
	Runs             []string `json:"runs"`
	Reinserted       int      `json:"reinserted"`
	RemovedAdded     int      `json:"removedAdded"`
	AccountsRestored int      `json:"accountsRestored"`
	PartiesRestored  int      `json:"partiesRestored"`
	AccountsKept     []string `json:"accountsKept"`
	AlreadyRestored  bool     `json:"alreadyRestored"`
}

type repairBackupRec struct {
	ID         primitive.ObjectID `bson:"_id"`
	Run        primitive.ObjectID `bson:"run"`
	Seq        int                `bson:"seq"`
	Kind       string             `bson:"kind"`
	Collection string             `bson:"collection"`
	DocID      interface{}        `bson:"doc_id"`
	Doc        bson.Raw           `bson:"doc"`
	Fields     []string           `bson:"fields"`
}

// RestoreLedgerRepair undoes every not yet restored run of the repair on the
// store: what it added is deleted, what it removed is put back and what it
// changed gets its pre-image back (newest run first, so the oldest pre-image
// wins). Records are marked restored, so a second restore does nothing.
func (store *Store) RestoreLedgerRepair() (LedgerRepairRestoreReport, error) {
	rep := LedgerRepairRestoreReport{Repair: LedgerRepairTag, StoreID: store.ID.Hex(), Runs: []string{}, AccountsKept: []string{}}
	hold := HoldStore(store.ID)
	defer hold.Release()

	col := store.repairDB().Collection(ledgerRepairBackupCollection)
	ctx, cancel := repairCtx(300 * time.Second)
	defer cancel()
	cur, err := col.Find(ctx, bson.M{"repair": LedgerRepairTag, "kind": bson.M{"$ne": "marker"}, "restored_at": bson.M{"$exists": false}},
		options.Find().SetSort(bson.D{{Key: "run", Value: -1}, {Key: "seq", Value: -1}}))
	if err != nil {
		return rep, err
	}
	var recs []repairBackupRec
	if err := cur.All(ctx, &recs); err != nil {
		return rep, err
	}
	if len(recs) == 0 {
		rep.AlreadyRestored = true
		return rep, nil
	}
	var runs [][]repairBackupRec
	for _, r := range recs {
		if len(runs) == 0 || runs[len(runs)-1][0].Run != r.Run {
			runs = append(runs, nil)
		}
		runs[len(runs)-1] = append(runs[len(runs)-1], r)
	}
	sdb := store.repairDB()
	for _, run := range runs {
		rep.Runs = append(rep.Runs, run[0].Run.Hex())
		// 1. what the run added to the ledger goes
		for _, r := range run {
			if r.Kind == repairKindAdded && (r.Collection == "ledger" || r.Collection == "posting") {
				res, err := sdb.Collection(r.Collection).DeleteOne(ctx, bson.M{"_id": r.DocID})
				if err != nil {
					return rep, err
				}
				rep.RemovedAdded += int(res.DeletedCount)
			}
		}
		// 2. what it removed comes back (oldest first)
		for i := len(run) - 1; i >= 0; i-- {
			r := run[i]
			if r.Kind != repairKindRemoved {
				continue
			}
			if _, err := sdb.Collection(r.Collection).InsertOne(ctx, r.Doc); err != nil && !mongo.IsDuplicateKeyError(err) {
				return rep, err
			}
			rep.Reinserted++
		}
		// 3. pre-images of the accounts and party balances
		for _, r := range run {
			switch r.Kind {
			case repairKindModified:
				if _, err := sdb.Collection(r.Collection).ReplaceOne(ctx, bson.M{"_id": r.DocID}, r.Doc, options.Replace().SetUpsert(true)); err != nil {
					return rep, err
				}
				rep.AccountsRestored++
			case repairKindParty:
				set, unset := bson.M{}, bson.M{}
				for _, f := range partyBalanceFields {
					if slices.Contains(r.Fields, f) {
						set[f] = r.Doc.Lookup(f)
					} else {
						unset[f] = ""
					}
				}
				upd := bson.M{}
				if len(set) > 0 {
					upd["$set"] = set
				}
				if len(unset) > 0 {
					upd["$unset"] = unset
				}
				if _, err := sdb.Collection(r.Collection).UpdateOne(ctx, bson.M{"_id": r.DocID}, upd); err != nil {
					return rep, err
				}
				rep.PartiesRestored++
			}
		}
		// 4. accounts the run created go, unless something posts to them now
		for _, r := range run {
			if r.Kind != repairKindAdded || r.Collection != "account" {
				continue
			}
			n, err := sdb.Collection("posting").CountDocuments(ctx, bson.M{"account_id": r.DocID})
			if err != nil {
				return rep, err
			}
			if n > 0 {
				rep.AccountsKept = append(rep.AccountsKept, idString(r.DocID))
				continue
			}
			if _, err := sdb.Collection("account").DeleteOne(ctx, bson.M{"_id": r.DocID}); err != nil {
				return rep, err
			}
			rep.RemovedAdded++
		}
		now := time.Now().UTC()
		if _, err := col.UpdateMany(ctx, bson.M{"repair": LedgerRepairTag, "run": run[0].Run}, bson.M{"$set": bson.M{"restored_at": now}}); err != nil {
			return rep, err
		}
		if _, err := col.UpdateOne(ctx, bson.M{"_id": ledgerRepairMarkerID}, bson.M{"$push": bson.M{"restores": bson.M{"run": run[0].Run, "at": now}}}); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

// RunLedgerRepairRestore restores one store (storeID) or every store.
func RunLedgerRepairRestore(storeID *primitive.ObjectID) ([]LedgerRepairRestoreReport, error) {
	out := []LedgerRepairRestoreReport{}
	ids, err := LedgerRepairStoreIDs(storeID)
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		id := id
		store, err := FindStoreByID(&id, bson.M{})
		if err != nil {
			if storeID != nil {
				return out, errors.New("store " + id.Hex() + ": " + err.Error())
			}
			continue
		}
		r, err := store.RestoreLedgerRepair()
		if err != nil {
			return out, errors.New("store " + id.Hex() + ": " + err.Error())
		}
		if storeID != nil || !r.AlreadyRestored {
			out = append(out, r)
		}
	}
	return out, nil
}

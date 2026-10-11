package models

import "sync"

// The CreateLedger of these documents splits payments into the sale's part and
// the overpaid part through package-level variables (totalSalesPaidAmount,
// extraSalesAmountPaid, extraSalesPayments and their twins). Accounting runs
// in a background goroutine per create/edit, so two documents of a kind being
// accounted at once shared — and corrupted — that state (a data race; journals
// of one document could take another's amounts). Each kind's CreateLedger now
// holds its lock while it uses that state.
var (
	salesLedgerStateMu                sync.Mutex
	salesReturnLedgerStateMu          sync.Mutex
	nonVATSalesLedgerStateMu          sync.Mutex
	nonVATSalesReturnLedgerStateMu    sync.Mutex
	quotationLedgerStateMu            sync.Mutex
	quotationSalesReturnLedgerStateMu sync.Mutex
)

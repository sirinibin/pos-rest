# StartERP adapter API — mapping to the existing pos-rest data

The StartERP UI talks to a new **adapter** mounted at **`/v1/erp/...`** (package `erp/`, registered in
`main.go` with one line). It implements the StartERP REST contract (`contract/API_CONTRACT.md`) on top of
the **existing** databases, collections and v1 handlers. Every v1 route is unchanged and the old app keeps
working on the same data.

Run the UI against it with `VITE_API_BASE_URL=http://localhost:2000 VITE_API_PREFIX=/v1/erp`.

## 1. Data-compatibility rules and how they are kept

| Rule (master prompt §2.1 / 2.1a–c / 3.1 / 3.3) | How the adapter keeps it |
|---|---|
| Never rename/remove/retype DBs, collections or bson fields | Writes go **through the existing v1 handlers** (in-process, with the caller's own token), so the legacy models build the documents. The adapter only adds **one** top-level subdocument, `erp`, to legacy documents it writes. Misspelled legacy keys stay (`divident`, `produc_id`, `receivabale_title`, `salesreturn`, `customerdeposit`, …). |
| Keep Redis counters, ICV, serial semantics | Numbers are allocated by the legacy create code (`<store>_invoice_counter`, `<store>_return_invoice_counter`, …). The adapter never writes a counter. Covered by `erp/integration_test.go`. |
| Every v1 route keeps working | Nothing in `controller/`, `models/` or the v1 router was changed. |
| Old data readable as-is | Mappers are lenient (int32/int64/Decimal128/string numbers, Arabic digits, missing sub-documents, old stores without `settings`, orders without embedded `payments`, `produc_id`). Tested against a legacy-shaped fixture (`erp/erpfixture`). |
| Adapter writes readable by the old app | Integration test creates sale → payment → return through the adapter and through the old v1 path and compares Redis counters, stock, ledger journals and the top-level field set; v1 `ViewOrder` / `ListSalesPayment` / `ViewSalesReturn` read the adapter documents. The Playwright e2e verifies UI-created customer/product/sale/return through v1 endpoints. |
| Debit Note = `customerdeposit`, Credit Note = `customerwithdrawal` | Labels only: `/deposits` → `customerdeposit`, `/withdrawals` → `customerwithdrawal`. |
| Drafts in separate collections | `/v1/erp/drafts/{type}` stores in `<legacy collection>_draft` (store DB). Drafts never call a create path; finalize runs the normal create then deletes the draft. Legacy `status:"draft"` documents in real collections are listed read-only. |
| ZATCA: delegate, keep chain | `POST /…/{id}/zatca/report`, `/stores/{id}/zatca/connect|disconnect` call `controller.ReportOrderToZatca`, `ReportSalesReturnToZatca`, `ReportCustomerDepositToZatca`, `ReportCustomerWithdrawalToZatca`, `ConnectStoreToZatca`, `DisconnectStoreFromZatca`. Per-collection PIH/ICV chain untouched. |

### The `erp` envelope (additive, on legacy documents the adapter wrote)

| Key | Meaning |
|---|---|
| `erp.v` | contract `version` (optimistic concurrency, `If-Match`) |
| `erp.ts` | legacy `updated_at` seen at the last adapter write. If the old app edits the document later, the version becomes `max(v+1, unix(updated_at))` |
| `erp.h` | contract `history[]` (max 200). Without it, history is synthesized from `created_*`/`updated_*` |
| `erp.cid` | client-generated id (`cus_…`), kept as an alias. Records are always returned with the legacy ObjectID hex as `id` |
| `erp.x` | contract fields with no legacy equivalent ("extras"), round-tripped as sent |
| `erp.ca`, `erp.cb` | created at/by through the adapter |
| `erp.del` | contract soft delete for resources whose legacy delete is **destructive** (PO, PR, RFQ, RFQ supplier) |
| `erp.hd` | hard-deleted through the adapter (hidden from the contract, still in legacy; see §4) |
| `erp.role` | users: StartERP role id when it is finer-grained than the legacy role |

Documents the adapter never wrote have no `erp` key and are never modified by reads.

### Conventions

* Contract dates `YYYY-MM-DDTHH:mm` are **Asia/Riyadh** local time. They map to legacy `date_str` (RFC 3339) and `date`.
* `storeId` → `store_id` and the store database `store_<id>`. Org-scoped resources (categories, brands, vendor/expense categories, accounts) are the union over the caller's stores.
* Warehouses: the legacy "main store" stock bucket (`warehouse_id` null, `warehouse_stocks.main_store`) is the **virtual** warehouse `ms_<storeId>`.
* Product stock in the contract is **absolute**. A change is written as legacy `stock_adjustments` (adding/removing) through `UpdateProduct`, so stock history and accounting stay legacy.
* Money: legacy totals are authoritative. `legacyTotals` (net, vat, paid, balance, paymentStatus, profit) are returned read-only.
* Payment `method` uses the legacy values (`cash`, `debit_card`, `credit_card`, `bank_card`, `bank_transfer`, `bank_cheque`, `customer_account`, …). Unknown values are rejected (400), because the legacy ledger would post them to no account.
* Errors: the legacy `errors` map is translated to the contract envelope `{error:{code,message,fields}}`, with legacy keys mapped to contract field paths (`items.0.qty`, `payments.1.amount`, …).

## 2. Endpoints other than resource CRUD

| Adapter endpoint | Implementation |
|---|---|
| `GET /meta` | capabilities `serverStock`, `clientIds`, `serverNumbers`, `serverTotals` = true, `zatca:"server"`, `realtime:false` |
| `POST /auth/login` | `user` (main DB): case-insensitive e-mail, bcrypt password, legacy JWT + Redis access/refresh tokens (`models.GenerateAccesstoken`). The tokens also work on v1. Inactive → 403 `inactive`. Own rate limiter (`ERP_LOGIN_LIMIT`, default 10/15 min/IP), 429 in the contract envelope |
| `POST /auth/refresh` | rotates: the used refresh token is revoked |
| `POST /auth/logout` | revokes access + refresh in Redis (204) |
| `GET /auth/me` | user, stores, role, effective perms |
| `POST /auth/signup` | validates owner rules (VAT 15 digits starting and ending with 3, CR 10 digits, company name, Saudi mobile, National Address), then calls the existing `controller.GuestRegister` in-process. Store extras (short code, plan, Arabic branch, phone2, trial end) go to `store.erp.x`. Owner gets `erp.role = r_admin`. Own limiter (`ERP_SIGNUP_LIMIT`, default 5/60 min) |
| `POST /{sales,sales-returns,deposits,withdrawals}/{id}/zatca/report` | existing report handlers. 409 `zatca_not_connected` / `reconnect_required`. Idempotent when the document is already reported or cleared. A failure returns 200 with `zatca.status:"failed"` and the error |
| `POST /stores/{id}/zatca/connect` (`{otp}`) | `ConnectStoreToZatca`. OTP must be 5–6 digits, else 400 `invalid_otp`. The UI snapshot, `certExpires` and `connectedAt` go to `store.erp.x.zatca` |
| `POST /stores/{id}/zatca/disconnect` | `DisconnectStoreFromZatca` |
| `/drafts/{type}[/{id}[/finalize]]` | 12 `*_draft` collections, see §1 |
| `Idempotency-Key` header (all writes) | `erp_idempotency` (main DB, TTL index 48 h). Same key → same response replayed with `Idempotent-Replayed: true`. Per-key lock; 401/429/5xx are not pinned |

RBAC: the prototype's system roles (`r_admin`, `r_manager`, `r_accountant`, `r_cashier`, `r_salesman`, `r_storekeeper`, `r_viewer`, …) map to legacy roles. `r_admin`, `r_manager` and `r_accountant` are written as legacy `Manager`; everything else as `SalesMan`. Legacy `Admin` is the platform super-admin and is never assigned or demoted by the adapter. Legacy `user_role` permissions are added to the role's permissions. Custom roles are stored in `erp_role`.

## 3. Resource overview

| Resource | Backing | Create | Update | Delete / restore / `?hard=1` |
|---|---|---|---|---|
| stores | main `store` | 403 (sign-up only) | `UpdateStore` (only changed keys; ZATCA keys server-owned except `phase`; masked secrets ignored) | 409 |
| users | main `user` | `CreateUser` | `UpdateUser` | delete → legacy; restore 409 |
| roles | built-in + legacy `user_role` (read-only) + `erp_role` | erp_role | erp_role (system 403, legacy 409) | erp_role |
| categories, brands, vendor/expense categories | store `product_category`, `product_brand`, `vendor_category`, `expense_category` | v1 | v1 | v1 (restore: categories/brands only) |
| customerCategories | **new** `erp_customer_category` (main) | ✓ | ✓ | ✓ |
| accounts | store `account` | read-only | read-only | read-only |
| warehouses | store `warehouse` + virtual `ms_<store>` | v1 | v1 | v1, restore 409; virtual warehouse read-only |
| products | store `product` (`product_stores.<store>`) | v1 (`{short}-P-0001` code when blank) | v1 (+ stock adjustments) | v1 |
| customers, vendors | store DB | v1 | v1 | v1 delete / undelete |
| employees, vehicles, signatures | store DB | v1 | v1 | v1; restore 409 |
| packages | main `customer_package` | v1 | v1 | v1; restore 409 |
| rfqSuppliers, rfqs | main `rfq_suppliers`, `rfq_received` | v1 | v1 | `erp.del` (legacy delete destructive); hard → legacy delete |
| sales | store `order` + `sales_payment` | `CreateOrder` | `UpdateOrder` (payments via `payments_input`) | **409** (issue a return) |
| salesReturns | store `salesreturn` + `sales_return_payment` | v1 | v1 | v1 delete / undelete |
| quotations (quotation\|invoice) | store `quotation` | v1 | v1 | v1; restore 409 |
| proformas | **new** `erp_proforma` | ✓ | ✓ | ✓ |
| deliveryNotes | store `delivery_note` | v1 | v1 | **409** |
| nonvatSales / nonvatReturns | `non_vat_sales` / `non_vat_sales_return` | v1 | v1 (full doc) | v1; restore 409 |
| quotationReturns | `quotation_sales_return` | v1 | v1 | **409** |
| purchases | store `purchase` + `purchase_payment` | v1 | v1 | **409** (legacy delete is a no-op) |
| purchaseOrders / purchaseRequests | `purchase_order` / `purchase_request` | v1 | v1 (full doc) | `erp.del`; hard → legacy delete |
| purchaseReturns | `purchasereturn` | v1 (`purchase_returned_by` = caller) | v1 | legacy delete is permanent → restore 409 |
| purchaseBills | **new** `erp_purchase_bill` | ✓ | ✓ | ✓ |
| stockTransfers | `stocktransfer` (completed) + **new** `erp_stock_transfer_pending` | v1 / pending | pending→completed creates the legacy transfer (`replacedId`) | **409** for completed |
| expenses | `expense` (legacy amount is VAT-inclusive) | v1 | v1 | v1; restore 409 |
| deposits (Debit Note) / withdrawals (Credit Note) | `customerdeposit` / `customerwithdrawal` | v1 | v1 | v1; restore 409 |
| capitals / capitalWithdrawals / dividends | `capital` / `capitalwithdrawal` / `divident` | v1 | v1; capitalWithdrawals **409** (legacy update ignores changes) | v1; restore 409 |
| salaries | `employee_salary_payment` | v1 | v1 (full doc) | v1; restore 409 |
| repairJobs | `repair_job` | v1 | v1 | v1; restore 409 |
| threads, notifications | **new** `erp_thread`, `erp_notification` | ✓ | ✓ | ✓ |

"Restore 409" means the legacy model has no undelete. Contract soft delete maps to legacy `deleted:true`.

### New collections (additive)

| Collection | DB | Purpose |
|---|---|---|
| `erp_idempotency` | main | Idempotency-Key replay (TTL 48 h on `created_at`) |
| `erp_role` | main | custom roles |
| `erp_customer_category` | main | customer categories (no legacy model) |
| `erp_proforma`, `erp_purchase_bill`, `erp_thread`, `erp_notification` | store | contract resources with no legacy equivalent |
| `erp_stock_transfer_pending` | store | pending transfers (legacy transfers move stock immediately) |
| `order_draft`, `quotation_draft`, `purchase_draft`, `salesreturn_draft`, `purchasereturn_draft`, `delivery_note_draft`, `purchase_order_draft`, `quotation_sales_return_draft`, `customerdeposit_draft`, `customerwithdrawal_draft`, `stocktransfer_draft`, `pos_cart_draft` | store | drafts (§2.1b) |

New-collection documents use `_id` = the contract id (`<prefix>_<hex>` when the client sends none) and are numbered from the store's serial settings (short-prefix serials).

## 4. Not supported, or supported differently (and why)

1. **Deleting** sales, delivery notes, quotation returns, completed stock transfers and purchases returns 409 `unsupported_legacy`. The legacy system has no delete for them, or its delete does nothing. Restoring is unsupported where legacy has no undelete (§3).
2. **Hard delete** (`?hard=1`) never physically removes a legacy document that carries counters, ledger or stock. It soft-deletes in legacy terms and sets `erp.hd`, so the record disappears from the contract but stays for the old app. The exception is PO/PR/RFQ/RFQ-supplier, whose legacy delete is already destructive and is called as-is.
3. **Adapter soft delete is invisible to the old app** for PO/PR/RFQ/RFQ-supplier (`erp.del`): the old app still lists them. Deleting them in legacy would be irreversible.
4. **Capital withdrawal edits** return 409: legacy `UpdateCapitalWithdrawal` ignores the payload (existing bug, not fixed here).
5. **Legacy validation wins.** Credit limits, stock checks, duplicate codes, date rules and so on are enforced by the legacy code and returned as contract field errors.
6. **Free-text lines** (no `productId`) become legacy service products (`erp.x.freeTextProduct`), because legacy lines need a product.
7. **Rounding:** `roundingAuto` is computed by the adapter and sent as an explicit `rounding_amount` (legacy `auto_rounding_amount` false). The flag is kept in `erp.x`.
8. **Payment dates:** the contract has minute resolution, and legacy `AdjustPayments` moves a payment that shares its minute with the invoice or a previous payment by +1 minute. This is unchanged legacy behaviour. Payment ledger groups follow the legacy per-minute grouping.
9. **Stock is asynchronous** in legacy (goroutines). The adapter waits up to 4 s for the ledger, so an immediate follow-up PATCH is not overwritten by legacy `AdjustPayments → order.Update()`. It also bumps the version of every product the document touches. Stock values can still trail by a moment.
10. **Pending stock transfers** have a new id once completed (`replacedId` points to the pending id).
11. **Users:** passwords can be set through the contract (min 8 characters). If none is given, a random one is generated and the user must reset it. Users can't be restored.
12. **Stores** can't be created (sign-up only) or deleted. ZATCA credentials are server-owned and masked (`••••`).
13. **ZATCA 381/383 swap (§8.5): not changed.** Legacy reports `customerdeposit` (Debit Note) as **381** and `customerwithdrawal` (Credit Note) and `salesreturn` as **383**. ZATCA defines 381 = credit note and 383 = debit note. The adapter delegates to the existing flow, so new documents are reported exactly as before. The fix (new documents only, behind a store setting that defaults to legacy, plus a historical report) needs owner and tax-advisor sign-off. Historical documents affected: `db.customerdeposit.find({"zatca.reporting_passed":true})`, the same on `customerwithdrawal` and `salesreturn`, in every `store_<id>` database.
14. Contract fields with no legacy equivalent are kept in `erp.x` (class **new** in the matrix). They round-trip through the adapter but the old app doesn't see them, and an edit in the old app doesn't change them.

## 5. Field matrix (generated)

Classes: **mapped** = written to / read from the named legacy field(s) through the v1 handler;
**computed** = read-only, derived from legacy data; **server** = envelope or server-owned;
**new** = stored in `erp.x` or in a new `erp_*` collection.

The matrix is generated by mutating each field on a record read from a real legacy document and diffing the
payload the adapter sends to the v1 handler. To regenerate it:

```
ERP_TEST_DB=1 MONGO_PORT=… REDIS_DSN=… ERP_GEN_MAPPING=/tmp/matrix.md ERP_CONTRACT_JSON=…/contract/contract.json \
  go test ./erp/ -run TestGenerateAdapterMapping -count=1
```

<!-- generated by TestGenerateAdapterMapping; do not edit by hand -->
Field totals — computed: 176 · mapped: 680 · new: 370 · server: 523

### stores — `/v1/erp/stores`

**Legacy** main DB `store` — collection `store` (main DB). Writes go through the existing v1 handlers.

- DELETE → 409 unsupported_legacy: Stores cannot be deleted from StartERP.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `address.additionalNo` | string | `national_address.additional_no_arabic`, `national_address.additional_no` (+ full object kept in `erp.x.address`) | mapped |
| `address.buildingNo` | string | `national_address.building_no_arabic`, `national_address.building_no` (+ full object kept in `erp.x.address`) | mapped |
| `address.cityAr` | string | `national_address.city_name_arabic` (+ full object kept in `erp.x.address`) | mapped |
| `address.cityEn` | string | `national_address.city_name` (+ full object kept in `erp.x.address`) | mapped |
| `address.countryAr` | string | read from legacy; not written by the adapter | computed |
| `address.countryEn` | string | `country_name` (+ full object kept in `erp.x.address`) | mapped |
| `address.districtAr` | string | `national_address.district_name_arabic` (+ full object kept in `erp.x.address`) | mapped |
| `address.districtEn` | string | `national_address.district_name` (+ full object kept in `erp.x.address`) | mapped |
| `address.postalCode` | string | `national_address.zipcode_arabic`, `national_address.zipcode` (+ full object kept in `erp.x.address`) | mapped |
| `address.shortAddress` | string | `national_address.short_code` (+ full object kept in `erp.x.address`) | mapped |
| `address.streetAr` | string | `national_address.street_name_arabic` (+ full object kept in `erp.x.address`) | mapped |
| `address.streetEn` | string | `national_address.street_name` (+ full object kept in `erp.x.address`) | mapped |
| `ai.apiKey` | string | `settings.rfq_llm_api_key` (+ full object kept in `erp.x.ai`) | mapped |
| `ai.billOcr` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.ai`) | mapped |
| `ai.model` | string | `settings.rfq_llm_model` (+ full object kept in `erp.x.ai`) | mapped |
| `ai.provider` | string | `settings.rfq_llm_provider` (+ full object kept in `erp.x.ai`) | mapped |
| `ai.rfq` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.ai`) | mapped |
| `bank.accountName` | string | `bank_account.account_name` (+ full object kept in `erp.x.bank`) | mapped |
| `bank.accountNo` | string | `bank_account.account_no` (+ full object kept in `erp.x.bank`) | mapped |
| `bank.iban` | string | `bank_account.iban` (+ full object kept in `erp.x.bank`) | mapped |
| `bank.name` | string | `bank_account.bank_name` (+ full object kept in `erp.x.bank`) | mapped |
| `bank.swift` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.bank`) | mapped |
| `branchAr` | string | `erp.x.branchAr` (adapter extras; round-trips, invisible to the old app) | new |
| `branchEn` | string | `branch_name` | mapped |
| `category` | string | `business_category` | mapped |
| `crNo` | string | `registration_number_in_arabic`, `registration_number` | mapped |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `currency.code` | string | read from legacy; not written by the adapter | computed |
| `currency.fractionAr` | string | read from legacy; not written by the adapter | computed |
| `currency.fractionEn` | string | read from legacy; not written by the adapter | computed |
| `currency.nameAr` | string | read from legacy; not written by the adapter | computed |
| `currency.nameEn` | string | read from legacy; not written by the adapter | computed |
| `dateFormat` | string | `erp.x.dateFormat` (adapter extras; round-trips, invisible to the old app) | new |
| `decimals` | number | `erp.x.decimals` (adapter extras; round-trips, invisible to the old app) | new |
| `defaultPrinter` | string | `erp.x.defaultPrinter` (adapter extras; round-trips, invisible to the old app) | new |
| `defaultSalesFormType` | string | `erp.x.defaultSalesFormType` (adapter extras; round-trips, invisible to the old app) | new |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `design.color` | string | `erp.x.design.color` (adapter extras; round-trips, invisible to the old app) | new |
| `design.font` | string | `erp.x.design.font` (adapter extras; round-trips, invisible to the old app) | new |
| `design.footerText` | string | `erp.x.design.footerText` (adapter extras; round-trips, invisible to the old app) | new |
| `design.showBank` | boolean | `erp.x.design.showBank` (adapter extras; round-trips, invisible to the old app) | new |
| `design.showFooter` | boolean | `erp.x.design.showFooter` (adapter extras; round-trips, invisible to the old app) | new |
| `design.showLogo` | boolean | `erp.x.design.showLogo` (adapter extras; round-trips, invisible to the old app) | new |
| `design.template` | string | `erp.x.design.template` (adapter extras; round-trips, invisible to the old app) | new |
| `design.watermark` | boolean | `erp.x.design.watermark` (adapter extras; round-trips, invisible to the old app) | new |
| `design.watermarkText` | string | `erp.x.design.watermarkText` (adapter extras; round-trips, invisible to the old app) | new |
| `email` | string | `email` | mapped |
| `emailSettings.accounts[].id` | string | mapped (empty in sample, not probed) (+ full object kept in `erp.x.emailSettings`) | mapped |
| `emailSettings.accounts[].imapHost` | string | mapped (empty in sample, not probed) (+ full object kept in `erp.x.emailSettings`) | mapped |
| `emailSettings.accounts[].imapPort` | number | mapped (empty in sample, not probed) (+ full object kept in `erp.x.emailSettings`) | mapped |
| `emailSettings.accounts[].label` | string | mapped (empty in sample, not probed) (+ full object kept in `erp.x.emailSettings`) | mapped |
| `emailSettings.accounts[].polling` | boolean | mapped (empty in sample, not probed) (+ full object kept in `erp.x.emailSettings`) | mapped |
| `emailSettings.accounts[].user` | string | mapped (empty in sample, not probed) (+ full object kept in `erp.x.emailSettings`) | mapped |
| `emailSettings.smtp.fromEmail` | string | `settings.outgoing_email_from_address` (+ full object kept in `erp.x.emailSettings`) | mapped |
| `emailSettings.smtp.fromName` | string | `settings.outgoing_email_from_name` (+ full object kept in `erp.x.emailSettings`) | mapped |
| `emailSettings.smtp.host` | string | `settings.outgoing_email_smtp_host` (+ full object kept in `erp.x.emailSettings`) | mapped |
| `emailSettings.smtp.port` | number | `settings.outgoing_email_smtp_port` (+ full object kept in `erp.x.emailSettings`) | mapped |
| `emailSettings.smtp.ssl` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.emailSettings`) | mapped |
| `emailSettings.smtp.tls` | boolean | `settings.outgoing_email_smtp_use_tls` (+ full object kept in `erp.x.emailSettings`) | mapped |
| `emailSettings.smtp.user` | string | `settings.outgoing_email_smtp_username` (+ full object kept in `erp.x.emailSettings`) | mapped |
| `flags.enable_ai_rfq_bot` | boolean | `settings.enable_ai_rfq_bot` (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_automobile_module` | boolean | `settings.enable_automobile_module` (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_cash_discount` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_commission` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_customer_packages` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_customer_po` | boolean | `settings.enable_customer_po_no` (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_draft_orders` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_employee_module` | boolean | `settings.enable_employee_module` (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_product_barcode` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_product_brand` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_product_country` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_product_rack` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_purchase_bills` | boolean | `settings.enable_purchase_bills_tracking` (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_rbac_module` | boolean | `settings.enable_rbac_module` (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_signature` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.flags`) | mapped |
| `flags.enable_warehouse_module` | boolean | `settings.enable_warehouse_module` (+ full object kept in `erp.x.flags`) | mapped |
| `flags.show_purchase_price_in_sales` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.flags`) | mapped |
| `flags.show_stock_in_sales` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.flags`) | mapped |
| `google.mapsKey` | string | `settings.google_maps_api_key` | mapped |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `invoiceBg` | null | `remove_invoice_background` | mapped |
| `labels.customer` | string | `erp.x.labels.customer` (adapter extras; round-trips, invisible to the old app) | new |
| `labels.product` | string | `erp.x.labels.product` (adapter extras; round-trips, invisible to the old app) | new |
| `labels.vendor` | string | `erp.x.labels.vendor` (adapter extras; round-trips, invisible to the old app) | new |
| `logo` | null | `remove_logo` | mapped |
| `nameAr` | string | `name_in_arabic` | mapped |
| `nameEn` | string | `name` | mapped |
| `openingBalances.ap` | number | accepted; no legacy effect for this value (+ full object kept in `erp.x.openingBalances`) | mapped |
| `openingBalances.ar` | number | accepted; no legacy effect for this value (+ full object kept in `erp.x.openingBalances`) | mapped |
| `openingBalances.asOf` | string | read from legacy; not written by the adapter | computed |
| `openingBalances.bank` | number | `settings.bank_opening_balance` (+ full object kept in `erp.x.openingBalances`) | mapped |
| `openingBalances.cash` | number | `settings.cash_opening_balance` (+ full object kept in `erp.x.openingBalances`) | mapped |
| `openingBalances.inventory` | number | accepted; no legacy effect for this value (+ full object kept in `erp.x.openingBalances`) | mapped |
| `phone` | string | `phone_in_arabic`, `phone` | mapped |
| `phone2` | string | `erp.x.phone2` (adapter extras; round-trips, invisible to the old app) | new |
| `plan` | string | `erp.x.plan` (adapter extras; round-trips, invisible to the old app) | new |
| `printA4` | boolean | `erp.x.printA4` (adapter extras; round-trips, invisible to the old app) | new |
| `printThermal` | boolean | `erp.x.printThermal` (adapter extras; round-trips, invisible to the old app) | new |
| `purchaseBills.ai` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.purchaseBills`) | mapped |
| `purchaseBills.email` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.purchaseBills`) | mapped |
| `purchaseBills.enabled` | boolean | `settings.enable_purchase_bills_tracking` (+ full object kept in `erp.x.purchaseBills`) | mapped |
| `purchaseBills.model` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.purchaseBills`) | mapped |
| `purchaseColumns` | array | `erp.x.purchaseColumns` (adapter extras; round-trips, invisible to the old app) | new |
| `rfq.autoEmail` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.rfq`) | mapped |
| `rfq.autoWhatsapp` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.rfq`) | mapped |
| `rfq.intro` | string | `settings.rfq_intro` (+ full object kept in `erp.x.rfq`) | mapped |
| `rfq.templates.delivery` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.rfq`) | mapped |
| `rfq.templates.invoice` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.rfq`) | mapped |
| `rfq.templates.quotation` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.rfq`) | mapped |
| `rfq.templates.rfq` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.rfq`) | mapped |
| `rfq.validityDays` | number | accepted; no legacy effect for this value (+ full object kept in `erp.x.rfq`) | mapped |
| `roundingMode` | string | `erp.x.roundingMode` (adapter extras; round-trips, invisible to the old app) | new |
| `salesColumns` | array | `erp.x.salesColumns` (adapter extras; round-trips, invisible to the old app) | new |
| `salesLockDays` | number | `erp.x.salesLockDays` (adapter extras; round-trips, invisible to the old app) | new |
| `scale` | number | `erp.x.scale` (adapter extras; round-trips, invisible to the old app) | new |
| `serials.capital.prefix` | string | `capital_deposit_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.capital.start` | number | `capital_deposit_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.capitalWithdrawal.prefix` | string | read from legacy; not written by the adapter | computed |
| `serials.capitalWithdrawal.start` | number | read from legacy; not written by the adapter | computed |
| `serials.customer.prefix` | string | `customer_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.customer.start` | number | `customer_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.deliveryNote.prefix` | string | `delivery_note_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.deliveryNote.start` | number | `delivery_note_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.deposit.prefix` | string | `customer_deposit_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.deposit.start` | number | `customer_deposit_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.dividend.prefix` | string | `divident_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.dividend.start` | number | `divident_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.employee.prefix` | string | read from legacy; not written by the adapter | computed |
| `serials.employee.start` | number | read from legacy; not written by the adapter | computed |
| `serials.expense.prefix` | string | `expense_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.expense.start` | number | `expense_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.nonvat.prefix` | string | `non_vat_sales_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.nonvat.start` | number | `non_vat_sales_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.nonvatReturn.prefix` | string | `non_vat_sales_return_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.nonvatReturn.start` | number | `non_vat_sales_return_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.product.prefix` | string | read from legacy; not written by the adapter | computed |
| `serials.product.start` | number | read from legacy; not written by the adapter | computed |
| `serials.proforma.prefix` | string | read from legacy; not written by the adapter | computed |
| `serials.proforma.start` | number | read from legacy; not written by the adapter | computed |
| `serials.purchase.prefix` | string | `purchase_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.purchase.start` | number | `purchase_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.purchaseBill.prefix` | string | read from legacy; not written by the adapter | computed |
| `serials.purchaseBill.start` | number | read from legacy; not written by the adapter | computed |
| `serials.purchaseOrder.prefix` | string | `purchase_order_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.purchaseOrder.start` | number | `purchase_order_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.purchaseRequest.prefix` | string | `purchase_request_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.purchaseRequest.start` | number | `purchase_request_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.purchaseReturn.prefix` | string | `purchase_return_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.purchaseReturn.start` | number | `purchase_return_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.quotation.prefix` | string | `quotation_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.quotation.start` | number | `quotation_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.quotationReturn.prefix` | string | `quotation_sales_return_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.quotationReturn.start` | number | `quotation_sales_return_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.repairJob.prefix` | string | read from legacy; not written by the adapter | computed |
| `serials.repairJob.start` | number | read from legacy; not written by the adapter | computed |
| `serials.rfq.prefix` | string | `rfq_received_serial_number.prefix` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.rfq.start` | number | `rfq_received_serial_number.start_from_count` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.salary.prefix` | string | read from legacy; not written by the adapter | computed |
| `serials.salary.start` | number | read from legacy; not written by the adapter | computed |
| `serials.sales.prefix` | string | `sales_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.sales.start` | number | `sales_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.salesReturn.prefix` | string | `sales_return_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.salesReturn.start` | number | `sales_return_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.stockTransfer.prefix` | string | `stock_transfer_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.stockTransfer.start` | number | `stock_transfer_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.vehicle.prefix` | string | read from legacy; not written by the adapter | computed |
| `serials.vehicle.start` | number | read from legacy; not written by the adapter | computed |
| `serials.vendor.prefix` | string | `vendor_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.vendor.start` | number | `vendor_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.withdrawal.prefix` | string | `customer_withdrawal_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `serials.withdrawal.start` | number | `customer_withdrawal_serial_number` (+ full object kept in `erp.x.serials`) | mapped |
| `short` | string | read-only, derived from the legacy document | computed |
| `sidebarHidden` | array | `erp.x.sidebarHidden` (adapter extras; round-trips, invisible to the old app) | new |
| `timezone` | string | read from legacy; not written by the adapter | computed |
| `titles.deliveryAr` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.titles`) | mapped |
| `titles.deliveryEn` | string | `settings.invoice.delivery_note_title` (+ full object kept in `erp.x.titles`) | mapped |
| `titles.invoiceAr` | string | `title_in_arabic` (+ full object kept in `erp.x.titles`) | mapped |
| `titles.invoiceEn` | string | `title` (+ full object kept in `erp.x.titles`) | mapped |
| `titles.purchaseReturnAr` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.titles`) | mapped |
| `titles.purchaseReturnEn` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.titles`) | mapped |
| `titles.quotationAr` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.titles`) | mapped |
| `titles.quotationEn` | string | `settings.invoice.quotation_title` (+ full object kept in `erp.x.titles`) | mapped |
| `titles.salesReturnAr` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.titles`) | mapped |
| `titles.salesReturnEn` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.titles`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatNo` | string | `vat_no_in_arabic`, `vat_no` | mapped |
| `vatPercent` | number | `vat_percent` | mapped |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |
| `website` | string | `erp.x.website` (adapter extras; round-trips, invisible to the old app) | new |
| `whatsapp.evolution.apiKey` | string | `settings.evolution_api_key` (+ full object kept in `erp.x.whatsapp`) | mapped |
| `whatsapp.evolution.instance` | string | `settings.evolution_instance_name` (+ full object kept in `erp.x.whatsapp`) | mapped |
| `whatsapp.evolution.status` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.whatsapp`) | mapped |
| `whatsapp.evolution.url` | string | `settings.evolution_api_url` (+ full object kept in `erp.x.whatsapp`) | mapped |
| `whatsapp.mode` | string | read from legacy; not written by the adapter | computed |
| `whatsapp.waba.businessAccountId` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.whatsapp`) | mapped |
| `whatsapp.waba.phoneNumberId` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.whatsapp`) | mapped |
| `whatsapp.waba.token` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.whatsapp`) | mapped |
| `whatsapp.waba.verified` | boolean | accepted; no legacy effect for this value (+ full object kept in `erp.x.whatsapp`) | mapped |
| `zatca.certExpires` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.zatca`) | mapped |
| `zatca.complianceChecks` | integer | accepted; no legacy effect for this value (+ full object kept in `erp.x.zatca`) | mapped |
| `zatca.connected` | boolean | read from legacy; not written by the adapter | computed |
| `zatca.connectedAt` | string | read from legacy; not written by the adapter | computed |
| `zatca.disconnectedAt` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.zatca`) | mapped |
| `zatca.keyId` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.zatca`) | mapped |
| `zatca.pcsid` | string | read from legacy; not written by the adapter | computed |
| `zatca.phase` | number | read from legacy; not written by the adapter | computed |
| `zatca.reconnectNeeded` | boolean | read from legacy; not written by the adapter | computed |
| `zatca.snapshot` | string | accepted; no legacy effect for this value (+ full object kept in `erp.x.zatca`) | mapped |

### users — `/v1/erp/users`

**Legacy** main DB `user` — collection `user` (main DB). Writes go through the existing v1 handlers.

- restore → 409: Restoring users is not supported by the existing system; create the user again.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `admin` | boolean | read from legacy; not written by the adapter | computed |
| `apiRole` | string | read from legacy; not written by the adapter | computed |
| `apiTokens[].createdAt` | string | `erp.x.apiTokens[].createdAt` (adapter extras; round-trips, invisible to the old app) | new |
| `apiTokens[].expires` | string|null | `erp.x.apiTokens[].expires` (adapter extras; round-trips, invisible to the old app) | new |
| `apiTokens[].id` | string | `erp.x.apiTokens[].id` (adapter extras; round-trips, invisible to the old app) | new |
| `apiTokens[].last4` | string | `erp.x.apiTokens[].last4` (adapter extras; round-trips, invisible to the old app) | new |
| `apiTokens[].lastUsed` | string|null | `erp.x.apiTokens[].lastUsed` (adapter extras; round-trips, invisible to the old app) | new |
| `apiTokens[].name` | string | `erp.x.apiTokens[].name` (adapter extras; round-trips, invisible to the old app) | new |
| `apiTokens[].prefix` | string | `erp.x.apiTokens[].prefix` (adapter extras; round-trips, invisible to the old app) | new |
| `apiTokens[].scopes` | array | `erp.x.apiTokens[].scopes` (adapter extras; round-trips, invisible to the old app) | new |
| `avatar` | string|null | accepted; no legacy effect for this value | mapped |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `email` | string | `email` | mapped |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `jobTitle` | string | `erp.x.jobTitle` (adapter extras; round-trips, invisible to the old app) | new |
| `lastLogin` | string | read-only, derived from the legacy document | computed |
| `mustChangePassword` | boolean | `erp.x.mustChangePassword` (adapter extras; round-trips, invisible to the old app) | new |
| `name` | string | `name` | mapped |
| `passwordChangedAt` | string | `erp.x.passwordChangedAt` (adapter extras; round-trips, invisible to the old app) | new |
| `phone` | string | `mob` | mapped |
| `role` | string | `role` | mapped |
| `status` | string | read-only, derived from the legacy document | computed |
| `storeIds` | array | `store_ids` | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### roles — `/v1/erp/roles`

**New adapter collection** `erp_role` (main DB) — no legacy equivalent; every field is stored as sent. system roles are built in (read-only); legacy `user_role` documents are listed read-only; custom roles are stored in `erp_role` (main DB)

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `createdAt` | string(datetime-local) | envelope / server-owned | server |
| `createdBy` | string | envelope / server-owned | server |
| `deleted` | boolean | envelope / server-owned | server |
| `description` | string | `erp_role.description` | new |
| `flags.deleteRecords` | boolean | `erp_role.flags.deleteRecords` | new |
| `flags.manageUsers` | boolean | `erp_role.flags.manageUsers` | new |
| `flags.viewReports` | boolean | `erp_role.flags.viewReports` | new |
| `history[].action` | string | `erp_role.history[].action` | new |
| `history[].at` | string | `erp_role.history[].at` | new |
| `history[].by` | string | `erp_role.history[].by` | new |
| `history[].changes[].field` | string | `erp_role.history[].changes[].field` | new |
| `history[].changes[].from` | string | `erp_role.history[].changes[].from` | new |
| `history[].changes[].to` | string | `erp_role.history[].changes[].to` | new |
| `id` | string | envelope / server-owned | server |
| `maxDiscount` | number | `erp_role.maxDiscount` | new |
| `name` | string | `erp_role.name` | new |
| `perms.customers.create` | boolean | `erp_role.perms.customers.create` | new |
| `perms.customers.delete` | boolean | `erp_role.perms.customers.delete` | new |
| `perms.customers.edit` | boolean | `erp_role.perms.customers.edit` | new |
| `perms.customers.export` | boolean | `erp_role.perms.customers.export` | new |
| `perms.customers.print` | boolean | `erp_role.perms.customers.print` | new |
| `perms.customers.view` | boolean | `erp_role.perms.customers.view` | new |
| `perms.finance.create` | boolean | `erp_role.perms.finance.create` | new |
| `perms.finance.delete` | boolean | `erp_role.perms.finance.delete` | new |
| `perms.finance.edit` | boolean | `erp_role.perms.finance.edit` | new |
| `perms.finance.export` | boolean | `erp_role.perms.finance.export` | new |
| `perms.finance.print` | boolean | `erp_role.perms.finance.print` | new |
| `perms.finance.view` | boolean | `erp_role.perms.finance.view` | new |
| `perms.hr.create` | boolean | `erp_role.perms.hr.create` | new |
| `perms.hr.delete` | boolean | `erp_role.perms.hr.delete` | new |
| `perms.hr.edit` | boolean | `erp_role.perms.hr.edit` | new |
| `perms.hr.export` | boolean | `erp_role.perms.hr.export` | new |
| `perms.hr.print` | boolean | `erp_role.perms.hr.print` | new |
| `perms.hr.view` | boolean | `erp_role.perms.hr.view` | new |
| `perms.inventory.create` | boolean | `erp_role.perms.inventory.create` | new |
| `perms.inventory.delete` | boolean | `erp_role.perms.inventory.delete` | new |
| `perms.inventory.edit` | boolean | `erp_role.perms.inventory.edit` | new |
| `perms.inventory.export` | boolean | `erp_role.perms.inventory.export` | new |
| `perms.inventory.print` | boolean | `erp_role.perms.inventory.print` | new |
| `perms.inventory.view` | boolean | `erp_role.perms.inventory.view` | new |
| `perms.purchases.create` | boolean | `erp_role.perms.purchases.create` | new |
| `perms.purchases.delete` | boolean | `erp_role.perms.purchases.delete` | new |
| `perms.purchases.edit` | boolean | `erp_role.perms.purchases.edit` | new |
| `perms.purchases.export` | boolean | `erp_role.perms.purchases.export` | new |
| `perms.purchases.print` | boolean | `erp_role.perms.purchases.print` | new |
| `perms.purchases.view` | boolean | `erp_role.perms.purchases.view` | new |
| `perms.reports.create` | boolean | `erp_role.perms.reports.create` | new |
| `perms.reports.delete` | boolean | `erp_role.perms.reports.delete` | new |
| `perms.reports.edit` | boolean | `erp_role.perms.reports.edit` | new |
| `perms.reports.export` | boolean | `erp_role.perms.reports.export` | new |
| `perms.reports.print` | boolean | `erp_role.perms.reports.print` | new |
| `perms.reports.view` | boolean | `erp_role.perms.reports.view` | new |
| `perms.sales.create` | boolean | `erp_role.perms.sales.create` | new |
| `perms.sales.delete` | boolean | `erp_role.perms.sales.delete` | new |
| `perms.sales.edit` | boolean | `erp_role.perms.sales.edit` | new |
| `perms.sales.export` | boolean | `erp_role.perms.sales.export` | new |
| `perms.sales.print` | boolean | `erp_role.perms.sales.print` | new |
| `perms.sales.view` | boolean | `erp_role.perms.sales.view` | new |
| `perms.settings.create` | boolean | `erp_role.perms.settings.create` | new |
| `perms.settings.delete` | boolean | `erp_role.perms.settings.delete` | new |
| `perms.settings.edit` | boolean | `erp_role.perms.settings.edit` | new |
| `perms.settings.export` | boolean | `erp_role.perms.settings.export` | new |
| `perms.settings.print` | boolean | `erp_role.perms.settings.print` | new |
| `perms.settings.view` | boolean | `erp_role.perms.settings.view` | new |
| `perms.vendors.create` | boolean | `erp_role.perms.vendors.create` | new |
| `perms.vendors.delete` | boolean | `erp_role.perms.vendors.delete` | new |
| `perms.vendors.edit` | boolean | `erp_role.perms.vendors.edit` | new |
| `perms.vendors.export` | boolean | `erp_role.perms.vendors.export` | new |
| `perms.vendors.print` | boolean | `erp_role.perms.vendors.print` | new |
| `perms.vendors.view` | boolean | `erp_role.perms.vendors.view` | new |
| `perms.workshop.create` | boolean | `erp_role.perms.workshop.create` | new |
| `perms.workshop.delete` | boolean | `erp_role.perms.workshop.delete` | new |
| `perms.workshop.edit` | boolean | `erp_role.perms.workshop.edit` | new |
| `perms.workshop.export` | boolean | `erp_role.perms.workshop.export` | new |
| `perms.workshop.print` | boolean | `erp_role.perms.workshop.print` | new |
| `perms.workshop.view` | boolean | `erp_role.perms.workshop.view` | new |
| `system` | boolean | `erp_role.system` | new |
| `updatedAt` | string(datetime-local) | envelope / server-owned | server |
| `updatedBy` | string | envelope / server-owned | server |
| `version` | integer | envelope / server-owned | server |

### categories — `/v1/erp/categories`

**Legacy** store DBs `product_category` (union) — collection `product_category` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `description` | string | `erp.x.description` (adapter extras; round-trips, invisible to the old app) | new |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `nameAr` | string | `erp.x.nameAr` (adapter extras; round-trips, invisible to the old app) | new |
| `nameEn` | string | `name` | mapped |
| `parentId` | string | `parent_id` | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### brands — `/v1/erp/brands`

**Legacy** store DBs `product_brand` (union) — collection `product_brand` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `country` | string | `erp.x.country` (adapter extras; round-trips, invisible to the old app) | new |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `logo` | string|null | `erp.x.logo` (adapter extras; round-trips, invisible to the old app) | new |
| `name` | string | `code`, `name` | mapped |
| `nameAr` | string | `erp.x.nameAr` (adapter extras; round-trips, invisible to the old app) | new |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |
| `website` | string | `erp.x.website` (adapter extras; round-trips, invisible to the old app) | new |

### customerCategories — `/v1/erp/customer-categories`

**New adapter collection** `erp_customer_category` (main DB) — no legacy equivalent; every field is stored as sent. 

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `createdAt` | string(datetime-local) | envelope / server-owned | server |
| `createdBy` | string | envelope / server-owned | server |
| `deleted` | boolean | envelope / server-owned | server |
| `history[].action` | string | `erp_customer_category.history[].action` | new |
| `history[].at` | string | `erp_customer_category.history[].at` | new |
| `history[].by` | string | `erp_customer_category.history[].by` | new |
| `history[].changes[].field` | string | `erp_customer_category.history[].changes[].field` | new |
| `history[].changes[].from` | string | `erp_customer_category.history[].changes[].from` | new |
| `history[].changes[].to` | string | `erp_customer_category.history[].changes[].to` | new |
| `id` | string | envelope / server-owned | server |
| `name` | string | `erp_customer_category.name` | new |
| `updatedAt` | string(datetime-local) | envelope / server-owned | server |
| `updatedBy` | string | envelope / server-owned | server |
| `version` | integer | envelope / server-owned | server |

### vendorCategories — `/v1/erp/vendor-categories`

**Legacy** store DBs `vendor_category` (union) — collection `vendor_category` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `name` | string | `name` | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### expenseCategories — `/v1/erp/expense-categories`

**Legacy** store DBs `expense_category` (union) — collection `expense_category` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `nameAr` | string | `erp.x.nameAr` (adapter extras; round-trips, invisible to the old app) | new |
| `nameEn` | string | `name` | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### accounts — `/v1/erp/accounts`

**Legacy** store DBs `account` (read-only) — collection `account` (store DB). Writes go through the existing v1 handlers (read-only resource).

- read-only

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `code` | string | read from legacy (read-only resource) | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `nameAr` | string | read from legacy (read-only resource) | computed |
| `nameEn` | string | read from legacy (read-only resource) | computed |
| `openingBalance` | number | read-only, derived from the legacy document | computed |
| `type` | string | read from legacy (read-only resource) | computed |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### warehouses — `/v1/erp/warehouses`

**Legacy** store DB `warehouse` + virtual main store — collection `warehouse` (store DB). Writes go through the existing v1 handlers.

- plus a virtual read-only warehouse `ms_<storeId>` = the legacy `main_store` stock bucket (warehouse_id null)

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `address` | string | `address` | mapped |
| `code` | string | `code` | mapped |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `manager` | string | `erp.x.manager` (adapter extras; round-trips, invisible to the old app) | new |
| `nameAr` | string | `name_in_arabic` | mapped |
| `nameEn` | string | `name` | mapped |
| `notes` | string | `erp.x.notes` (adapter extras; round-trips, invisible to the old app) | new |
| `phone` | string | `phone` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### products — `/v1/erp/products`

**Legacy** store DB `product` (product_stores[store]) — collection `product` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `adjustments[].after` | number | `erp.x.adjustments[].after` (adapter extras; round-trips, invisible to the old app) | new |
| `adjustments[].before` | number | `erp.x.adjustments[].before` (adapter extras; round-trips, invisible to the old app) | new |
| `adjustments[].by` | string | `erp.x.adjustments[].by` (adapter extras; round-trips, invisible to the old app) | new |
| `adjustments[].date` | string | `erp.x.adjustments[].date` (adapter extras; round-trips, invisible to the old app) | new |
| `adjustments[].id` | string | `erp.x.adjustments[].id` (adapter extras; round-trips, invisible to the old app) | new |
| `adjustments[].note` | string | `erp.x.adjustments[].note` (adapter extras; round-trips, invisible to the old app) | new |
| `adjustments[].qty` | number | `erp.x.adjustments[].qty` (adapter extras; round-trips, invisible to the old app) | new |
| `adjustments[].reason` | string | `erp.x.adjustments[].reason` (adapter extras; round-trips, invisible to the old app) | new |
| `adjustments[].type` | string | `erp.x.adjustments[].type` (adapter extras; round-trips, invisible to the old app) | new |
| `adjustments[].warehouseId` | string | `erp.x.adjustments[].warehouseId` (adapter extras; round-trips, invisible to the old app) | new |
| `barcode` | string | `bar_code` | mapped |
| `brandId` | string | mapped (not probed) | mapped |
| `categoryIds` | array | `category_id` | mapped |
| `code` | string | `item_code` | mapped |
| `components[].productId` | string | mapped (empty in sample, not probed) | mapped |
| `components[].qty` | number | mapped (empty in sample, not probed) | mapped |
| `country` | string | `country_name` | mapped |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `images` | array | `images` | mapped |
| `isService` | boolean | `is_service` | mapped |
| `isSet` | boolean | `is_set` | mapped |
| `keywords` | array | `additional_keywords` | mapped |
| `linkedIds` | array | `linked_product_ids` | mapped |
| `nameAr` | string | `name_in_arabic` | mapped |
| `nameEn` | string | `name` | mapped |
| `note` | string | `note` | mapped |
| `partNo` | string | `part_number` | mapped |
| `prefixPartNo` | string | `prefix_part_number` | mapped |
| `priceHistory[].date` | string | `erp.x.priceHistory[].date` (adapter extras; round-trips, invisible to the old app) | new |
| `priceHistory[].price` | number | `erp.x.priceHistory[].price` (adapter extras; round-trips, invisible to the old app) | new |
| `priceHistory[].source` | string | `erp.x.priceHistory[].source` (adapter extras; round-trips, invisible to the old app) | new |
| `pricing.autoRetail` | boolean | `product_stores.<storeId>.auto_update_retail_price_from_last_purchase` (+ full object kept in `erp.x.pricing`) | mapped |
| `pricing.autoWholesale` | boolean | `product_stores.<storeId>.auto_update_wholesale_price_from_last_purchase` (+ full object kept in `erp.x.pricing`) | mapped |
| `pricing.max` | number | accepted; no legacy effect for this value (+ full object kept in `erp.x.pricing`) | mapped |
| `pricing.min` | number | accepted; no legacy effect for this value (+ full object kept in `erp.x.pricing`) | mapped |
| `pricing.purchase` | number | `product_stores.<storeId>.purchase_unit_price_with_vat`, `product_stores.<storeId>.purchase_unit_price` (+ full object kept in `erp.x.pricing`) | mapped |
| `pricing.retail` | number | `product_stores.<storeId>.retail_unit_price_with_vat`, `product_stores.<storeId>.retail_unit_price` (+ full object kept in `erp.x.pricing`) | mapped |
| `pricing.retailMargin` | number | `product_stores.<storeId>.retail_margin_percent` (+ full object kept in `erp.x.pricing`) | mapped |
| `pricing.wholesale` | number | `product_stores.<storeId>.wholesale_unit_price_with_vat`, `product_stores.<storeId>.wholesale_unit_price` (+ full object kept in `erp.x.pricing`) | mapped |
| `pricing.wholesaleMargin` | number | `product_stores.<storeId>.wholesale_margin_percent` (+ full object kept in `erp.x.pricing`) | mapped |
| `rack` | string | `rack` | mapped |
| `stock.<warehouseId>.min` | number | accepted; no legacy effect for this value (+ full object kept in `erp.x.stock`) | mapped |
| `stock.<warehouseId>.qty` | number | `product_stores.<storeId>.stock_adjustments` (+ full object kept in `erp.x.stock`) | mapped |
| `stock.<warehouseId>.rack` | string | `product_stores.<storeId>.warehouse_racks.main_store` (+ full object kept in `erp.x.stock`) | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `unit` | string | `unit` | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatPercent` | number | `erp.x.vatPercent` (adapter extras; round-trips, invisible to the old app) | new |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### customers — `/v1/erp/customers`

**Legacy** store DB `customer` — collection `customer` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `address.additionalNo` | string | `national_address.additional_no_arabic`, `national_address.additional_no` (+ full object kept in `erp.x.address`) | mapped |
| `address.buildingNo` | string | `national_address.building_no_arabic`, `national_address.building_no` (+ full object kept in `erp.x.address`) | mapped |
| `address.cityAr` | string | `national_address.city_name_arabic` (+ full object kept in `erp.x.address`) | mapped |
| `address.cityEn` | string | `national_address.city_name` (+ full object kept in `erp.x.address`) | mapped |
| `address.countryAr` | string | read from legacy; not written by the adapter | computed |
| `address.countryEn` | string | `country_code`, `country_name` (+ full object kept in `erp.x.address`) | mapped |
| `address.districtAr` | string | `national_address.district_name_arabic` (+ full object kept in `erp.x.address`) | mapped |
| `address.districtEn` | string | `national_address.district_name` (+ full object kept in `erp.x.address`) | mapped |
| `address.postalCode` | string | `national_address.zipcode_arabic`, `national_address.zipcode` (+ full object kept in `erp.x.address`) | mapped |
| `address.streetAr` | string | `national_address.street_name_arabic` (+ full object kept in `erp.x.address`) | mapped |
| `address.streetEn` | string | `national_address.street_name` (+ full object kept in `erp.x.address`) | mapped |
| `category` | array | `erp.x.category` (adapter extras; round-trips, invisible to the old app) | new |
| `code` | string | read from legacy; not written by the adapter | computed |
| `crNo` | string | `registration_number` | mapped |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `creditLimit` | number | `credit_limit` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `email` | string | `email` | mapped |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `nameAr` | string | `name_in_arabic` | mapped |
| `nameEn` | string | `name` | mapped |
| `note` | string | `erp.x.note` (adapter extras; round-trips, invisible to the old app) | new |
| `openingBalance` | number | `opening_balance` | mapped |
| `openingBalanceDate` | string | `opening_balance_date` | mapped |
| `openingBalanceType` | string | `opening_balance_type` | mapped |
| `phone` | string | `phone_in_arabic`, `phone` | mapped |
| `phone2` | string | `phone2` | mapped |
| `phone2Ar` | string | `phone2_in_arabic` | mapped |
| `phoneAr` | string | `phone_in_arabic` | mapped |
| `photo` | string|null | `erp.x.photo` (adapter extras; round-trips, invisible to the old app) | new |
| `remarks` | string | `remarks` | mapped |
| `status` | string | `erp.x.status` (adapter extras; round-trips, invisible to the old app) | new |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `titleAr` | string | `title_in_arabic` | mapped |
| `titleEn` | string | `title` | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatNo` | string | `vat_no_in_arabic`, `vat_no` | mapped |
| `vatPercent` | number | `erp.x.vatPercent` (adapter extras; round-trips, invisible to the old app) | new |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |
| `website` | string | `erp.x.website` (adapter extras; round-trips, invisible to the old app) | new |

### vendors — `/v1/erp/vendors`

**Legacy** store DB `vendor` — collection `vendor` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `address.additionalNo` | string | `national_address.additional_no_arabic`, `national_address.additional_no` (+ full object kept in `erp.x.address`) | mapped |
| `address.buildingNo` | string | `national_address.building_no_arabic`, `national_address.building_no` (+ full object kept in `erp.x.address`) | mapped |
| `address.cityAr` | string | `national_address.city_name_arabic` (+ full object kept in `erp.x.address`) | mapped |
| `address.cityEn` | string | `national_address.city_name` (+ full object kept in `erp.x.address`) | mapped |
| `address.countryAr` | string | read from legacy; not written by the adapter | computed |
| `address.countryEn` | string | `country_code`, `country_name` (+ full object kept in `erp.x.address`) | mapped |
| `address.districtAr` | string | `national_address.district_name_arabic` (+ full object kept in `erp.x.address`) | mapped |
| `address.districtEn` | string | `national_address.district_name` (+ full object kept in `erp.x.address`) | mapped |
| `address.postalCode` | string | `national_address.zipcode_arabic`, `national_address.zipcode` (+ full object kept in `erp.x.address`) | mapped |
| `address.streetAr` | string | `national_address.street_name_arabic` (+ full object kept in `erp.x.address`) | mapped |
| `address.streetEn` | string | `national_address.street_name` (+ full object kept in `erp.x.address`) | mapped |
| `bank.accountName` | string | `erp.x.bank.accountName` (adapter extras; round-trips, invisible to the old app) | new |
| `bank.accountNo` | string | `erp.x.bank.accountNo` (adapter extras; round-trips, invisible to the old app) | new |
| `bank.iban` | string | `erp.x.bank.iban` (adapter extras; round-trips, invisible to the old app) | new |
| `bank.name` | string | `erp.x.bank.name` (adapter extras; round-trips, invisible to the old app) | new |
| `bank.swift` | string | `erp.x.bank.swift` (adapter extras; round-trips, invisible to the old app) | new |
| `category` | array | read from legacy; not written by the adapter | computed |
| `code` | string | read from legacy; not written by the adapter | computed |
| `crNo` | string | `registration_number` | mapped |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `creditLimit` | number | `credit_limit` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `email` | string | `email` | mapped |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `logo` | null | read from legacy; not written by the adapter | computed |
| `nameAr` | string | `name_in_arabic` | mapped |
| `nameEn` | string | `name` | mapped |
| `note` | string | `erp.x.note` (adapter extras; round-trips, invisible to the old app) | new |
| `openingBalance` | number | `opening_balance` | mapped |
| `openingBalanceDate` | string | `opening_balance_date` | mapped |
| `openingBalanceType` | string | `opening_balance_type` | mapped |
| `phone` | string | `phone_in_arabic`, `phone` | mapped |
| `phone2` | string | `erp.x.phone2` (adapter extras; round-trips, invisible to the old app) | new |
| `productCategories` | array | `product_categories` | mapped |
| `remarks` | string | `remarks` | mapped |
| `sponsor` | string | `sponsor` | mapped |
| `status` | string | `erp.x.status` (adapter extras; round-trips, invisible to the old app) | new |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `useRemarksInPurchases` | boolean | `use_remarks_in_purchases` | mapped |
| `vatNo` | string | `vat_no_in_arabic`, `vat_no` | mapped |
| `vatPercent` | number | `vat_percent` | mapped |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |
| `website` | string | `erp.x.website` (adapter extras; round-trips, invisible to the old app) | new |

### employees — `/v1/erp/employees`

**Legacy** store DB `employee` — collection `employee` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `advances[].amount` | number | `erp.x.advances[].amount` (adapter extras; round-trips, invisible to the old app) | new |
| `advances[].by` | string | `erp.x.advances[].by` (adapter extras; round-trips, invisible to the old app) | new |
| `advances[].date` | string | `erp.x.advances[].date` (adapter extras; round-trips, invisible to the old app) | new |
| `advances[].reference` | string | `erp.x.advances[].reference` (adapter extras; round-trips, invisible to the old app) | new |
| `basicSalary` | number | `salary` | mapped |
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `department` | string | `erp.x.department` (adapter extras; round-trips, invisible to the old app) | new |
| `email` | string | `erp.x.email` (adapter extras; round-trips, invisible to the old app) | new |
| `employmentType` | string | `erp.x.employmentType` (adapter extras; round-trips, invisible to the old app) | new |
| `endDate` | string | `erp.x.endDate` (adapter extras; round-trips, invisible to the old app) | new |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `housing` | number | `erp.x.housing` (adapter extras; round-trips, invisible to the old app) | new |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `jobTitle` | string | `position` | mapped |
| `joinDate` | string | `joining_date` | mapped |
| `nameAr` | string | `name_in_arabic` | mapped |
| `nameEn` | string | `name` | mapped |
| `nationalId` | string | `iqama_no` | mapped |
| `openingBalance` | number | `opening_balance` | mapped |
| `openingBalanceDate` | string | `opening_balance_date` | mapped |
| `openingBalanceType` | string | `opening_balance_type` | mapped |
| `otherAllowances` | number | `erp.x.otherAllowances` (adapter extras; round-trips, invisible to the old app) | new |
| `phone` | string | `mob1` | mapped |
| `photo` | null | `erp.x.photo` (adapter extras; round-trips, invisible to the old app) | new |
| `status` | string | `is_active` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `transport` | number | `erp.x.transport` (adapter extras; round-trips, invisible to the old app) | new |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### vehicles — `/v1/erp/vehicles`

**Legacy** store DB `vehicle` — collection `vehicle` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `code` | string | `erp.x.code` (adapter extras; round-trips, invisible to the old app) | new |
| `color` | string | `color` | mapped |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `customerId` | string | `customer_id` | mapped |
| `customerName` | string | read from legacy; not written by the adapter | computed |
| `customerNameAr` | string | read from legacy; not written by the adapter | computed |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `make` | string | `brand` | mapped |
| `model` | string | `model` | mapped |
| `notes` | string | `remarks` | mapped |
| `photo` | null | `erp.x.photo` (adapter extras; round-trips, invisible to the old app) | new |
| `plate` | string | `vehicle_number` | mapped |
| `plateAr` | string | `erp.x.plateAr` (adapter extras; round-trips, invisible to the old app) | new |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |
| `vin` | string | `chassis_number` | mapped |
| `year` | number | `year` | mapped |

### signatures — `/v1/erp/signatures`

**Legacy** store DB `signature` — collection `signature` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `default` | boolean | `erp.x.default` (adapter extras; round-trips, invisible to the old app) | new |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `docs` | array | `erp.x.docs` (adapter extras; round-trips, invisible to the old app) | new |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `image` | string | read from legacy; not written by the adapter | computed |
| `name` | string | `name` | mapped |
| `nameAr` | string | `erp.x.nameAr` (adapter extras; round-trips, invisible to the old app) | new |
| `source` | string | `erp.x.source` (adapter extras; round-trips, invisible to the old app) | new |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### packages — `/v1/erp/packages`

**Legacy** main DB `customer_package` (store_id) — collection `customer_package` (main DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `code` | string | `code` | mapped |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `customerId` | string | `customer_id` | mapped |
| `customerName` | string | `customer_name` | mapped |
| `customerNameAr` | string | `customer_name_ar` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `nameAr` | string | `name_in_arabic` | mapped |
| `nameEn` | string | `name` | mapped |
| `notes` | string | `notes` | mapped |
| `price` | number | `price` | mapped |
| `services` | array | `services` | mapped |
| `status` | string | `status` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `usage[].at` | string | `erp.x.usage[].at` (adapter extras; round-trips, invisible to the old app) | new |
| `usage[].by` | string | `erp.x.usage[].by` (adapter extras; round-trips, invisible to the old app) | new |
| `usage[].note` | string | `erp.x.usage[].note` (adapter extras; round-trips, invisible to the old app) | new |
| `used` | number | `used` | mapped |
| `validDays` | number | `valid_days` | mapped |
| `validFrom` | string | `valid_from` | mapped |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |
| `visits` | number | `visits` | mapped |

### rfqSuppliers — `/v1/erp/rfq-suppliers`

**Legacy** main DB `rfq_suppliers` (store_id) — collection `rfq_suppliers` (main DB). Writes go through the existing v1 handlers.

- soft delete kept in `erp.del` (legacy delete is destructive; `?hard=1` calls it)

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `address` | string | `address` | mapped |
| `categories` | array | `categories` | mapped |
| `city` | string | `purchase_market` | mapped |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `deleted` | boolean | `erp.del` (`erp.del` when the legacy delete is destructive) | server |
| `distanceKm` | number|null | `erp.x.distanceKm` (adapter extras; round-trips, invisible to the old app) | new |
| `email` | string | `email` | mapped |
| `hasWhatsapp` | boolean | `erp.x.hasWhatsapp` (adapter extras; round-trips, invisible to the old app) | new |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `lastContact` | string|null | `erp.x.lastContact` (adapter extras; round-trips, invisible to the old app) | new |
| `lat` | number|null | `latitude` | mapped |
| `lng` | number|null | `longitude` | mapped |
| `name` | string | `name` | mapped |
| `notes` | string | `erp.x.notes` (adapter extras; round-trips, invisible to the old app) | new |
| `phone` | string | `phone` | mapped |
| `rating` | number | `rating` | mapped |
| `source` | string | read from legacy; not written by the adapter | computed |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### sales — `/v1/erp/sales`

**Legacy** store DB `order` (+ `sales_payment`) — collection `order` (store DB). Writes go through the existing v1 handlers.

- DELETE → 409 unsupported_legacy: Sales invoices cannot be deleted in the existing system; issue a sales return (credit note) instead.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `address` | string | `address` | mapped |
| `cashDiscount` | number | `cash_discount` | mapped |
| `code` | string | read from legacy; not written by the adapter | computed |
| `commission` | number | `commission_payment_method`, `commission` | mapped |
| `commissionMethod` | string | `commission_payment_method` | mapped |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `customerId` | string|null | `customer_id` | mapped |
| `customerName` | string | read from legacy; not written by the adapter | computed |
| `customerNameAr` | string | read from legacy; not written by the adapter | computed |
| `date` | string(datetime-local) | `date_str` | mapped |
| `deleted` | boolean | `erp.del` (`erp.del` when the legacy delete is destructive) | server |
| `discount` | number | `discount_with_vat`, `discount` | mapped |
| `formType` | string | `erp.x.formType` (adapter extras; round-trips, invisible to the old app) | new |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `items[].nameAr` | string | `products[].name_in_arabic` | mapped |
| `items[].nameEn` | string | `products[].name` | mapped |
| `items[].partNo` | string | `products[].part_number` | mapped |
| `items[].productId` | string|null | `products[].item_code`, `products[].name_in_arabic`, `products[].part_number`, `products[].product_id` | mapped |
| `items[].purchasePrice` | number | `products[].purchase_unit_price_with_vat`, `products[].purchase_unit_price` | mapped |
| `items[].qty` | number | `products[].quantity` | mapped |
| `items[].retailPrice` | number | accepted; no legacy effect for this value | mapped |
| `items[].unit` | string | `products[].unit` | mapped |
| `items[].unitDiscount` | number | `products[].unit_discount_percent_with_vat`, `products[].unit_discount_percent`, `products[].unit_discount_with_vat`, `products[].unit_discount` | mapped |
| `items[].unitPrice` | number | `products[].unit_price_with_vat`, `products[].unit_price` | mapped |
| `items[].vatPercent` | number | read from legacy; not written by the adapter | computed |
| `items[].warehouseId` | string|null | `products[].warehouse_code`, `products[].warehouse_id` | mapped |
| `items[].wholesalePrice` | number | accepted; no legacy effect for this value | mapped |
| `km` | string|number | `km_driven` | mapped |
| `payments[].amount` | number | `payments_input[].amount` | mapped |
| `payments[].date` | string(datetime-local) | `payments_input[].date_str` | mapped |
| `payments[].description` | string | `payments_input[].description` | mapped |
| `payments[].id` | string | `payments_input[].id` | mapped |
| `payments[].method` | string | `payments_input[].method` | mapped |
| `phone` | string | `phone` | mapped |
| `poNo` | string | `customer_po_no` | mapped |
| `posMeta` | object | `erp.x.posMeta` (adapter extras; round-trips, invisible to the old app) | new |
| `posType` | string | `erp.x.posType` (adapter extras; round-trips, invisible to the old app) | new |
| `proformaCode` | string | `erp.x.proformaCode` (adapter extras; round-trips, invisible to the old app) | new |
| `proformaId` | string|null | `erp.x.proformaId` (adapter extras; round-trips, invisible to the old app) | new |
| `quotationCode` | string | read from legacy; not written by the adapter | computed |
| `quotationId` | string|null | `quotation_code`, `quotation_id` | mapped |
| `remarks` | string | `remarks` | mapped |
| `repairJobId` | string|null | `repair_job_id` | mapped |
| `rounding` | number | `rounding_amount` | mapped |
| `roundingAuto` | boolean | `erp.x.roundingAuto`; the adapter computes the rounding and sends it as explicit `rounding_amount` (`auto_rounding_amount` false) | mapped |
| `salesman` | string | `erp.x.salesman` (adapter extras; round-trips, invisible to the old app) | new |
| `shipping` | number | `shipping_handling_fees` | mapped |
| `stockByRepairJob` | boolean | `erp.x.stockByRepairJob` (adapter extras; round-trips, invisible to the old app) | new |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatNo` | string | `vat_no` | mapped |
| `vatPercent` | number | `products[].purchase_unit_price_with_vat`, `products[].unit_price_with_vat`, `vat_percent` | mapped |
| `vehicleId` | string|null | `vehicle_id` | mapped |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |
| `zatca.attemptAt` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.error` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.hash` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.icv` | integer | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.invoiceType` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.pih` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.qr` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.reportedAt` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.signature` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.status` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.uuid` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.xml` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.xmlUrl` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |

### quotations — `/v1/erp/quotations`

**Legacy** store DB `quotation` (type quotation|invoice) — collection `quotation` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `address` | string | `address` | mapped |
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `customerId` | string|null | `customer_id` | mapped |
| `customerName` | string | read from legacy; not written by the adapter | computed |
| `customerNameAr` | string | read from legacy; not written by the adapter | computed |
| `date` | string(datetime-local) | `date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `deliveryDays` | number | `delivery_days` | mapped |
| `deliveryFrom` | string | `delivery_from` | mapped |
| `discount` | number | `discount_with_vat`, `discount` | mapped |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `items[].nameAr` | string | `products[].name_in_arabic` | mapped |
| `items[].nameEn` | string | `products[].name` | mapped |
| `items[].partNo` | string | `products[].part_number` | mapped |
| `items[].productId` | string|null | `products[].item_code`, `products[].name_in_arabic`, `products[].part_number`, `products[].product_id` | mapped |
| `items[].purchasePrice` | number | `products[].purchase_unit_price_with_vat`, `products[].purchase_unit_price` | mapped |
| `items[].qty` | number | `products[].quantity` | mapped |
| `items[].retailPrice` | number | accepted; no legacy effect for this value | mapped |
| `items[].unit` | string | `products[].unit` | mapped |
| `items[].unitDiscount` | number | `products[].unit_discount_percent_with_vat`, `products[].unit_discount_percent`, `products[].unit_discount_with_vat`, `products[].unit_discount` | mapped |
| `items[].unitPrice` | number | `products[].unit_price_with_vat`, `products[].unit_price` | mapped |
| `items[].vatPercent` | number | read from legacy; not written by the adapter | computed |
| `items[].warehouseId` | string|null | `products[].warehouse_code`, `products[].warehouse_id` | mapped |
| `items[].wholesalePrice` | number | accepted; no legacy effect for this value | mapped |
| `markup` | number | `erp.x.markup` (adapter extras; round-trips, invisible to the old app) | new |
| `orderIds` | array | `order_ids` | mapped |
| `payments[].amount` | number | mapped (empty in sample, not probed) | mapped |
| `payments[].date` | string(datetime-local) | mapped (empty in sample, not probed) | mapped |
| `payments[].description` | string | mapped (empty in sample, not probed) | mapped |
| `payments[].id` | string | mapped (empty in sample, not probed) | mapped |
| `payments[].method` | string | mapped (empty in sample, not probed) | mapped |
| `phone` | string | `phone` | mapped |
| `proformaIds` | array | `erp.x.proformaIds` (adapter extras; round-trips, invisible to the old app) | new |
| `remarks` | string | `remarks` | mapped |
| `rfqId` | null | `rfq_received_id` | mapped |
| `shipping` | number | `shipping_handling_fees` | mapped |
| `status` | string | `status` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `type` | string | `type` | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `validityDays` | number | `validity_days` | mapped |
| `vatNo` | string | `vat_no` | mapped |
| `vatPercent` | number | `products[].purchase_unit_price_with_vat`, `products[].unit_price_with_vat`, `vat_percent` | mapped |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |
| `zatca.status` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |

### proformas — `/v1/erp/proformas`

**New adapter collection** `erp_proforma` (store DB) — no legacy equivalent; every field is stored as sent. 

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `address` | string | `erp_proforma.address` | new |
| `code` | string | `erp_proforma.code` | new |
| `createdAt` | string(datetime-local) | envelope / server-owned | server |
| `createdBy` | string | envelope / server-owned | server |
| `customerId` | string|null | `erp_proforma.customerId` | new |
| `customerName` | string | `erp_proforma.customerName` | new |
| `customerNameAr` | string | `erp_proforma.customerNameAr` | new |
| `date` | string(datetime-local) | `erp_proforma.date` | new |
| `deleted` | boolean | envelope / server-owned | server |
| `deliveryDays` | number | `erp_proforma.deliveryDays` | new |
| `deliveryFrom` | string | `erp_proforma.deliveryFrom` | new |
| `discount` | number | `erp_proforma.discount` | new |
| `history[].action` | string | `erp_proforma.history[].action` | new |
| `history[].at` | string | `erp_proforma.history[].at` | new |
| `history[].by` | string | `erp_proforma.history[].by` | new |
| `history[].changes[].field` | string | `erp_proforma.history[].changes[].field` | new |
| `history[].changes[].from` | string | `erp_proforma.history[].changes[].from` | new |
| `history[].changes[].to` | string | `erp_proforma.history[].changes[].to` | new |
| `id` | string | envelope / server-owned | server |
| `items[].nameAr` | string | `erp_proforma.items[].nameAr` | new |
| `items[].nameEn` | string | `erp_proforma.items[].nameEn` | new |
| `items[].partNo` | string | `erp_proforma.items[].partNo` | new |
| `items[].productId` | string|null | `erp_proforma.items[].productId` | new |
| `items[].purchasePrice` | number | `erp_proforma.items[].purchasePrice` | new |
| `items[].qty` | number | `erp_proforma.items[].qty` | new |
| `items[].retailPrice` | number | `erp_proforma.items[].retailPrice` | new |
| `items[].unit` | string | `erp_proforma.items[].unit` | new |
| `items[].unitDiscount` | number | `erp_proforma.items[].unitDiscount` | new |
| `items[].unitPrice` | number | `erp_proforma.items[].unitPrice` | new |
| `items[].vatPercent` | number | `erp_proforma.items[].vatPercent` | new |
| `items[].warehouseId` | string|null | `erp_proforma.items[].warehouseId` | new |
| `items[].wholesalePrice` | number | `erp_proforma.items[].wholesalePrice` | new |
| `orderIds` | array | `erp_proforma.orderIds` | new |
| `payments[].amount` | number | `erp_proforma.payments[].amount` | new |
| `payments[].date` | string(datetime-local) | `erp_proforma.payments[].date` | new |
| `payments[].description` | string | `erp_proforma.payments[].description` | new |
| `payments[].id` | string | `erp_proforma.payments[].id` | new |
| `payments[].method` | string | `erp_proforma.payments[].method` | new |
| `phone` | string | `erp_proforma.phone` | new |
| `quotationCode` | string | `erp_proforma.quotationCode` | new |
| `quotationId` | string | `erp_proforma.quotationId` | new |
| `remarks` | string | `erp_proforma.remarks` | new |
| `rfqId` | null | `erp_proforma.rfqId` | new |
| `shipping` | number | `erp_proforma.shipping` | new |
| `status` | string | `erp_proforma.status` | new |
| `storeId` | string | `erp_proforma.storeId` | new |
| `type` | string | `erp_proforma.type` | new |
| `updatedAt` | string(datetime-local) | envelope / server-owned | server |
| `updatedBy` | string | envelope / server-owned | server |
| `validityDays` | number | `erp_proforma.validityDays` | new |
| `vatNo` | string | `erp_proforma.vatNo` | new |
| `vatPercent` | number | `erp_proforma.vatPercent` | new |
| `version` | integer | envelope / server-owned | server |
| `zatca.status` | string | `erp_proforma.zatca.status` | new |

### salesReturns — `/v1/erp/sales-returns`

**Legacy** store DB `salesreturn` (+ `sales_return_payment`) — collection `salesreturn` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `customerId` | string|null | `customer_id` | mapped |
| `customerName` | string | read from legacy; not written by the adapter | computed |
| `customerNameAr` | string | read from legacy; not written by the adapter | computed |
| `date` | string(datetime-local) | `date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `discount` | number | `discount_with_vat`, `discount` | mapped |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `items[].nameAr` | string | `products[].name_in_arabic` | mapped |
| `items[].nameEn` | string | `products[].name` | mapped |
| `items[].partNo` | string | `products[].part_number` | mapped |
| `items[].productId` | string|null | `products[].item_code`, `products[].name_in_arabic`, `products[].part_number`, `products[].product_id` | mapped |
| `items[].purchasePrice` | number | `products[].purchase_unit_price_with_vat`, `products[].purchase_unit_price` | mapped |
| `items[].qty` | number | `products[].quantity` | mapped |
| `items[].retailPrice` | number | accepted; no legacy effect for this value | mapped |
| `items[].unit` | string | `products[].unit` | mapped |
| `items[].unitDiscount` | number | `products[].unit_discount_percent_with_vat`, `products[].unit_discount_percent`, `products[].unit_discount_with_vat`, `products[].unit_discount` | mapped |
| `items[].unitPrice` | number | `products[].unit_price_with_vat`, `products[].unit_price` | mapped |
| `items[].vatPercent` | number | read from legacy; not written by the adapter | computed |
| `items[].warehouseId` | string|null | `products[].warehouse_code`, `products[].warehouse_id` | mapped |
| `items[].wholesalePrice` | number | accepted; no legacy effect for this value | mapped |
| `orderCode` | string | read from legacy; not written by the adapter | computed |
| `orderId` | string | `order_code`, `order_id` | mapped |
| `payments[].amount` | number | mapped (empty in sample, not probed) | mapped |
| `payments[].date` | string(datetime-local) | mapped (empty in sample, not probed) | mapped |
| `payments[].description` | string | mapped (empty in sample, not probed) | mapped |
| `payments[].id` | string | mapped (empty in sample, not probed) | mapped |
| `payments[].method` | string | mapped (empty in sample, not probed) | mapped |
| `remarks` | string | `remarks` | mapped |
| `shipping` | number | `shipping_handling_fees` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatPercent` | number | `products[].purchase_unit_price_with_vat`, `products[].unit_price_with_vat`, `vat_percent` | mapped |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |
| `zatca.attemptAt` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.error` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.hash` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.icv` | integer | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.invoiceType` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.pih` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.qr` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.reportedAt` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.signature` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.status` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.uuid` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.xml` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.xmlUrl` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |

### deliveryNotes — `/v1/erp/delivery-notes`

**Legacy** store DB `delivery_note` — collection `delivery_note` (store DB). Writes go through the existing v1 handlers.

- DELETE → 409 unsupported_legacy: Delivery notes cannot be deleted in the existing system.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `customerId` | string|null | `customer_id` | mapped |
| `customerName` | string | read from legacy; not written by the adapter | computed |
| `customerNameAr` | string | read from legacy; not written by the adapter | computed |
| `date` | string(datetime-local) | `date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `deliveredAt` | string | `erp.x.deliveredAt` (adapter extras; round-trips, invisible to the old app) | new |
| `deliveredBy` | string | mapped, validated (probe rejected: deliveredBy: unknown id) | mapped |
| `estDelivery` | string | `erp.x.estDelivery` (adapter extras; round-trips, invisible to the old app) | new |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `items[].nameAr` | string | `products[].name_in_arabic` | mapped |
| `items[].nameEn` | string | `products[].name` | mapped |
| `items[].partNo` | string | `products[].part_number` | mapped |
| `items[].productId` | string|null | `products[].item_code`, `products[].name_in_arabic`, `products[].part_number`, `products[].product_id`, `products[].unit` | mapped |
| `items[].purchasePrice` | number | `products[].purchase_unit_price_with_vat`, `products[].purchase_unit_price` | mapped |
| `items[].qty` | number | `products[].quantity` | mapped |
| `items[].retailPrice` | number | accepted; no legacy effect for this value | mapped |
| `items[].unit` | string | `products[].unit` | mapped |
| `items[].unitDiscount` | number | `products[].unit_discount_with_vat`, `products[].unit_discount` | mapped |
| `items[].unitPrice` | number | `products[].unit_price_with_vat`, `products[].unit_price` | mapped |
| `items[].vatPercent` | number | read from legacy; not written by the adapter | computed |
| `items[].warehouseId` | string|null | accepted; no legacy effect for this value | mapped |
| `items[].wholesalePrice` | number | accepted; no legacy effect for this value | mapped |
| `orderCode` | string | read from legacy; not written by the adapter | computed |
| `orderId` | string | `order_code`, `order_id` | mapped |
| `payments[].amount` | number | `erp.x.payments[].amount` (adapter extras; round-trips, invisible to the old app) | new |
| `payments[].date` | string(datetime-local) | `erp.x.payments[].date` (adapter extras; round-trips, invisible to the old app) | new |
| `payments[].description` | string | `erp.x.payments[].description` (adapter extras; round-trips, invisible to the old app) | new |
| `payments[].id` | string | `erp.x.payments[].id` (adapter extras; round-trips, invisible to the old app) | new |
| `payments[].method` | string | `erp.x.payments[].method` (adapter extras; round-trips, invisible to the old app) | new |
| `quotationId` | null | `erp.x.quotationId` (adapter extras; round-trips, invisible to the old app) | new |
| `remarks` | string | `remarks` | mapped |
| `status` | string | `erp.x.status` (adapter extras; round-trips, invisible to the old app) | new |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### nonvatSales — `/v1/erp/nonvat-sales`

**Legacy** store DB `non_vat_sales` — collection `non_vat_sales` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `address` | string | `address` | mapped |
| `cashDiscount` | number | `cash_discount` | mapped |
| `code` | string | read from legacy; not written by the adapter | computed |
| `commission` | number | `erp.x.commission` (adapter extras; round-trips, invisible to the old app) | new |
| `commissionMethod` | string | `erp.x.commissionMethod` (adapter extras; round-trips, invisible to the old app) | new |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `customerId` | string|null | `customer_id` | mapped |
| `customerName` | string | read from legacy; not written by the adapter | computed |
| `customerNameAr` | string | read from legacy; not written by the adapter | computed |
| `date` | string(datetime-local) | `date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `discount` | number | `discount_with_vat`, `discount` | mapped |
| `formType` | string | `erp.x.formType` (adapter extras; round-trips, invisible to the old app) | new |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `items[].nameAr` | string | `products[].name_in_arabic` | mapped |
| `items[].nameEn` | string | `products[].name` | mapped |
| `items[].partNo` | string | `products[].part_number` | mapped |
| `items[].productId` | string|null | `products[].item_code`, `products[].product_id` | mapped |
| `items[].purchasePrice` | number | `products[].purchase_unit_price_with_vat`, `products[].purchase_unit_price` | mapped |
| `items[].qty` | number | `products[].quantity` | mapped |
| `items[].retailPrice` | number | accepted; no legacy effect for this value | mapped |
| `items[].unit` | string | `products[].unit` | mapped |
| `items[].unitDiscount` | number | `products[].unit_discount_percent_with_vat`, `products[].unit_discount_percent`, `products[].unit_discount_with_vat`, `products[].unit_discount` | mapped |
| `items[].unitPrice` | number | `products[].unit_price_with_vat`, `products[].unit_price` | mapped |
| `items[].vatPercent` | number | read from legacy; not written by the adapter | computed |
| `items[].warehouseId` | string|null | `products[].warehouse_code`, `products[].warehouse_id` | mapped |
| `items[].wholesalePrice` | number | accepted; no legacy effect for this value | mapped |
| `payments[].amount` | number | mapped (empty in sample, not probed) | mapped |
| `payments[].date` | string(datetime-local) | mapped (empty in sample, not probed) | mapped |
| `payments[].description` | string | mapped (empty in sample, not probed) | mapped |
| `payments[].id` | string | mapped (empty in sample, not probed) | mapped |
| `payments[].method` | string | mapped (empty in sample, not probed) | mapped |
| `phone` | string | `phone` | mapped |
| `remarks` | string | `remarks` | mapped |
| `rounding` | number | `rounding_amount` | mapped |
| `roundingAuto` | boolean | `erp.x.roundingAuto`; the adapter computes the rounding and sends it as explicit `rounding_amount` (`auto_rounding_amount` false) | mapped |
| `shipping` | number | `shipping_handling_fees` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatNo` | string | `vat_no` | mapped |
| `vatPercent` | number | read from legacy; not written by the adapter | computed |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### nonvatReturns — `/v1/erp/nonvat-returns`

**Legacy** store DB `non_vat_sales_return` — collection `non_vat_sales_return` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `customerId` | string|null | `customer_id` | mapped |
| `customerName` | string | read from legacy; not written by the adapter | computed |
| `customerNameAr` | string | read from legacy; not written by the adapter | computed |
| `date` | string(datetime-local) | `date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `discount` | number | `discount_with_vat`, `discount` | mapped |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `items[].nameAr` | string | `products[].name_in_arabic` | mapped |
| `items[].nameEn` | string | `products[].name` | mapped |
| `items[].partNo` | string | `products[].part_number` | mapped |
| `items[].productId` | string|null | `products[].item_code`, `products[].product_id` | mapped |
| `items[].purchasePrice` | number | `products[].purchase_unit_price_with_vat`, `products[].purchase_unit_price` | mapped |
| `items[].qty` | number | `products[].quantity` | mapped |
| `items[].retailPrice` | number | accepted; no legacy effect for this value | mapped |
| `items[].unit` | string | `products[].unit` | mapped |
| `items[].unitDiscount` | number | `products[].unit_discount_percent_with_vat`, `products[].unit_discount_percent`, `products[].unit_discount_with_vat`, `products[].unit_discount` | mapped |
| `items[].unitPrice` | number | `products[].unit_price_with_vat`, `products[].unit_price` | mapped |
| `items[].vatPercent` | number | read from legacy; not written by the adapter | computed |
| `items[].warehouseId` | string|null | `products[].warehouse_code`, `products[].warehouse_id` | mapped |
| `items[].wholesalePrice` | number | accepted; no legacy effect for this value | mapped |
| `orderCode` | string | read from legacy; not written by the adapter | computed |
| `orderId` | string | `non_vat_sales_code`, `non_vat_sales_id` | mapped |
| `payments[].amount` | number | mapped (empty in sample, not probed) | mapped |
| `payments[].date` | string(datetime-local) | mapped (empty in sample, not probed) | mapped |
| `payments[].description` | string | mapped (empty in sample, not probed) | mapped |
| `payments[].id` | string | mapped (empty in sample, not probed) | mapped |
| `payments[].method` | string | mapped (empty in sample, not probed) | mapped |
| `remarks` | string | `remarks` | mapped |
| `shipping` | number | `shipping_handling_fees` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatPercent` | number | read from legacy; not written by the adapter | computed |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### quotationReturns — `/v1/erp/quotation-returns`

**Legacy** store DB `quotation_sales_return` — collection `quotation_sales_return` (store DB). Writes go through the existing v1 handlers.

- DELETE → 409 unsupported_legacy: Quotation sales returns cannot be deleted in the existing system.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `customerId` | string|null | `customer_id` | mapped |
| `customerName` | string | read from legacy; not written by the adapter | computed |
| `customerNameAr` | string | read from legacy; not written by the adapter | computed |
| `date` | string(datetime-local) | `date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `discount` | number | `discount_with_vat`, `discount` | mapped |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `items[].nameAr` | string | `products[].name_in_arabic` | mapped |
| `items[].nameEn` | string | `products[].name` | mapped |
| `items[].partNo` | string | `products[].part_number` | mapped |
| `items[].productId` | string|null | `products[].item_code`, `products[].product_id` | mapped |
| `items[].purchasePrice` | number | `products[].purchase_unit_price_with_vat`, `products[].purchase_unit_price` | mapped |
| `items[].qty` | number | `products[].quantity` | mapped |
| `items[].retailPrice` | number | accepted; no legacy effect for this value | mapped |
| `items[].unit` | string | `products[].unit` | mapped |
| `items[].unitDiscount` | number | `products[].unit_discount_percent_with_vat`, `products[].unit_discount_percent`, `products[].unit_discount_with_vat`, `products[].unit_discount` | mapped |
| `items[].unitPrice` | number | `products[].unit_price_with_vat`, `products[].unit_price` | mapped |
| `items[].vatPercent` | number | read from legacy; not written by the adapter | computed |
| `items[].warehouseId` | string|null | `products[].warehouse_code`, `products[].warehouse_id` | mapped |
| `items[].wholesalePrice` | number | accepted; no legacy effect for this value | mapped |
| `orderCode` | string | read from legacy; not written by the adapter | computed |
| `payments[].amount` | number | mapped (empty in sample, not probed) | mapped |
| `payments[].date` | string(datetime-local) | mapped (empty in sample, not probed) | mapped |
| `payments[].description` | string | mapped (empty in sample, not probed) | mapped |
| `payments[].id` | string | mapped (empty in sample, not probed) | mapped |
| `payments[].method` | string | mapped (empty in sample, not probed) | mapped |
| `quotationId` | string | `quotation_code`, `quotation_id` | mapped |
| `remarks` | string | `remarks` | mapped |
| `shipping` | number | `shipping_handling_fees` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatPercent` | number | `products[].purchase_unit_price_with_vat`, `products[].unit_price_with_vat`, `vat_percent` | mapped |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### purchases — `/v1/erp/purchases`

**Legacy** store DB `purchase` (+ `purchase_payment`) — collection `purchase` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `address` | string | `address` | mapped |
| `billId` | string|null | `erp.x.billId` (adapter extras; round-trips, invisible to the old app) | new |
| `cashDiscount` | number | `cash_discount` | mapped |
| `code` | string | read from legacy; not written by the adapter | computed |
| `commission` | number | `commission_payment_method`, `commission` | mapped |
| `commissionMethod` | string | `commission_payment_method` | mapped |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `date` | string | `date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `discount` | number | `discount_with_vat`, `discount` | mapped |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `items[].nameAr` | string | `products[].name_in_arabic` | mapped |
| `items[].nameEn` | string | `products[].name` | mapped |
| `items[].partNo` | string | `products[].part_number` | mapped |
| `items[].productId` | string|null | `products[].item_code`, `products[].name_in_arabic`, `products[].part_number`, `products[].product_id` | mapped |
| `items[].purchasePrice` | number | read from legacy; not written by the adapter | computed |
| `items[].qty` | number | `products[].quantity` | mapped |
| `items[].retailPrice` | number | `products[].retail_unit_price_with_vat`, `products[].retail_unit_price` | mapped |
| `items[].unit` | string | `products[].unit` | mapped |
| `items[].unitDiscount` | number | `products[].unit_discount_percent_with_vat`, `products[].unit_discount_percent`, `products[].unit_discount_with_vat`, `products[].unit_discount` | mapped |
| `items[].unitPrice` | number | `products[].purchase_unit_price_with_vat`, `products[].purchase_unit_price` | mapped |
| `items[].vatPercent` | number | read from legacy; not written by the adapter | computed |
| `items[].warehouseId` | string|null | `products[].warehouse_code`, `products[].warehouse_id` | mapped |
| `items[].wholesalePrice` | number | `products[].wholesale_unit_price_with_vat`, `products[].wholesale_unit_price` | mapped |
| `payments[].amount` | number | mapped (empty in sample, not probed) | mapped |
| `payments[].date` | string(datetime-local) | mapped (empty in sample, not probed) | mapped |
| `payments[].description` | string | mapped (empty in sample, not probed) | mapped |
| `payments[].id` | string | mapped (empty in sample, not probed) | mapped |
| `payments[].method` | string | mapped (empty in sample, not probed) | mapped |
| `phone` | string | `phone` | mapped |
| `poCode` | string | `erp.x.poCode` (adapter extras; round-trips, invisible to the old app) | new |
| `poId` | string|null | `erp.x.poId` (adapter extras; round-trips, invisible to the old app) | new |
| `remarks` | string | `remarks` | mapped |
| `rounding` | number | `rounding_amount` | mapped |
| `roundingAuto` | boolean | `erp.x.roundingAuto`; the adapter computes the rounding and sends it as explicit `rounding_amount` (`auto_rounding_amount` false) | mapped |
| `shipping` | number | `shipping_handling_fees` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatNo` | string | `vat_no` | mapped |
| `vatPercent` | number | `products[].purchase_unit_price_with_vat`, `products[].retail_unit_price_with_vat`, `vat_percent` | mapped |
| `vendorId` | string | mapped (not probed) | mapped |
| `vendorInvoiceNo` | string | `vendor_invoice_no` | mapped |
| `vendorName` | string | read from legacy; not written by the adapter | computed |
| `vendorNameAr` | string | read from legacy; not written by the adapter | computed |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### purchaseOrders — `/v1/erp/purchase-orders`

**Legacy** store DB `purchase_order` — collection `purchase_order` (store DB). Writes go through the existing v1 handlers.

- soft delete kept in `erp.del` (legacy delete is destructive; `?hard=1` calls it)

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `address` | string | `address` | mapped |
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `date` | string | `date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `discount` | number | `discount_with_vat`, `discount` | mapped |
| `expectedDate` | string | `expected_date_str` | mapped |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `items[].nameAr` | string | `products[].name_in_arabic` | mapped |
| `items[].nameEn` | string | `products[].name` | mapped |
| `items[].partNo` | string | `products[].part_number` | mapped |
| `items[].productId` | string|null | `products[].item_code`, `products[].product_id` | mapped |
| `items[].purchasePrice` | number | read from legacy; not written by the adapter | computed |
| `items[].qty` | number | `products[].quantity` | mapped |
| `items[].retailPrice` | number | accepted; no legacy effect for this value | mapped |
| `items[].unit` | string | `products[].unit` | mapped |
| `items[].unitDiscount` | number | `products[].unit_discount_percent_with_vat`, `products[].unit_discount_percent`, `products[].unit_discount_with_vat`, `products[].unit_discount` | mapped |
| `items[].unitPrice` | number | `products[].purchase_unit_price_with_vat`, `products[].purchase_unit_price` | mapped |
| `items[].vatPercent` | number | read from legacy; not written by the adapter | computed |
| `items[].warehouseId` | string|null | `products[].warehouse_code`, `products[].warehouse_id` | mapped |
| `items[].wholesalePrice` | number | accepted; no legacy effect for this value | mapped |
| `payments[].amount` | number | `erp.x.payments[].amount` (adapter extras; round-trips, invisible to the old app) | new |
| `payments[].date` | string(datetime-local) | `erp.x.payments[].date` (adapter extras; round-trips, invisible to the old app) | new |
| `payments[].description` | string | `erp.x.payments[].description` (adapter extras; round-trips, invisible to the old app) | new |
| `payments[].id` | string | `erp.x.payments[].id` (adapter extras; round-trips, invisible to the old app) | new |
| `payments[].method` | string | `erp.x.payments[].method` (adapter extras; round-trips, invisible to the old app) | new |
| `prCode` | string | read from legacy; not written by the adapter | computed |
| `prId` | string | `purchase_request_id` | mapped |
| `purchaseId` | string | `purchase_code`, `purchase_id` | mapped |
| `remarks` | string | `remarks` | mapped |
| `shipping` | number | `shipping_handling_fees` | mapped |
| `status` | string | `status` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatPercent` | number | `products[].purchase_unit_price_with_vat`, `vat_percent` | mapped |
| `vendorId` | string | mapped (not probed) | mapped |
| `vendorInvoiceNo` | string | `vendor_invoice_no` | mapped |
| `vendorName` | string | read from legacy; not written by the adapter | computed |
| `vendorNameAr` | string | read from legacy; not written by the adapter | computed |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### purchaseRequests — `/v1/erp/purchase-requests`

**Legacy** store DB `purchase_request` — collection `purchase_request` (store DB). Writes go through the existing v1 handlers.

- soft delete kept in `erp.del` (legacy delete is destructive; `?hard=1` calls it)

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `approvedAt` | string | `erp.x.approvedAt` (adapter extras; round-trips, invisible to the old app) | new |
| `approvedBy` | string | `erp.x.approvedBy` (adapter extras; round-trips, invisible to the old app) | new |
| `assignedTo` | string | mapped, validated (probe rejected: assignedTo: unknown id) | mapped |
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `date` | string | `date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `items[].nameAr` | string | `products[].name_in_arabic` | mapped |
| `items[].nameEn` | string | `products[].name` | mapped |
| `items[].partNo` | string | `products[].part_number` | mapped |
| `items[].productId` | string|null | `products[].item_code`, `products[].product_id` | mapped |
| `items[].purchasePrice` | number | read from legacy; not written by the adapter | computed |
| `items[].qty` | number | `products[].quantity` | mapped |
| `items[].retailPrice` | number | accepted; no legacy effect for this value | mapped |
| `items[].unit` | string | `products[].unit` | mapped |
| `items[].unitDiscount` | number | `products[].unit_discount_percent_with_vat`, `products[].unit_discount_percent`, `products[].unit_discount_with_vat`, `products[].unit_discount` | mapped |
| `items[].unitPrice` | number | `products[].purchase_unit_price_with_vat`, `products[].purchase_unit_price` | mapped |
| `items[].vatPercent` | number | read from legacy; not written by the adapter | computed |
| `items[].warehouseId` | string|null | accepted; no legacy effect for this value | mapped |
| `items[].wholesalePrice` | number | accepted; no legacy effect for this value | mapped |
| `notes` | string | read from legacy; not written by the adapter | computed |
| `poId` | string | `purchase_order_id` | mapped |
| `reason` | string | `erp.x.reason` (adapter extras; round-trips, invisible to the old app) | new |
| `requestedBy` | string | `erp.x.requestedBy` (adapter extras; round-trips, invisible to the old app) | new |
| `status` | string | `status` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatPercent` | number | `products[].purchase_unit_price_with_vat`, `vat_percent` | mapped |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### purchaseReturns — `/v1/erp/purchase-returns`

**Legacy** store DB `purchasereturn` (+ `purchase_return_payment`) — collection `purchasereturn` (store DB). Writes go through the existing v1 handlers.

- restore → 409: Purchase returns are deleted permanently by the existing system and cannot be restored.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `date` | string | `date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `discount` | number | `discount_with_vat`, `discount` | mapped |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `items[].nameAr` | string | `products[].name_in_arabic` | mapped |
| `items[].nameEn` | string | `products[].name` | mapped |
| `items[].partNo` | string | `products[].part_number` | mapped |
| `items[].productId` | string|null | `products[].item_code`, `products[].product_id` | mapped |
| `items[].purchasePrice` | number | read from legacy; not written by the adapter | computed |
| `items[].qty` | number | `products[].quantity` | mapped |
| `items[].retailPrice` | number | accepted; no legacy effect for this value | mapped |
| `items[].unit` | string | `products[].unit` | mapped |
| `items[].unitDiscount` | number | `products[].unit_discount_percent_with_vat`, `products[].unit_discount_percent`, `products[].unit_discount_with_vat`, `products[].unit_discount` | mapped |
| `items[].unitPrice` | number | `products[].purchasereturn_unit_price_with_vat`, `products[].purchasereturn_unit_price` | mapped |
| `items[].vatPercent` | number | read from legacy; not written by the adapter | computed |
| `items[].warehouseId` | string|null | `products[].warehouse_code`, `products[].warehouse_id` | mapped |
| `items[].wholesalePrice` | number | accepted; no legacy effect for this value | mapped |
| `payments[].amount` | number | mapped (empty in sample, not probed) | mapped |
| `payments[].date` | string(datetime-local) | mapped (empty in sample, not probed) | mapped |
| `payments[].description` | string | mapped (empty in sample, not probed) | mapped |
| `payments[].id` | string | mapped (empty in sample, not probed) | mapped |
| `payments[].method` | string | mapped (empty in sample, not probed) | mapped |
| `purchaseCode` | string | read from legacy; not written by the adapter | computed |
| `purchaseId` | string | mapped (not probed) | mapped |
| `remarks` | string | `remarks` | mapped |
| `shipping` | number | `shipping_handling_fees` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatPercent` | number | `products[].purchasereturn_unit_price_with_vat`, `vat_percent` | mapped |
| `vendorId` | string | mapped (not probed) | mapped |
| `vendorInvoiceNo` | string | `vendor_invoice_no` | mapped |
| `vendorName` | string | read from legacy; not written by the adapter | computed |
| `vendorNameAr` | string | read from legacy; not written by the adapter | computed |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### purchaseBills — `/v1/erp/purchase-bills`

**New adapter collection** `erp_purchase_bill` (store DB) — no legacy equivalent; every field is stored as sent. 

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `amount` | number | `erp_purchase_bill.amount` | new |
| `code` | string | `erp_purchase_bill.code` | new |
| `confidence` | number | `erp_purchase_bill.confidence` | new |
| `createdAt` | string(datetime-local) | envelope / server-owned | server |
| `createdBy` | string | envelope / server-owned | server |
| `deleted` | boolean | envelope / server-owned | server |
| `fileName` | string | `erp_purchase_bill.fileName` | new |
| `history[].action` | string | `erp_purchase_bill.history[].action` | new |
| `history[].at` | string | `erp_purchase_bill.history[].at` | new |
| `history[].by` | string | `erp_purchase_bill.history[].by` | new |
| `history[].changes[].field` | string | `erp_purchase_bill.history[].changes[].field` | new |
| `history[].changes[].from` | string | `erp_purchase_bill.history[].changes[].from` | new |
| `history[].changes[].to` | string | `erp_purchase_bill.history[].changes[].to` | new |
| `id` | string | envelope / server-owned | server |
| `lines[].nameAr` | string | `erp_purchase_bill.lines[].nameAr` | new |
| `lines[].nameEn` | string | `erp_purchase_bill.lines[].nameEn` | new |
| `lines[].partNo` | string | `erp_purchase_bill.lines[].partNo` | new |
| `lines[].productId` | string|null | `erp_purchase_bill.lines[].productId` | new |
| `lines[].purchasePrice` | number | `erp_purchase_bill.lines[].purchasePrice` | new |
| `lines[].qty` | number | `erp_purchase_bill.lines[].qty` | new |
| `lines[].retailPrice` | number | `erp_purchase_bill.lines[].retailPrice` | new |
| `lines[].unit` | string | `erp_purchase_bill.lines[].unit` | new |
| `lines[].unitDiscount` | number | `erp_purchase_bill.lines[].unitDiscount` | new |
| `lines[].unitPrice` | number | `erp_purchase_bill.lines[].unitPrice` | new |
| `lines[].vatPercent` | number | `erp_purchase_bill.lines[].vatPercent` | new |
| `lines[].warehouseId` | string|null | `erp_purchase_bill.lines[].warehouseId` | new |
| `lines[].wholesalePrice` | number | `erp_purchase_bill.lines[].wholesalePrice` | new |
| `purchaseId` | null | `erp_purchase_bill.purchaseId` | new |
| `receivedAt` | string | `erp_purchase_bill.receivedAt` | new |
| `source` | string | `erp_purchase_bill.source` | new |
| `status` | string | `erp_purchase_bill.status` | new |
| `storeId` | string | `erp_purchase_bill.storeId` | new |
| `threadId` | string | `erp_purchase_bill.threadId` | new |
| `updatedAt` | string(datetime-local) | envelope / server-owned | server |
| `updatedBy` | string | envelope / server-owned | server |
| `vat` | number | `erp_purchase_bill.vat` | new |
| `vendorId` | string | `erp_purchase_bill.vendorId` | new |
| `vendorInvoiceNo` | string | `erp_purchase_bill.vendorInvoiceNo` | new |
| `vendorName` | string | `erp_purchase_bill.vendorName` | new |
| `vendorNameAr` | string | `erp_purchase_bill.vendorNameAr` | new |
| `version` | integer | envelope / server-owned | server |

### stockTransfers — `/v1/erp/stock-transfers`

**Legacy** store DB `stocktransfer` (completed) + `erp_stock_transfer_pending` (NEW, pending) — collection `stocktransfer` (store DB). Writes go through the existing v1 handlers.

- DELETE → 409 unsupported_legacy: Stock transfers cannot be deleted in the existing system; create a reverse transfer instead.
- status `pending` transfers live in `erp_stock_transfer_pending` (no legacy equivalent: legacy transfers move stock immediately); completing one creates the legacy `stocktransfer` (new id, `replacedId` = pending id)

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `code` | string | read from legacy; not written by the adapter | computed |
| `completedAt` | string|null | read from legacy; not written by the adapter | computed |
| `completedBy` | string|null | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `date` | string | `date_str` | mapped |
| `deleted` | boolean | `erp.del` (`erp.del` when the legacy delete is destructive) | server |
| `fromWarehouseId` | string | `from_warehouse_code`, `from_warehouse_id` | mapped |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `items[].nameAr` | string | read from legacy; not written by the adapter | computed |
| `items[].nameEn` | string | `products[].name` | mapped |
| `items[].productId` | string | `products[].item_code`, `products[].part_number`, `products[].product_id`, `products[].purchase_unit_price`, `products[].unit` | mapped |
| `items[].qty` | number | `products[].quantity` | mapped |
| `items[].unit` | string | read from legacy; not written by the adapter | computed |
| `items[].unitPrice` | number | `products[].unit_price_with_vat`, `products[].unit_price` | mapped |
| `remarks` | string | `remarks` | mapped |
| `status` | string | read from legacy; not written by the adapter | computed |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `toWarehouseId` | string | `to_warehouse_code`, `to_warehouse_id` | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatPercent` | number | `vat_percent` | mapped |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### expenses — `/v1/erp/expenses`

**Legacy** store DB `expense` — collection `expense` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `amount` | number | `amount` | mapped |
| `categoryId` | string | `category_id[]` | mapped |
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `date` | string | `date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `description` | string | `description` | mapped |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `method` | string | `payment_method` | mapped |
| `notes` | string | `erp.x.notes` (adapter extras; round-trips, invisible to the old app) | new |
| `payee` | string | read from legacy; not written by the adapter | computed |
| `receipt` | null | `erp.x.receipt` (adapter extras; round-trips, invisible to the old app) | new |
| `reference` | string | `vendor_invoice_no` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatAmount` | number | `amount` | mapped |
| `vendorId` | string | mapped (not probed) | mapped |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### deposits — `/v1/erp/deposits`

**Legacy** store DB `customerdeposit` — collection `customerdeposit` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `amount` | number | `payments[].amount` | mapped |
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `customerId` | string | `customer_id` | mapped |
| `customerName` | string | read from legacy; not written by the adapter | computed |
| `customerNameAr` | string | read from legacy; not written by the adapter | computed |
| `date` | string | `date_str`, `payments[].date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `method` | string | `payment_method`, `payments[].method` | mapped |
| `notes` | string | `description`, `remarks` | mapped |
| `orderCode` | string | read from legacy; not written by the adapter | computed |
| `orderId` | null | `payments[].invoice_code`, `payments[].invoice_id`, `payments[].invoice_type` | mapped |
| `reference` | string | `bank_reference_no`, `payments[].bank_reference` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |
| `zatca.invoiceType` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.reportedAt` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.status` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.uuid` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |

### withdrawals — `/v1/erp/withdrawals`

**Legacy** store DB `customerwithdrawal` — collection `customerwithdrawal` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `amount` | number | `payments[].amount` | mapped |
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `customerId` | string | `customer_id` | mapped |
| `customerName` | string | read from legacy; not written by the adapter | computed |
| `customerNameAr` | string | read from legacy; not written by the adapter | computed |
| `date` | string | `date_str`, `payments[].date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `method` | string | `payment_method`, `payments[].method` | mapped |
| `notes` | string | `description`, `remarks` | mapped |
| `orderCode` | string | read from legacy; not written by the adapter | computed |
| `orderId` | null | `payments[].invoice_code`, `payments[].invoice_id`, `payments[].invoice_type` | mapped |
| `reference` | string | `bank_reference_no`, `payments[].bank_reference` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `type` | string | read-only, derived from the legacy document | computed |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |
| `zatca.invoiceType` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.reportedAt` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.status` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |
| `zatca.uuid` | string | server-owned: legacy `zatca.*` / `uuid` / `hash` / `prev_hash` / `invoice_count_value` (existing ZATCA flow) | computed |

### capitals — `/v1/erp/capitals`

**Legacy** store DB `capital` — collection `capital` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `amount` | number | `amount` | mapped |
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `date` | string | `date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `investor` | string | mapped, validated (probe rejected: investor: must be the name of an existing user) | mapped |
| `method` | string | `payment_method` | mapped |
| `notes` | string | `description` | mapped |
| `reference` | string | `erp.x.reference` (adapter extras; round-trips, invisible to the old app) | new |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### capitalWithdrawals — `/v1/erp/capital-withdrawals`

**Legacy** store DB `capitalwithdrawal` — collection `capitalwithdrawal` (store DB). Writes go through the existing v1 handlers.

- PATCH/PUT of mapped fields → 409: Capital withdrawals cannot be edited in the existing system (its update ignores changes); delete and re-create it.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `amount` | number | `amount` | mapped |
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `date` | string | `date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `investor` | string | mapped, validated (probe rejected: investor: must be the name of an existing user) | mapped |
| `method` | string | `payment_method` | mapped |
| `notes` | string | `description` | mapped |
| `reference` | string | `erp.x.reference` (adapter extras; round-trips, invisible to the old app) | new |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### dividends — `/v1/erp/dividends`

**Legacy** store DB `divident` — collection `divident` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `amount` | number | `amount` | mapped |
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `date` | string | `date_str` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `method` | string | `payment_method` | mapped |
| `notes` | string | `description` | mapped |
| `recipient` | string | mapped, validated (probe rejected: recipient: must be the name of an existing user) | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### salaries — `/v1/erp/salaries`

**Legacy** store DB `employee_salary_payment` — collection `employee_salary_payment` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `advanceDeducted` | number | `erp.x.advanceDeducted` (adapter extras; round-trips, invisible to the old app) | new |
| `basicSalary` | number | read-only, derived from the legacy document | computed |
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `deductions` | number | `erp.x.deductions` (adapter extras; round-trips, invisible to the old app) | new |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `employeeId` | string | mapped (not probed) | mapped |
| `employeeName` | string | read from legacy; not written by the adapter | computed |
| `employeeNameAr` | string | read-only, derived from the legacy document | computed |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `housing` | number | `erp.x.housing` (adapter extras; round-trips, invisible to the old app) | new |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `method` | string | `payment_method` | mapped |
| `netSalary` | number | `amount` | mapped |
| `notes` | string | `description` | mapped |
| `otherAllowances` | number | `erp.x.otherAllowances` (adapter extras; round-trips, invisible to the old app) | new |
| `paymentDate` | string | `date_str`, `date` | mapped |
| `period` | string | mapped, validated (probe rejected: period: YYYY-MM) | mapped |
| `reference` | string | `erp.x.reference` (adapter extras; round-trips, invisible to the old app) | new |
| `status` | string | read-only, derived from the legacy document | computed |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `totalAllowances` | number | `erp.x.totalAllowances` (adapter extras; round-trips, invisible to the old app) | new |
| `totalEarnings` | number | `erp.x.totalEarnings` (adapter extras; round-trips, invisible to the old app) | new |
| `transport` | number | `erp.x.transport` (adapter extras; round-trips, invisible to the old app) | new |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### repairJobs — `/v1/erp/repair-jobs`

**Legacy** store DB `repair_job` — collection `repair_job` (store DB). Writes go through the existing v1 handlers.

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `additional` | number | `erp.x.additional` (adapter extras; round-trips, invisible to the old app) | new |
| `additionalDesc` | string | `erp.x.additionalDesc` (adapter extras; round-trips, invisible to the old app) | new |
| `code` | string | read from legacy; not written by the adapter | computed |
| `complaint` | string | `complaint` | mapped |
| `complaintAr` | string | `erp.x.complaintAr` (adapter extras; round-trips, invisible to the old app) | new |
| `completedAt` | string | `erp.x.completedAt` (adapter extras; round-trips, invisible to the old app) | new |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `customerId` | string | `customer_id` | mapped |
| `customerName` | string | read from legacy; not written by the adapter | computed |
| `customerNameAr` | string | `erp.x.customerNameAr` (adapter extras; round-trips, invisible to the old app) | new |
| `date` | string | `date` | mapped |
| `deleted` | boolean | `deleted` (`erp.del` when the legacy delete is destructive) | server |
| `estDelivery` | string | mapped, validated (probe rejected: estDelivery: invalid date) | mapped |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `inspection` | string | `inspection` | mapped |
| `labour` | number | `labour_charge` | mapped |
| `make` | string | `brand` | mapped |
| `model` | string | `model` | mapped |
| `nonvatId` | null | `non_vat_sales_id` | mapped |
| `odometer` | number | `km` | mapped |
| `parts[].nameAr` | string | accepted; no legacy effect for this value | mapped |
| `parts[].nameEn` | string | `parts[].name` | mapped |
| `parts[].partNo` | string | `parts[].part_number` | mapped |
| `parts[].productId` | string | `parts[].product_id` | mapped |
| `parts[].purchasePrice` | number | `parts[].purchase_unit_price` | mapped |
| `parts[].qty` | number | `parts[].qty`, `parts[].total_price_with_vat`, `parts[].total_price` | mapped |
| `parts[].unitDiscount` | number | `parts[].total_price_with_vat`, `parts[].total_price`, `parts[].unit_discount_with_vat`, `parts[].unit_discount` | mapped |
| `parts[].unitPrice` | number | `parts[].total_price_with_vat`, `parts[].total_price`, `parts[].unit_price_with_vat`, `parts[].unit_price` | mapped |
| `parts[].warehouseId` | string | read from legacy; not written by the adapter | computed |
| `plate` | string | `vehicle_number` | mapped |
| `quotationId` | null | `quotation_id` | mapped |
| `saleId` | string | `order_id` | mapped |
| `status` | string | `status` | mapped |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `technicianIds` | array | `technician_ids` | mapped |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `vatPercent` | number | `vat_percent` | mapped |
| `vehicleId` | string | mapped (not probed) | mapped |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |
| `workDone` | string | `work_done` | mapped |

### rfqs — `/v1/erp/rfqs`

**Legacy** main DB `rfq_received` (store_id) — collection `rfq_received` (main DB). Writes go through the existing v1 handlers.

- soft delete kept in `erp.del` (legacy delete is destructive; `?hard=1` calls it)

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `attachment` | string | `erp.x.attachment` (adapter extras; round-trips, invisible to the old app) | new |
| `attachmentData` | string|null | `erp.x.attachmentData` (adapter extras; round-trips, invisible to the old app) | new |
| `code` | string | read from legacy; not written by the adapter | computed |
| `createdAt` | string(datetime-local) | `created_at` | server |
| `createdBy` | string | `created_by_name` / `erp.cb` | server |
| `customerId` | string|null | `customer_id` | mapped |
| `customerName` | string | `customer_name` | mapped |
| `deleted` | boolean | `erp.del` (`erp.del` when the legacy delete is destructive) | server |
| `history[].action` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].at` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].by` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].field` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].from` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `history[].changes[].to` | string | `erp.h` (else synthesized from legacy audit fields) | server |
| `id` | string | `_id` (hex; client ids kept as alias `erp.cid`) | server |
| `items[].confidence` | number | accepted; no legacy effect for this value | mapped |
| `items[].id` | string | read from legacy; not written by the adapter | computed |
| `items[].name` | string | `products[].name` | mapped |
| `items[].notes` | string | `products[].notes` | mapped |
| `items[].qty` | number | `products[].quantity` | mapped |
| `items[].unit` | string | `products[].unit` | mapped |
| `markup` | number | `erp.x.markup` (adapter extras; round-trips, invisible to the old app) | new |
| `message` | string | `text_content` | mapped |
| `poIds` | array | `erp.x.poIds` (adapter extras; round-trips, invisible to the old app) | new |
| `processedAt` | string | `erp.x.processedAt` (adapter extras; round-trips, invisible to the old app) | new |
| `quotationId` | null | `erp.x.quotationId` (adapter extras; round-trips, invisible to the old app) | new |
| `receivedAt` | string | read from legacy; not written by the adapter | computed |
| `replies[].channel` | string | `erp.x.replies[].channel` (adapter extras; round-trips, invisible to the old app) | new |
| `replies[].leadDays` | number | `erp.x.replies[].leadDays` (adapter extras; round-trips, invisible to the old app) | new |
| `replies[].prices` | array | `erp.x.replies[].prices` (adapter extras; round-trips, invisible to the old app) | new |
| `replies[].receivedAt` | string | `erp.x.replies[].receivedAt` (adapter extras; round-trips, invisible to the old app) | new |
| `replies[].supplierId` | string | `erp.x.replies[].supplierId` (adapter extras; round-trips, invisible to the old app) | new |
| `selection` | object | `erp.x.selection` (adapter extras; round-trips, invisible to the old app) | new |
| `sendErrors` | object | `erp.x.sendErrors` (adapter extras; round-trips, invisible to the old app) | new |
| `sendStatus.sup0` | string | `erp.x.sendStatus.sup0` (adapter extras; round-trips, invisible to the old app) | new |
| `sendStatus.sup1` | string | `erp.x.sendStatus.sup1` (adapter extras; round-trips, invisible to the old app) | new |
| `sendStatus.sup2` | string | `erp.x.sendStatus.sup2` (adapter extras; round-trips, invisible to the old app) | new |
| `source` | string | read from legacy; not written by the adapter | computed |
| `status` | string | read-only, derived from the legacy document | computed |
| `storeId` | string | `store_id` (also selects the store database `store_<id>`) | mapped |
| `supplierIds` | array | read from legacy; not written by the adapter | computed |
| `threadId` | string | `erp.x.threadId` (adapter extras; round-trips, invisible to the old app) | new |
| `updatedAt` | string(datetime-local) | `updated_at` | server |
| `updatedBy` | string | `updated_by_name` | server |
| `version` | integer | `erp.v` (else derived from `updated_at`) | server |

### threads — `/v1/erp/threads`

**New adapter collection** `erp_thread` (store DB) — no legacy equivalent; every field is stored as sent. 

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `account` | string | `erp_thread.account` | new |
| `channel` | string | `erp_thread.channel` | new |
| `createdAt` | string(datetime-local) | envelope / server-owned | server |
| `createdBy` | string | envelope / server-owned | server |
| `deleted` | boolean | envelope / server-owned | server |
| `history[].action` | string | `erp_thread.history[].action` | new |
| `history[].at` | string | `erp_thread.history[].at` | new |
| `history[].by` | string | `erp_thread.history[].by` | new |
| `history[].changes[].field` | string | `erp_thread.history[].changes[].field` | new |
| `history[].changes[].from` | string | `erp_thread.history[].changes[].from` | new |
| `history[].changes[].to` | string | `erp_thread.history[].changes[].to` | new |
| `id` | string | envelope / server-owned | server |
| `messages[].at` | string | `erp_thread.messages[].at` | new |
| `messages[].attachment` | string | `erp_thread.messages[].attachment` | new |
| `messages[].dir` | string | `erp_thread.messages[].dir` | new |
| `messages[].from` | string | `erp_thread.messages[].from` | new |
| `messages[].id` | string | `erp_thread.messages[].id` | new |
| `messages[].status` | string | `erp_thread.messages[].status` | new |
| `messages[].text` | string | `erp_thread.messages[].text` | new |
| `name` | string | `erp_thread.name` | new |
| `pinned` | boolean | `erp_thread.pinned` | new |
| `purchaseCol` | string | `erp_thread.purchaseCol` | new |
| `purchaseId` | null | `erp_thread.purchaseId` | new |
| `rfqId` | string | `erp_thread.rfqId` | new |
| `storeId` | string | `erp_thread.storeId` | new |
| `supplierId` | string | `erp_thread.supplierId` | new |
| `unread` | number | `erp_thread.unread` | new |
| `updatedAt` | string(datetime-local) | envelope / server-owned | server |
| `updatedBy` | string | envelope / server-owned | server |
| `version` | integer | envelope / server-owned | server |

### notifications — `/v1/erp/notifications`

**New adapter collection** `erp_notification` (store DB) — no legacy equivalent; every field is stored as sent. 

| Contract field | Type | Mapping | Class |
|---|---|---|---|
| `at` | string | `erp_notification.at` | new |
| `bodyAr` | string | `erp_notification.bodyAr` | new |
| `bodyEn` | string | `erp_notification.bodyEn` | new |
| `createdAt` | string(datetime-local) | envelope / server-owned | server |
| `createdBy` | string | envelope / server-owned | server |
| `deleted` | boolean | envelope / server-owned | server |
| `history[].action` | string | `erp_notification.history[].action` | new |
| `history[].at` | string | `erp_notification.history[].at` | new |
| `history[].by` | string | `erp_notification.history[].by` | new |
| `history[].changes[].field` | string | `erp_notification.history[].changes[].field` | new |
| `history[].changes[].from` | string | `erp_notification.history[].changes[].from` | new |
| `history[].changes[].to` | string | `erp_notification.history[].changes[].to` | new |
| `id` | string | envelope / server-owned | server |
| `link` | string | `erp_notification.link` | new |
| `read` | boolean | `erp_notification.read` | new |
| `storeId` | string | `erp_notification.storeId` | new |
| `titleAr` | string | `erp_notification.titleAr` | new |
| `titleEn` | string | `erp_notification.titleEn` | new |
| `tone` | string | `erp_notification.tone` | new |
| `type` | string | `erp_notification.type` | new |
| `updatedAt` | string(datetime-local) | envelope / server-owned | server |
| `updatedBy` | string | envelope / server-owned | server |
| `version` | integer | envelope / server-owned | server |

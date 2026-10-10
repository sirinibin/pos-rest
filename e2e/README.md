# API end-to-end and functional tests (v2)

`e2e/api` tests the StartERP adapter API (`/v1/erp`) the way the web app uses it: over HTTP against a
**running server** built from this repo, with MongoDB and Redis. Routing, middleware, the legacy v1 handlers
the adapter calls, background work (stock, ledger) and the per-store databases all run as in production.

Every test signs up its own company (store + owner) through `POST /auth/signup`, so tests share no data and run
in parallel. When `E2E_MONGO_URI` is set the store's database is dropped when the test ends.

| File | What it covers |
|---|---|
| `client_test.go`, `fixtures_test.go`, `main_test.go` | HTTP client and JSON helpers, sign-up per country, staff users per role, products/customers/vendors, route coverage report |
| `sales_e2e_test.go` | Sales and sales returns: totals and VAT against an independent calculation, payments and statuses, stock, customer balance, numbering, concurrency, idempotency |
| `documents_e2e_test.go` | Quotations, proformas, delivery notes, non-VAT sales/returns, quotation returns, drafts, POS records and numbers, packages, vehicles, repair jobs, signatures |
| `purchases_e2e_test.go` | Purchases and returns, vendor balances, purchase orders/requests/bills, stock transfers, warehouses, products (stock, history, search, facets), masters, RFQs |
| `finance_e2e_test.go` | Expenses, debit/credit notes, capital, dividends, salaries, accounts, list stats and every dashboard figure against an oracle, store-timezone day boundaries, all supported countries, billing |
| `security_e2e_test.go` | Sign-up/login/refresh/logout/password rules, store isolation for every resource, role permissions, users and roles, store settings |
| `concurrency_e2e_test.go` | Owner, manager, salesman and cashier of three stores (SA, SA, AE) selling at the same moment, then paying credit sales off at the same moment: invoice numbers unique and gapless per store, stock out exactly once per line, payment status, customer balances, list stats and dashboards against an independent calculation |
| `robustness_e2e_test.go` | Every adapter route with hostile input: no crash, no 5xx, no hang, 401 without a token |
| `zatca_sandbox_e2e_test.go` | Build tags `e2e zatca`. ZATCA Phase 2 on ZATCA's non-production sandbox with ZATCA's test taxpayer (VAT 399999999900003, CRN 4030360927, sandbox OTP 12345): device onboarding, simplified invoice reporting, ICV/PIH chain, standard invoice clearance, credit note, debit note, re-onboarding after a sensitive change, disconnect |

Checks for bugs that are reported but not fixed yet use `KnownBug(t, id, …)`: the test logs `KNOWN BUG <id>` and
passes while the bug is there, and fails once it is fixed so the check becomes a normal assertion. Open bugs are
listed in the project notes (`notes/backend-bugs-found-2026-10-10.md`).

## Run

```
# MongoDB on :27017 and Redis on :6379
MONGO_HOST=127.0.0.1 MONGO_PORT=27017 REDIS_DSN=127.0.0.1:6379 e2e/run.sh            # whole suite
MONGO_HOST=127.0.0.1 MONGO_PORT=27017 REDIS_DSN=127.0.0.1:6379 e2e/run.sh -run TestSales -v

# ZATCA sandbox (internet, Python 3, Java 11+)
ci/zatca-sdk.sh
E2E_ZATCA=1 MONGO_HOST=127.0.0.1 MONGO_PORT=27017 REDIS_DSN=127.0.0.1:6379 e2e/run.sh -run TestZatcaSandbox -v
```

`run.sh` builds the server, starts it on `:2010` (`API_PORT`) against the test database `t1_e2e` (`MONGO_DB`, only
`t1_*`/`test*`/`erp_test*` names are accepted) and runs `go test -tags e2e ./e2e/...`. `E2E_RACE=1` builds the
server with the race detector and counts the races it logs. Against a server that is already running:
`E2E_BASE_URL=http://127.0.0.1:2010 E2E_MONGO_URI=mongodb://127.0.0.1:27017 go test -tags e2e ./e2e/api/`.

`ci/zatca-sdk.sh` installs `ZatcaPython/venv` and ZATCA's Java SDK into `ZatcaPython/utilities/fatoora-cli-simulation`:
the production server's copy (SDK 238-R3.4.4) from github.com/sirinibin/zatca-sdk, checked against a pinned SHA-256.

## CI

`.github/workflows/deploy_v2.yml`: the `e2e` job runs this suite against a race-detector build of the server and
gates the deploy. `zatca-sandbox` runs the ZATCA tests and is report-only, so a ZATCA sandbox outage can't block a
deploy. The `api` job also checks that every `/v1/erp` route is called by the in-process tests
(`ERP_ROUTE_COVERAGE_MIN=100`, recorder in `erp/route_coverage_test.go`).

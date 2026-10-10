# End-to-end tests

| Path | What it is |
|---|---|
| `seed/` | `go run ./e2e/seed` upserts the e2e admin user (`E2E_EMAIL` / `E2E_PASSWORD`, defaults `e2e-admin@startpos.test` / `E2e-Passw0rd!`). Refuses any `MONGO_DB` whose name lacks "e2e" or "test". Also used by the frontend's Playwright suite. |
| `api/` | API e2e tests (`//go:build e2e`) that call a running server over HTTP: health, auth and rate limiting, store, customer and product create/validate/read/search. |
| `api/flows_e2e_test.go` | Functional tests of whole business flows, each in its own fresh store: purchases (VAT, payment status, stock in), sales (totals, profit, stock out, customer credit balance, calculate-net-total parity, invoice codes under concurrency), sale payments, sale updates, sales and purchase returns (stock back, over-returns refused), quotations, expenses, customer deposits/withdrawals, store isolation, plus table-driven validation cases. Every money document is checked for a balanced double-entry ledger. |
| `api/flows2_e2e_test.go` | Capital, capital withdrawals and dividends (edits, ledger), employees and salaries, sales and purchase cash discounts, warehouses and stock transfers, delivery notes and purchase orders, non-VAT sales, catalogue masters, users. |
| `api/flows3_e2e_test.go` | Quotation invoices and their returns and refunds; purchase, sales-return and purchase-return payments (caps, status, delete); non-VAT sales returns; purchase requests; previous/next/last navigation. |
| `api/flows4_e2e_test.go` | Vehicles, repair jobs (VAT totals, unique numbers), customer packages, signatures. |
| `api/flows5_e2e_test.go` | Negative line quantities on every document, stock transfer limits and edits, ZATCA secrets kept out of store responses, onboarding error codes, sales marked for ZATCA in stores off ZATCA. |
| `api/flows6_e2e_test.go` | Non-VAT returns sent without "selected" flags, 400 on validation errors, blank names, user creation and role grants, purchase request decisions by the assignee, role permissions on writes (RBAC module on and off), the RFQ supplier list shape, half-cent refunds., service category names. |
| `api/zatca_concurrency_e2e_test.go` (`-tags "e2e zatca"`) | Several users of one store and users of different stores reporting sales and credit notes to the ZATCA sandbox at once: own certificate, unique ICVs, unforked PIH chain, right "created by"; stores onboarding at once. |
| `api/dashboard_e2e_test.go` | The dashboard month summary and outstanding add up to the documents entered; trial balance (debits = credits) and the cash account. |
| `api/security_e2e_test.go` | A user limited to some stores can't reach other stores (query, body, path, ZATCA, MCP); managers only manage their own staff; no password hashes in responses. |
| `api/zatca_sandbox_e2e_test.go` | Build tags `e2e zatca`: ZATCA Phase 2 against ZATCA's sandbox with a new non-production store (VAT 399999999900003, CRN 4030360927): onboarding, simplified and standard invoices, a credit note, disconnect. Needs the `ZatcaPython/venv` and internet. |
| `api/robustness_e2e_test.go` | Calls every route in `main.go` with bad, empty and hostile input (zero/missing store ids, JSON null, broken JSON, wrong types) and fails on any dropped connection (a handler panic) or hang. Host-level and third-party routes are skipped (see `sweepSkip`). |
| `restart/` | Restarts the built server the way a deploy does (systemd socket activation, SIGTERM, new process) under steady HTTP and HTTPS traffic and fails on any dropped request. Needs `./pos-rest` built at the repo root. |
| `fixtures/store.json` | A valid store payload. |

Run locally against MongoDB on :27017 and Redis on :6379:

```
export MONGO_DB=pos_e2e REDIS_DSN=localhost:6379
go run ./e2e/seed
go build -o pos-rest . && API_PORT=2000 ./pos-rest &
go test -tags e2e ./e2e/... -count=1 -v
```

GitHub Actions runs these, the unit/API tests, the integration tests
(`-tags integration`), a race-detector job (unit + integration with `-race`,
then this suite against a `-race` build of the server), the ZATCA sandbox
tests (report only, so a sandbox outage can't block a deploy) and the
frontend's full-stack Playwright suite in
`.github/workflows/tests.yml` before every test and production deploy.

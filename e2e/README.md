# End-to-end tests

| Path | What it is |
|---|---|
| `seed/` | `go run ./e2e/seed` upserts the e2e admin user (`E2E_EMAIL` / `E2E_PASSWORD`, defaults `e2e-admin@startpos.test` / `E2e-Passw0rd!`). Refuses any `MONGO_DB` whose name lacks "e2e" or "test". Also used by the frontend's Playwright suite. |
| `api/` | API e2e tests (`//go:build e2e`) that call a running server over HTTP: health, auth and rate limiting, store, customer and product create/validate/read/search. |
| `api/flows_e2e_test.go` | Functional tests of whole business flows, each in its own fresh store: purchases (VAT, payment status, stock in), sales (totals, profit, stock out, customer credit balance, calculate-net-total parity, invoice codes under concurrency), sale payments, sale updates, sales and purchase returns (stock back, over-returns refused), quotations, expenses, customer deposits/withdrawals, store isolation, plus table-driven validation cases. Every money document is checked for a balanced double-entry ledger. |
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
then this suite against a `-race` build of the server) and the frontend's
full-stack Playwright suite in
`.github/workflows/tests.yml` before every test and production deploy.

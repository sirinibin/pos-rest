# Backend Instructions

## Environment
- Go backend on port 2000
- After every change: `kill $(lsof -ti :2000) 2>/dev/null; cd backend && go run main.go &`
- Wait for it to listen on port 2000 before testing.

## Deploy rules (backend/deploy.sh)
deploy.sh enforces three gates before every deploy — all three must pass:

1. **No uncommitted changes** — `git status --porcelain` must be empty.
   Commit or stash everything before running deploy.sh.

2. **All tests pass** — `go test ./... -count=1` must exit 0.
   Skipped: TestResolveDateKeyword_TimezoneOffset_SA (flaky near midnight UTC).

3. **No build output** — `go build` must produce zero output.
   Any output from go build (warnings, notes) causes an abort.

## GitHub Actions
`deploy_test.yml` / `deploy_prod.yml` call `tests.yml` and deploy only if all of it passes:
unit + API tests, integration tests (`-tags integration`, MongoDB + Redis services, a versitygw S3 server for the
`S3_TEST_*` storage tests),
API e2e (`-tags e2e`, see `e2e/README.md`), a race-detector job (`-race` on unit + integration,
and the e2e suite against a `-race` server build), a report-only ZATCA sandbox job (`-tags "e2e zatca"`,
test VAT 399999999900003 / CRN 4030360927, never Production; it takes ZATCA's Java Fatoora SDK from the public
https://github.com/sirinibin/zatca-sdk repo, which is where the SDK comes from wherever it is needed) and the frontend's full-stack Playwright suite
(reactjs-pos, same branch).

Store access: `controller.StoreAccessMiddleware` answers 403 when a request names a store (query
`search[store_id]`/`store_id`, body `store_id`, `/v1/store/{id}`, ZATCA connect `id`) the user may not
use. A new endpoint that takes a store under another key needs its own check.
`controller.RBACWriteMiddleware` answers 403 for a create/update/delete (`POST /v1/x`, `PUT|PATCH|DELETE /v1/x/{id}`)
the user's roles don't grant, in stores with `settings.enable_rbac_module` on; map new resources in `rbacResources`.
Concurrency: recompute-and-save of product stock goes through `store.RefreshProductStock` / `store.UpdateProductLocked`
(per-product lock); the signed-in user comes from `models.UserFromClaims(claims)`, never a package variable.
Account balances are recomputed under a per-account lock; customer/vendor `credit_balance` and `account` are written
only by `SetCreditBalance` (`Update()` leaves them out). Document counters start with `SetNX`. Check-then-insert
(user email, the 30-second receipt duplicate guard) holds `models.LockKey`. `models.ClientsMu` guards websocket clients.
ZATCA: a store is one device, so invoices, credit and debit notes share one ICV (`zatca.icv`) and one PIH chain
(`models.NextZatcaChainLink`, under the store's "zatca" queue); `invoice_count_value` stays the per-type document
number. A document ZATCA accepted answers 409 `already_reported` if reported again. Unsigned XML goes to
`zatcaXMLPath` (store id in the name).
Validation errors answer 400: a handler that writes no status calls `ensureStatus(w, http.StatusBadRequest)`. Note `TestHealthCheck_ServicesDown` only holds with no MongoDB/Redis.

## Usage
```
backend/deploy.sh   # deploys to both test and production
```

## Low-downtime restarts
Every deploy path (deploy.sh, deploy_quick.sh, the deploy workflows) restarts the API through
`deploy/remote_restart.sh`: rename the new binary in, `systemctl restart`, wait for `/v1/health`,
roll back to the previous binary if it never becomes healthy. Never `fuser -k` or `systemctl stop`
the API in a deploy. The server drains in-flight requests on SIGTERM (`SHUTDOWN_TIMEOUT`, default 20s)
and binds its port before connecting to MongoDB (`lifecycle/`). With the one-time
`deploy/enable_socket_activation.sh <service> <port>` on the server, systemd holds the port across
restarts and no request is refused. Ports: production 2000, test 2002, v2 2004 (HTTPS = port+1).

## After every backend change
1. Write Go tests for changed logic.
2. `go test ./...` to verify.
3. Commit all changed files.
4. Run `backend/deploy.sh`.

## Branch isolation rule (NON-NEGOTIABLE)

The `v2` branch is a **completely separate product line** from `master`/`test`.

1. **Never merge, cherry-pick, or rebase between `v2` and `master`/`test` in either direction.**
   Features for v2 stay on v2. Features for master/test stay there. No exceptions.

2. **The one allowed exception — GitHub Actions workflow files only:**
   GitHub only reads `.github/workflows/` from the default branch (`master`).
   When adding a new workflow file to `v2`, copy that file to `master` using:
   `git checkout v2 -- .github/workflows/<file>.yml`
   This is a file copy only — NOT a merge. No other files cross the v2 boundary.

3. **`master` and `test` may share changes freely.** This rule only applies to the v2 boundary.

4. **v2 service details:** `start-api-v2`, port 2004, path `/home/ubuntu/go/src/github.com/sirinibin/pos-rest-v2`,
   API at `https://startpos-api-v2.gulfunionozone.com`. Deploy with `deploy_v2.sh` from the `v2` branch only.

## API Test Sync Rule (non-negotiable)
**Every time a backend API is enhanced or a new endpoint is added:**
- Add or update test cases in `controller/*_test.go` for the changed handler
- New validation rules → new table-driven test row
- New response field → updated assertion
- New endpoint → new `_Unauthenticated` test + pure-function tests + integration stub
- The test suite must stay at 0 failures after every commit
- Run `go test ./... -count=1` before and after every change to confirm

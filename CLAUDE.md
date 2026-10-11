# Backend Instructions

## Environment
- Go backend on port 2000
- After every change: `kill $(lsof -ti :2000) 2>/dev/null; cd backend && go run main.go &`
- Wait for it to listen on port 2000 before testing.

## Deploy rules (backend/deploy.sh)
deploy.sh is the deploy gate:

1. **No uncommitted changes** — `git status --porcelain` must be empty.
   Commit or stash everything before running deploy.sh.

2. **Every test suite passes** — `ci/run_all_tests.sh` runs, locally on a copy of HEAD
   and stopping at the first failure: unit + API tests, the ZATCA signing-key tests, integration
   (MongoDB + Redis + versitygw S3), API e2e (incl. the robustness sweep and restart test), and the
   frontend's full-stack Playwright suite on every screen size, the race detector and the ZATCA
   sandbox tests. Deploy only after all of it passes (user rule 2026-10-11): `QUICK=1
   ci/run_all_tests.sh` (laptop screen only, no race detector or ZATCA sandbox) is for local
   iteration, and deploy.sh refuses to run with `QUICK` set. ZATCA sandbox tests (ZATCA's Java Fatoora SDK comes from
   https://github.com/sirinibin/zatca-sdk; test VAT 399999999900003 / CRN 4030360927, never Production).
   Skipped: TestResolveDateKeyword_TimezoneOffset_SA (flaky near midnight UTC).
   Needs Docker running (throwaway MongoDB/Redis/S3 on ports 37017/36379/38333, API on 32000, so a
   dev stack on 27017/6379/2000 is untouched), go, node 18+, java 17+, python3. reactjs-pos comes
   from GitHub at master (`FRONTEND_REF` to change) or `FRONTEND_DIR=<checkout>`. Settings are
   in `ci/lib.sh` (keep it identical to reactjs-pos's `ci/lib.sh`).

3. **No build output** — `go build` must produce zero output.
   Any output from go build (warnings, notes) causes an abort.

## No GitHub Actions
GitHub Actions is not used (user decision 2026-10-11, cost): `.github/workflows/` is gone and must not be added
back (`deploy/workflows_test.go` fails if it exists). `deploy.sh` is the only deploy path, run from the user's Mac
(it needs Docker and the server SSH key); a push to `master` deploys nothing. It tests once, builds the binary once,
deploys it to the test API (start-api-test 2002) and, only after that is healthy, the same binary to production
(start-api 2000). There is no `test` branch (removed 2026-10-11): push to `master` only. A failed deploy is rolled
back by `remote_restart.sh`. `deploy_quick.sh` is the emergency path with tests skipped.

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
`zatcaXMLPath` (store id in the name). The Fatoora SDK signs only with a SEC1 key: given PKCS#8 (what its `-csr` writes) it
leaves SignatureValue empty yet reports success, so the signer converts with `sdk_private_key` and refuses an
unsigned result (`ZatcaPython/tests`, run in the unit job). Calls to ZATCA (`api_helper.post_request_with_retries`)
accept 200 and 202 (passed with warnings), time out, and retry only answers ZATCA never handled (429, 5xx except 504,
Cloudflare 52x, connection errors).
Validation errors answer 400: a handler that writes no status calls `ensureStatus(w, http.StatusBadRequest)`. Note `TestHealthCheck_ServicesDown` only holds with no MongoDB/Redis.

## Usage
```
backend/deploy.sh              # every suite, one build, test API (health-checked), then production
backend/deploy.sh --test-only  # same, stop after the test API
```

## Low-downtime restarts
Every deploy path (deploy.sh, deploy_quick.sh) restarts the API through
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

The `v2` branch is a **completely separate product line** from `master`.

1. **Never merge, cherry-pick, or rebase between `v2` and `master` in either direction.**
   Features for v2 stay on v2. Features for master stay there. No exceptions.

2. **No exceptions for workflow files either:** GitHub Actions is not used, so nothing under
   `.github/workflows/` is copied between `v2` and `master`.

3. **There is no `test` branch.** The test server gets master's tested build from `deploy.sh`.

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

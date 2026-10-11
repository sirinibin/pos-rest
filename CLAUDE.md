# Backend Instructions

## Environment
- Go backend on port 2000
- After every change: `kill $(lsof -ti :2000) 2>/dev/null; cd backend && go run main.go &`
- Wait for it to listen on port 2000 before testing.

## Deploy rules (backend/deploy.sh)
deploy.sh enforces three gates before every deploy — all three must pass:

1. **No uncommitted changes** — `git status --porcelain` must be empty.
   Commit or stash everything before running deploy.sh.

2. **All tests pass** — `go test ./... -count=1` must exit 0 (no skipped tests).
   For v2, deploy_v2.sh also runs lint, the race detector, the full DB-backed
   suite with MongoDB + Redis, and ci/smoke.sh before deploying.

3. **No build output** — `go build` must produce zero output.
   Any output from go build (warnings, notes) causes an abort.

## Usage
```
backend/deploy.sh   # deploys to both test and production
```

## v2 deploy (deploy_v2.sh, the only deployment route)
**No GitHub Actions (user, 2026-10-11: too costly).** Every workflow on v2 is manual-only
(`workflow_dispatch`); a push to v2 deploys nothing. The owner releases by running `./deploy_v2.sh`
from their Mac checkout of v2. Don't wait for, or report, Actions runs after a push; a change is
live once the owner's `deploy_v2.sh` run has passed. The user (2026-10-11): the full A-Z test set
must pass before anything deploys; the deploy step only runs after every gate passed in the same
run, and `--force` skips only the branch check, never a test. `.github/workflows/deploy_v2.yml` stays as the
job list `ci/deploysync` checks the script against: change both together.
```
./deploy_v2.sh                  # every CI gate in turn, then deploy (v2 branch, clean tree)
./deploy_v2.sh --check          # every CI gate, no deploy
./deploy_v2.sh --only unit,api  # just those steps, no deploy
```
Runs the same gates as `.github/workflows/deploy_v2.yml` one after another and deploys only when
all pass: lint (go vet, staticcheck, silent build), unit (`-race`, no DB, >= 2000 passing, Card
Bridge), api (`-tags integration`, MongoDB + Redis + `ci/s3-test-server.sh`, >= 2500, full route
coverage), smoke (`ci/smoke.sh`), e2e (`e2e/run.sh`, race server, >= 300), ui-adapter (`e2e-starterp`:
the frontend master build against /v1/erp, read back through v1; not in the workflow), race-db
(`go test -race ./erp` with the DB, a gate since 2026-10-11), then the ZATCA sandbox (report only:
it depends on ZATCA's own sandbox being up), then the existing build/upload/swap plus the Card Bridge
setup files, then the merged `claude/*` branch cleanup. Stops at the first failing gate with its
output and a per-step summary with every suite's passed/failed/skipped counts; a skipped test fails
the run unless it passed elsewhere (`ci/skip-gate.sh`: unit's DB tests must pass in api). Logs in `.deploy-logs/<time>/`. Needs Go, jq, Docker (S3 test
server; also starts MongoDB/Redis when none run on 27017/6379), and Java 11+ + python3 for the
ZATCA step (skipped without). `ci/deploysync` tests keep the script in step with the workflow.
Environment: the script sets `ERP_TEST_DB=1`, `S3_TEST_REQUIRED=1` and `TZ=Europe/London` itself for
the DB steps; `MONGO_HOST`/`MONGO_PORT`/`REDIS_DSN` default to 127.0.0.1:27017 / :6379 (only `t1_*`
test databases are touched); the ZATCA step clones the Fatoora SDK from
https://github.com/sirinibin/zatca-sdk (`ci/zatca-sdk.sh`, `ZATCA_SDK_ZIP` for a local copy); the
deploy step needs the SSH key at `~/Downloads/startuptech-v2.pem` (`SSH_KEY` to change).

## After every backend change
1. Write Go tests for changed logic.
2. `go test ./...` to verify.
3. Commit all changed files.
4. Run `backend/deploy.sh`.

## Branch isolation rule (NON-NEGOTIABLE)

The `v2` branch is a **completely separate product line** from `master`/`test`.

1. **Never merge, cherry-pick, or rebase between `v2` and `master`/`test` in either direction.**
   Features for v2 stay on v2. Features for master/test stay there. No exceptions.

2. **No files cross the v2 boundary, workflow files included.** GitHub Actions is no longer used
   (2026-10-11), and Claude never touches `master`; the owner maintains master themselves.

3. **`master` and `test` may share changes freely.** This rule only applies to the v2 boundary.

4. **v2 service details:** `start-api-v2`, port 2004, path `/home/ubuntu/go/src/github.com/sirinibin/pos-rest-v2`,
   API at `https://startpos-api-v2.gulfunionozone.com`. Deploy with `deploy_v2.sh` from the `v2` branch only.

## Sync rule (NON-NEGOTIABLE)

This repo's `v2` branch is edited from multiple environments (Claude Code web app, local Claude Code, etc.).
Commits may land on `origin/v2` at any time from another session.

**Before starting any task:**
```bash
git fetch origin && git rebase origin/v2
```
Resolve any conflicts before proceeding. Never start work on a stale copy.

**Before every push:**
```bash
git fetch origin && git rebase origin/v2
ERP_TEST_DB=1 MONGO_HOST=127.0.0.1 MONGO_PORT=27017 REDIS_DSN=127.0.0.1:6379 go test -tags integration ./... -count=1
```
Both steps must pass cleanly. If the rebase pulls in new commits, re-run the tests.
Never push if tests are red or if the rebase is unresolved.

## Auto-push rule (NON-NEGOTIABLE)

Once all tests pass, **push immediately without asking** — to `origin/v2`:
```bash
git fetch origin && git rebase origin/v2
ERP_TEST_DB=1 MONGO_HOST=127.0.0.1 MONGO_PORT=27017 REDIS_DSN=127.0.0.1:6379 go test -tags integration ./... -count=1
git push origin v2
```
Do not wait for confirmation. A green test suite is the only gate.

## API Test Sync Rule (non-negotiable)
**Every time a backend API is enhanced or a new endpoint is added:**
- Add or update test cases in `controller/*_test.go` for the changed handler
- New validation rules → new table-driven test row
- New response field → updated assertion
- New endpoint → new `_Unauthenticated` test + pure-function tests + integration stub
- The test suite must stay at 0 failures after every commit
- Run `go test ./... -count=1` before and after every change to confirm

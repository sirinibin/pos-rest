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
   CI (.github/workflows/deploy_v2.yml) also runs lint, the race detector, the
   full DB-backed suite with MongoDB + Redis, and ci/smoke.sh before deploying.

3. **No build output** — `go build` must produce zero output.
   Any output from go build (warnings, notes) causes an abort.

## Usage
```
backend/deploy.sh   # deploys to both test and production
```

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

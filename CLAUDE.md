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
unit + API tests, integration tests (`-tags integration`, MongoDB + Redis services),
API e2e (`-tags e2e`, see `e2e/README.md`) and the frontend's full-stack Playwright suite
(reactjs-pos, same branch). Note `TestHealthCheck_ServicesDown` only holds with no MongoDB/Redis.

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

## API Test Sync Rule (non-negotiable)
**Every time a backend API is enhanced or a new endpoint is added:**
- Add or update test cases in `controller/*_test.go` for the changed handler
- New validation rules → new table-driven test row
- New response field → updated assertion
- New endpoint → new `_Unauthenticated` test + pure-function tests + integration stub
- The test suite must stay at 0 failures after every commit
- Run `go test ./... -count=1` before and after every change to confirm

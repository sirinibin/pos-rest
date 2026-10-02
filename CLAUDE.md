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

## Usage
```
backend/deploy.sh   # deploys to both test and production
```

## After every backend change
1. Write Go tests for changed logic.
2. `go test ./...` to verify.
3. Commit all changed files.
4. Run `backend/deploy.sh`.

## API Test Sync Rule (non-negotiable)
**Every time a backend API is enhanced or a new endpoint is added:**
- Add or update test cases in `controller/*_test.go` for the changed handler
- New validation rules → new table-driven test row
- New response field → updated assertion
- New endpoint → new `_Unauthenticated` test + pure-function tests + integration stub
- The test suite must stay at 0 failures after every commit
- Run `go test ./... -count=1` before and after every change to confirm

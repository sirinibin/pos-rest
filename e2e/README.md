# End-to-end tests

| Path | What it is |
|---|---|
| `seed/` | `go run ./e2e/seed` upserts the e2e admin user (`E2E_EMAIL` / `E2E_PASSWORD`, defaults `e2e-admin@startpos.test` / `E2e-Passw0rd!`). Refuses any `MONGO_DB` whose name lacks "e2e" or "test". Also used by the frontend's Playwright suite. |
| `api/` | API e2e tests (`//go:build e2e`) that call a running server over HTTP: health, auth and rate limiting, store, customer and product create/validate/read/search. |
| `fixtures/store.json` | A valid store payload. |

Run locally against MongoDB on :27017 and Redis on :6379:

```
export MONGO_DB=pos_e2e REDIS_DSN=localhost:6379
go run ./e2e/seed
go build -o pos-rest . && API_PORT=2000 ./pos-rest &
go test -tags e2e ./e2e/... -count=1 -v
```

GitHub Actions runs these, the unit/API tests, the integration tests
(`-tags integration`) and the frontend's full-stack Playwright suite in
`.github/workflows/tests.yml` before every test and production deploy.

# StartERP UI e2e against the `/v1/erp` adapter

Drives the StartERP UI (compiled prototype or a rebuilt `dist`) against this backend's adapter, using a
legacy-shaped fixture seeded into a **test** database. Everything the UI writes is then checked through the
old v1 endpoints.

Covered: UI login, legacy records visible (incl. an old-shape invoice), create a customer, create a product,
create a sale with payment, and create a sales return. Then v1 `GET /v1/customer|product|order|sales-payment|sales-return`
read them back.

```
cd e2e-starterp
npm ci                                   # @playwright/test only; browsers must already be installed
MONGO_PORT=27017 REDIS_DSN=127.0.0.1:6379 \
PLAYWRIGHT_BROWSERS_PATH=/path/to/browsers \
UI_FILE=/path/to/prototype/index.html \  # or UI_DIR=/path/to/starterp-web/dist
./run.sh
```

`run.sh` builds the API, then runs `go run ./e2e-starterp/seed -reset` into `MONGO_DB` (default `t1_e2e`;
anything not prefixed `t1_`/`test`/`erp_test` is refused). It starts the API on `:2010` (`API_PORT`) and
`server.mjs` on `:5180` (`UI_PORT`): the server serves the UI and proxies `/v1/*` to `/v1/erp/*`, because the
compiled prototype has a fixed `/v1` prefix. It then runs Playwright and stops both servers. A rebuilt UI can
instead be built with `VITE_API_BASE_URL=http://localhost:2010 VITE_API_PREFIX=/v1/erp`.

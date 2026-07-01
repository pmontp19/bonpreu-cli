# Plan & Tasks — bonpreu-cli

> Phase 2 (Plan) + Phase 3 (Tasks) of the spec-driven workflow. Implements `docs/SPEC.md`. Human reviews before/while implementing.

## Plan (Phase 2)

### Components & dependencies
```
config ──────────────┐
client (HTTP+jar+HAR) ┼─▶ api (typed endpoints) ─▶ cli (cobra cmds + formatters + guard) ─▶ cmd/bonpreu/main
resolve (id→uuid) ────┘
```

### Build order (sequential foundations, then parallel surfaces)
1. **Foundation**: config (0600), client (cookiejar + header/CSRF middleware + DoJSON), cobra root + global flags. *(everything depends on this)*
2. **HAR import + whoami**: HAR parser → session; `whoami` validates. *(unblocks all live testing)*
3. **id resolver**: numeric→uuid (cache + scrape) + search. *(unblocks cart/product that take `<id>`)*
4. Read surface (search/product/categories/related), Cart surface, Delivery/slots surface, Orders surface, `checkout open`. *(parallelizable after 1-3)*
5. Spending guard `--max` wiring into cart mutations. *(needs cart + a total reader)*
6. Polish: `--json` consistency, errors→stderr, man/help, testdata sanitization.

### Risks & mitigations
| Risk | Mitigation |
|---|---|
| `__QUERY_INITIAL_STATE__` markup changes | regex extract is narrow; add a fixture test; fallback to UUID-only with clear error |
| CSRF token handling differs on real mutations vs reads | capture is documented; client captures rotated token from response headers if present; test against httptest first, then 1 live mutation |
| WAF challenge (403) on CLI TLS/headers | mirror browser headers exactly; if still blocked, add uTLS later (deferred) |
| Spending guard races concurrent cart | fail-closed: always GET cart, compute projected total, refuse before write |
| `ecom-request-source-version` stale | pin from HAR; on rejection, instruct re-import (v2: re-derive) |

## Tasks (Phase 3)

Ordered by dependency. Each ≤ ~5 files, one focused session.

- [x] **T1 — Foundation: config + client + cobra root**  ✅ 2026-06-30
  - Acceptance: `bonpreu --version` works; `~/.bonpreu/` created with `0600`; client injects csrf/client-route-id/page-view-id/ecom-request-source headers; `DoJSON` round-trips via httptest.
  - Verify: `go test ./internal/{config,client}/...` + `go run ./cmd/bonpreu --version`
  - Files: `internal/config/config.go`, `internal/client/client.go`, `cmd/bonpreu/main.go`, `go.mod` (cobra)

- [x] **T2 — HAR import + whoami**  ✅ 2026-06-30 (live: 19 items / 65.46€)
  - Acceptance: `bonpreu import-har --file x.har` writes `config.json`+`cookies.json` (0600) with cookies, csrf, regionId, deliveryDestinationId, ecom-request-source-version; `bonpreu whoami` GETs `carts/active` and prints account/cart total.
  - Verify: unit test HAR parser on a sanitized fixture; manual `whoami` against live session.
  - Files: `internal/client/har.go`, `internal/cli/auth.go`

- [x] **T3 — id resolver + search**  ✅ 2026-06-30 (live search text+json; cache fills; scrape unit-tested — slug `/products/_/<id>` pending live confirm at T5)
  - Acceptance: `bonpreu search <q> --json` returns products `{productId,retailerProductId,name,price}`; resolver maps numeric→uuid via cache then `__QUERY_INITIAL_STATE__` scrape; cache persists at `~/.bonpreu/cache.json`.
  - Verify: resolver unit test (uuid passthrough, cache hit, scrape fallback on fixture HTML); live `search`.
  - Files: `internal/api/catalog.go`, `internal/client/resolve.go`, `internal/cli/search.go`

- [x] **T4 — Read surface**  ✅ 2026-06-30 (live: categories, related, product by retailerId+uuid)
  - Acceptance: `product <id>`, `categories`, `related <retailerId>` work with `--json`.
  - Verify: httptest tests for each endpoint path/body; live smoke.
  - Files: `internal/api/catalog.go`, `internal/cli/catalog.go`

- [x] **T5 — Cart surface + spending guard**  ✅ 2026-06-30 (live: get/add/remove/set/add-many + `--max` fail-closed; ApplyQuantity signed-delta + guard + readItems unit-tested)
  - Acceptance: `cart get/add/remove/set/clear` via `apply-quantity` (signed delta); `--max` fails-closed (refuses if projected total > cap or total unreadable); `add-many` from stdin JSON-lines.
  - Verify: httptest asserts signed-delta body + path; guard math unit tests; live add/remove/set→0 restored.
  - Files: `internal/api/cart.go`, `internal/cli/cart.go`, `internal/cli/guard.go`

- [x] **T6 — Delivery + slots**  ✅ 2026-06-30 (httptest: addresses query, v2 grid flatten w/ availability, reservation body; session-default dest/region, fail-closed on missing) — ✅ live 2026-07-01: `addresses --method home`, `slots --group home` + `--group cc` (v2 grid flattened, availability), `slots reserve <id>` (returned window/method/expiry); all `--json` valid
  - Acceptance: `delivery addresses --method home|cc`; `slots --group home|cc` flattens the v2 grid; `slots reserve <id>`; both `--json`.
  - Verify: httptest on slots v2 grid fixture; live `slots` for both groups.
  - Files: `internal/api/delivery.go`, `internal/cli/delivery.go`

- [x] **T7 — Orders (read-only) + checkout open**  ✅ 2026-07-01 (httptest: orders list parse+limit, bare-array fallback, decorated denormalize; checkout browserOpenArgs per-OS; `checkout open --json` prints URL) — ✅ live 2026-07-01: `orders list` empty history clean (`--json` → `null`); `orders show <realId>` fully verified live (32 lines, total, `--json` valid). 🐛 live testing exposed a wrong `decorated` shape assumption — real payload has `result`=root-id string, line items under `entities.order[id].items`, and product price nested at `price.current` (not a top-level `result` array with inline price). Fixed `GetOrder` + updated httptest fixture to the real shape.
  - Acceptance: `orders list` + `orders show <id>` (denormalize `entities.product`); `checkout open` opens browser at `/checkout`.
  - Verify: httptest on `decorated` fixture; live `orders list`.
  - Files: `internal/api/orders.go`, `internal/cli/orders.go`, `internal/cli/checkout.go`

- [x] **T8 — Polish**  ✅ 2026-07-01 (import-har --json + cart clear empty-JSON gaps closed; go vet + go test -race green; README quickstart; testdata gitignored/untracked so no PII/cookies committed)
  - Acceptance: every command honors `--json` (valid JSON, diagnostics on stderr, exit codes); `go vet`+`go test -race` green; README quickstart; testdata sanitized (no PII/cookies).
  - Verify: full `go test ./... -race -cover`; manual JSON-pipe check.
  - Files: `internal/cli/*.go`, `README.md`, `testdata/`

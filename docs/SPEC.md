# Spec: bonpreu-cli

> Phase 1 (Specify) artifact of the spec-driven workflow. Human review required before Phase 2 (Plan).
> Source of truth for the API: `docs/bonpreu-api-discovery.md`. Reuse patterns: `docs/reference-mercadona-clis.md`.

## Objective

Unofficial, agent-friendly CLI for `compraonline.bonpreuesclat.cat` packaged as a single static Go binary. Search the catalog, build/modify a cart, choose a delivery slot + shipping method, all from the terminal with `--json` output. The user finishes the order (Braintree 3DS) in the web/app — the CLI deliberately stops at a fully-configured cart.

**Users:** power users scripting their grocery runs, and LLM agents operating a shopping cart via deterministic JSON I/O.

**MVP = Read + Cart + Slots/Shipping (+ `checkout open` handoff, + read-only order history).** Order placement is out of scope forever for this spec.

## Tech Stack

- **Language:** Go 1.26 (module `github.com/pmontp19/bonpreu-cli`)
- **CLI:** `github.com/spf13/cobra` (subcommand routing, persistent flags, completion)
- **HTTP / JSON:** stdlib `net/http` (+ `net/http/cookiejar`), `encoding/json`
- **Config/cookies:** JSON (stdlib). No YAML/TOML lib.
- **Tests:** stdlib `testing`, `net/http/httptest`
- **No uTLS in MVP** (Bonpreu uses AWS WAF, not Akamai). Add only if challenged.

## Commands

```sh
# Build & install
go build -o bin/bonpreu ./cmd/bonpreu
go install ./cmd/bonpreu

# Test & lint
go test ./...
go test -race -cover ./...
go vet ./...

# Run (dev)
go run ./cmd/bonpreu <command> [flags]
```

**Global flags:** `--json` (machine output to stdout), `--config <path>` (default `~/.bonpreu/config.json`), `--max <eur>` / `BONPREU_MAX_EUR` (spending guard), `-v/--verbose` (logs to stderr).

**Command surface (MVP):**

```
import-har --file <path>            # parse HAR → cookies + csrf + region/dest defaults; runs whoami
whoami                             # verify session (GET carts/active or addresses)

search <query> [--limit N]         # GET product-pages/search → products (uuid+retailerId+price)
product <id>                       # PUT v6/products (batch of 1); id = uuid, or resolve retailerId→uuid
categories [--depth N]             # GET categories
related <retailerId>               # GET products/related

cart get                           # GET carts/active
cart add    <id> [qty] [--max]     # apply-quantity {qty:+N}      (qty default 1)
cart remove <id> [qty] [--max]     # apply-quantity {qty:-N}
cart set    <id> <qty> [--max]     # GET cart, compute signed delta, apply-quantity
cart clear  [--max]                # remove every line
cart add-many -f - [--max]         # stdin JSON lines: {"id":..,"qty":..}; price-then-write

delivery addresses [--method home|cc]   # GET delivery-addresses?deliveryMethod=
slots [--group home|cc] [--days N]      # POST ecomslots/v2/slots (grid → flattened list)
slots reserve <slotId>                  # POST ecomslots/v1/slots/reservation

orders list [--limit N]            # GET order/v6/orders (read-only)
orders show <orderId>              # order detail (confirm exact endpoint in impl)

checkout open                      # open default browser at /checkout (handoff for 3DS)
```

`<id>` accepts a UUID (used directly) or a numeric `retailerProductId` (resolved to UUID — see Open Questions).

## Project Structure

```
cmd/bonpreu/main.go            # cobra root + wiring; thin
internal/
  client/                      # HTTP client + cookiejar + CSRF/tracking middleware + HAR parser
  config/                      # ~/.bonpreu/ load/store (0600), config.json + cookies.json
  api/                         # typed endpoints: search, product, cart, slots, delivery, orders
  cli/                         # cobra commands + json/text formatters + spending guard
docs/                          # api-discovery, reference-mercadona-clis, this SPEC
testdata/                      # fixtures (recorded JSON responses, sample HAR)
```

## Code Style

Idiomatic Go, `gofmt`/`goimports`, no comments unless asked. Table-driven tests. Errors wrap with `fmt.Errorf("...: %w", err)`; never `panic` in libraries. Data/structured output → stdout, diagnostics → stderr, non-zero exit on error.

```go
// internal/client/client.go
type Client struct {
    http    *http.Client
    cfg     *config.Session
    baseURL string
}

// ApplyQuantity performs a signed-delta cart mutation and returns the full cart.
func (c *Client) ApplyQuantity(ctx context.Context, items []CartItem) (*Cart, error) {
    body, err := json.Marshal(items)
    if err != nil {
        return nil, fmt.Errorf("encode cart items: %w", err)
    }
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
        c.baseURL+"/api/cart/v1/carts/active/apply-quantity?cartProductSorting=CATEGORIES", bytes.NewReader(body))
    var cart Cart
    if err := c.doJSON(req, &cart); err != nil {
        return nil, err
    }
    return &cart, nil
}
```

## Testing Strategy

- **Unit (table-driven):** HAR parser, config load/store, JSON→struct unmarshalling, formatters, retailerId↔UUID resolver, spending-guard math. Fixtures in `testdata/`.
- **Client (httptest):** spin up `httptest.Server`, assert request path/method/headers (csrf, client-route-id, ecom-request-source, cookie) + body (signed delta), return canned responses captured from the live site.
- **Manual (live account):** `import-har` → `whoami`, then a full read→cart→slots walk. Recorded request/response pairs live in `testdata/` (sanitized: no PII/cookies).
- **Coverage target:** `internal/client`, `internal/api`, `internal/config` ≥ 80%.
- Write endpoints are **never** exercised in automated tests — only against `httptest` mocks.

## Boundaries

- **Always do:** run `go test ./...` + `go vet` before considering work done; write `~/.bonpreu/*` with `0600`; re-read the cart total before any mutation for the spending guard; print **no** secrets/cookies to stdout; `--max` fails closed (refuses if it cannot read the total).
- **Ask first:** add an external dependency; change the on-disk `config.json`/`cookies.json` schema; touch a live write endpoint outside a manual run.
- **Never do:** commit a HAR, cookies, tokens, or PII; implement order placement / payment; disable or bypass the spending guard silently; call endpoints above a human rate.

## Success Criteria

Manual verification against the live test account (cookies imported via HAR):

1. `import-har` + `whoami` returns the account name/cart total without error.
2. `search iogurt --json` returns ≥1 product with `{productId(uuid), retailerProductId, price}`.
3. `cart add <uuid> 2` → `cart get` shows that line at qty 2; `cart remove <uuid> 1` → qty 1; `cart set <uuid> 0` removes it. Net cart restored.
4. `slots --group home` and `slots --group cc` both return available slots; `slots reserve <id>` succeeds (or returns a clear error).
5. `delivery addresses --method home|cc` lists the saved address / pickup point.
6. `orders list` returns prior orders read-only.
7. `--max 5` refuses a `cart add` whose total would exceed 5€, exit 1.
8. `checkout open` opens the browser at `/checkout`.
9. Every command supports `--json` emitting valid JSON to stdout with diagnostics on stderr.

## Open Questions

1. ~~**retailerId → UUID resolution.**~~ **RESOLVED 2026-06-30.** No JSON API lookup exists (`product-pages?retailerProductId=` ignores the param). The product page HTML embeds `window.__QUERY_INITIAL_STATE__={queries[].state.data.product.{productId,retailerProductId}}` (pure JSON). Resolution strategy: `<id>` = UUID used directly; numeric retailerId resolved via (a) local cache populated by `search`, fallback (b) scrape `__QUERY_INITIAL_STATE__` from `GET /products/<any>/<retailerId>`.
2. ~~**`orders show` endpoint.**~~ **RESOLVED 2026-06-30, shape corrected 2026-07-01 via live order.** `GET /api/order/v6/orders/<orderId>/decorated` returns a normalizr payload where `result` is the **root order-id string** (not a line-item array). Denormalize via: `entities.order[result]` carries the order metadata — `status`, totals under `orderTotals.totalPrice`/`finalPrice`, and line items under `items[]` (`{product: <uuid>, quantity}`, no inline price). `entities.product[<uuid>]` holds each product; its price is nested at `price.current` (not a flat `{amount,currency}`). Per-line price is taken from the product's unit `price.current`.
3. **`ecom-request-source-version` drift** — it's a build-hash string (`2.0.0-<date>-<commit>`); decide whether to pin (from HAR) or re-derive if the server rejects it (cf. ivorpad's Algolia app-id self-refresh).
4. ~~**WAF/cookie expiry UX**~~ **RESOLVED 2026-07-01.** `client.HTTPError` treats `401`/`403` as expiry (`Expired()`), and its `Error()` replaces the opaque upstream body with an actionable "re-run `bonpreu import-har --file <fresh.har>`" instruction; the raw body stays on `Body` for verbose diagnostics.
5. **Region default** — pin the primary address's `resolvedRegionId` from HAR, or make `--region` explicit for search/slots.

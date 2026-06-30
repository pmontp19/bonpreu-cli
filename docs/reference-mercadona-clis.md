# Reference CLIs — Mercadona (reuse analysis)

Unofficial Mercadona CLIs studied while designing `bonpreu-cli`. Mercadona's API differs from Bonpreu's (Bearer JWT + durable refresh token vs Bonpreu's OIDC cookie session), but several patterns are directly reusable.

---

## 1. `ivorpad/mercadona-cli` — Go, 266★ (the mature one)

Repo: https://github.com/ivorpad/mercadona-cli · single static Go binary, agent-friendly (`--json`), no runtime deps.

### Architecture (mirrors what we want)
```
cmd/mercadona/          # entrypoint
internal/
  client/               # HTTP client + transport + HAR parser + per-domain helpers
    client.go           # core client (DoJSON, BaseURL, CustomerID)
    transport.go        # uTLS fingerprint + header injection + retry
    auth.go             # token refresh on 401
    har.go              # HAR → refresh_token/cookie extraction
    algolia.go          # search (self-refreshing app-id discovery)
    cart.go  checkout.go  catalog.go  postal.go
  config/               # ~/.mercadona/ config.toml (0600) + token.json
```
Only two internal packages (`client`, `config`) — deliberately flat. Helpers are files, not subpackages.

### Auth pattern (the interesting part)
- **Bearer SimpleJWT + durable `refresh_token`.** First login is browser-only (reCAPTCHA Enterprise / Google OAuth).
- After first login: `POST /api/auth/tokens/ {refresh_token}` **renews headless forever** (no captcha). Every `401 token_not_valid` → auto-refresh + retry.
- **Three import methods** (preference order):
  1. `import-har --file X.har` — parses HAR, extracts refresh_token + cookie + customer_id from auth responses and Bearer/Cookie headers (never reads the password body).
  2. `import-curl --file s.txt` — from DevTools "Copy as cURL" (no refresh, can't renew).
  3. `set-refresh <token>` — manual write.
- Secrets in `~/.mercadona/config.toml` (perms `0600`); cached session in `token.json`. Env vars (`MERCADONA_TOKEN`, etc.) override for one-off runs.

### Other reusable ideas
- **Spending guard** `--max` / `MERCADONA_MAX_EUR`: any cart/checkout over the cap is refused, exit 1. `checkout submit` **fails closed** (refuses if it can't read the total). Precedence flag > env > config.
- **Web-like headers + uTLS fingerprint** to stay in Akamai monitor mode.
- **Self-refreshing search credentials**: Algolia app-id rotates, so the CLI re-discovers it from the live SPA bundle on stale-creds signal. → Analogous to our need to re-derive `ecom-request-source-version` if it changes.
- **Data to stdout, logs/errors to stderr, exit 1 on error** — script/agent friendly.
- **`batch -f -`**: many search terms in one request; `cart set-many -f -`: many `<id> <qty>` in one write, priced first so `--max` refuses before writing.
- **Recipes** in the README (price a plain-word list, sort category by €/kg, find real offers via `price_decreased`) — good `--json` UX templates.
- **Distribution**: npm wrapper that downloads the prebuilt binary on install; GoReleaser cross-compile; OIDC trusted npm publishing.

### Checkout flow (for contrast with Bonpreu)
4 REST calls under `/api/customers/<cid>/...`: `POST checkouts/` → `PUT checkouts/<id>/delivery-info/` → `GET checkouts/<id>/` (read total for guard) → `POST checkouts/<id>/orders/` (**empty body → order placed**). **No payment step at all** — Mercadona charges the saved card server-side (merchant-initiated, no 3DS in loop). This is exactly why Bonpreu's checkout is harder (Braintree 3DS challenge in the loop) and why we stop at cart+slot.

### Reuse verdict for bonpreu-cli
| Take | Adaptation needed |
|---|---|
| Flat `client` + `config` layout | ✅ as-is |
| `import-har` parser (HAR → cookies+csrf+ids) | ✅ reuse logic; Bonpreu has no refresh_token, so extract cookies + `x-csrf-token` + `regionId`/`deliveryDestinationId` instead |
| Spending guard `--max` (fail-closed) | ✅ as-is |
| stdout/stderr/exit-code contract, `--json` | ✅ as-is |
| Config `0600`, env override, flag>env>config precedence | ✅ as-is |
| Auto-refresh on 401 | ❌ N/A — Bonpreu has no refresh; detect expired session → instruct re-import |
| uTLS | ⏸ defer — Bonpreu uses AWS WAF not Akamai; add only if challenged |
| `transport.go` header injection | ✅ reuse; inject csrf/client-route-id/page-view-id/ecom-request-source(-version) |
| npm/GoReleaser distribution | ✅ later |

---

## 2. `juanrubio/mercadona-cli` — TypeScript/Node, 5★ (the simpler one)

Repo: https://github.com/juanrubio/mercadona-cli · ISC.

### Pattern
- **Manual JWT copy** from browser `localStorage` (`MO-user.accessToken`). **No refresh** — re-import when the ~6-week token expires.
- **Multi-profile** (`-p <profile>`, `profile list/use`, `setup --name --email --postal-code --warehouse --token`).
- State in `~/.mercadona/` (treat as sensitive).
- Commands: search, categories, cart (add/remove/clear/add-from-list/add-from-order), **orders** (inspect/draft editing/diff/repeat/replace/ensure-minimum/slot/summary/export/save), lists, addresses, slots, checkout (create/slot/confirm/status).

### Reuse verdict for bonpreu-cli
| Take | Adaptation |
|---|---|
| Multi-profile idea | ⏸ optional later (Bonpreu: one account/region per profile) |
| Rich order-history commands (repeat/replace/draft) | ✅ good v2 source; Bonpreu has `GET /api/order/v6/orders` |
| `order ... summary --whatsapp` formatter | ✅ nice UX, cheap to add |
| TypeScript | ❌ we chose Go |

---

## Summary — what to steal
1. **ivorpad's structure & contracts**: flat `client`/`config`, HAR import, `--max` fail-closed guard, stdout/stderr/exit, `--json`, `0600` config, flag>env>config.
2. **juanrubio's order UX**: repeat/replace/summary as a v2 surface.
3. **Neither** solves Bonpreu's two specific traits: cookie-session (no refresh) and 3DS checkout (out of scope).

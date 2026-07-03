# bonpreu-cli

Unofficial, agent-friendly CLI for `compraonline.bonpreuesclat.cat` — search the catalog, build a cart, pick a delivery slot. Single static Go binary, `--json` output, no runtime deps.

> **Unofficial.** Bonpreu/Esclat has no public API. Bring your own credentials (HAR import); use at a sane request rate. This talks to the same HTTP endpoints the website does. "Bonpreu" and "Esclat" are trademarks referenced only to describe compatibility.

## Status

MVP complete: **Read + Cart + Slots/Shipping + read-only orders + `checkout open` handoff**. Order placement (Braintree 3DS) is deliberately out of scope — the CLI leaves a fully-configured cart and the user completes checkout in the web/app.

API surface is reverse-engineered and documented in [`docs/bonpreu-api-discovery.md`](docs/bonpreu-api-discovery.md); reusable patterns from similar projects in [`docs/reference-mercadona-clis.md`](docs/reference-mercadona-clis.md).

## Quickstart

```sh
# 1. Install (requires Go; or `go build -o bin/bonpreu ./cmd/bonpreu` from a clone).
#    Installs to $(go env GOPATH)/bin — make sure that's on your PATH.
go install github.com/pmontp19/bonpreu-cli/cmd/bonpreu@latest

# 2. Log in at compraonline.bonpreuesclat.cat in a browser, then capture the
#    session (writes ~/.bonpreu/*, 0600). Recommended: devtools → right-click an
#    authenticated /api/… request → Copy → "Copy as cURL", then:
pbpaste | bonpreu import-curl -
bonpreu whoami                          # verify account auth

# 3. Shop
bonpreu search iogurt --json
bonpreu cart add <id> 2                 # <id> = UUID or numeric retailerProductId
bonpreu cart get
bonpreu slots --group home
bonpreu slots reserve <slotId>
bonpreu checkout open                   # finish 3DS in the browser
```

Every command accepts `--json` for machine-readable output on stdout (diagnostics go to stderr).
Guard spending with `--max <eur>` (or `BONPREU_MAX_EUR`, or `default_max_eur` in config.json): cart
mutations that would push the projected total over the cap are refused, and it fails closed if the
total (or the config) can't be read.

State lives under `~/.bonpreu/` (override with `BONPREU_HOME`): `cookies.json` (session, 0600),
`config.json` (spending cap + `delivery use` defaults; override its path with `--config <path>`),
`cache.json` (id lookups).

## Commands

`<id>` accepts either a UUID or a numeric `retailerProductId`; the CLI resolves and caches the mapping.

**Session**
```sh
bonpreu import-curl -                 # import from a devtools "Copy as cURL" (recommended; stdin or --file)
bonpreu import-har --file login.har   # parse a HAR export, save cookies+csrf (0600) — needs an unsanitized HAR
bonpreu whoami                        # verify account auth + print cart summary
bonpreu loyalty                       # Guardiola (loyalty wallet) balance
```

**Catalog**
```sh
bonpreu search <query> [-l/--limit 30]   # search, with retailerId + uuid + price
bonpreu product <id>                     # a single product
bonpreu related <retailerId>             # related product uuids
bonpreu categories [-d/--depth 4]        # category tree
```

**Cart**
```sh
bonpreu cart get
bonpreu cart add <id> [qty=1]
bonpreu cart remove <id> [qty=1]
bonpreu cart set <id> <qty>
bonpreu cart add-many [-f file|-]        # JSON-lines: {"id":..,"qty":..} per line
bonpreu cart clear
bonpreu cart voucher <code> [<code>...]  # apply discount/voucher code(s)
bonpreu favorites list                   # starred products ("Preferits")
bonpreu regulars list                    # frequently-bought products ("Productes recurrents")
bonpreu regulars fill                    # auto-fill the cart from purchase history ("Compra ràpida")
```

**Delivery**
```sh
bonpreu delivery addresses [-m/--method home|cc] [--postal <prefix>]   # addresses or pickup points
bonpreu delivery use <destinationId> [-g/--group home|cc]
bonpreu slots [-g/--group home|cc] [-d/--days 7] [--destination <id>]
bonpreu slots reserve <slotId> [-g/--group home|cc] [--destination <id>]
```

**Checkout & orders**
```sh
bonpreu checkout open        # opens the browser at /checkout to finish 3DS
bonpreu orders list [-n/--limit 0]
bonpreu orders show <orderId>
bonpreu wallet list          # saved payment methods (read-only)
```

`bonpreu <command> --help` shows flags for any command; `bonpreu completion <shell>` generates a
shell-completion script (bash/zsh/fish/powershell — a stock Cobra feature, no custom values).

## Auth model

OIDC cookie session (no client-side refresh token). Log in once in a browser, then capture the session — `bonpreu import-curl` (recommended) or `bonpreu import-har` — to extract the session cookies + CSRF token. Re-import when the session expires.

The account session rides on the `global_sid` cookie, which is a **session cookie with no `Max-Age`/`Expires`** — its lifetime is decided server-side (OIDC session TTL, on the order of hours/days), so it cannot be predicted from the cookie itself. The long-lived `VISITORID` cookie (`Max-Age` ≈ 91 days) is only a visitor/tracking id: it keeps *anonymous* access alive (search, guest cart) long after the login session has lapsed, so a working cart is **not** proof you are still logged in. Use `bonpreu whoami`, which verifies account-level auth (the homepage `session.isLoggedIn` flag) rather than just guest-cart access, and re-import when it reports the session is anonymous/expired.

**Capturing a session — prefer `import-curl`.** Recent Chrome (~v118+) **sanitizes HAR exports**, stripping the `Cookie` and `x-csrf-token` headers, so a default HAR makes `import-har` fail with "no session cookies found". The reliable one-click path is to copy a single authenticated request as cURL — devtools Network panel → right-click any `/api/…` request made while logged in → Copy → **Copy as cURL** — and pipe it in:

```sh
pbpaste | bonpreu import-curl -        # or: bonpreu import-curl --file req.txt
```

"Copy as cURL" is not sanitized, so it carries the cookies and CSRF token. `import-har` still works if you enable DevTools' "Allow to generate HAR with sensitive data".

The CSRF token rotates server-side; the CLI refreshes it automatically from the homepage when a mutation is rejected, so a stale token no longer forces a re-import — only an expired *account* session does.

## Layout

```
cmd/bonpreu/        # entrypoint
internal/
  client/           # HTTP client + cookiejar + CSRF middleware + HAR parser
  config/           # ~/.bonpreu/ (0600) config + cookies.json
  api/              # typed endpoints (search, product, cart, slots, delivery)
  cli/              # flag parsing, json/text formatters, spending guard
docs/               # API discovery + reference CLIs
```

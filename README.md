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

# 2. Log in at compraonline.bonpreuesclat.cat in a browser, export a HAR,
#    then import the session (writes ~/.bonpreu/*, 0600):
bonpreu import-har --file login.har
bonpreu whoami                          # verify the session

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
bonpreu import-har --file login.har   # parse a HAR export, save cookies+csrf (0600)
bonpreu whoami                        # verify the session, print cart summary
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
```

`bonpreu <command> --help` shows flags for any command; `bonpreu completion <shell>` generates a
shell-completion script (bash/zsh/fish/powershell — a stock Cobra feature, no custom values).

## Auth model

OIDC cookie session (no client-side refresh token). Login once in a browser, export a HAR, `bonpreu import-har` extracts the session cookies + CSRF token. Re-import when the session (~3 months, verified against the real `VISITORID` cookie's `Max-Age`) expires.

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

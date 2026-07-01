# bonpreu-cli

Unofficial, agent-friendly CLI for `compraonline.bonpreuesclat.cat` — search the catalog, build a cart, pick a delivery slot. Single static Go binary, `--json` output, no runtime deps.

> **Unofficial.** Bonpreu/Esclat has no public API. Bring your own credentials (HAR import); use at a sane request rate. This talks to the same HTTP endpoints the website does. "Bonpreu" and "Esclat" are trademarks referenced only to describe compatibility.

## Status

MVP complete: **Read + Cart + Slots/Shipping + read-only orders + `checkout open` handoff**. Order placement (Braintree 3DS) is deliberately out of scope — the CLI leaves a fully-configured cart and the user completes checkout in the web/app.

API surface is reverse-engineered and documented in [`docs/bonpreu-api-discovery.md`](docs/bonpreu-api-discovery.md); reusable patterns from similar projects in [`docs/reference-mercadona-clis.md`](docs/reference-mercadona-clis.md).

## Quickstart

```sh
# 1. Build
go build -o bin/bonpreu ./cmd/bonpreu

# 2. Log in at compraonline.bonpreuesclat.cat in a browser, export a HAR,
#    then import the session (writes ~/.bonpreu/*, 0600):
bin/bonpreu import-har --file login.har
bin/bonpreu whoami                      # verify the session

# 3. Shop
bin/bonpreu search iogurt --json
bin/bonpreu cart add <id> 2             # <id> = UUID or numeric retailerProductId
bin/bonpreu cart get
bin/bonpreu slots --group home
bin/bonpreu slots reserve <slotId>
bin/bonpreu checkout open               # finish 3DS in the browser
```

Every command accepts `--json` for machine-readable output on stdout (diagnostics go to stderr).
Guard spending with `--max <eur>` (or `BONPREU_MAX_EUR`, or `default_max_eur` in config.json): cart
mutations that would push the projected total over the cap are refused, and it fails closed if the
total (or the config) can't be read.

State lives under `~/.bonpreu/` (override with `BONPREU_HOME`): `cookies.json` (session, 0600),
`config.json` (spending cap; override its path with `--config <path>`), `cache.json` (id lookups).

## Auth model

OIDC cookie session (no client-side refresh token). Login once in a browser, export a HAR, `bonpreu import-har` extracts the session cookies + CSRF token. Re-import when the session (~3 months) expires.

## Build

```sh
go build -o bin/bonpreu ./cmd/bonpreu
```

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

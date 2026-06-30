# bonpreu-cli

Unofficial, agent-friendly CLI for `compraonline.bonpreuesclat.cat` — search the catalog, build a cart, pick a delivery slot. Single static Go binary, `--json` output, no runtime deps.

> **Unofficial.** Bonpreu/Esclat has no public API. Bring your own credentials (HAR import); use at a sane request rate. This talks to the same HTTP endpoints the website does. "Bonpreu" and "Esclat" are trademarks referenced only to describe compatibility.

## Status

**Pre-spec.** API surface is reverse-engineered and documented in [`docs/bonpreu-api-discovery.md`](docs/bonpreu-api-discovery.md); reusable patterns from similar projects in [`docs/reference-mercadona-clis.md`](docs/reference-mercadona-clis.md).

Scope (MVP): **Read + Cart + Slots/Shipping**. Order placement (Braintree 3DS) is deliberately out of scope — the CLI leaves a fully-configured cart and the user completes checkout in the web/app.

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

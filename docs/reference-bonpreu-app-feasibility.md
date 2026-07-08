# Bonpreu Android app — feasibility findings (APK decompile + live MITM capture)

Decompiled `com.bonpreu.mobile.android` v0.461.0 (`.xapk`, source: APKPure) with `jadx`, then installed it on a rooted Android emulator (system-trusted mitmproxy CA already provisioned on the AVD from an earlier Android app RE session) and captured live traffic for a real, logged-in account. Session dates: 2026-07-07 (static decompile) and 2026-07-08 (live capture). This follows a standard rooted-emulator + system-trusted-CA MITM playbook: decompile with `jadx`, grep the Retrofit route annotations for a static route table, then confirm hosts/auth/bodies against live traffic.

**Verdict: the mobile app talks to a completely separate backend gateway from the website, with its own auth scheme that includes a refresh token — something the web flow doesn't have.** Confirmed live: host, full header set, the auth flow (including `refreshToken`), and ~40 endpoints covering loyalty, order history, shopping lists, wallet, and the checkout-summary/payment-method screen. Deliberately **not** confirmed: `v2/checkout`, `complete3ds`, `payment/complete` — the live session stopped at the checkout-summary screen and never submitted a real payment, so the actual order-creation call is still unconfirmed (see §6).

---

## 1. What the APK actually is

`manifest.json` inside the `.xapk`: `package_name: com.bonpreu.mobile.android`, `name: "BonpreuEsclat Online"`, `version_name: v0.461.0`, `min_sdk_version: 29`, `target_sdk_version: 36`. Signature check (`apksigner verify --print-certs`) shows a `CN=Android, OU=Android, O=Google Inc.` signer plus a Play Store Source Stamp block — consistent with a Play App Signing build (what an APKPure mirror of a real Play listing looks like), not a from-scratch repackage. Permissions and decompiled code are unremarkable for a grocery app — nothing suggesting tampering.

**Bonpreu's app is a white-labeled build of Ocado's own consumer app.** Every class lives under `com.ocado.mobile.android.*`. Ocado Smart Platform (OSP) licenses its full e-commerce stack to grocers worldwide (Kroger, Sobeys, Coles, ICA, Casino/Monoprix, Aeon — Bonpreu/Esclat evidently too). This explains naming `bonpreu-api-discovery.md` already had without knowing why (`ecomslots`, `checkoutwalk`, `webproductpagews`, `itemGroups`/`basketUpdateResult`) — that's Ocado's own platform vocabulary, reused per tenant, not Bonpreu-specific design. Live capture added the concrete tenant identifiers: the mobile gateway host is `api.bpe.osp.tech` (`bpe` = Bon Preu Esclat) and every request carries `bannerid: dcbcfd72-cf23-44a2-8e14-8a38edd645a3` — Bonpreu's OSP tenant ID. Device registration (`PUT /v1/mobileDevice/{id}`, see §3) returned a pointer to `api-euw1-mobilecustomerrocketws.bpe.osp.tech`, confirming the AWS `eu-west-1` region and a `rocket`-branded internal service naming that matches the `sit.cymesfood.osp.tech` staging host found in the static pass.

## 2. Method

```
unzip Compra+online_v0.461.0_APKPure.xapk                        # base APK + 2 config splits
jadx -d jadx-out --no-res -j4 com.bonpreu.mobile.android.apk      # 39,668 .java files
grep -rlE '@(GET|POST|PUT|DELETE|PATCH)\("' com/ocado             # 72 Retrofit *RestClient interfaces, ~180 relative paths

# live half, reusing an already-provisioned rooted AVD + system-trusted mitm CA:
adb install-multiple com.bonpreu.mobile.android.apk config.arm64_v8a.apk config.xxxhdpi.apk
mitmdump -p 8888 -w bonpreu.flow                                  # emulator's WiFi is statically configured to proxy through 10.0.2.2:8888
# app launched, user logged in manually (real account, real password — not typed by the agent),
# then: search, product detail, add-to-list, favorites, order history, and a manual walk to the
# checkout-summary/payment-method screen — stopping before "Finalitza la compra"
```

There's no custom route-table annotation here — Ocado's app uses plain `retrofit2.http` annotations directly on Kotlin interfaces named `*RestClient`, one per feature module. The static pass could recover relative paths + DTO class names but **not** the base host/auth scheme (Dagger qualifier annotations are stripped by R8 at build time) — that gap is now closed by §3.

## 3. Confirmed live: host, headers, and the auth flow

**Base URL:** `https://api.bpe.osp.tech/rocket-osp/{relative-path-from-the-RestClient}` — a single gateway + prefix for every module. (The static-analysis pass had guessed per-module host prefixes by package name; that guess was wrong — see the wallet cross-check in §5. The real answer is simpler: one host, one prefix, and the `v1`/`v2`/`v3`/`v4` version is just part of the relative path, exactly as jadx showed it.)

**Every mobile-API request carries this header set** (confirmed across dozens of live calls):
```
accept: application/json,*/*
accept-currency: EUR
accept-language: ca-ES
accept-encoding: gzip
analytics-session-id: <uuid, one per app session>
authorization: token:<opaque base64url blob>          — NOT "Bearer <jwt>", a custom scheme
bannerid: dcbcfd72-cf23-44a2-8e14-8a38edd645a3          — Bonpreu's OSP tenant ID (static, same for every install)
client-features: image-http-redirects
ecom-request-source: android
ecom-request-source-version: v0.461.0 (29651442)
experimentationuserid: <uuid>
sessionsequenceno: <incrementing integer, one per request in the app session>
user-agent: BonPreu-Android-Application/v0.461.0 (Android/12) sdk_gphone64_arm64
x-api-key: su95KBXYOL67yMpPxwNH8Eu4iGLk4TT235I5P8S7      — static, baked into the APK, same for every install
```
The `authorization` token is **not a JWT** (no `.`-separated header/payload/signature) — it's an opaque blob, most likely an encrypted/opaque server-side session reference rather than a self-contained token. `checkout-summary` additionally sends `analytics-source-id`; some pre-login calls (device registration) omit `accept-currency`/`accept-language`/etc. entirely.

**Auth flow, confirmed end-to-end:**
1. `GET /rocket-osp/v1/authorize/uris` → `{"authenticationUri": "https://www.compraonline.bonpreuesclat.cat/authorize?response_type=code&client_id=mobile&state=<uuid>&language=ca-ES", "reauthenticationUri": "...&login_prompt=true...", "registrationUri": "https://www.compraonline.bonpreuesclat.cat/register?language=ca-ES", "state": "<uuid>"}`. This is the same OIDC authorize endpoint the **web** flow uses, just with `client_id=mobile` instead of the web's client id — the mobile app opens this in a browser/Custom Tab for the user to log in.
2. After login, the server redirects to a custom URI scheme: `bonpreu-atm://login?code=<authorizationCode>` (device-side deep link, never leaves the device).
3. `POST /rocket-osp/v1/authorize` body `{"authorizationCode": "<code>", "redirectUri": "bonpreu-atm://login"}` → **`{"token": "<opaque>", "refreshToken": "<opaque>"}`**.

**This is the answer to `bonpreu-api-discovery.md`'s "Token refresh mechanism" pending item.** The web flow genuinely has no client-side refresh (cookies only, confirmed there). The **mobile** flow issues a real `refreshToken` alongside the session `token` — a `POST /rocket-osp/v1/authorize/refresh` route exists per the static analysis (§2/old §4.3) but didn't fire during this session (the token never got old enough to need it), so its request/response shape is still unconfirmed. If bonpreu-cli ever adopts this mobile auth path, it would need one initial browser-based login (the same browser-then-paste-the-code pattern `gh`/Claude Code/the Codex CLI use) but could then refresh indefinitely instead of requiring repeated `import-curl`.

**Device registration** (fires once per fresh install, before login): `GET /v1/mobileDevice/{deviceId}` → `404 {"reason": "DEVICE_NOT_REGISTERED"}` → `PUT /v1/mobileDevice/{deviceId}` (30-byte binary body, not JSON) → `201` with the body `"https://api-euw1-mobilecustomerrocketws.bpe.osp.tech/v1/mobileDevice/{deviceId}"` → subsequent `GET /v1/mobileDevice/{deviceId}` now returns `200 {"token": "<opaque>"}` — a **third**, device-bound token distinct from the user's `token`/`refreshToken` pair. Its purpose wasn't investigated further this session (not blocking for anything on the CLI's roadmap).

## 4. Confirmed live endpoints (`api.bpe.osp.tech/rocket-osp/...`)

Everything below actually fired during the session and returned a real response (200/201 unless noted). Grouped by area; full raw dump lives in the session's scratchpad, not the repo.

**Catalog / home / device:**
`GET v1/adverts`, `GET v1/pop-up`, `PUT v1/adverts/batch-query/{homepage|search}`, `GET v1/category/categoryList`, `GET v4/products/search`, `GET v4/products/search/redirects`, `GET v3/products/search/suggestions/primary`, `GET v2/products/{retailerProductId}/bop`, `GET v3/products/bop/{retailerProductId}/relatedFeatured`, `PUT v1/products` (batch enrichment, same idea as the web's `PUT /api/webproductpagews/v6/products`), `GET v1/products/featured`, `GET v2/componentised-area/{loggedinhomepage|loggedouthomepage}`, `GET v1/tabs`, `GET v1/recipes/components/home`, `GET v1/customer-segments/short-code-lists`, `GET v1/customer-preferences`, `GET v1/fop-configuration`, `GET v1/communications-block`, `GET v1/content-experience-user-id`, `GET/PUT v1/mobileDevice/{deviceId}[/config|/features]`.

**Account:** `GET v1/user/current` → `{firstName, username, fullName, retailerCustomerId, instantShopAvailable, externalCustomerIds: {salesforceId}}`. `GET v1/user/current/details/form`. `GET v1/user/loyalty` → `{"balance": {"display": "MONEY", "units": 0, "money": {"amount": "X.XX", "currency": "EUR"}}, "registered": true, "schemeUrl": "https://www.bonpreuesclat.cat/group/bonpreu/espai-privat"}` — a **clean JSON loyalty endpoint**, unlike the web CLI's current HTML-scrape of `__INITIAL_STATE__` on `/settings/loyalty`. `GET v1/user/subscriptions/delivery/active` (BPAS) → `{"messages": {"advertisingMessage", "marketingMessage", "isFreeTrial"}, "allSubscriptionsLink": "https://www.compraonline.bonpreuesclat.cat/delivery-pass/subscription", "customerHasAddress"}`.

**Orders:** `GET v3/orders`, `GET v3/orders/paginated`, `GET v3/orders/not-cancelled-count`, `GET v2/orders/recent`, `GET v1/orders/failed-payments`, `GET v1/orders/{orderId}/delivery-options`.

**Cart:** `GET v1/carts/active` → full cart shape confirmed: `{cartId, region: {regionId, retailerRegionId, retailerRegionName}, deliveryDestinationId, isCheckedOut, isOrderEdit, defaultCheckoutGroup/activeCheckoutGroup: {orderId, checkoutRestrictions, minimumCheckoutThreshold, remainingAmountToThreshold, taxation, shippingGroupType, isEditEligible}, totals: {savingsPrice, itemPriceAfterPromos, itemsRetailPrice}, items: [{productId, retailerProductId, quantity, quantityRestrictionGroup: {quantityRestrictionGroupId, contribution}, basketLines}], lastModified, checkoutCorrelationId, orderEditExpired}`. Note `retailerProductId` is present and — per spot-check — is the **same ID scheme** the web already uses, so no extra ID-mapping layer is needed between web and mobile. Also: `GET v1/carts/active/cart-view`, `GET v2/carts/active/cart-view` (not diffed against v1 this session).

**Shopping lists:** `GET v1/product-lists/basic?includingProduct={productId}` → `[]` for an account with no lists yet (query param `includingProduct` wasn't visible in the static pass — refines it). List-creation UI was exercised but the corresponding `POST v1/product-lists` call wasn't confirmed to fire (UI interaction issue, not an API block) — still a static-only candidate.

**Checkout / payment (up to, not including, order creation):**
- `POST v1/carts/active/checkout-start` body `{"shippingGroupType": "default customer collection"}` → `{"checkoutRestrictions": ["MISSING_SLOT"], "minimumCheckoutThreshold": {...}, "orderId", "shippingGroupType", "shippingGroupTypeDisplayName", "remainingAmountToThreshold"}`.
- `GET v1/carts/active/checkout-summary?fetchAllocatedPaymentChecks=false&fetchPaymentSelectorInfo=false` → rich payload: `cartId`, `region`, `orderId`, `cartStatus: "NEVER_CHECKED_OUT"`, a `bufferReservationMessageV2` block containing the **exact PSD2 pre-authorization copy shown on-screen** ("l'import de l'autorització bancària serà un 4% superior al total de la compra..."), and `items[]` with full per-line pricing (`itemPrice`, `catalogPrice.each/raw/unit`, `appliedPromotions`, `description`, `brand`, `size`, `isVerifiedPurchase`).
- `GET v1/carts/active/checkout-walk`, `GET v1/cow` — checkout-progress/walk state (matches `CheckoutWalkRestClient`).
- `GET v2/wallet` → `{"externalWallet": false, "walletItems": [{"walletItemId", "fundingInstrumentId", "default", "method": "CARD_TOKEN", "methodV2": "CARDS", "title": "<last 4 digits>", "type": "MasterCard", "expiryMonth", "expiryYear", "expired": true}]}` — **note this is structurally different from the web's confirmed `GET /api/walletservice/v3/wallet-items`** (field names `title`/`type` vs. the web's `details.lastFourDigits`/`details.cardType`, no `pspName`) — two independently-shaped wallet views onto presumably the same underlying vault.
- Slots: `GET v1/slot/configuration`, `GET v4/slot/next-available`, `POST v4/slot`, `GET v3/delivery-destinations/{id}/slots`, `POST v3/delivery-destinations/{id}/slots/continuation`, `POST v2/delivery/locations`.

**Deliberately not triggered — still unconfirmed:** `POST v2/checkout` (order creation), `POST v1/checkout/complete3ds`, `POST v1/payment/complete`. The live session stopped at the checkout-summary/payment-method screen (both saved cards showed as expired, "Finalitza la compra" was visible but never tapped) specifically to avoid submitting a real charge. Everything about these three routes is still exactly what §4.1 of the pre-live version of this doc said from static analysis alone — request/response DTOs known by name (`CheckoutDto`/`Complete3DSDto`/`PaymentCompleteDto`), bodies unknown.

## 5. What the static-analysis pass got wrong

The original version of this doc guessed `<Kotlin-package-derived-name>` ≈ `<API path prefix>` (e.g. `WalletRestClient` tagged `"payment"` in its Kotlin metadata → guessed `/api/payment/...`). **That guess was wrong in the specific way flagged as a caveat**, but the actual truth turned out simpler, not more complex: there's no per-module prefix at all — every RestClient's relative path hangs directly off one shared `api.bpe.osp.tech/rocket-osp/` gateway. The lesson for future static-only passes on OSP-based apps: don't try to reverse-engineer the base URL from package/metadata naming — it's not derived from that; a single live request settles it immediately.

## 6. Next steps

- **Order creation is still the one gap.** Confirming `v2/checkout`/`complete3ds`/`payment/complete` requires either (a) a live account with a *non-expired* saved card willing to complete a real small purchase, or (b) finding a way to observe the request shape without completing payment (e.g. the app may validate/build the request body client-side before sending — worth checking the jadx `CheckoutDto` Kotlin data class fields statically, which this session didn't get to). Static DTO field extraction (jadx already has the decompiled classes) is the cheaper next move before attempting another live pass.
- **`POST v1/authorize/refresh`** never fired (token didn't age out during the session). Worth confirming its shape if a future session runs long enough, or by statically reading `LoginRestClient`'s Kotlin data classes.
- **Shopping-list creation** (`POST v1/product-lists`) didn't confirm live despite the UI attempt — retry with more careful UI interaction, or just synthesize the request from the DTO shape in jadx and test directly.
- Any of the confirmed-live paths above that get implemented in the CLI need their **own auth path** (mobile `token`/`refreshToken`, not the existing cookie jar) — this is an architecture decision (a second `internal/client`-style transport), not just a docs update, since bonpreu-cli's current session model is entirely cookie-based.

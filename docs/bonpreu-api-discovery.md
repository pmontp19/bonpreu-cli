# Bonpreu Esclt API Reverse Engineering Documentation

**Last Updated:** 2026-06-30
**Status:** In Progress - Slot reservation CONFIRMED, Checkout flow mapped to 3DS page, order placement endpoint pending (likely server-side after payment)

> **2026-07-07/08:** APK decompile + a live MITM-captured session on the official Android app (see [`docs/reference-bonpreu-app-feasibility.md`](reference-bonpreu-app-feasibility.md)) confirmed the app talks to a **separate mobile gateway** (`api.bpe.osp.tech/rocket-osp/...`, distinct from every `www.compraonline.bonpreuesclat.cat/api/...` endpoint below) with its **own auth scheme that includes a refresh token** (`POST v1/authorize` → `{token, refreshToken}`) — something the web's cookie-only session doesn't have. Confirmed live: loyalty (clean JSON, no HTML scrape needed), order history, wallet (different shape than the web's), shopping-list read, and the full checkout-summary/payment-method screen. Still unconfirmed: the actual order-creation call (`v2/checkout`/`complete3ds`/`payment/complete`) — the live session deliberately stopped before submitting a real payment. Also confirmed: the app is a white-labeled **Ocado Smart Platform** build — explains the `ecomslots`/`checkoutwalk`/`webproductpagews` naming below.

---

## ⚡ 2026-06-30 Live verification (browser capture, logged-in session)

> Supersedes several earlier assumptions. All below confirmed by monitoring real requests on the live SPA.

### Auth — OIDC token exchange is SERVER-SIDE (resolves Step 5)
- `GET /sso-login?code={code}&state=web__{uuid}` → `GET /sso-login/auth?code=...&state=...` → **302 to `/`**.
- The exchange uses **PKCE**: the `sso.codeVerifier` cookie (HttpOnly) is sent to `/sso-login/auth`, the server exchanges code+verifier, and the response **sets the session cookies** (`VISITORID` authenticated, `global_sid`, `sso.loginComplete=true`) and **clears** `sso.nonce` / `sso.state` / `sso.codeVerifier`.
- **There is NO client-side access/refresh token.** Auth = cookies. → Confirms the CLI must use HAR/cookie import; no headless renewal possible.
- **Session lifetime (corrected 2026-07-03):** the login session rides on `global_sid`, whose `Set-Cookie` carries **no `Max-Age`/`Expires`** — it is a *session cookie* with a server-decided OIDC TTL (hours/days), NOT predictable from the cookie. The `VISITORID` cookie's `Max-Age=7884000` (~91 days) is a **visitor/tracking id, not the auth session**: it keeps *anonymous* access alive (search, guest cart) long after login has lapsed. Earlier notes conflated the two — do NOT treat ~3 months as the session lifetime.
- **Detecting expiry (RELIABLE signal):** a working active cart is **not** proof of login — the guest cart responds anonymously. The reliable signal is the boolean `window.__INITIAL_STATE__.session.isLoggedIn` on the homepage (server-rendered from the cookies). `bonpreu whoami` (`api.GetAccountStatus`) checks it. Verified live 2026-07-03: a logged-in session had `isLoggedIn == true` with `GET /api/order/v6/orders` → 200 and `/settings/*` pages SSR-ing full state; an expired session had `isLoggedIn == false` with `orders` → 401, `wallet`/`slots`/`delivery-addresses` → 400 "Missing customer id", and `/settings/*` pages returning a ~15 KB login-redirect stub with no `__INITIAL_STATE__`.
  - ⚠️ **Do NOT use `session.customerSession.data` as the auth signal** (an earlier revision of this doc and `GetAccountStatus` did — it was wrong). `customerSession` is a client-side-fetched Redux resource (`{isFetching,fetchError,didInvalidate,lastUpdated,data,…}`); its `data` is `null` in the SSR HTML for **both** logged-in and anonymous sessions, so keying off it reports everyone as anonymous. Only `isLoggedIn` is populated server-side.
- **Stale CSRF vs. dead session (two distinct failure modes):**
  - The `x-csrf-token` **rotates server-side**; a token captured at import time goes stale within a live session. GETs (`search`, cart read) don't validate it and keep working, but state-changing POSTs (`cart apply-quantity`, `ecomslots`) return **403**. The client now auto-recovers: on a 403 it re-fetches `/`, re-reads `session.csrf.token`, and retries once (`client.RefreshCSRF` + `doWithCSRFRetry`), persisting the fresh token via `SyncSession`. Verified live 2026-07-03.
  - A genuinely expired **account** session is the `isLoggedIn == false` case above and is only fixed by re-importing. If a 403 survives the CSRF refresh (token was already current), the original error stands so the "re-import" message surfaces.
- **HAR export is now sanitized (Chrome ≥ ~v118):** "Export HAR" **strips the `Cookie` and `x-csrf-token` request headers** (verified 2026-07-03 on an 18 MB export: 0 entries carried a `Cookie` header; `x-csrf-token` also absent; homepage `__INITIAL_STATE__` still present so CSRF is recoverable, but cookies are not). `import-har` therefore fails with "no session cookies found" on a default export. **`import-curl` is the reliable capture path**: devtools → right-click request → Copy → "Copy as cURL" carries the request verbatim (cookies + csrf) because it is a different, non-sanitized code path. Parsed by `client.ParseCurl`; the CLI then derives a missing CSRF from `/`, verifies `isLoggedIn`, and resolves the home destination/region from the account. HAR import still works if the user explicitly enables "Allow to generate HAR with sensitive data".

### Cart — single endpoint, SIGNED-DELTA quantity (corrects earlier add-items/remove-items notes)
- **Canonical mutation endpoint:** `POST /api/cart/v1/carts/active/apply-quantity?cartProductSorting=CATEGORIES`
- **Body:** array of only the changed items: `[{"productId":"<uuid>","quantity":<signed delta>,"meta":{...}}]`
  - `quantity: 1`  → add 1 unit (delta **+N**)
  - `quantity: -1` → remove 1 unit (delta **-N**)
  - Verified live: initial add sent `quantity:1`; the **+** button (1→2) also sent `quantity:1`; the **−** button (2→1) sent `quantity:-1`.
- **Response:** the FULL cart (`basketUpdateResult.itemGroups[]` with every line, prices, totals). One call = mutation + fresh state.
- ⚠️ The earlier `add-items` / `remove-items` endpoints documented below are **NOT what the SPA uses** (likely legacy/alternative). The line "add-items with quantity:1 decrements" was **wrong**.
- `GET /api/cart/v1/carts/active` works **without login** (anonymous cookies).

### Catalog / Search (public — anonymous cookies + csrf token suffice)
- **Real product search (prices + UUID + retailerProductId):** `GET /api/webproductpagews/v6/product-pages/search?q=<term>&maxPageSize=300&maxProductsToDecorate=30&tag=web` → `{productGroups[].decoratedProducts[]}`. This is the primary search endpoint and resolves the retailerId↔UUID mapping.
- **No single GET-by-id product endpoint exists.** Product detail = `PUT /api/webproductpagews/v6/products` with body `[uuid,...]` (batch, works with 1).
- **Related products:** `GET /api/webproductpagews/v5/products/related?retailerProductId=<numeric>` → array of UUIDs.
- **Category tree (NEW):** `GET /api/webproductpagews/v1/categories?decoration=false&categoryDepth=4` → nested `{categoryId(UUID), retailerCategoryId(numeric), name, childCategories}`.
- Autocomplete `suggestions/{primary,refined,follow-on}` return **only strings**, not product objects.
- Secondary: `POST /graphql` (e.g. `getRecipeAdvertsForSearchPage`).

### Delivery / Slots — v2 grid (corrects earlier v1 standardSlots)
- **Slots:** `POST /api/ecomslots/v2/slots` (NOT v1). Body:
  ```json
  {"deliveryDestinationId":"<uuid>","regionId":"<uuid>","displayConfiguration":"DELIVERY_METHOD",
   "shippingGroupType":"default customer collection" | "default home delivery",
   "numberOfDays":7,"analyticsData":{...}}
  ```
  Response: `{minimumOrderValue:{amount:"35.00"}, carriers:[{gridSlots:[{day, slots:[{slotId, slotWindow, deliveryPrice, attributes:["AVAILABLE"]}]}]}]}` — a **day grid**.
- **Addresses / Pickup points:** `GET /api/ecomdeliverydestinations/v4/delivery-addresses?deliveryMethod=HOME_DELIVERY` (or `CUSTOMER_COLLECTION`). Returns `[{deliveryDestinationId, addressId, formattedAddress, name, deliveryType:"BRANDED_VAN", deliveryMethod, resolvedRegionId, postalCode, isPrimary, propositions[]}]`.
- **Shipping group:** `GET /api/checkout-groups/v1/default-shipping-group/{CUSTOMER_COLLECTION|HOME_DELIVERY}?deliveryDestinationId=<uuid>` → `{defaultShippingGroupType}`. Type strings: `"default customer collection"` / `"default home delivery"`.
- Slot reservation stays: `POST /api/ecomslots/v1/slots/reservation`.

### Required request headers (all API calls)
`x-csrf-token` (UUID), `client-route-id` (UUID), `page-view-id` (UUID), `ecom-request-source: web`, `ecom-request-source-version: 2.0.0-<date>-<commit>`, plus web-like `User-Agent`/`Accept`.

### CSRF token source (RESOLVED 2026-06-30, critical)
- The `x-csrf-token` is **server-validated** (an invented UUID → `403`). It is **not** in cookies, localStorage, any GET/document response header, or the auth flow.
- It lives in the SSR-hydrated blob embedded in the homepage HTML: **`window.__INITIAL_STATE__.session.csrf.token`** (a pure-JSON assignment inside a `<script>`).
- The CLI must obtain it from the homepage document body (the HAR's `GET /` response, or by fetching `/` after import). `client-route-id`/`page-view-id` are NOT validated (fake UUIDs accepted). E2E-validated: a raw `apply-quantity` POST with the real csrf + signed-delta + fake route/page IDs returned `200`.

---

## Table of Contents
1. [Overview](#overview)
2. [Authentication](#authentication)
3. [API Endpoints](#api-endpoints)
4. [Request/Response Examples](#requestresponse-examples)
5. [Technical Architecture](#technical-architecture)
6. [Pending Discovery](#pending-discovery)

---

## Overview

This document tracks the reverse engineering of the Bonpreu Esclt online grocery ecommerce API (`https://www.compraonline.bonpreuesclat.cat/`).

**Key Findings:**
- Authenticated endpoints require session-based auth via OpenID Connect
- Cart operations use client-side optimistic updates
- API versioning: v4-v6 used across different services
- AWS WAF protection in place
- Region-locked API behavior (user restricted to delivery location)

**Test Account:**
- User: Pere
- Region: St. Pere de Ribes (00000000-0000-0000-0000-000000000002)
- Cart Size: 26 items (€56.33)
- Last Order: 2025-12-21 (€158.29)

---

## Authentication

### OpenID Connect Authorization Code Flow

**Servers:**
- Authorization/Login: `https://app.bonpreu.cat/openid-connect-server-webapp/`
- Callback: `https://www.compraonline.bonpreuesclat.cat/sso-login`

**Step 1: Authorization Request Redirect**

User navigates to: `https://www.compraonline.bonpreuesclat.cat/login`

Main site (302) redirects to:
```
GET https://app.bonpreu.cat/openid-connect-server-webapp/authorize?
  lang=ca-ES&
  ui_locales=ca-ES&
  response_type=code&
  client_id=28cd05c7-aa5c-4414-b384-b23e366d7875&
  scope=openid%20profile%20email&
  redirect_uri=https://www.compraonline.bonpreuesclat.cat/sso-login&
  nonce=14735f49e6caa&
  state=web__b907eea7-8806-403d-90ec-b584f7762fac
```

**Key Parameters:**
- `response_type=code`: Standard OAuth2 authorization code flow
- `client_id`: `28cd05c7-aa5c-4414-b384-b23e366d7875` (ecommerce app)
- `scope`: `openid profile email` (requested claims)
- `redirect_uri`: `https://www.compraonline.bonpreuesclat.cat/sso-login` (callback endpoint)
- `nonce`: Random value for security
- `state`: CSRF protection token (format: `web__{uuid}`)

**Step 2: OpenID Connect Server Redirect**

Authorize endpoint (302) redirects to login form:
```
Location: https://app.bonpreu.cat/openid-connect-server-webapp/login?lang=ca-ES&channel=osp
```

**Step 3: Login Form Submission**

User enters credentials and submits form via:
```
POST https://app.bonpreu.cat/openid-connect-server-webapp/login
Content-Type: application/x-www-form-urlencoded

username={email}&password={password}&_csrf={csrf-token}
```

**Step 4: Authorization Code Response** (on successful login)

Server (302) redirects to callback with authorization code:
```
Location: https://www.compraonline.bonpreuesclat.cat/sso-login?code={authorization-code}&state={state}
```

**Step 5: Token Exchange** (frontend)

Frontend exchanges authorization code for session/ID token. Likely endpoint:
```
POST /api/auth/token or similar
Content-Type: application/json

{
  "code": "{authorization-code}",
  "client_id": "28cd05c7-aa5c-4414-b384-b23e366d7875",
  "redirect_uri": "https://www.compraonline.bonpreuesclat.cat/sso-login"
}
```

**Step 6: Session Established**

Server returns session cookies (VISITORID, AWSALB, etc.), and frontend stores JWT or maintains session for authenticated requests.

### Session Cookies

| Cookie | Purpose | Duration |
|--------|---------|----------|
| `VISITORID` | Session identifier | 7,884,000 seconds (~3 months) |
| `AWSALB` | AWS load balancer | 604,800 seconds (7 days) |
| `AWSALBCORS` | CORS-enabled ALB cookie | 604,800 seconds (7 days) |
| `global_sid` | Global session ID | Session-based |
| `aws-waf-token` | AWS WAF token | Auto-refreshed |
| `language` | User language preference | Persistent |

### Required Request Headers

```
Authorization: Bearer {token}  # If token-based
x-csrf-token: {CSRF token UUID}
client-route-id: {UUID}
page-view-id: {UUID}
ecom-request-source: web
ecom-request-source-version: 2.0.0-2026-01-09-11h51m16s-{commit-hash}
```

### CSRF Protection

- CSRF token required for state-modifying operations
- Token included in response headers
- Tokens appear to be session-specific UUIDs

---

## API Endpoints

### Authenticated Endpoints

All require valid session cookies + request headers.

#### Products

##### GET `/api/webproductpagews/v5/product-pages`
**Purpose:** Fetch featured/catalog products
**Visibility:** Authenticated

**Query Parameters:**
- `limit`: number of products (default: 30)
- `offset`: pagination offset (default: 0)
- `tag`: filter tags (multiple allowed)
  - `web`: web-specific products
  - `lihp`: featured products

**Response:** Product list with pricing, images, availability

**Example Request:**
```
GET /api/webproductpagews/v5/product-pages?limit=30&offset=0&tag=web&tag=lihp
```

---

##### PUT `/api/webproductpagews/v6/products`
**Purpose:** Fetch enriched product data for items in cart
**Visibility:** Authenticated

**Request Body:** Array of product UUIDs
```json
[
  "1ebcc8d9-c7df-4f77-a814-d511f6cee3db",
  "114843d5-31b1-4f33-ad62-f000958106ee",
  "75426928-d21a-46ac-aa3b-0abadd7f18d8",
  "ad1f1cca-d679-4df1-a570-5eda1b4deaec",
  "e5d2638b-4b30-4bbf-8c40-674fdc2d1a78",
  "a602983c-9e87-4907-88d1-a96ad519191b",
  "536a31b4-b6bd-4edb-801c-808d3353e0dd",
  "b00a0810-56e7-4e4c-b28a-0ad0d8d56a53"
]
```

**Response:** Enriched product data including:
- Product name, brand, pack size
- Pricing (unit price + price per measurement)
- Current quantity in cart
- Availability status
- Images with multiple resolutions
- Icons/attributes (refrigerated, Km0, organic, etc.)
- Last purchased date

**Example Response:**
```json
{
  "products": [
    {
      "productId": "1ebcc8d9-c7df-4f77-a814-d511f6cee3db",
      "retailerProductId": "59289",
      "type": "REGULAR",
      "name": "GRANJA ARMENGOL Iogurt natural cremós Km0",
      "brand": "GRANJA ARMENGOL",
      "packSizeDescription": "0.5kg",
      "price": {
        "amount": "1.39",
        "currency": "EUR"
      },
      "unitPrice": {
        "price": {
          "amount": "2.78",
          "currency": "EUR"
        },
        "unit": "fop.price.per.kg"
      },
      "available": true,
      "quantityInBasket": 2,
      "maxQuantityReached": false,
      "favorite": {
        "lastBought": "2025-12-20T00:31:39.897Z"
      }
    }
  ]
}
```

---

#### Delivery & Checkout

##### GET `/api/ecomdeliverydestinations/v4/delivery-addresses/{addressId}`
**Purpose:** Get user's delivery address details
**Visibility:** Authenticated

**Path Parameters:**
- `addressId`: UUID of delivery address (e.g., `00000000-0000-0000-0000-000000000003`)

**Response:** Address with coordinates, delivery type, timezone

---

##### GET `/api/checkout-groups/v1/default-shipping-group/CUSTOMER_COLLECTION`
**Purpose:** Get shipping group info for collection-based delivery
**Visibility:** Authenticated

**Query Parameters:**
- `deliveryDestinationId`: UUID of delivery address

**Response:** Shipping group configuration

---

##### POST `/api/ecomslots/v1/slots`
**Purpose:** Fetch available delivery slots for a given address and region
**Visibility:** Authenticated
**Method:** POST

**Request Headers:**
- `x-csrf-token`: CSRF token (UUID)
- `client-route-id`: UUID
- `page-view-id`: UUID
- `ecom-request-source`: web
- `ecom-request-source-version`: {version}

**Request Body:**
```json
{
  "shippingGroupType": "default customer collection",
  "deliveryDestinationId": "00000000-0000-0000-0000-000000000003",
  "regionId": "00000000-0000-0000-0000-000000000002",
  "analyticsData": {
    "sessionId": "{uuid}",
    "viewingLocation": "SLOT_BOOKING_PAGE",
    "platform": "WEB",
    "pageViewId": "{uuid}"
  }
}
```

**Response:** Array of available slots with time windows and delivery prices
```json
{
  "standardSlots": [
    {
      "slotId": "b1bfd8df-8dd4-4acb-9702-e70ed915f308",
      "slotWindow": {
        "startTime": "2026-01-11T08:00:00Z",
        "endTime": "2026-01-11T09:00:00Z"
      },
      "expiryTime": "2026-01-09T21:41:04.623Z",
      "available": true,
      "deliveryPrice": {
        "currency": "EUR",
        "amount": "1.99"
      },
      "carrierId": "d6c0904d-6dc2-4ce8-8641-3117ab939315",
      "repriceable": false,
      "attributes": ["AVAILABLE"]
    }
  ]
}
```

---

##### POST `/api/ecomslots/v1/slots/reservation`
**Purpose:** Reserve/select a delivery slot for checkout
**Visibility:** Authenticated
**Method:** POST
**Status:** ✅ CONFIRMED - Discovered via network monitoring

**Request Headers:**
- `x-csrf-token`: CSRF token (UUID)
- `client-route-id`: UUID
- `page-view-id`: UUID
- `ecom-request-source`: web
- `ecom-request-source-version`: {version}

**Request Body:**
```json
{
  "regionId": "00000000-0000-0000-0000-000000000002",
  "slotId": "669e10a1-784f-47b9-870b-d82e6d74fafc",
  "deliveryDestinationId": "00000000-0000-0000-0000-000000000003"
}
```

**Response:** Full slot confirmation with reservation details
```json
{
  "slot": {
    "slotId": "669e10a1-784f-47b9-870b-d82e6d74fafc",
    "slotWindow": {
      "startTime": "2026-01-13T10:00:00Z",
      "endTime": "2026-01-13T11:00:00Z"
    },
    "type": "STANDARD",
    "expiryTime": "2026-01-10T09:01:33.296Z",
    "latestEditableTime": "2026-01-12T21:00:00Z",
    "guaranteedEditableDateTime": "2026-01-12T21:00:00Z",
    "address": {
      "addressId": "470abaae-e8d3-4aff-99b2-f24a8d46de9c",
      "nickname": "St. Pere de Ribes",
      "address": "C-246a, 4, 08812 Vilanoveta, Barcelona, Spain",
      "primary": false,
      "latitude": "41.2327195",
      "longitude": "1.7441243",
      "regionId": "00000000-0000-0000-0000-000000000002"
    },
    "deliveryMethod": "CUSTOMER_COLLECTION",
    "timeZoneId": "Europe/Madrid",
    "carrierId": "d6c0904d-6dc2-4ce8-8641-3117ab939315"
  },
  "minimumCheckoutThresholdData": {
    "minimumCheckoutThreshold": {
      "amount": "35.00",
      "currency": "EUR"
    },
    "originalMinimumCheckoutThreshold": {
      "amount": "35.00",
      "currency": "EUR"
    },
    "minimumCheckoutThresholdChanged": false,
    "taxation": "TAX_INCLUDED"
  }
}
```

**Note:** Slot reservation returns expiry windows and editable time limits. The `expiryTime` indicates when the slot reservation expires if checkout is not completed. User can edit the slot until `latestEditableTime`.

---

##### POST `/api/cart/v1/carts/active/checkout-start`
**Purpose:** Initialize checkout workflow for a cart
**Visibility:** Authenticated
**Method:** POST
**Status:** ✅ CONFIRMED - Discovered via network monitoring (reqid=2434)

**Request:** Empty JSON object `{}`
**Response:** Checkout workflow state

**Note:** Called when user clicks "Finalitza la compra" (Complete purchase) button. Triggers page navigation to checkout workflow pages (potser-et-falta → topoffers → no-et-perdis → checkout/summary).

---

##### POST `/api/ecomslots/v1/slots/extend`
**Purpose:** Extend slot expiry time during checkout
**Visibility:** Authenticated
**Method:** POST
**Status:** ✅ CONFIRMED - Discovered via network monitoring (reqid=2438)

**Note:** Called during checkout flow to extend the slot reservation expiry time, ensuring user has enough time to complete payment without slot expiring.

---

##### GET `/api/checkoutwalk/v1/checkout-walk`
**Purpose:** Get current checkout workflow state/progress
**Visibility:** Authenticated
**Method:** GET
**Status:** ✅ CONFIRMED - Discovered via network monitoring (reqid=2435)

**Note:** Called to retrieve checkout page information during the checkout flow progression.

---

#### Checkout & Orders

##### POST `/api/orders/v1/create` (Estimated)
**Purpose:** Create order after slot selection and payment
**Visibility:** Authenticated
**Method:** POST
**Status:** ❌ NOT YET IDENTIFIED - Payment flow discovered, order placement likely server-side

**Checkout Flow Discovered:**
1. User clicks "Finalitza la compra" button on `/checkout/summary` page
2. POST to `/api/cart/v1/carts/active/checkout-start` initiates checkout
3. Page displays "S'està confirmant la compra. Si us plau, espera...." (Confirming purchase...)
4. Browser navigates to `/checkout/3ds` page showing 3D Secure payment iframe
5. Payment iframe loads from `https://pspweb.compraonline.bonpreuesclat.cat/psp/` subdomain
6. Payment provider: **Braintree** (with Cardinal Commerce 3DS integration)
7. Order creation likely triggered **server-side after payment authentication completes**

**Payment Gateway Integration:**
- Endpoint: `https://pspweb.compraonline.bonpreuesclat.cat/psp/Braintree/bannerId/{bannerId}/checkout`
- Provider: Braintree (PayPal owned payment processor)
- 3DS: Cardinal Commerce device fingerprinting detected
- Token: `parameterToken` query parameter used for payment session

**Expected Request Body:**
```json
{
  "cartId": "{cartId}",
  "slotId": "{slotId}",
  "deliveryDestinationId": "{addressId}",
  "paymentMethod": "CREDIT_CARD | DEBIT_CARD | PAYPAL",
  "shippingGroup": "default customer collection"
}
```

**Expected Response:** Order confirmation with:
- Order ID
- Total amount
- Estimated delivery date/time
- Order status (PENDING, CONFIRMED, etc.)

**Alternative endpoint paths to test:**
- `/api/checkout/v1/orders`
- `/api/ecommerce/v1/place-order`
- `/api/payment/v1/complete-order`

**Next Step:** Complete payment authentication in 3DS iframe to trigger order creation and identify the exact endpoint/request structure. Order may be created:
- Via callback webhook from payment provider (async)
- Via server-side order confirmation endpoint after 3DS completes
- Check `/api/order/v6/orders` for newly created orders after payment

---

#### Cart Management

##### POST `/api/cart/v1/carts/active/apply-quantity?cartProductSorting=CATEGORIES`
**Purpose:** Add, remove or update item quantity in the active cart (CANONICAL — supersedes add-items/remove-items)
**Visibility:** Authenticated
**Method:** POST
**Status:** ✅ CONFIRMED 2026-06-30 via live network capture

> ⚠️ The SPA uses **this single endpoint** for all cart mutations. `quantity` is a **signed delta**: `+N` adds, `-N` removes. The response returns the full cart. See the "2026-06-30 Live verification" block at the top.

**Request Headers:**
- `x-csrf-token`: CSRF token (UUID)
- `client-route-id`: UUID
- `page-view-id`: UUID
- `ecom-request-source`: web
- `ecom-request-source-version`: {version}

**Request Body:** Array of items to add
```json
[{
  "productId": "33d94bf5-5f3a-48ca-bd02-43d4cb1ed9b7",
  "quantity": 1,
  "meta": {
    "itemListName": "logged-in-homepage: featured",
    "featured": {
      "campaignId": "2sCb31PhzSmbZAkwFyw3h7",
      "campaignName": "Destacats home Caja Roja Nestlé iSM"
    },
    "favorite": false,
    "pageViewId": "fa8cce68-60ea-42db-bce0-59d192274e95",
    "pageType": "HOMEPAGE"
  }
}]
```

**Response:** Full cart state with basketUpdateResult
```json
{
  "basketUpdateResult": {
    "items": [{
      "productId": "33d94bf5-5f3a-48ca-bd02-43d4cb1ed9b7",
      "quantity": 1,
      "price": {"currency": "EUR", "amount": "12.95"},
      "regularPrice": {"currency": "EUR", "amount": "12.95"},
      "totalPrices": {
        "regularPrice": {"currency": "EUR", "amount": "12.95"},
        "finalPrice": {"currency": "EUR", "amount": "12.95"}
      },
      "maxQuantityReached": false
    }],
    "totals": {
      "itemsRetailPrice": {"currency": "EUR", "amount": "69.28"},
      "itemPriceAfterPromos": {"currency": "EUR", "amount": "69.28"},
      "savingsPrice": {"currency": "EUR", "amount": "0.00"}
    }
  }
}
```

---

##### POST `/api/cart/v1/carts/active/remove-items`
**Purpose:** Remove item(s) from active cart
**Visibility:** Authenticated
**Method:** POST

**Request Headers:** (same as add-items)
- `x-csrf-token`: CSRF token (UUID)
- `client-route-id`: UUID
- `page-view-id`: UUID
- `ecom-request-source`: web
- `ecom-request-source-version`: {version}

**Request Body:** Array of items to remove
```json
[{
  "productId": "1ebcc8d9-c7df-4f77-a814-d511f6cee3db",
  "quantity": 1
}]
```

**Response:** Full cart state after removal (same structure as add-items)

**Note:** Both add and remove use `quantity: 1` increments. To decrement, call add-items with quantity: 1 (decreases from 3→2). To fully remove, use remove-items.

> ⚠️ SUPERSEDED 2026-06-30: The above note is **incorrect**. The SPA does not use add-items/remove-items at all — it uses `apply-quantity` with a **signed delta** (`quantity: 1` adds, `quantity: -1` removes). See the live-verification block at the top of this doc.

---

##### GET `/api/cart/v1/carts/active`
**Purpose:** Get active cart state
**Visibility:** Authenticated

**Response:** Complete cart object with cartId, items, checkout restrictions, totals

```json
{
  "cartId": "66f4dc14-73ba-4e1a-8de4-cc2cb18f045a",
  "regionId": "00000000-0000-0000-0000-000000000002",
  "cartStatus": "NEVER_CHECKED_OUT",
  "deliveryDestinationId": "00000000-0000-0000-0000-000000000003",
  "items": [...],
  "defaultCheckoutGroup": {
    "canCheckout": false,
    "basketAboveThreshold": true,
    "checkoutRestrictions": ["MISSING_SLOT"]
  }
}
```

---

##### POST `/api/cart/v1/carts/active/vouchers`
**Purpose:** Apply discount/voucher code(s) to the active cart.
**Visibility:** Authenticated
**Status:** ✅ IMPLEMENTED 2026-07-01 (`bonpreu cart voucher <code>...`) — confirmed via HAR capture (button: voucher input on `/checkout/summary`).

**Request body:** array of code strings, e.g. `["TEST5"]`.

**Response:**
```json
{
  "pricingNotifications": [],
  "vouchersAddResult": [
    {"inBasket": false, "newlyAdded": false, "valid": false, "validationErrorCode": "CODE_NOT_FOUND", "voucherId": "test5"}
  ]
}
```
- One `vouchersAddResult` entry per submitted code — a mix of valid/invalid codes in one call is not an error, each is reported independently. `voucherId` in the response is lowercased regardless of the casing submitted.
- Only the invalid-code path (`CODE_NOT_FOUND`) was captured live; a successful application's exact `vouchersAddResult` shape (whether it carries a discount amount inline, or that only shows up in the cart's `totals` on the next `GET /carts/active`) is unconfirmed — the CLI surfaces the raw result either way and doesn't assume more than `valid`/`newlyAdded`/`inBasket`/`validationErrorCode`.
- No remove-voucher endpoint has been captured yet.

---

#### Orders

##### GET `/api/order/v6/orders`
**Purpose:** Get user's order history
**Visibility:** Authenticated

**Response:** List of past orders with dates, amounts, addresses, status

---

#### Wallet / Payment Methods

##### GET `/api/walletservice/v3/wallet-items`
**Purpose:** List saved payment methods (`/settings/wallet` page)
**Visibility:** Authenticated
**Status:** ✅ IMPLEMENTED 2026-07-01 (`wallet list`) — read-only, no auth beyond the standard session headers

**Response:** Array of saved cards, no request body/params

```json
[
  {
    "customerId": "...",
    "fundingInstrumentId": "...",
    "bannerId": "...",
    "defaultWalletItem": true,
    "details": {
      "lastFourDigits": "XXXX",
      "bin": "XXXXXX",
      "cardType": "MasterCard",
      "expiryMonth": "06",
      "expiryYear": "2026",
      "pspName": "Braintree"
    },
    "walletItemId": "...",
    "paymentMethod": "BRAINTREE",
    "creationTime": "2026-05-11T23:16:28.605068211Z",
    "paymentInstrumentType": "CARD_TOKEN",
    "paymentMethodType": "CARDS",
    "expired": true
  }
]
```

**Note:** `expired` reflects card expiry (past `expiryMonth`/`expiryYear`), not session/token expiry. Listing is read-only and does not touch the Braintree/3DS payment flow, which stays out of scope (see SPEC.md).

---

#### Loyalty (Guardiola)

##### GET `/settings/loyalty` — no JSON API, SSR-only
**Purpose:** Guardiola (loyalty wallet) balance
**Visibility:** Authenticated
**Status:** ✅ IMPLEMENTED 2026-07-01 (`bonpreu loyalty`) — confirmed via HAR capture: no XHR/fetch to any `/api/*` path carries this data on page load.

- The balance is **server-rendered only**, embedded in `window.__INITIAL_STATE__.data.customer.loyalty` on the `/settings/loyalty` document response — same mechanism as the homepage's CSRF token (see above), just a different page and a different sub-tree.
- Shape: `{"error":null,"fetchError":false,"isFetching":false,"lastUpdated":<epoch ms>,"lastModified":null,"balance":{"units":461,"money":{"amount":"4.61","currency":"EUR"}},"registered":true}`. `units` is the amount in cents; `money.amount` is the same value pre-formatted as a decimal string.
- **The CLI must GET the full HTML page and scrape it** (`client.ExtractAppState`, the `__INITIAL_STATE__`-only sibling of `ExtractInitialState` used for product-page scraping) — there is no lighter JSON endpoint.

---

#### Regulars, Favorites ("Preferits") & Instant Shop ("Compra ràpida")

##### GET `/api/webproductpagews/v5/product-pages/regulars?limit=<n>&tag=web&tag=regulars`
**Purpose:** Backs *both* the "Productes recurrents" tab and "Preferits" ("Favorites") — one call returns both lists.
**Visibility:** Authenticated
**Status:** ✅ IMPLEMENTED 2026-07-01 (`bonpreu regulars list`, `bonpreu favorites list`) — confirmed via HAR capture.

- Response: `{"productGroups":[{"type":"regular","products":[...]}, {"type":"favorite","products":[...]}]}`. Each `products[]` entry is `{"productId":"<uuid>", "product": {...full decorated product, only if within budget...}}`.
- **`limit` caps total decoration across *all* groups combined, not per group.** With `limit=50` and 14 "regular" + 286 "favorite" items on a real account, only the first 50 entries (14 regular + first 36 favorite) carried the nested `product` object; the remaining 250 favorites came back as bare `{"productId":"..."}` with no `product` key at all.
- ⚠️ **Do not rely on the page's partial decoration for favorites.** `bonpreu favorites list` always re-fetches every returned `productId` via `PUT /api/webproductpagews/v6/products` (same batch endpoint cart/regulars enrichment already uses) — confirmed live: a single PUT with all 286 ids in one call returns all 286 decorated, no chunking needed.
- The "regular" group's `product.regular` sub-object carries `{"quantity":<int>, "frequency":"WEEKLY"|"FORTNIGHTLY"}` — this is what `InstantShop` (below) draws from. The "favorite" group has no such field (a favorite is a plain star, not a purchase-frequency signal).
- Plainer alternative for regulars only (no decoration, no favorites): `GET /api/regulars/v1/regulars` → `[{"productId","quantity","frequency"}]`. Not used by the CLI since the decorated endpoint above already gives names/prices in one call for the "regular" group.

##### POST `/api/cart/v2/instant-shop`
**Purpose:** "Compra ràpida" — server-side picks products from purchase history and adds them to the active cart in one shot.
**Visibility:** Authenticated
**Status:** ✅ IMPLEMENTED 2026-07-01 (`bonpreu regulars fill`) — confirmed via HAR capture (button: "Afegeix els productes a la cistella").

- **Request:** empty JSON object `{}` — no way to preview or influence what gets added.
- **Response:** `{"addedProducts":[{"productId","quantity"},...], "basketUpdateResult":{"items":[...],"lastModified","totals":{...}}, "limitedPromotionIds":[], "pricingNotifications":[]}`.
- ⚠️ `basketUpdateResult` here is **items** (flat array), not `itemGroups` like `apply-quantity`'s response — a different shape from the `Cart` struct used elsewhere. The CLI reads only `addedProducts` and `basketUpdateResult.totals.itemPriceAfterPromos` from it; it does not try to force this into the `Cart`/`Lines()` model.
- Since the server decides what to add, the CLI's `--max` spending guard can't do its usual pre-flight check (no known delta before the call). It instead calls the endpoint, and if the resulting total exceeds `--max`, immediately reverses exactly `addedProducts` via `apply-quantity` with negated quantities and reports a refusal — enforcement after the fact rather than before, since there is no dry-run.

---

### Public Endpoints

#### Search & Browse

##### GET `/api/search/v1/suggestions/primary`
**Purpose:** Product search autocomplete
**Visibility:** Public (no auth required)

**Query Parameters:**
- `searchTerm`: search query string (can be empty)
- `limit`: max results (typical: 20000)
- `regionId`: delivery region UUID (e.g., `00000000-0000-0000-0000-000000000002`)

**Response:** Array of 20,000+ product names/categories

**Example:**
```
GET /api/search/v1/suggestions/primary?searchTerm=&limit=20000&regionId=00000000-0000-0000-0000-000000000002
```

---

##### GET `/api/search/v1/redirects/active`
**Purpose:** Get active search redirects
**Visibility:** Public

**Query Parameters:**
- `regionId`: delivery region UUID

---

#### Advertising

##### GET `/api/adverts/v1/pop-up`
**Purpose:** Fetch advertisement popups
**Visibility:** Public

**Query Parameters:**
- `platform`: WEB

---

## Request/Response Examples

### Adding Product to Cart (UI Observed Flow)

**Observation:** When user clicks "Afegeix al carretó" button:
1. Cart count updates immediately in header
2. Button changes to quantity spinners
3. Multiple API calls triggered
4. Cart total updated

**API Calls Observed:**
1. `PUT /api/webproductpagews/v6/products` - Refresh cart product data
2. `GET /api/ecomdeliverydestinations/v4/delivery-addresses/{id}` - Fetch delivery info
3. `GET /api/search/v1/suggestions/primary` - Refresh search cache
4. `POST /api/ecomslots/v1/slots/next-available-slot` - Update available slots
5. `GET /api/order/v6/orders` - Refresh order history

**⚠️ Cart mutation endpoint NOT YET FOUND** - Appears to be client-side optimistic update with a hidden backend call.

---

### Updating Cart Quantity

**Observation:** Clicking + or - button on quantity spinner:
- Cart count updates immediately
- New quantity reflected in UI
- Cart total recalculates

**⚠️ Specific mutation endpoint NOT YET IDENTIFIED**

---

## Technical Architecture

### Frontend Stack
- **Framework:** React (component-based)
- **State Management:** Likely Redux or Context API (no localStorage cart)
- **Optimization:** Optimistic UI updates (cart changes reflect before API response)

### Backend Stack
- **Framework:** Java/Spring Boot (inferred from `.jar` patterns)
- **CDN:** CloudFront (caching layer)
- **Security:** AWS WAF (challenge tokens)
- **Load Balancing:** AWS ALB (AWSALB cookies)

### API Versioning
- v1: Search, adverts, cart management
- v4: Delivery destinations
- v5: Product pages
- v6: Product enrichment

### Data Models

**Product ID Types:**
- `productId`: UUID (internal system)
- `retailerProductId`: Legacy numeric ID (e.g., "59289")

**Cart Entry:**
```
{
  productId: UUID,
  quantity: number,
  lastBought?: ISO8601 timestamp
}
```

**Region Locking:**
- User locked to: `00000000-0000-0000-0000-000000000002` (St. Pere de Ribes)
- All API queries require `regionId` parameter
- Affects product availability, pricing, delivery slots

---

## Pending Discovery

### 🔴 Critical (Blocking full API wrapper)
- [x] **Cart mutation endpoint** - FOUND: `POST /api/cart/v1/carts/active/apply-quantity` (signed delta). `add-items`/`remove-items` are NOT used by the SPA.
- [x] **Cart quantity semantics** - RESOLVED: signed delta. `+N` add, `-N` remove.
- [x] **Product search returning objects** - FOUND: `GET /api/webproductpagews/v6/product-pages/search?q=`
- [x] **Category tree** - FOUND: `GET /api/webproductpagews/v1/categories?categoryDepth=4`
- [x] **OIDC token exchange (Step 5)** - RESOLVED: server-side `GET /sso-login/auth?code=&state=` (PKCE via `sso.codeVerifier` cookie). No client-side token.
- [x] **Slots endpoint** - FOUND & corrected: `POST /api/ecomslots/v2/slots` (v2 grid, not v1). Home vs C&C via `shippingGroupType`.
- [x] **Shipping groups + pickup points** - FOUND: `delivery-addresses?deliveryMethod=HOME_DELIVERY|CUSTOMER_COLLECTION`; types `default home delivery` / `default customer collection`.
- [x] **Slot reservation endpoint** - FOUND: `POST /api/ecomslots/v1/slots/reservation`
- [x] **Checkout initiation** - FOUND: `POST /api/cart/v1/carts/active/checkout-start`
- [x] **Checkout workflow** - FOUND: `GET /api/checkoutwalk/v1/checkout-walk`
- [x] **Slot extension** - FOUND: `POST /api/ecomslots/v1/slots/extend`
- [x] **Payment flow identified** - Braintree 3DS via pspweb.compraonline.bonpreuesclat.cat
- ⚠️ **Order placement endpoint** - STILL THE ONLY BLOCKER, but the shape is now fully mapped: `POST v2/checkout` (mobile gateway), request/response DTOs recovered from the app's decompiled Kotlin classes (`reference-bonpreu-app-feasibility.md` §4.1). Its response is a 3-way branch — immediate completion, a plain web 3DS redirect (scriptable), or a Braintree client-SDK step (`payment/complete`, needs a real Braintree nonce — not just an HTTP body). A real session was walked up to the checkout-summary/payment-method screen and deliberately stopped before submitting payment, so which branch actually fires for a real card is still unconfirmed. Deliberately OUT OF SCOPE for the CLI regardless (user completes order in web/app).

### 🟡 Important (Nice to have)
- [x] OpenID Connect login flow reverse engineering - COMPLETE (web)
- [x] Token refresh mechanism — CONFIRMED to exist on the **mobile** gateway (`api.bpe.osp.tech`, separate auth from the web cookie session): `POST v1/authorize` returns `{token, refreshToken}` live-verified 2026-07-08. `POST v1/authorize/refresh` itself didn't fire live (token never aged out), but its exact shape was recovered statically: it authenticates via the **device token** (not the session token) in the `Authorization` header, body `{refreshToken}`, response `{token, refreshToken: nullable}` — so a CLI implementation needs to persist three values (session token, refreshToken, device token), not just one. Web flow still has no client-side refresh at all. Adopting this for the CLI needs a second, mobile-specific auth transport — see `reference-bonpreu-app-feasibility.md` §3.
- [ ] WAF token generation & refresh
- [x] Favorites/watchlist management - shipped via web scraping (`bonpreu favorites list`); APK shows a cleaner native `GET v3/favorites` (still unconfirmed live)
- [x] Shopping lists API — `GET v1/product-lists/basic?includingProduct={id}` CONFIRMED live 2026-07-08 (returns `[]` for an account with none yet); `POST v1/product-lists` (create) attempted live but didn't confirm — see `reference-bonpreu-app-feasibility.md` §4/§6
- [ ] Promotions/offer details API — candidate `PromotionRestClient`/`PromotionSelectorRestClient` paths in `reference-bonpreu-app-feasibility.md`, unconfirmed

### 🟢 Optional (Enhancement)
- [ ] Product reviews/ratings — candidate `ProductReviewRestClient` paths in `reference-bonpreu-app-feasibility.md`, unconfirmed (didn't trigger live — product used in the test session had no reviews UI)
- [ ] User profile endpoints
- [x] Delivery pass (BPAS) management — `GET v1/user/subscriptions/delivery/active` CONFIRMED live 2026-07-08: `{messages: {advertisingMessage, marketingMessage, isFreeTrial}, allSubscriptionsLink, customerHasAddress}`
- [ ] Recent searches history
- [ ] Analytics/tracking endpoints

---

## Known Constraints

1. **Region Locking:** API restricts operations to user's assigned region
2. **Session Expiry:** Cookies expire after 3 months or shorter if inactive
3. **WAF Protection:** AWS WAF enforces challenge tokens for suspicious traffic
4. **CSRF Protection:** All mutations require valid CSRF token
5. **Rate Limiting:** Not yet tested, but WAF may enforce limits
6. **Product Availability:** Changes per region and time

---

## Testing Progress

| Action | Status | Notes |
|--------|--------|-------|
| ✅ Login (SSO) | COMPLETE | OpenID Connect flow working |
| ✅ Browse products | COMPLETE | Search & catalog APIs functional |
| ✅ View cart | COMPLETE | Can fetch enriched product data |
| ✅ Add to cart | COMPLETE | `POST /api/cart/v1/carts/active/apply-quantity` `{quantity:+1}` (live 2026-06-30) |
| ✅ Update quantity | COMPLETE | Same endpoint, `{quantity:+1}` (delta) |
| ✅ Remove from cart | COMPLETE | Same endpoint, `{quantity:-1}` (delta) |
| ✅ Get available slots | COMPLETE | `POST /api/ecomslots/v2/slots` (v2 grid) |
| ✅ Select/reserve slot | COMPLETE | `POST /api/ecomslots/v1/slots/reservation` |
| ✅ Shipping method (home/C&C) | COMPLETE | `delivery-addresses?deliveryMethod=` + `shippingGroupType` |
| 🟹 Place order | OUT OF SCOPE | 3DS in browser; CLI stops at cart+slot. User finishes order in web/app |
| ✅ Auth reverse engineering | COMPLETE | OIDC server-side exchange confirmed (PKCE) |

---

## References

**Main Site:** https://www.compraonline.bonpreuesclat.cat/
**Auth Server:** https://app.bonpreu.cat/openid-connect-server-webapp/
**Image CDN:** https://www.compraonline.bonpreuesclat.cat/images-v3/
**Analytics:** Google Analytics (GTM), Clarity, Evergage

---

## Next Steps

### Completed ✅
1. ~~**Test quantity update & remove operations**~~ - Both endpoints documented (add-items with quantity:1, remove-items)
2. ~~**Document OpenID Connect code exchange**~~ - Full authorization code flow with client_id and parameters documented

### Remaining Priority Work
1. ✅ **Map delivery slot selection** - Slots fetched via `POST /api/ecomslots/v1/slots`, reservation via `POST /api/ecomslots/v1/slots/reservation`
2. ❌ **Identify checkout/order placement endpoint** - Monitor network during checkout process (next: click "Valida la compra" button)
3. **Identify token exchange endpoint** - Trace where authorization code is exchanged for session/JWT token
4. **Test cart edge cases** - Max quantity limits, unavailable items, promotion interactions
5. **Token refresh mechanism** - How session/JWT tokens are refreshed when expired
6. **Test API rate limits** - Check if WAF enforces request throttling on high-volume requests

---

## Implementation Strategy: API Wrapper

### Architecture Overview

The Bonpreu Esclt API wrapper should follow this structure:

```
BonpreuAPI
├── Authentication
│   ├── OpenID Connect flow (browser-based)
│   ├── Cookie management
│   └── CSRF token refresh
├── Cart Operations
│   ├── getCart()
│   ├── addToCart(productId, quantity)
│   ├── removeFromCart(productId, quantity)
│   └── updateCartQuantity()
├── Product Search
│   ├── searchProducts(query)
│   ├── getProductDetails(productIds)
│   └── getCatalog(limit, offset, tags)
├── Delivery & Slots
│   ├── getAvailableSlots(addressId)
│   ├── selectSlot(slotId)
│   └── getDeliveryAddresses()
└── Orders (Partial)
    ├── placeOrder() [NOT YET IMPLEMENTED]
    └── getOrderHistory()
```

### Key Implementation Challenges

1. **reCAPTCHA on Login**
   - Block: Login form requires reCAPTCHA v3
   - Solution: Use headless browser (Playwright/Puppeteer) or 2captcha paid service
   - Alternative: User manually logs in once, extracts cookies via CLI tool

2. **CSRF Token Management**
   - Token rotates on every mutation request
   - Must extract fresh token from response headers before next mutation
   - Pattern: `response.headers.get('x-csrf-token')`

3. **Client-Side Slot Selection**
   - Slot selection handled in browser state, not API
   - No explicit "reserve" endpoint found yet
   - Slot data POSTed to (unknown) order creation endpoint during checkout

4. **AWS WAF Protection**
   - Challenge tokens required (auto-managed by browser)
   - Potential rate limiting not yet tested
   - May require exponential backoff on 403 responses

### Minimal Viable Wrapper

```typescript
class BonpreuAPI {
  private cookies: Record<string, string> = {};
  private csrfToken = '';
  private clientRouteId = '';
  private pageViewId = '';

  constructor(sessionCookies: Record<string, string>) {
    this.cookies = sessionCookies;
  }

  // Cart operations (fully functional)
  async getCart() { /* GET /api/cart/v1/carts/active */ }
  async addToCart(productId: string, qty: number) { /* POST add-items */ }
  async removeFromCart(productId: string, qty: number) { /* POST remove-items */ }

  // Search (fully functional)
  async searchProducts(query: string) { /* GET /api/search/v1/suggestions */ }

  // Slots (fully functional)
  async getAvailableSlots(addressId: string) { /* POST /api/ecomslots/v1/slots */ }
  async selectSlot(slotId: string, addressId: string) { /* POST /api/ecomslots/v1/slots/reservation */ }

  // Orders (NOT IMPLEMENTED - order endpoint unknown)
  async placeOrder(slotId: string, paymentMethod: string) {
    // TODO: Discover correct endpoint and implement
    throw new Error('Order placement not yet implemented');
  }
}
```

### Critical Next Step

**The main blocker is the Order Placement endpoint.** To find it:
1. Manually complete checkout flow in browser
2. Monitor Network tab for POST requests to `/api/orders/*` or `/api/checkout/*`
3. Capture the request body and response structure
4. Document the full order flow including payment integration

Once the order endpoint is identified, the API wrapper will be feature-complete for basic click-and-collect checkout operations.

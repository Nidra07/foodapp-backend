# foodapp-backend

Go/Gin modular monolith backend for the multi-vendor food delivery platform. **Phase 9 of 9 (feature-complete per the original roadmap)** — Identity & Auth, Users, Restaurants, Menu, Cart, Orders, Payments, Delivery, Notifications, Settlements, Admin RBAC, and Search are all implemented. The remaining roadmap item is Testing / Security Hardening / Observability / Deployment — not a new module, but a pass across everything below. See "Known gaps" for the consolidated list that pass needs to cover.

## Stack

Go 1.22 · Gin · PostgreSQL (+ PostGIS) · sqlc · pgx/v5 · Redis · JWT (golang-jwt) · Docker Compose

See `docs/assumptions.md` for the reasoning behind sqlc-over-GORM, OTP-only auth, and refresh-token rotation design.

## Getting started

This code was generated without network access, so dependencies have never been downloaded/built here. Before it will compile, run these **in order**, locally:

```bash
# 1. Install tool dependencies (once)
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# 2. Start Postgres, Redis, MinIO
cp .env.example .env
make docker-up          # or: docker compose up -d postgres redis minio

# 3. Run migrations
make migrate-up

# 4. Generate sqlc code (this creates internal/platform/db/sqlc — required for the build)
make sqlc-generate

# 5. Fetch Go module dependencies
go mod tidy

# 6. Run the API
make run                # http://localhost:8080
```

Check health:
```bash
curl localhost:8080/healthz
curl localhost:8080/readyz
```

## Try the auth flow

```bash
# 1. Request an OTP (mock sender logs the code to the console — check server logs)
curl -X POST localhost:8080/api/v1/auth/otp/request \
  -H 'Content-Type: application/json' \
  -d '{"identifier":"+919876543210","purpose":"login"}'

# 2. Verify it (use the code printed in the server logs)
curl -X POST localhost:8080/api/v1/auth/otp/verify \
  -H 'Content-Type: application/json' \
  -d '{"identifier":"+919876543210","code":"123456","purpose":"login","role":"customer"}'

# 3. Use the returned access_token
curl localhost:8080/api/v1/users/me -H 'Authorization: Bearer <access_token>'
```

## Try restaurant onboarding + menu setup

```bash
# Onboard a restaurant (as an authenticated restaurant_owner)
curl -X POST localhost:8080/api/v1/restaurants \
  -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Spice Route","address_line1":"12 MG Road","city":"Bengaluru","state":"KA","postal_code":"560001","lat":12.9716,"lng":77.5946}'

# Set operating hours (day_of_week: 0=Sunday)
curl -X PUT localhost:8080/api/v1/restaurants/$RESTAURANT_ID/hours \
  -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"day_of_week":1,"open_time":"10:00","close_time":"22:00"}'

# Set service radius
curl -X PUT localhost:8080/api/v1/restaurants/$RESTAURANT_ID/service-area \
  -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"radius_km":5}'

# Upload required KYC documents (fssai_license, gst_certificate, pan_card), then:
curl -X POST localhost:8080/api/v1/restaurants/$RESTAURANT_ID/submit -H "Authorization: Bearer $OWNER_TOKEN"

# As admin: approve
curl -X POST localhost:8080/api/v1/admin/restaurants/$RESTAURANT_ID/review \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{"approve":true}'

# Owner: go live
curl -X PATCH localhost:8080/api/v1/restaurants/$RESTAURANT_ID/accepting-orders \
  -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' -d '{"accepting":true}'

# Add a menu category + item
curl -X POST localhost:8080/api/v1/restaurants/$RESTAURANT_ID/menu/categories \
  -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' -d '{"name":"Mains"}'

curl -X POST localhost:8080/api/v1/restaurants/$RESTAURANT_ID/menu/items \
  -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"category_id":"'$CATEGORY_ID'","name":"Butter Chicken","food_type":"non_veg","base_price":320}'

# Customer: browse nearby + view menu (public, no auth)
curl "localhost:8080/api/v1/restaurants/nearby?lat=12.9716&lng=77.5946"
curl "localhost:8080/api/v1/restaurants/$RESTAURANT_ID/menu"
```

## Try cart + checkout

```bash
# Add an item to cart (as an authenticated customer)
curl -X POST localhost:8080/api/v1/cart/items \
  -H "Authorization: Bearer $CUSTOMER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"restaurant_id":"'$RESTAURANT_ID'","menu_item_id":"'$ITEM_ID'","quantity":2}'

# View cart (live-priced against current menu)
curl localhost:8080/api/v1/cart -H "Authorization: Bearer $CUSTOMER_TOKEN"

# Checkout — converts the cart into an immutable order snapshot
curl -X POST localhost:8080/api/v1/orders/checkout \
  -H "Authorization: Bearer $CUSTOMER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"payment_method":"upi","address_line1":"221B Baker St","city":"Bengaluru","state":"KA","postal_code":"560001","lat":12.97,"lng":77.59,"contact_phone":"+919876543210"}'

# Track it
curl localhost:8080/api/v1/orders/track/FD-8K3N2Q -H "Authorization: Bearer $CUSTOMER_TOKEN"

# Restaurant side: view the live order queue, advance status
curl localhost:8080/api/v1/restaurants/$RESTAURANT_ID/orders/queue -H "Authorization: Bearer $OWNER_TOKEN"
curl -X PATCH localhost:8080/api/v1/restaurants/$RESTAURANT_ID/orders/$ORDER_ID/status \
  -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' -d '{"status":"confirmed"}'
```

## Try payments (mock gateway)

```bash
# Initiate payment for an order placed with method "upi"/"card"/"wallet" (not "cod")
curl -X POST localhost:8080/api/v1/payments/initiate \
  -H "Authorization: Bearer $CUSTOMER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"order_id":"'$ORDER_ID'","method":"upi"}'
# -> returns gateway_order_id; the mock gateway has no real checkout UI,
#    so simulate success by signing gateway_order_id|gateway_payment_id
#    with MockGateway.Sign() (see internal/modules/payments/infrastructure/mock_gateway.go)
#    and POSTing the result to /payments/capture

curl -X POST localhost:8080/api/v1/payments/capture \
  -H "Authorization: Bearer $CUSTOMER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"gateway_order_id":"'$GATEWAY_ORDER_ID'","gateway_payment_id":"mock_pay_123","gateway_signature":"'$SIGNATURE'"}'

# Admin: issue a refund
curl -X POST localhost:8080/api/v1/admin/payments/refund \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"order_id":"'$ORDER_ID'","amount":360,"reason":"restaurant unable to fulfill"}'
```

## Try delivery

```bash
# Register as a delivery partner (as an authenticated delivery_partner user)
curl -X POST localhost:8080/api/v1/delivery/partner/register \
  -H "Authorization: Bearer $PARTNER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"vehicle_type":"bike","vehicle_number":"KA01AB1234"}'

# Admin: approve KYC
curl -X POST localhost:8080/api/v1/delivery/partners/$PARTNER_ID/kyc-review \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{"approve":true}'

# Partner: go online + ping location
curl -X PATCH localhost:8080/api/v1/delivery/partner/online \
  -H "Authorization: Bearer $PARTNER_TOKEN" -H 'Content-Type: application/json' -d '{"online":true}'
curl -X PATCH localhost:8080/api/v1/delivery/partner/location \
  -H "Authorization: Bearer $PARTNER_TOKEN" -H 'Content-Type: application/json' -d '{"lat":12.97,"lng":77.59}'

# Restaurant/admin: dispatch the order to the nearest available partner
curl -X POST localhost:8080/api/v1/delivery/orders/$ORDER_ID/dispatch -H "Authorization: Bearer $OWNER_TOKEN"

# Partner: accept, pick up, deliver
curl -X POST localhost:8080/api/v1/delivery/partner/assignments/$ASSIGNMENT_ID/accept -H "Authorization: Bearer $PARTNER_TOKEN"
curl -X POST localhost:8080/api/v1/delivery/partner/assignments/$ASSIGNMENT_ID/pickup -H "Authorization: Bearer $PARTNER_TOKEN"

# Customer: read their delivery code to hand to the partner
curl localhost:8080/api/v1/delivery/orders/$ORDER_ID/code -H "Authorization: Bearer $CUSTOMER_TOKEN"

curl -X POST localhost:8080/api/v1/delivery/partner/assignments/$ASSIGNMENT_ID/deliver \
  -H "Authorization: Bearer $PARTNER_TOKEN" -H 'Content-Type: application/json' -d '{"otp":"1234"}'
```

## Try notifications

```bash
# Customer: check their notification feed (populated automatically by
# order/payment/delivery events — nothing to trigger manually)
curl localhost:8080/api/v1/notifications -H "Authorization: Bearer $CUSTOMER_TOKEN"
curl localhost:8080/api/v1/notifications/unread-count -H "Authorization: Bearer $CUSTOMER_TOKEN"

# Mark one read, or all
curl -X POST localhost:8080/api/v1/notifications/$NOTIFICATION_ID/read -H "Authorization: Bearer $CUSTOMER_TOKEN"
curl -X POST localhost:8080/api/v1/notifications/read-all -H "Authorization: Bearer $CUSTOMER_TOKEN"

# Opt out of a channel for a category (e.g. no SMS for cancellations)
curl -X PUT localhost:8080/api/v1/notifications/preferences \
  -H "Authorization: Bearer $CUSTOMER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"category":"order_cancelled","channel":"sms","enabled":false}'
```

## Try settlements

```bash
# Restaurant/partner: register a payout account
curl -X PUT localhost:8080/api/v1/restaurants/$RESTAURANT_ID/settlements/payout-account \
  -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"account_holder_name":"Spice Route","account_number":"1234567890","ifsc_code":"HDFC0001234","bank_name":"HDFC Bank"}'

# Admin: open a weekly cycle, process it, review, and pay out
curl -X POST localhost:8080/api/v1/admin/settlements/cycles \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"cycle_start":"2026-08-11","cycle_end":"2026-08-18"}'

curl -X POST localhost:8080/api/v1/admin/settlements/cycles/$CYCLE_ID/process -H "Authorization: Bearer $ADMIN_TOKEN"

curl localhost:8080/api/v1/admin/settlements/cycles/$CYCLE_ID/restaurants -H "Authorization: Bearer $ADMIN_TOKEN"

curl -X POST localhost:8080/api/v1/admin/settlements/restaurant-settlements/$SETTLEMENT_ID/pay \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"reference":"NEFT-20260818-0042"}'

# Restaurant: view their own settlement history
curl localhost:8080/api/v1/restaurants/$RESTAURANT_ID/settlements -H "Authorization: Bearer $OWNER_TOKEN"
```

## Try admin RBAC

```bash
# List the seeded permission catalog and the seeded super_admin role
curl localhost:8080/api/v1/admin/rbac/permissions -H "Authorization: Bearer $ADMIN_TOKEN"
curl localhost:8080/api/v1/admin/rbac/roles -H "Authorization: Bearer $ADMIN_TOKEN"

# Create a narrower role and set its permissions (requires the caller to
# already hold admin.manage_roles — grant super_admin to yourself first
# via the users/roles endpoint below if you're bootstrapping a fresh DB)
curl -X POST localhost:8080/api/v1/admin/rbac/roles \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"finance_admin","description":"Handles refunds and payouts"}'

curl -X PUT localhost:8080/api/v1/admin/rbac/roles/$ROLE_ID/permissions \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"permission_codes":["payments.refund","settlements.pay"]}'

# Grant a role to a user
curl -X POST localhost:8080/api/v1/admin/rbac/users/$USER_ID/roles/$ROLE_ID -H "Authorization: Bearer $ADMIN_TOKEN"

# View the audit trail (populated automatically by restaurant approval,
# refunds, and settlement payouts — see docs/assumptions.md for which
# actions are and aren't audited yet)
curl localhost:8080/api/v1/admin/rbac/audit-log -H "Authorization: Bearer $ADMIN_TOKEN"
```

## Try search

```bash
# Public — works with or without a token; searches are logged either way
curl "localhost:8080/api/v1/search/restaurants?q=biryani&lat=12.9716&lng=77.5946"
curl "localhost:8080/api/v1/search/items?q=paneer%20tikka&lat=12.9716&lng=77.5946"

# Trending searches (last 24h by default)
curl localhost:8080/api/v1/search/trending

# Admin: zero-result searches — what customers wanted but couldn't find
curl localhost:8080/api/v1/admin/search/zero-results -H "Authorization: Bearer $ADMIN_TOKEN"
```

## Project layout

```
cmd/api/main.go                          composition root
internal/config/                         env-based config, validated at startup
internal/platform/                       shared infra: db, redis, logger, errors, middleware, response envelope
internal/modules/<domain>/
  domain/         entities + repository interface (no framework imports)
  application/     use-case orchestration
  infrastructure/ sqlc-backed repository implementation, external provider adapters
  interfaces/http/ gin handlers + route registration
db/migrations/                           versioned SQL migrations (golang-migrate format)
db/queries/                              hand-written SQL consumed by sqlc
docs/assumptions.md                      every judgment call made, and why
docs/model-catalog.md                    every model, its table, and a consistency audit
deployments/docker/Dockerfile            multi-stage production build
docker-compose.yml                       local dev: postgres+postgis, redis, minio, api
```

## What's left: Testing, Hardening, Observability, Deployment

All nine feature-phase modules are built. What remains per the original roadmap isn't a new module — it's a pass across everything above. A first pass has started (see below); this section is the consolidated checklist for the rest of it, pulled from every "Known limitations" note across `docs/assumptions.md` and `docs/model-catalog.md`. Roughly in priority order (money-correctness and security first):

**Fixed in the first hardening pass** (see `docs/assumptions.md`'s "Testing / Hardening pass" section for detail):
- ~~`orders.Create` and `SetRolePermissions` are not wrapped in DB transactions~~ — both now run inside real transactions
- ~~`ProcessCycle` in Settlements can be re-run after payouts are marked paid with no guard~~ — now hard-rejected if any settlement in the cycle is already paid
- A first automated test suite exists (Orders and Delivery state machine unit tests) — still thin, see below

**Money correctness still open (highest priority — real financial risk if shipped as-is):**
- No reconciliation job for `payment_transactions.status` vs `orders.payment_status` drift, or for assignment-status vs order-status drift in Delivery
- No clawback mechanism if a refund happens after a settlement already paid out on the pre-refund amount
- No idempotency key enforcement on `/orders/checkout` — a client retry could double-order

**Security / access control:**
- Fine-grained permission gating (`RequirePermission`) is only used on the Admin RBAC module's own routes — ~15 pre-existing admin endpoints from earlier phases still rely solely on the coarse `RequireRole("admin")` check
- Restaurant-side order/staff routes don't verify the requesting staff belongs to the specific restaurant in the URL
- No webhook-envelope signature verification on the Payments webhook endpoint before parsing the body
- No migration script to grant existing `primary_role='admin'` users into the RBAC system
- Audit logging covers only 3 of many admin/state-changing actions across the platform

**Third-party integrations (all currently mocked):**
- SMS/Email OTP delivery, Payments gateway, push/SMS/email notification senders, and Settlements payout accounts are all mock/placeholder implementations — every one needs a real provider wired in behind its existing abstraction interface before production
- Payout account "tokenization" in Settlements currently stores the raw account number — needs real tokenization or encryption at rest

**Performance / scale (not urgent at pilot volume, will matter beyond it):**
- `GetFullMenu` does N+1 queries per item — needs a Redis cache-aside layer
- Delivery partner location pings write directly to Postgres on every call — needs a Redis GEO live layer
- Notifications send inline in the HTTP request path — needs an outbox + background worker
- `HasPermission` hits the database on every permission-gated request — needs a short-TTL cache

**Product/business-logic placeholders (functional but simplified):**
- Order pricing is flat (5% tax, ₹40 delivery fee), no promotions/discounts
- Delivery dispatch is single-candidate with no auto-retry on reject/timeout
- Delivery partner pay is a flat ₹30/delivery fee, no distance/time/surge/incentive component
- Restaurant service areas are radius-based, not polygon-based
- Restaurant commission is calculated at settlement time using the current rate, not one snapshotted per-order
- No delivery partner rating/review flow — columns exist, nothing writes them
- Search has no typo tolerance/fuzzy matching, and rank-then-distance ordering is unvalidated against actual product intent

**Testing and observability (not built at all yet):**
- Automated test coverage is now non-zero (Orders and Delivery state machine unit tests, runnable via `make test`) but still extremely thin — no integration tests, no HTTP-handler tests, nothing that touches a real or test Postgres instance. This is still the single largest remaining gap, just no longer a literal zero.
- No structured metrics/tracing beyond the request-logging middleware built in Phase 1
- No CI/CD pipeline, no staging environment config beyond the `APP_ENV` distinction in `config.go`

## Adding a new module

Follow the same four-folder pattern as the existing modules:
1. Write the migration in `db/migrations/`
2. Write sqlc queries in `db/queries/`, run `make sqlc-generate`
3. `domain/entity.go` — struct + `Repository` interface
4. `infrastructure/repository.go` — sqlc adapter
5. `application/*.go` — use cases
6. `interfaces/http/*.go` — handlers, mounted in `cmd/api/main.go`

For a module that needs to read across an existing module, define a small consumer-side interface in `application/` naming only the methods you need — don't import the other module's full service. See `orders/application`'s `CartReader`/`RestaurantReader`, `payments/application`'s `OrderReader`/`OrderPaymentUpdater`, `delivery/application`'s `OrderReader`/`OrderStatusUpdater`, `settlements/application`'s `OrderAggregateReader`/`DeliveryAggregateReader`/`RestaurantReader`/`PartnerLookup`, or `AuditLogger` (deliberately duplicated across Restaurants/Payments/Settlements — see its doc comment) for the pattern. For permission-gated (not just role-gated) routes, use `middleware.RequirePermission` with a code from the catalog seeded in `0012_admin_rbac.up.sql`.

Two documented exceptions to the interface-based cross-module read pattern exist, both for the same reason: Notifications' `DeviceLookupAdapter`/`ContactLookupAdapter` (Phase 6) and Search's direct table reads (Phase 9) both involve SQL-level concerns (joins, full-text ranking) that don't decompose cleanly into per-record interface calls — see each module's doc comments for the full reasoning before adding a third such exception.

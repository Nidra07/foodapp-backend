# Model Catalog

Full catalog grows as each phase lands. This covers every model shipped so far.

## Phase 1 models

| Model | File Path | DB Table | Purpose | Related Models | Owning Module |
|---|---|---|---|---|---|
| `User` | `internal/modules/users/domain/entity.go` | `users` | Core identity record for every human in the system (any role) | `UserRole`, `OTPChallenge`, `RefreshToken`, `Device` | Users |
| `UserRole` (row, no Go struct yet — table only) | — | `user_roles` | Tracks every role ever granted to a user, beyond the single `primary_role` | `User` | Users |
| `OTPChallenge` | `internal/modules/identity/domain/entity.go` | `otp_challenges` | One-time-password issuance/verification record | `User` (via identifier, not FK) | Identity & Authentication |
| `Device` | `internal/modules/identity/domain/entity.go` | `devices` | Per-device registration (push token, platform) for a user | `User`, `RefreshToken` | Identity & Authentication |
| `RefreshToken` | `internal/modules/identity/domain/entity.go` | `refresh_tokens` | Hashed refresh token with rotation-family tracking | `User`, `Device` | Identity & Authentication |
| `LoginAttempt` (table only, no Go struct yet) | — | `login_attempts` | Audit trail of login successes/failures for rate limiting & fraud signals | — | Identity & Authentication |

### Consistency check (Phase 1 scope)

1. ✅ Every DB table above has a corresponding Go domain type (except two intentionally thin log/join tables — `user_roles`, `login_attempts` — which are written/read via sqlc directly without a dedicated struct since nothing outside the repository layer needs to hold one in memory yet).
2. ✅ Every model has a migration: `0002_users.up.sql`, `0003_auth.up.sql`.
3. ✅ Every FK (`user_roles.user_id`, `otp_challenges` has no FK by design — identifier-based, `devices.user_id`, `refresh_tokens.user_id`, `refresh_tokens.device_id`) references an existing table.
4. ✅ API DTOs (`verifyOTPBody`, `updateMeBody`, etc. in the `interfaces/http` packages) reference only fields that exist on the domain models above.
5. N/A — no order state machine yet (Phase 4).
6. N/A — no payment state machine yet (Phase 4).
7. ✅ Critical Phase 1 workflow (OTP verify → token issuance) has failure handling: expired/consumed/attempt-exceeded OTP, refresh-token reuse detection, Redis-down fail-open on rate limiting (documented in `docs/assumptions.md`).
8. ✅ No duplicate model represents the same concept.
9. ⚠️ Core domain models NOT yet present at that point (expected — later phases): Restaurant, MenuItem, Cart, Order, Payment, DeliveryPartner, and 50+ others per the master spec's 67-module list.
10. ✅ No orphan tables — every table in `0001`–`0003` migrations is referenced by the model list above.

## Phase 2 models

| Model | File Path | DB Table | Purpose | Related Models | Owning Module |
|---|---|---|---|---|---|
| `Restaurant` | `internal/modules/restaurants/domain/entity.go` | `restaurants` | Core vendor profile: location, status, KYC state, commission rate | `User` (owner), `StaffMember`, `Document`, `OperatingHours`, `ServiceArea`, `Category` | Restaurants |
| `StaffMember` | `internal/modules/restaurants/domain/entity.go` | `restaurant_staff` | Delegated restaurant-scoped staff account with a permission array | `Restaurant`, `User` | Restaurants |
| `Document` | `internal/modules/restaurants/domain/entity.go` | `restaurant_documents` | KYC document (FSSAI, GST, PAN, etc.) with admin review state | `Restaurant`, `User` (reviewer) | Restaurants |
| `OperatingHours` | `internal/modules/restaurants/domain/entity.go` | `restaurant_operating_hours` | Per-day open/close time, supports split shifts via multiple rows | `Restaurant` | Restaurants |
| `ServiceArea` | `internal/modules/restaurants/domain/entity.go` | `restaurant_service_areas` | Radius-based delivery reach for a restaurant | `Restaurant` | Restaurants |
| `Category` | `internal/modules/menu/domain/entity.go` | `menu_categories` | Menu section (e.g. "Starters", "Mains") | `Restaurant`, `Item` | Menu |
| `Item` | `internal/modules/menu/domain/entity.go` | `menu_items` | A sellable dish with base price, food type, availability | `Category`, `VariantGroup`, `AddonGroup` | Menu |
| `VariantGroup` | `internal/modules/menu/domain/entity.go` | `menu_item_variant_groups` | Selectable size/type group for an item (e.g. "Size") | `Item`, `Variant` | Menu |
| `Variant` | `internal/modules/menu/domain/entity.go` | `menu_item_variants` | A specific variant option with a price that replaces base_price | `VariantGroup` | Menu |
| `AddonGroup` | `internal/modules/menu/domain/entity.go` | `menu_addon_groups` | Selectable extras group for an item (e.g. "Toppings") | `Item`, `Addon` | Menu |
| `Addon` | `internal/modules/menu/domain/entity.go` | `menu_addons` | A specific add-on with a price that adds to the selected price | `AddonGroup` | Menu |

### Consistency check (Phase 2 scope)

1. ✅ Every DB table above has a corresponding Go domain type.
2. ✅ Every model has a migration: `0004_restaurants.up.sql`, `0005_menu.up.sql`.
3. ✅ Every FK (`restaurants.owner_user_id` → `users`, `restaurant_staff.restaurant_id/user_id`, `restaurant_documents.restaurant_id`, `restaurant_operating_hours.restaurant_id`, `restaurant_service_areas.restaurant_id`, `menu_categories.restaurant_id`, `menu_items.restaurant_id/category_id`, `menu_item_variant_groups.menu_item_id`, `menu_item_variants.variant_group_id`, `menu_addon_groups.menu_item_id`, `menu_addons.addon_group_id`) references an existing table.
4. ✅ API DTOs in both modules' `interfaces/http` packages reference only fields on the domain models above.
5. **Restaurant status state machine**: `draft → pending_approval → {approved, rejected}`, plus `approved → suspended`/`closed` via separate admin/owner actions. Enforced in `RestaurantService.SubmitForApproval` / `AdminReview` — invalid transitions return `CodeConflict`.
6. N/A — no payment state machine yet (Phase 4).
7. ✅ Failure handling present for: incomplete onboarding (missing hours/documents blocks submission), invalid state transitions (conflict error), cross-restaurant category mismatch on item creation (validation error), negative prices (validation error).
8. ✅ No duplicate model represents the same concept. (Note: `restaurant_owner`/`restaurant_staff` *roles* already existed on `User` from Phase 1 — Phase 2 does not duplicate those, it only adds the restaurant-scoped `StaffMember` join with fine-grained permissions.)
9. ⚠️ Still not present (expected — later phases): Cart, Order, OrderItem, Payment, DeliveryPartner, DeliveryAssignment, Settlement, Notification, Promotion, Rating/Review, Address (customer saved addresses), and the rest of the 67-module list.
10. ✅ No orphan tables — every table in `0004`/`0005` migrations is referenced by the model list above.

### Known limitation carried from Phase 2

`restaurants.location` (PostGIS `geography`) is written on create but not decoded back into Go on read — see `docs/assumptions.md` "Phase 2 — Restaurants & Menu" for the reasoning and the follow-up needed if raw lat/lng is required on restaurant detail views.

## Phase 3 models

| Model | File Path | DB Table | Purpose | Related Models | Owning Module |
|---|---|---|---|---|---|
| `Cart` | `internal/modules/cart/domain/entity.go` | `carts` | A customer's single active shopping cart, scoped to one restaurant | `User`, `Restaurant`, `Item` | Cart |
| `Item` (cart) | `internal/modules/cart/domain/entity.go` | `cart_items` | A menu item + optional variant + quantity in a cart | `Cart`, `menu_items`, `menu_item_variants` | Cart |
| `ItemAddon` (cart) | `internal/modules/cart/domain/entity.go` | `cart_item_addons` | Selected add-ons for a cart item (multi-select) | `Item`, `menu_addons` | Cart |
| `Order` | `internal/modules/orders/domain/entity.go` | `orders` | Immutable snapshot of a completed checkout: pricing, delivery address, status | `User` (customer), `Restaurant`, `OrderItem` | Orders |
| `OrderItem` | `internal/modules/orders/domain/entity.go` | `order_items` | Snapshotted line item (name/price frozen at order time) | `Order`, `OrderItemAddon` | Orders |
| `OrderItemAddon` | `internal/modules/orders/domain/entity.go` | `order_item_addons` | Snapshotted add-on selection for an order item | `OrderItem` | Orders |
| `StatusHistoryEntry` | `internal/modules/orders/domain/entity.go` | `order_status_history` | Full audit trail of every order status transition | `Order`, `User` (actor) | Orders |

### Consistency check (Phase 3 scope)

1. ✅ Every DB table above has a corresponding Go domain type.
2. ✅ Every model has a migration: `0006_cart.up.sql`, `0007_orders.up.sql`.
3. ✅ Every FK (`carts.customer_id/restaurant_id`, `cart_items.cart_id/menu_item_id/variant_id`, `cart_item_addons.cart_item_id/addon_id`, `orders.customer_id/restaurant_id/cancelled_by`, `order_items.order_id/menu_item_id`, `order_item_addons.order_item_id`, `order_status_history.order_id/changed_by`) references an existing table.
4. ✅ API DTOs in both modules' `interfaces/http` packages reference only fields on the domain models above.
5. ✅ **Order status state machine** documented and enforced: `placed → confirmed → preparing → ready_for_pickup → out_for_delivery → delivered`, with `cancelled` reachable only from `placed`/`confirmed`/`preparing` (`domain.Status.CanTransitionTo`, checked in `OrderService.UpdateStatus`/`Cancel`). See `docs/assumptions.md` for the known gap this creates (can't cancel a `ready_for_pickup` order via the API yet).
6. N/A — no payment state machine yet (Phase 4 — Payments module); `orders.payment_status` exists as a simple field for now, defaulting to `pending`.
7. ✅ Failure handling present for: empty cart at checkout, unavailable cart items blocking checkout, below-minimum-order-amount, invalid status transitions, missing cancellation reason, cross-account order access (see access-control note in `docs/assumptions.md`).
8. ✅ No duplicate model represents the same concept. (Cart's `Item`/`ItemAddon` and Orders' `OrderItem`/`OrderItemAddon` look similar but are deliberately distinct: one is live/mutable, one is an immutable snapshot — this is the core Phase 3 design decision, not accidental duplication.)
9. ⚠️ Still not present (expected — later phases): Payment, DeliveryPartner, DeliveryAssignment, Settlement, Notification, Promotion, Rating/Review, saved Address book, and the rest of the 67-module list.
10. ✅ No orphan tables — every table in `0006`/`0007` migrations is referenced by the model list above.

### Known limitations carried from Phase 3

- `orders.Create` (repository) is not wrapped in a DB transaction — see `docs/assumptions.md` for the fix needed before production traffic.
- Restaurant-side order routes don't yet verify the requesting owner/staff belongs to the specific restaurant in the URL — see `docs/assumptions.md`.
- No idempotency key on `/orders/checkout` — a double-tap or client retry on a flaky connection could create two orders from the same cart. Flagging as a near-term follow-up (the platform's `Idempotency-Key` header is already allowed through CORS in `middleware/cors.go` but not yet enforced anywhere).

## Phase 4 models

| Model | File Path | DB Table | Purpose | Related Models | Owning Module |
|---|---|---|---|---|---|
| `Transaction` | `internal/modules/payments/domain/entity.go` | `payment_transactions` | One attempt to pay for an order via a gateway | `Order`, `User` (customer) | Payments |
| `SavedMethod` | `internal/modules/payments/domain/entity.go` | `saved_payment_methods` | A customer's tokenized reusable payment method | `User` | Payments |
| `Refund` | `internal/modules/payments/domain/entity.go` | `refunds` | A refund issued against a captured transaction | `Transaction`, `Order`, `User` (initiator) | Payments |

### Consistency check (Phase 4 scope)

1. ✅ Every DB table above has a corresponding Go domain type.
2. ✅ Every model has a migration: `0008_payments.up.sql`.
3. ✅ Every FK (`payment_transactions.order_id/customer_id`, `saved_payment_methods.customer_id`, `refunds.payment_transaction_id/order_id/initiated_by`) references an existing table.
4. ✅ API DTOs in `interfaces/http` reference only fields on the domain models above.
5. ✅ **Payment transaction state machine**: `initiated → {authorized, captured, failed}`, `captured → {refunded, partially_refunded}` via `Refund`. Enforced informally in `PaymentService` (e.g. `Capture` rejects re-processing a `failed` transaction; `Refund` rejects a non-`captured` transaction) rather than a formal `CanTransitionTo` table like Orders has — flagged as worth formalizing the same way if this module grows more transition paths (e.g. `authorized → captured` as a distinct manual-capture flow, not yet implemented).
6. ✅ Payment state machine cross-checked against Order state: `PaymentService.Capture`/`Refund` update `orders.payment_status` through the `OrderPaymentUpdater` interface rather than writing to the orders table directly, keeping the state boundary explicit. See `docs/assumptions.md` for the known drift risk if that secondary update fails after a successful capture.
7. ✅ Failure handling present for: paying for a cancelled/already-paid order, invalid gateway signature (transaction marked failed, not silently ignored), refunding an uncaptured payment, refunding more than the remaining refundable balance, unrecognized webhook event types (acknowledged, not errored, to avoid retry storms).
8. ✅ No duplicate model represents the same concept.
9. ⚠️ Still not present (expected — later phases): DeliveryPartner, DeliveryAssignment, Settlement (restaurant payout — distinct from customer-facing Payments), Notification, Promotion, Rating/Review, saved Address book, and the rest of the 67-module list.
10. ✅ No orphan tables — every table in `0008` migration is referenced by the model list above.

### Known limitations carried from Phase 4

- No webhook-envelope signature verification before parsing the body — see `docs/assumptions.md`.
- No reconciliation job for `payment_transactions.status = captured` vs `orders.payment_status` drift — see `docs/assumptions.md`.
- Refunds assume synchronous gateway completion; no async refund-webhook handling yet.
- COD orders have no path to ever leave `payment_status = pending` until the Delivery module adds a "mark cash collected" action.

## Phase 5 models

| Model | File Path | DB Table | Purpose | Related Models | Owning Module |
|---|---|---|---|---|---|
| `Partner` | `internal/modules/delivery/domain/entity.go` | `delivery_partners` | A delivery partner's profile, vehicle, verification status, live availability | `User` | Delivery |
| `Assignment` | `internal/modules/delivery/domain/entity.go` | `delivery_assignments` | One offer of an order's delivery to a specific partner, with its lifecycle | `Order`, `Restaurant`, `Partner` | Delivery |

### Consistency check (Phase 5 scope)

1. ✅ Every DB table above has a corresponding Go domain type.
2. ✅ Every model has a migration: `0009_delivery.up.sql`.
3. ✅ Every FK (`delivery_partners.user_id`, `delivery_assignments.order_id/restaurant_id/delivery_partner_id`) references an existing table.
4. ✅ API DTOs in `interfaces/http` reference only fields on the domain models above.
5. ✅ **Assignment state machine**: `offered → {accepted, rejected, cancelled}`, `accepted → {picked_up, cancelled}`, `picked_up → {delivered, cancelled}`, with `delivered`/`rejected`/`cancelled` all terminal. Enforced via `domain.AssignmentStatus.CanTransitionTo`, mirroring the pattern from Orders' state machine (Phase 3) rather than inventing a new approach.
6. ✅ Cross-checked against Order state: `MarkPickedUp` and `MarkDelivered` push the order to `out_for_delivery`/`delivered` through the `OrderStatusUpdater` interface. See `docs/assumptions.md` for the known drift risk if that secondary update fails (same class of issue as the Payments module's order-sync risk).
7. ✅ Failure handling present for: dispatching to an order that already has an active assignment, no available partners nearby, going online before KYC verification, wrong partner attempting to act on someone else's assignment (ownership-checked in the handler), incorrect delivery OTP.
8. ✅ No duplicate model represents the same concept.
9. ⚠️ Still not present (expected — later phases): Settlement (restaurant/partner payout), Notification, Promotion, Rating/Review (columns exist on `Partner` but nothing writes them yet — see `docs/assumptions.md`), saved Address book, Admin RBAC (fine-grained, beyond the coarse role check used so far), Search, and the rest of the 67-module list.
10. ✅ No orphan tables — every table in `0009` migration is referenced by the model list above.

### Known limitations carried from Phase 5

- Dispatch is single-candidate with no auto-retry on reject/timeout — see `docs/assumptions.md`.
- Partner location writes go straight to Postgres on every ping, no Redis-backed live layer yet — flagged as the most likely near-term performance bottleneck.
- No reconciliation job for assignment-status vs order-status drift (same class of gap as Phase 4's payment/order drift).
- Assignment cancellation doesn't auto-trigger re-dispatch.

## Phase 6 models

| Model | File Path | DB Table | Purpose | Related Models | Owning Module |
|---|---|---|---|---|---|
| `Notification` | `internal/modules/notifications/domain/entity.go` | `notifications` | One send attempt of one event, on one channel, to one user | `User` | Notifications |
| `Preference` | `internal/modules/notifications/domain/entity.go` | `notification_preferences` | A user's explicit opt-out/opt-in override for a (category, channel) pair | `User` | Notifications |

### Consistency check (Phase 6 scope)

1. ✅ Every DB table above has a corresponding Go domain type.
2. ✅ Every model has a migration: `0010_notifications.up.sql`.
3. ✅ Every FK (`notifications.user_id`, `notification_preferences.user_id`) references an existing table.
4. ✅ API DTOs in `interfaces/http` reference only fields on the domain models above.
5. N/A — no order-like multi-state workflow; `send_status` is a simple `pending → {sent, failed, skipped}` set by exactly one write in `NotificationService.send`, not a state machine with multiple transition sources.
6. ✅ Cross-checked against Orders/Payments/Delivery: each of those modules' `notify` calls only fire on genuine committed state changes (see each module's own consistency-check entries), and none of them let a notification failure roll back or block the state change itself — documented in `docs/assumptions.md`.
7. ✅ Failure handling present for: missing device tokens (marked `skipped`, not `failed` — there was nothing to fail), missing phone/email for SMS/email channels (same), sender errors (marked `failed` with the reason recorded), unknown category (falls back to `in_app` only, never silently drops the notification entirely).
8. ✅ No duplicate model represents the same concept.
9. ⚠️ Still not present (expected — remaining roadmap): Settlement, Promotion, Rating/Review, saved Address book, Admin RBAC (fine-grained), Search.
10. ✅ No orphan tables — every table in `0010` migration is referenced by the model list above.

### Known limitations carried from Phase 6

- No outbox/background worker — notification sends happen inline in the HTTP request path that triggered them, which will slow down unrelated API calls if a channel provider is slow. See `docs/assumptions.md` — this is the single biggest structural gap in the module.
- Default channel-per-category mapping is a reasonable guess, not a confirmed product spec.
- No proximity-based notifications yet (e.g. "partner is 5 minutes away") — blocked on the Redis-backed live-location layer flagged as missing in Phase 5.

## Phase 7 models

| Model | File Path | DB Table | Purpose | Related Models | Owning Module |
|---|---|---|---|---|---|
| `PayoutAccount` | `internal/modules/settlements/domain/entity.go` | `payout_accounts` | A restaurant's or delivery partner's bank details for payout | `Restaurant` or `Partner` (polymorphic via owner_type/owner_id) | Settlements |
| `Cycle` | `internal/modules/settlements/domain/entity.go` | `settlement_cycles` | A fixed date-range payout batch | `RestaurantSettlement`, `DeliverySettlement` | Settlements |
| `RestaurantSettlement` | `internal/modules/settlements/domain/entity.go` | `restaurant_settlements` | What one restaurant is owed for one cycle | `Cycle`, `Restaurant`, `PayoutAccount` | Settlements |
| `DeliverySettlement` | `internal/modules/settlements/domain/entity.go` | `delivery_settlements` | What one delivery partner is owed for one cycle | `Cycle`, `Partner`, `PayoutAccount` | Settlements |

### Consistency check (Phase 7 scope)

1. ✅ Every DB table above has a corresponding Go domain type.
2. ✅ Every model has a migration: `0011_settlements.up.sql`.
3. ✅ Every FK that CAN be a real FK is one (`restaurant_settlements.cycle_id/restaurant_id/payout_account_id`, `delivery_settlements.cycle_id/delivery_partner_id/payout_account_id`). `payout_accounts.owner_id` is intentionally NOT a FK (polymorphic across `restaurants`/`delivery_partners`) — enforced in application code instead, documented in the migration's table comment.
4. ✅ API DTOs in `interfaces/http` reference only fields on the domain models above.
5. ✅ **Cycle/settlement state machines**: cycle `open → processing → completed`; settlement `pending → {paid, failed}`. Both simpler than Orders'/Delivery's/Payments' state machines (fewer states, no branching), enforced informally in `SettlementService` (e.g. `ProcessCycle` rejects an already-`completed` cycle) rather than a formal transition table — reasonable given the smaller state space, but flagged as worth the same `CanTransitionTo` treatment if this module grows more states later.
6. ✅ Cross-checked against Orders/Delivery: `ProcessCycle` reads committed, terminal-state data only (`orders.status = 'delivered' AND payment_status = 'paid'`, `delivery_assignments.status = 'delivered'`) — it never settles against in-flight orders or deliveries, so there's no risk of paying out for something that later gets cancelled or refunded. (There IS a risk of a REFUND happening after a settlement already paid out on the pre-refund amount — no clawback mechanism exists yet, not addressed in this phase.)
7. ✅ Failure handling present for: opening a cycle with start >= end, processing an already-completed cycle, paying an already-paid settlement, paying a restaurant/partner with no payout account on file, invalid/too-short account numbers.
8. ✅ No duplicate model represents the same concept.
9. ⚠️ Still not present (expected — remaining roadmap): Admin RBAC (fine-grained, beyond the coarse role check used throughout), Search, and the Testing/Hardening/Deployment phase.
10. ✅ No orphan tables — every table in `0011` migration is referenced by the model list above.

### Known limitations carried from Phase 7

- No payout-gateway abstraction — payouts are manually confirmed, not automated (see `docs/assumptions.md`).
- `ProcessCycle` is only safely re-runnable before any payouts in that cycle are marked paid — no guard against re-processing a partially-paid cycle yet.
- No clawback mechanism if a refund happens after a settlement already paid out on the pre-refund amount.
- Payout account "tokenization" currently stores the raw account number — a real data-minimization gap, worse than its Phase 4 (Payments) counterpart, flagged clearly rather than glossed over.
- Commission is calculated at settlement time using the restaurant's current rate, not a rate snapshotted per-order — worth confirming against actual business rules.

## Phase 8 models

| Model | File Path | DB Table | Purpose | Related Models | Owning Module |
|---|---|---|---|---|---|
| `Role` | `internal/modules/adminrbac/domain/entity.go` | `admin_roles` | A named bundle of permissions (e.g. "finance_admin") | `Permission` (via join table), `User` (via `admin_user_roles`) | Admin RBAC |
| `Permission` | `internal/modules/adminrbac/domain/entity.go` | `admin_permissions` | One fixed, seeded capability code (e.g. "settlements.pay") | `Role` | Admin RBAC |
| `AuditEntry` | `internal/modules/adminrbac/domain/entity.go` | `admin_audit_log` | Append-only record of one admin action | `User` (admin actor), polymorphic resource reference | Admin RBAC |

### Consistency check (Phase 8 scope)

1. ✅ Every DB table above has a corresponding Go domain type. `admin_role_permissions` and `admin_user_roles` are join tables with no dedicated Go struct — same treatment as `user_roles` back in Phase 1, since nothing outside the repository layer needs to hold a join row in memory.
2. ✅ Every model has a migration: `0012_admin_rbac.up.sql`.
3. ✅ Every FK that can be real is real (`admin_role_permissions.role_id/permission_id`, `admin_user_roles.user_id/role_id/granted_by`, `admin_audit_log.admin_user_id`). `admin_audit_log.resource_id` is intentionally NOT a FK (polymorphic across every resource type in the system) — same pattern as `payout_accounts.owner_id` in Phase 7.
4. ✅ API DTOs in `interfaces/http` reference only fields on the domain models above.
5. N/A — no multi-state workflow for roles/permissions themselves (a role either has a permission or doesn't; no pending/approved states).
6. ✅ Cross-checked against the three retrofitted actions (Restaurants' `AdminReview`, Payments' `Refund`, Settlements' `PayRestaurantSettlement`/`PayDeliverySettlement`): each writes its audit entry only AFTER the underlying state change durably succeeds, never before — an audit log entry for an action that didn't actually happen would be worse than no entry at all.
7. ✅ Failure handling present for: deleting a system role (forbidden), modifying a system role's permissions (forbidden), setting an unknown permission code (validation error), granting/revoking roles (idempotent via `ON CONFLICT DO NOTHING` / plain delete).
8. ✅ No duplicate model represents the same concept. (Note: this module's `Role`/`Permission` are a DIFFERENT concept from `users.primary_role` / the coarse `RequireRole` middleware used throughout the rest of the codebase — the coexistence of both systems is deliberate and documented in `docs/assumptions.md`, not an accidental duplication.)
9. ⚠️ Still not present (expected — remaining roadmap): Search, and the Testing/Hardening/Deployment phase.
10. ✅ No orphan tables — every table in `0012` migration is referenced by the model list above.

### Known limitations carried from Phase 8

- Fine-grained permission gating (`RequirePermission`) is only used on this module's own routes, not retrofitted onto ~15 pre-existing admin endpoints from earlier phases — see `docs/assumptions.md`.
- Audit logging is only wired into 3 of the platform's many admin/state-changing actions.
- `HasPermission` hits the database on every gated request — no caching layer yet.
- `SetRolePermissions` is not transaction-wrapped — same class of gap as `orders.Create`.
- No migration script to grant existing `primary_role='admin'` users into the new RBAC system — needed before relying on `RequirePermission` in practice.

## Phase 9 models

| Model | File Path | DB Table | Purpose | Related Models | Owning Module |
|---|---|---|---|---|---|
| `RestaurantResult` | `internal/modules/search/domain/entity.go` | (view over `restaurants`) | A ranked, distance-aware restaurant search hit | `Restaurant` (Phase 2, read-only from this module) | Search |
| `ItemResult` | `internal/modules/search/domain/entity.go` | (view over `menu_items` + `restaurants`) | A ranked, distance-aware menu item search hit | `Item` (Phase 2), `Restaurant` | Search |
| `TrendingTerm` | `internal/modules/search/domain/entity.go` | (aggregate over `search_logs`) | A query string + how often it was searched in a window | — | Search |
| (log row, no dedicated struct) | — | `search_logs` | Append-only record of one search, for trending/analytics | `User` (nullable) | Search |

### Consistency check (Phase 9 scope)

1. ✅ Every table/generated-column addition has a corresponding Go read path. `search_logs` itself has no dedicated Go struct beyond the aggregate `TrendingTerm` view, since nothing needs an individual log row in memory — same treatment as `login_attempts` back in Phase 1.
2. ✅ Every change has a migration: `0013_search.up.sql` (adds `search_vector` generated columns to `restaurants`/`menu_items`, adds `search_logs`).
3. ✅ `search_logs.user_id` FK references `users`; `search_vector` columns don't need FKs (generated columns on existing tables).
4. ✅ API DTOs in `interfaces/http` reference only fields on the domain models above.
5. N/A — no state machine; a search is a single request/response, not a multi-step workflow.
6. ✅ Cross-checked against Restaurants/Menu: search results only ever include `status = 'approved' AND is_accepting_orders = true` restaurants and `is_active = true AND is_available = true` items — the same "live and orderable" filter used by `ListNearbyRestaurants`/`GetFullMenu` in Phases 2-3, applied consistently here too so search can't surface something a customer then can't actually order.
7. ✅ Failure handling present for: empty query string (validation error), missing lat/lng (validation error) — search failures degrade to "no results" rather than erroring where reasonable (e.g. zero matches is a valid 200 response, not a 404).
8. ✅ No duplicate model represents the same concept. (`RestaurantResult`/`ItemResult` are deliberately separate, thinner read-models from the full `restaurants.domain.Restaurant`/`menu.domain.Item` structs — a search hit doesn't need every field a full detail view does, and keeping them separate avoids Search depending on Restaurants'/Menu's domain packages at all, consistent with the "Search reads tables directly, not through other modules' services" exception already documented.)
9. ⚠️ Still not present (expected — final roadmap item): the Testing/Security-Hardening/Observability/Deployment phase.
10. ✅ No orphan tables — `search_logs` is referenced above; the generated columns aren't separate tables.

### Known limitations carried from Phase 9

- No typo tolerance or fuzzy matching — see `docs/assumptions.md`.
- Rank-then-distance sort ordering is unvalidated against actual product intent — see `docs/assumptions.md`.
- No search-specific rate limiting or bot filtering beyond the platform-wide global limiter.
- `OptionalAuth` collapses all token failure modes into "proceed unauthenticated" — fine for Search, flagged for future reuse.

This is the final planned module per the original roadmap's feature phases. The one remaining item is **Testing / Security Hardening / Observability / Deployment** — not a new domain module, but a pass across everything built in Phases 1-9. See `README.md`'s "Known gaps" section for the consolidated list of what that pass would need to address, and this file's "Known limitations carried from Phase N" entries throughout for the full detailed history.

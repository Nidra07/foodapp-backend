// Package application orchestrates Orders use cases. Checkout is the
// centerpiece: it reads the customer's PRICED cart (via CartReader),
// validates it, computes tax/delivery fee, and writes an immutable order
// snapshot — see domain.PlaceOrderInput for why order rows never
// reference live menu prices after this point.
package application

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"

	cartdomain "github.com/foodapp/backend/internal/modules/cart/domain"
	notificationsdomain "github.com/foodapp/backend/internal/modules/notifications/domain"
	"github.com/foodapp/backend/internal/modules/orders/domain"
	restaurantsdomain "github.com/foodapp/backend/internal/modules/restaurants/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

// CartReader is the subset of the Cart module's service this package
// needs. Depending on an interface (rather than *cartapp.CartService
// directly) keeps this package's public surface honest about what it
// actually uses and makes it trivial to fake in tests.
type CartReader interface {
	GetSummary(ctx context.Context, cartID uuid.UUID) (*cartdomain.CartSummary, error)
	GetMyCart(ctx context.Context, customerID uuid.UUID) (*cartdomain.CartSummary, error)
	ClearCart(ctx context.Context, customerID uuid.UUID) error
}

// RestaurantReader is the subset of the Restaurants module this package
// needs, to validate a restaurant is live and to read its min-order
// amount before accepting a checkout.
type RestaurantReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*restaurantsdomain.Restaurant, error)
}

type PricingConfig struct {
	TaxRatePct     float64 // e.g. 5.0 for 5% GST on food — flat platform-wide rate for Phase 3, see docs/assumptions.md
	FlatDeliveryFee float64 // flat fee until distance/surge-based pricing (Delivery module, later phase) lands
}

// Notifier is a small consumer-defined interface (same cross-module
// pattern as CartReader/RestaurantReader above) so this package can tell
// customers about order events without importing the Notifications
// module's full application service. Notify is fire-and-forget from the
// caller's perspective — see notifications/application's own comment on
// why send failures never propagate back as an order-operation failure.
type Notifier interface {
	Notify(ctx context.Context, in notificationsdomain.NotifyInput)
}

type OrderService struct {
	repo       domain.Repository
	cart       CartReader
	restaurant RestaurantReader
	notifier   Notifier // may be nil; every call site checks before using
	pricing    PricingConfig
}

func NewOrderService(repo domain.Repository, cart CartReader, restaurant RestaurantReader, notifier Notifier, pricing PricingConfig) *OrderService {
	return &OrderService{repo: repo, cart: cart, restaurant: restaurant, notifier: notifier, pricing: pricing}
}

// Checkout converts the customer's current cart into an order. This is
// the single most important write path in the platform — see inline
// comments for each validation gate.
func (s *OrderService) Checkout(ctx context.Context, customerID uuid.UUID, paymentMethod domain.PaymentMethod, address domain.DeliveryAddress, specialInstructions *string) (*domain.Order, error) {
	summary, err := s.cart.GetMyCart(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if summary.Cart == nil || len(summary.Items) == 0 {
		return nil, apperr.New(apperr.CodeValidation, "cart is empty")
	}
	if summary.HasUnavailable {
		return nil, apperr.New(apperr.CodeConflict, "one or more items in your cart are no longer available; please review your cart")
	}

	restaurant, err := s.restaurant.GetByID(ctx, summary.Cart.RestaurantID)
	if err != nil {
		return nil, err
	}
	if !restaurant.IsLive() {
		return nil, apperr.New(apperr.CodeConflict, "this restaurant is not currently accepting orders")
	}
	if summary.Subtotal < restaurant.MinOrderAmount {
		return nil, apperr.Validation(
			fmt.Sprintf("minimum order amount is %.2f", restaurant.MinOrderAmount),
			map[string]interface{}{"min_order_amount": restaurant.MinOrderAmount, "current_subtotal": summary.Subtotal},
		)
	}

	items := make([]domain.PlaceOrderItem, len(summary.Items))
	for i, pi := range summary.Items {
		addons := make([]domain.PlaceOrderItemAddon, len(pi.Addons))
		for j, a := range pi.Addons {
			addons[j] = domain.PlaceOrderItemAddon{Name: a.Name, Price: a.Price}
		}
		menuItemID := pi.Item.MenuItemID
		items[i] = domain.PlaceOrderItem{
			MenuItemID: &menuItemID, ItemName: pi.ItemName, VariantName: pi.VariantName,
			UnitPrice: pi.UnitPrice, Quantity: pi.Item.Quantity, LineTotal: pi.LineTotal,
			SpecialInstructions: pi.Item.SpecialInstructions, Addons: addons,
		}
	}

	taxAmount := round2(summary.Subtotal * s.pricing.TaxRatePct / 100)
	deliveryFee := s.pricing.FlatDeliveryFee
	discountAmount := 0.0 // no Promotions module yet — see docs/assumptions.md
	total := round2(summary.Subtotal + taxAmount + deliveryFee - discountAmount)

	orderNumber, err := generateOrderNumber()
	if err != nil {
		return nil, apperr.Internal(err)
	}

	order, err := s.repo.Create(ctx, domain.PlaceOrderInput{
		CustomerID: customerID, RestaurantID: restaurant.ID, Items: items,
		Subtotal: summary.Subtotal, TaxAmount: taxAmount, DeliveryFee: deliveryFee,
		DiscountAmount: discountAmount, TotalAmount: total, PaymentMethod: paymentMethod,
		DeliveryAddress: address, SpecialInstructions: specialInstructions,
	}, orderNumber)
	if err != nil {
		return nil, err
	}

	_ = s.repo.RecordStatusChange(ctx, order.ID, domain.StatusPlaced, &customerID, nil)

	// Cart is only cleared after the order is durably created — if
	// order creation fails, the customer's cart must still be intact so
	// they can retry checkout without re-adding everything.
	_ = s.cart.ClearCart(ctx, customerID)

	s.notify(ctx, order, notificationsdomain.CategoryOrderPlaced, "Order placed", fmt.Sprintf("Your order %s has been placed.", order.OrderNumber))

	return order, nil
}

func (s *OrderService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *OrderService) GetByNumber(ctx context.Context, orderNumber string) (*domain.Order, error) {
	return s.repo.GetByNumber(ctx, orderNumber)
}

func (s *OrderService) GetWithItems(ctx context.Context, id uuid.UUID) (*domain.Order, []*domain.OrderItem, error) {
	order, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	items, err := s.repo.ListItems(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return order, items, nil
}

func (s *OrderService) ListMyOrders(ctx context.Context, customerID uuid.UUID, page, pageSize int) ([]*domain.Order, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}
	return s.repo.ListByCustomer(ctx, customerID, page, pageSize)
}

func (s *OrderService) ListRestaurantOrders(ctx context.Context, restaurantID uuid.UUID, filter domain.ListFilter) ([]*domain.Order, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	return s.repo.ListByRestaurant(ctx, restaurantID, filter)
}

func (s *OrderService) ListActiveQueue(ctx context.Context, restaurantID uuid.UUID) ([]*domain.Order, error) {
	return s.repo.ListActiveByRestaurant(ctx, restaurantID)
}

// UpdateStatus advances an order through the state machine, enforcing
// valid transitions (domain.Status.CanTransitionTo) so, e.g., a
// restaurant can't mark an order "delivered" before it's even been
// picked up. actorID is recorded in the audit trail.
func (s *OrderService) UpdateStatus(ctx context.Context, orderID uuid.UUID, newStatus domain.Status, actorID uuid.UUID) (*domain.Order, error) {
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if !order.Status.CanTransitionTo(newStatus) {
		return nil, apperr.New(apperr.CodeConflict, fmt.Sprintf("cannot transition order from %s to %s", order.Status, newStatus))
	}

	updated, err := s.repo.UpdateStatus(ctx, orderID, newStatus)
	if err != nil {
		return nil, err
	}
	_ = s.repo.RecordStatusChange(ctx, orderID, newStatus, &actorID, nil)

	if category, title, body, ok := statusNotification(updated); ok {
		s.notify(ctx, updated, category, title, body)
	}

	return updated, nil
}

// Cancel is a distinct action from UpdateStatus(cancelled) because it
// requires a reason and records who cancelled — both are shown to the
// customer/restaurant in the order timeline.
func (s *OrderService) Cancel(ctx context.Context, orderID uuid.UUID, reason string, cancelledBy uuid.UUID) (*domain.Order, error) {
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if !order.Status.CanTransitionTo(domain.StatusCancelled) {
		return nil, apperr.New(apperr.CodeConflict, fmt.Sprintf("order in status %s can no longer be cancelled", order.Status))
	}
	if strings.TrimSpace(reason) == "" {
		return nil, apperr.Validation("cancellation reason is required", nil)
	}

	updated, err := s.repo.Cancel(ctx, orderID, reason, cancelledBy)
	if err != nil {
		return nil, err
	}
	notes := reason
	_ = s.repo.RecordStatusChange(ctx, orderID, domain.StatusCancelled, &cancelledBy, &notes)
	s.notify(ctx, updated, notificationsdomain.CategoryOrderCancelled, "Order cancelled", fmt.Sprintf("Your order %s was cancelled: %s", updated.OrderNumber, reason))
	return updated, nil
}

func (s *OrderService) GetStatusHistory(ctx context.Context, orderID uuid.UUID) ([]*domain.StatusHistoryEntry, error) {
	return s.repo.ListStatusHistory(ctx, orderID)
}

func (s *OrderService) SetPaymentStatus(ctx context.Context, orderID uuid.UUID, status domain.PaymentStatus) error {
	return s.repo.SetPaymentStatus(ctx, orderID, status)
}

// SumSettlementData exposes the Settlements module's read need without
// that module importing OrderService's full surface — see
// settlements/application's OrderAggregateReader.
func (s *OrderService) SumSettlementData(ctx context.Context, restaurantID uuid.UUID, from, to time.Time) (int64, float64, error) {
	return s.repo.SumSettlementData(ctx, restaurantID, from, to)
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}

// notify is a nil-safe wrapper so every call site doesn't need its own
// "if s.notifier != nil" check — Notifications is an optional dependency
// (a nil notifier is fine, e.g. in tests) not a required one.
func (s *OrderService) notify(ctx context.Context, order *domain.Order, category notificationsdomain.Category, title, body string) {
	if s.notifier == nil {
		return
	}
	s.notifier.Notify(ctx, notificationsdomain.NotifyInput{
		UserID: order.CustomerID, Category: category, Title: title, Body: body,
		Data: map[string]interface{}{"order_id": order.ID.String(), "order_number": order.OrderNumber},
	})
}

// statusNotification maps an order status to the customer-facing
// notification for it. Not every status is customer-notification-worthy
// as a distinct event (e.g. "preparing" already fires via
// defaultChannels at in_app-only priority) — this function is the one
// place that decision lives, so it can be revisited without hunting
// through UpdateStatus's control flow.
func statusNotification(o *domain.Order) (notificationsdomain.Category, string, string, bool) {
	switch o.Status {
	case domain.StatusConfirmed:
		return notificationsdomain.CategoryOrderConfirmed, "Order confirmed", fmt.Sprintf("%s confirmed your order %s.", "The restaurant", o.OrderNumber), true
	case domain.StatusPreparing:
		return notificationsdomain.CategoryOrderPreparing, "Preparing your order", fmt.Sprintf("Your order %s is being prepared.", o.OrderNumber), true
	case domain.StatusReadyForPickup:
		return notificationsdomain.CategoryOrderReady, "Order ready", fmt.Sprintf("Your order %s is ready and waiting for pickup.", o.OrderNumber), true
	case domain.StatusOutForDelivery:
		return notificationsdomain.CategoryOrderOutForDelivery, "Order out for delivery", fmt.Sprintf("Your order %s is on its way!", o.OrderNumber), true
	case domain.StatusDelivered:
		return notificationsdomain.CategoryOrderDelivered, "Order delivered", fmt.Sprintf("Your order %s has been delivered. Enjoy!", o.OrderNumber), true
	default:
		return "", "", "", false
	}
}

// generateOrderNumber creates a short, human-readable, collision-resistant
// code like "FD-8K3N2Q" — friendly enough to read over a phone call for
// support purposes, unlike a raw UUID.
func generateOrderNumber() (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I to avoid ambiguity
	b := make([]byte, 6)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}
	return "FD-" + string(b), nil
}

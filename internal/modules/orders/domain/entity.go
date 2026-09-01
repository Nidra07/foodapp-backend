package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPlaced          Status = "placed"
	StatusConfirmed       Status = "confirmed"
	StatusPreparing       Status = "preparing"
	StatusReadyForPickup  Status = "ready_for_pickup"
	StatusOutForDelivery  Status = "out_for_delivery"
	StatusDelivered       Status = "delivered"
	StatusCancelled       Status = "cancelled"
)

// allowedTransitions defines the order state machine. Any transition not
// listed here is rejected with CodeConflict — see OrderService.UpdateStatus.
// Cancellation is only allowed up through 'preparing': once food is
// ready for pickup, cancelling would waste already-prepared food, so
// that path should go through a support/refund flow instead (future
// module), not a simple status update.
var allowedTransitions = map[Status][]Status{
	StatusPlaced:         {StatusConfirmed, StatusCancelled},
	StatusConfirmed:      {StatusPreparing, StatusCancelled},
	StatusPreparing:      {StatusReadyForPickup, StatusCancelled},
	StatusReadyForPickup: {StatusOutForDelivery},
	StatusOutForDelivery: {StatusDelivered},
	StatusDelivered:      {},
	StatusCancelled:      {},
}

func (s Status) CanTransitionTo(target Status) bool {
	for _, allowed := range allowedTransitions[s] {
		if allowed == target {
			return true
		}
	}
	return false
}

func (s Status) IsTerminal() bool {
	return s == StatusDelivered || s == StatusCancelled
}

type PaymentStatus string

const (
	PaymentPending  PaymentStatus = "pending"
	PaymentPaid     PaymentStatus = "paid"
	PaymentFailed   PaymentStatus = "failed"
	PaymentRefunded PaymentStatus = "refunded"
)

type PaymentMethod string

const (
	PaymentCOD  PaymentMethod = "cod"
	PaymentUPI  PaymentMethod = "upi"
	PaymentCard PaymentMethod = "card"
	PaymentWallet PaymentMethod = "wallet"
)

type DeliveryAddress struct {
	Line1      string
	Line2      *string
	City       string
	State      string
	PostalCode string
	Lat        float64
	Lng        float64
	Phone      string
}

type Order struct {
	ID                  uuid.UUID
	OrderNumber         string
	CustomerID          uuid.UUID
	RestaurantID        uuid.UUID
	Status              Status
	Subtotal            float64
	TaxAmount           float64
	DeliveryFee         float64
	DiscountAmount      float64
	TotalAmount         float64
	PaymentStatus       PaymentStatus
	PaymentMethod       PaymentMethod
	DeliveryAddress     DeliveryAddress
	SpecialInstructions *string
	CancellationReason  *string
	CancelledBy         *uuid.UUID
	EstimatedDeliveryAt *time.Time
	PlacedAt            time.Time
	ConfirmedAt         *time.Time
	ReadyAt             *time.Time
	PickedUpAt          *time.Time
	DeliveredAt         *time.Time
	CancelledAt         *time.Time
	CreatedAt           time.Time
}

type OrderItem struct {
	ID                  uuid.UUID
	OrderID             uuid.UUID
	MenuItemID          *uuid.UUID
	ItemName            string
	VariantName         *string
	UnitPrice           float64
	Quantity            int
	LineTotal           float64
	SpecialInstructions *string
	Addons              []OrderItemAddon
}

type OrderItemAddon struct {
	ID          uuid.UUID
	OrderItemID uuid.UUID
	AddonName   string
	AddonPrice  float64
}

type StatusHistoryEntry struct {
	ID        uuid.UUID
	OrderID   uuid.UUID
	Status    Status
	ChangedBy *uuid.UUID
	Notes     *string
	CreatedAt time.Time
}

// PlaceOrderInput carries everything needed to create an order from a
// priced cart snapshot — the application layer is responsible for
// resolving cart contents into this shape BEFORE calling Create, so the
// repository (and the orders table) never has to know the cart even
// exists.
type PlaceOrderInput struct {
	CustomerID          uuid.UUID
	RestaurantID         uuid.UUID
	Items               []PlaceOrderItem
	Subtotal            float64
	TaxAmount           float64
	DeliveryFee         float64
	DiscountAmount      float64
	TotalAmount         float64
	PaymentMethod       PaymentMethod
	DeliveryAddress     DeliveryAddress
	SpecialInstructions *string
}

type PlaceOrderItem struct {
	MenuItemID          *uuid.UUID
	ItemName            string
	VariantName         *string
	UnitPrice           float64
	Quantity            int
	LineTotal           float64
	SpecialInstructions *string
	Addons              []PlaceOrderItemAddon
}

type PlaceOrderItemAddon struct {
	Name  string
	Price float64
}

type ListFilter struct {
	Status   *Status
	Page     int
	PageSize int
}

// Repository is the persistence port for the Orders module.
type Repository interface {
	Create(ctx context.Context, in PlaceOrderInput, orderNumber string) (*Order, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Order, error)
	GetByNumber(ctx context.Context, orderNumber string) (*Order, error)
	ListByCustomer(ctx context.Context, customerID uuid.UUID, page, pageSize int) ([]*Order, int64, error)
	ListByRestaurant(ctx context.Context, restaurantID uuid.UUID, filter ListFilter) ([]*Order, int64, error)
	ListActiveByRestaurant(ctx context.Context, restaurantID uuid.UUID) ([]*Order, error)

	UpdateStatus(ctx context.Context, id uuid.UUID, status Status) (*Order, error)
	Cancel(ctx context.Context, id uuid.UUID, reason string, cancelledBy uuid.UUID) (*Order, error)
	SetPaymentStatus(ctx context.Context, id uuid.UUID, status PaymentStatus) error

	ListItems(ctx context.Context, orderID uuid.UUID) ([]*OrderItem, error)
	RecordStatusChange(ctx context.Context, orderID uuid.UUID, status Status, changedBy *uuid.UUID, notes *string) error
	ListStatusHistory(ctx context.Context, orderID uuid.UUID) ([]*StatusHistoryEntry, error)

	// SumSettlementData supports the Settlements module — see
	// db/queries/orders.sql's SumSettlementDataForRestaurant comment for
	// why this is keyed on delivered_at, not placed_at.
	SumSettlementData(ctx context.Context, restaurantID uuid.UUID, from, to time.Time) (orderCount int64, grossSubtotal float64, err error)
}

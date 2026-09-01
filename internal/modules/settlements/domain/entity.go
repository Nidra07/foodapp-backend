package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type OwnerType string

const (
	OwnerRestaurant       OwnerType = "restaurant"
	OwnerDeliveryPartner  OwnerType = "delivery_partner"
)

type CycleStatus string

const (
	CycleOpen       CycleStatus = "open"
	CycleProcessing CycleStatus = "processing"
	CycleCompleted  CycleStatus = "completed"
)

type SettlementStatus string

const (
	SettlementPending    SettlementStatus = "pending"
	SettlementProcessing SettlementStatus = "processing"
	SettlementPaid       SettlementStatus = "paid"
	SettlementFailed     SettlementStatus = "failed"
)

type PayoutAccount struct {
	ID                 uuid.UUID
	OwnerType          OwnerType
	OwnerID            uuid.UUID
	AccountHolderName  string
	AccountNumberLast4 string
	AccountToken       string
	IFSCCode           string
	BankName           *string
	IsVerified         bool
	CreatedAt          time.Time
}

type Cycle struct {
	ID          uuid.UUID
	CycleStart  time.Time
	CycleEnd    time.Time
	Status      CycleStatus
	CreatedAt   time.Time
	ProcessedAt *time.Time
}

type RestaurantSettlement struct {
	ID                uuid.UUID
	CycleID           uuid.UUID
	RestaurantID      uuid.UUID
	OrderCount        int
	GrossSubtotal     float64
	CommissionAmount  float64
	NetPayable        float64
	Status            SettlementStatus
	PayoutAccountID   *uuid.UUID
	PayoutReference   *string
	FailureReason     *string
	CreatedAt         time.Time
	PaidAt            *time.Time
}

type DeliverySettlement struct {
	ID                uuid.UUID
	CycleID           uuid.UUID
	DeliveryPartnerID uuid.UUID
	DeliveryCount     int
	GrossEarnings     float64
	IncentiveAmount   float64
	NetPayable        float64
	Status            SettlementStatus
	PayoutAccountID   *uuid.UUID
	PayoutReference   *string
	FailureReason     *string
	CreatedAt         time.Time
	PaidAt            *time.Time
}

// Repository is the persistence port for the Settlements module.
type Repository interface {
	UpsertPayoutAccount(ctx context.Context, a *PayoutAccount) (*PayoutAccount, error)
	GetPayoutAccount(ctx context.Context, ownerType OwnerType, ownerID uuid.UUID) (*PayoutAccount, error)
	SetPayoutAccountVerified(ctx context.Context, id uuid.UUID, verified bool) error

	CreateCycle(ctx context.Context, start, end time.Time) (*Cycle, error)
	GetCycle(ctx context.Context, id uuid.UUID) (*Cycle, error)
	ListCycles(ctx context.Context, page, pageSize int) ([]*Cycle, error)
	SetCycleStatus(ctx context.Context, id uuid.UUID, status CycleStatus) error

	// ListRestaurantsWithActivity / ListPartnersWithActivity drive
	// ProcessCycle: only restaurants/partners with completed activity in
	// the window get a settlement row computed.
	ListRestaurantsWithActivity(ctx context.Context, from, to time.Time) ([]uuid.UUID, error)
	ListPartnersWithActivity(ctx context.Context, from, to time.Time) ([]uuid.UUID, error)

	UpsertRestaurantSettlement(ctx context.Context, s *RestaurantSettlement) (*RestaurantSettlement, error)
	GetRestaurantSettlement(ctx context.Context, id uuid.UUID) (*RestaurantSettlement, error)
	ListRestaurantSettlementsForRestaurant(ctx context.Context, restaurantID uuid.UUID, page, pageSize int) ([]*RestaurantSettlement, error)
	ListRestaurantSettlementsForCycle(ctx context.Context, cycleID uuid.UUID) ([]*RestaurantSettlement, error)
	MarkRestaurantSettlementPaid(ctx context.Context, id uuid.UUID, reference string, payoutAccountID uuid.UUID) (*RestaurantSettlement, error)
	MarkRestaurantSettlementFailed(ctx context.Context, id uuid.UUID, reason string) (*RestaurantSettlement, error)

	UpsertDeliverySettlement(ctx context.Context, s *DeliverySettlement) (*DeliverySettlement, error)
	GetDeliverySettlement(ctx context.Context, id uuid.UUID) (*DeliverySettlement, error)
	ListDeliverySettlementsForPartner(ctx context.Context, partnerID uuid.UUID, page, pageSize int) ([]*DeliverySettlement, error)
	ListDeliverySettlementsForCycle(ctx context.Context, cycleID uuid.UUID) ([]*DeliverySettlement, error)
	MarkDeliverySettlementPaid(ctx context.Context, id uuid.UUID, reference string, payoutAccountID uuid.UUID) (*DeliverySettlement, error)
	MarkDeliverySettlementFailed(ctx context.Context, id uuid.UUID, reason string) (*DeliverySettlement, error)
}

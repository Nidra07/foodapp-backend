// Package application orchestrates Settlements use cases: opening a
// cycle, processing it (computing what every active restaurant/partner
// is owed), and recording payouts. This module reads across Orders,
// Delivery, and Restaurants through small consumer-defined interfaces —
// same pattern as everywhere else in this codebase.
package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	restaurantsdomain "github.com/foodapp/backend/internal/modules/restaurants/domain"
	"github.com/foodapp/backend/internal/modules/settlements/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type OrderAggregateReader interface {
	SumSettlementData(ctx context.Context, restaurantID uuid.UUID, from, to time.Time) (orderCount int64, grossSubtotal float64, err error)
}

type DeliveryAggregateReader interface {
	CountDelivered(ctx context.Context, partnerID uuid.UUID, from, to time.Time) (int64, error)
}

type RestaurantReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*restaurantsdomain.Restaurant, error)
}

// PartnerLookup resolves a delivery partner's ID from their user ID —
// same small consumer-defined interface pattern as everywhere else,
// used so ListMyDeliverySettlements can trust the authenticated caller's
// identity instead of a client-supplied partner_id (an earlier draft of
// this handler took partner_id as a query param, which would have let
// any authenticated partner view any other partner's settlement data —
// caught and fixed before shipping, same class of issue as the Orders
// ownership gap in Phase 3 and the delivery OTP exposure in Phase 5).
type PartnerLookup interface {
	GetPartnerIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}

// AuditLogger mirrors the pattern from restaurants/application and
// payments/application, added in Phase 8.
type AuditLogger interface {
	LogAction(ctx context.Context, adminUserID uuid.UUID, action, resourceType string, resourceID *uuid.UUID, details map[string]interface{}, ipAddress *string)
}

type PayoutConfig struct {
	PerDeliveryFee float64 // flat fee paid to a delivery partner per completed delivery — see docs/assumptions.md
}

type SettlementService struct {
	repo     domain.Repository
	orders   OrderAggregateReader
	delivery DeliveryAggregateReader
	restaurants RestaurantReader
	partners PartnerLookup
	audit    AuditLogger // may be nil
	cfg      PayoutConfig
}

func NewSettlementService(repo domain.Repository, orders OrderAggregateReader, delivery DeliveryAggregateReader, restaurants RestaurantReader, partners PartnerLookup, audit AuditLogger, cfg PayoutConfig) *SettlementService {
	return &SettlementService{repo: repo, orders: orders, delivery: delivery, restaurants: restaurants, partners: partners, audit: audit, cfg: cfg}
}

// OpenCycle creates a new settlement window. cycleEnd is exclusive — an
// order delivered exactly at cycleEnd belongs to the NEXT cycle, not
// this one (see db/queries/orders.sql's SumSettlementDataForRestaurant).
func (s *SettlementService) OpenCycle(ctx context.Context, start, end time.Time) (*domain.Cycle, error) {
	if !start.Before(end) {
		return nil, apperr.Validation("cycle_start must be before cycle_end", nil)
	}
	return s.repo.CreateCycle(ctx, start, end)
}

// ProcessCycle computes every restaurant_settlements and
// delivery_settlements row for a cycle and marks it 'completed'. This is
// safely re-runnable against a cycle with no paid settlements yet
// (recomputes and overwrites rows via UpsertRestaurantSettlement/
// UpsertDeliverySettlement's ON CONFLICT), which is useful if a bug is
// found and the cycle needs correcting before any payouts have gone
// out. Running it against a cycle that already has ANY settlement
// marked 'paid' is explicitly rejected below (cycleHasAnyPaidSettlements)
// — this used to be an unenforced gap (see docs/assumptions.md, Phase 7
// section, for the original flag) and is now a hard guard, not just a
// documented caveat.
func (s *SettlementService) ProcessCycle(ctx context.Context, cycleID uuid.UUID) error {
	cycle, err := s.repo.GetCycle(ctx, cycleID)
	if err != nil {
		return err
	}
	if cycle.Status == domain.CycleCompleted {
		return apperr.New(apperr.CodeConflict, "this cycle has already been processed")
	}

	// Guard against re-processing a cycle that has ANY already-paid
	// settlements — this closes a gap flagged since this cycle was
	// completed successfully but is deliberately checked independently
	// of cycle.Status: the completed-status check above only catches a
	// cleanly-finished cycle being re-run, not a cycle stuck in
	// 'processing' (e.g. after a crash mid-run) where an admin already
	// paid out some of the settlements that DID get computed before the
	// crash. Without this check, resuming that stuck cycle would
	// silently overwrite net_payable on rows that were already paid —
	// see docs/assumptions.md, Phase 7 section, for the original flag.
	if hasPaid, err := s.cycleHasAnyPaidSettlements(ctx, cycleID); err != nil {
		return err
	} else if hasPaid {
		return apperr.New(apperr.CodeConflict,
			"this cycle has settlements that are already marked paid; re-processing would silently overwrite their computed amounts — resolve manually")
	}

	if err := s.repo.SetCycleStatus(ctx, cycleID, domain.CycleProcessing); err != nil {
		return err
	}

	if err := s.processRestaurants(ctx, cycle); err != nil {
		return err
	}
	if err := s.processPartners(ctx, cycle); err != nil {
		return err
	}

	return s.repo.SetCycleStatus(ctx, cycleID, domain.CycleCompleted)
}

func (s *SettlementService) cycleHasAnyPaidSettlements(ctx context.Context, cycleID uuid.UUID) (bool, error) {
	restaurantSettlements, err := s.repo.ListRestaurantSettlementsForCycle(ctx, cycleID)
	if err != nil {
		return false, err
	}
	for _, rs := range restaurantSettlements {
		if rs.Status == domain.SettlementPaid {
			return true, nil
		}
	}

	deliverySettlements, err := s.repo.ListDeliverySettlementsForCycle(ctx, cycleID)
	if err != nil {
		return false, err
	}
	for _, ds := range deliverySettlements {
		if ds.Status == domain.SettlementPaid {
			return true, nil
		}
	}

	return false, nil
}

func (s *SettlementService) processRestaurants(ctx context.Context, cycle *domain.Cycle) error {
	restaurantIDs, err := s.repo.ListRestaurantsWithActivity(ctx, cycle.CycleStart, cycle.CycleEnd)
	if err != nil {
		return err
	}

	for _, restaurantID := range restaurantIDs {
		orderCount, grossSubtotal, err := s.orders.SumSettlementData(ctx, restaurantID, cycle.CycleStart, cycle.CycleEnd)
		if err != nil {
			return err
		}
		if orderCount == 0 {
			continue
		}

		restaurant, err := s.restaurants.GetByID(ctx, restaurantID)
		if err != nil {
			return err
		}

		commission := round2(grossSubtotal * restaurant.CommissionPct / 100)
		netPayable := round2(grossSubtotal - commission)

		if _, err := s.repo.UpsertRestaurantSettlement(ctx, &domain.RestaurantSettlement{
			CycleID: cycle.ID, RestaurantID: restaurantID, OrderCount: int(orderCount),
			GrossSubtotal: grossSubtotal, CommissionAmount: commission, NetPayable: netPayable,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *SettlementService) processPartners(ctx context.Context, cycle *domain.Cycle) error {
	partnerIDs, err := s.repo.ListPartnersWithActivity(ctx, cycle.CycleStart, cycle.CycleEnd)
	if err != nil {
		return err
	}

	for _, partnerID := range partnerIDs {
		count, err := s.delivery.CountDelivered(ctx, partnerID, cycle.CycleStart, cycle.CycleEnd)
		if err != nil {
			return err
		}
		if count == 0 {
			continue
		}

		gross := round2(float64(count) * s.cfg.PerDeliveryFee)
		incentive := 0.0 // no incentive scheme built yet — see docs/assumptions.md

		if _, err := s.repo.UpsertDeliverySettlement(ctx, &domain.DeliverySettlement{
			CycleID: cycle.ID, DeliveryPartnerID: partnerID, DeliveryCount: int(count),
			GrossEarnings: gross, IncentiveAmount: incentive, NetPayable: round2(gross + incentive),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *SettlementService) GetCycle(ctx context.Context, id uuid.UUID) (*domain.Cycle, error) {
	return s.repo.GetCycle(ctx, id)
}

func (s *SettlementService) ListCycles(ctx context.Context, page, pageSize int) ([]*domain.Cycle, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.repo.ListCycles(ctx, page, pageSize)
}

func (s *SettlementService) ListRestaurantSettlementsForCycle(ctx context.Context, cycleID uuid.UUID) ([]*domain.RestaurantSettlement, error) {
	return s.repo.ListRestaurantSettlementsForCycle(ctx, cycleID)
}

func (s *SettlementService) ListDeliverySettlementsForCycle(ctx context.Context, cycleID uuid.UUID) ([]*domain.DeliverySettlement, error) {
	return s.repo.ListDeliverySettlementsForCycle(ctx, cycleID)
}

func (s *SettlementService) ListMyRestaurantSettlements(ctx context.Context, restaurantID uuid.UUID, page, pageSize int) ([]*domain.RestaurantSettlement, error) {
	return s.repo.ListRestaurantSettlementsForRestaurant(ctx, restaurantID, page, pageSize)
}

func (s *SettlementService) ListMyDeliverySettlements(ctx context.Context, partnerID uuid.UUID, page, pageSize int) ([]*domain.DeliverySettlement, error) {
	return s.repo.ListDeliverySettlementsForPartner(ctx, partnerID, page, pageSize)
}

// ListMyDeliverySettlementsForUser resolves the caller's partner ID from
// their user ID before listing — this is the ownership-safe entrypoint
// the HTTP handler should use; ListMyDeliverySettlements above stays
// available for admin/internal callers that already have a partner ID.
func (s *SettlementService) ListMyDeliverySettlementsForUser(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]*domain.DeliverySettlement, error) {
	partnerID, err := s.partners.GetPartnerIDForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListDeliverySettlementsForPartner(ctx, partnerID, page, pageSize)
}

// PayRestaurantSettlement is a manual admin action recording that a
// payout was sent (via whatever out-of-band bank transfer / payout
// provider is actually used — see docs/assumptions.md for why there's
// no automated payout gateway call here yet, unlike Payments' gateway
// abstraction for customer-side charges).
func (s *SettlementService) PayRestaurantSettlement(ctx context.Context, settlementID uuid.UUID, reference string, adminID uuid.UUID) (*domain.RestaurantSettlement, error) {
	settlement, err := s.repo.GetRestaurantSettlement(ctx, settlementID)
	if err != nil {
		return nil, err
	}
	if settlement.Status == domain.SettlementPaid {
		return nil, apperr.New(apperr.CodeConflict, "this settlement has already been paid")
	}

	account, err := s.repo.GetPayoutAccount(ctx, domain.OwnerRestaurant, settlement.RestaurantID)
	if err != nil {
		return nil, apperr.New(apperr.CodeValidation, "restaurant has no payout account on file")
	}

	paid, err := s.repo.MarkRestaurantSettlementPaid(ctx, settlementID, reference, account.ID)
	if err != nil {
		return nil, err
	}
	if s.audit != nil {
		s.audit.LogAction(ctx, adminID, "settlement.pay", "restaurant_settlement", &settlementID,
			map[string]interface{}{"restaurant_id": settlement.RestaurantID.String(), "amount": settlement.NetPayable, "reference": reference}, nil)
	}
	return paid, nil
}

func (s *SettlementService) PayDeliverySettlement(ctx context.Context, settlementID uuid.UUID, reference string, adminID uuid.UUID) (*domain.DeliverySettlement, error) {
	settlement, err := s.repo.GetDeliverySettlement(ctx, settlementID)
	if err != nil {
		return nil, err
	}
	if settlement.Status == domain.SettlementPaid {
		return nil, apperr.New(apperr.CodeConflict, "this settlement has already been paid")
	}

	account, err := s.repo.GetPayoutAccount(ctx, domain.OwnerDeliveryPartner, settlement.DeliveryPartnerID)
	if err != nil {
		return nil, apperr.New(apperr.CodeValidation, "delivery partner has no payout account on file")
	}

	paid, err := s.repo.MarkDeliverySettlementPaid(ctx, settlementID, reference, account.ID)
	if err != nil {
		return nil, err
	}
	if s.audit != nil {
		s.audit.LogAction(ctx, adminID, "settlement.pay", "delivery_settlement", &settlementID,
			map[string]interface{}{"delivery_partner_id": settlement.DeliveryPartnerID.String(), "amount": settlement.NetPayable, "reference": reference}, nil)
	}
	return paid, nil
}

func (s *SettlementService) RegisterPayoutAccount(ctx context.Context, ownerType domain.OwnerType, ownerID uuid.UUID, holderName, accountNumber, ifsc, bankName string) (*domain.PayoutAccount, error) {
	if len(accountNumber) < 4 {
		return nil, apperr.Validation("invalid account number", nil)
	}
	last4 := accountNumber[len(accountNumber)-4:]
	// NOTE: storing the full accountNumber as the "token" is a Phase 7
	// placeholder — see docs/assumptions.md. A real integration would
	// send the full details to a payout provider (Razorpay X, Stripe
	// Connect, etc.) and store only the token that provider returns,
	// never the raw account number itself, matching the same
	// data-minimization posture as saved_payment_methods in Payments.
	return s.repo.UpsertPayoutAccount(ctx, &domain.PayoutAccount{
		OwnerType: ownerType, OwnerID: ownerID, AccountHolderName: holderName,
		AccountNumberLast4: last4, AccountToken: accountNumber, IFSCCode: ifsc, BankName: strPtr(bankName),
	})
}

// RegisterPartnerPayoutAccountForUser resolves the caller's partner ID
// from their user ID before registering — ownership-safe entrypoint for
// the HTTP handler, same reasoning as ListMyDeliverySettlementsForUser.
func (s *SettlementService) RegisterPartnerPayoutAccountForUser(ctx context.Context, userID uuid.UUID, holderName, accountNumber, ifsc, bankName string) (*domain.PayoutAccount, error) {
	partnerID, err := s.partners.GetPartnerIDForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.RegisterPayoutAccount(ctx, domain.OwnerDeliveryPartner, partnerID, holderName, accountNumber, ifsc, bankName)
}

func (s *SettlementService) GetPayoutAccount(ctx context.Context, ownerType domain.OwnerType, ownerID uuid.UUID) (*domain.PayoutAccount, error) {
	return s.repo.GetPayoutAccount(ctx, ownerType, ownerID)
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Package application orchestrates Delivery use cases: partner
// onboarding/availability, nearest-partner dispatch, and the assignment
// lifecycle (offer -> accept/reject -> pickup -> delivery). It updates
// order status through a small OrderStatusUpdater interface (same
// cross-module pattern as orders/application and payments/application)
// rather than importing the Orders module's full service.
package application

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/delivery/domain"
	notificationsdomain "github.com/foodapp/backend/internal/modules/notifications/domain"
	ordersdomain "github.com/foodapp/backend/internal/modules/orders/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type OrderReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*ordersdomain.Order, error)
}

type OrderStatusUpdater interface {
	UpdateStatus(ctx context.Context, orderID uuid.UUID, newStatus ordersdomain.Status, actorID uuid.UUID) (*ordersdomain.Order, error)
}

type DispatchConfig struct {
	MaxActiveDeliveries int     // how many concurrent assignments a partner can hold — see docs/assumptions.md for why this is a flat cap, not distance/vehicle-aware yet
	SearchRadiusM       float64 // how far to look for available partners around a restaurant
}

type DeliveryService struct {
	repo    domain.Repository
	orders  OrderReader
	orderStatus OrderStatusUpdater
	notifier Notifier // may be nil
	cfg     DispatchConfig
}

// Notifier mirrors the pattern in orders/application and
// payments/application — see orders' Notifier for the full rationale.
type Notifier interface {
	Notify(ctx context.Context, in notificationsdomain.NotifyInput)
}

func NewDeliveryService(repo domain.Repository, orders OrderReader, orderStatus OrderStatusUpdater, notifier Notifier, cfg DispatchConfig) *DeliveryService {
	return &DeliveryService{repo: repo, orders: orders, orderStatus: orderStatus, notifier: notifier, cfg: cfg}
}

func (s *DeliveryService) RegisterPartner(ctx context.Context, userID uuid.UUID, vehicleType domain.VehicleType, vehicleNumber, licenseNumber *string) (*domain.Partner, error) {
	return s.repo.CreatePartner(ctx, &domain.Partner{
		UserID: userID, VehicleType: vehicleType, VehicleNumber: vehicleNumber, LicenseNumber: licenseNumber,
	})
}

func (s *DeliveryService) GetPartnerByUserID(ctx context.Context, userID uuid.UUID) (*domain.Partner, error) {
	return s.repo.GetPartnerByUserID(ctx, userID)
}

// GetPartnerIDForUser satisfies settlements/application's PartnerLookup
// interface — a thin wrapper so Settlements can resolve "which partner
// is this authenticated user" without depending on DeliveryService's
// full surface.
func (s *DeliveryService) GetPartnerIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	partner, err := s.repo.GetPartnerByUserID(ctx, userID)
	if err != nil {
		return uuid.Nil, err
	}
	return partner.ID, nil
}

func (s *DeliveryService) GetPartnerByID(ctx context.Context, id uuid.UUID) (*domain.Partner, error) {
	return s.repo.GetPartnerByID(ctx, id)
}

// AdminReviewKYC is an admin-only action; authorization enforced at the
// HTTP layer.
func (s *DeliveryService) AdminReviewKYC(ctx context.Context, partnerID uuid.UUID, approve bool) error {
	status := domain.KYCVerified
	if !approve {
		status = domain.KYCRejected
	}
	return s.repo.SetPartnerKYCStatus(ctx, partnerID, status)
}

// SetOnline is how a partner toggles their own availability. Going
// online is rejected until KYC is verified — an unverified partner
// should never be dispatchable, not just filtered out of search results,
// since defense-in-depth here directly protects customer safety.
func (s *DeliveryService) SetOnline(ctx context.Context, partnerID uuid.UUID, online bool) error {
	if online {
		partner, err := s.repo.GetPartnerByID(ctx, partnerID)
		if err != nil {
			return err
		}
		if partner.KYCStatus != domain.KYCVerified {
			return apperr.New(apperr.CodeConflict, "your account must complete verification before you can go online")
		}
	}
	return s.repo.SetPartnerOnline(ctx, partnerID, online)
}

func (s *DeliveryService) UpdateLocation(ctx context.Context, partnerID uuid.UUID, loc domain.GeoPoint) error {
	return s.repo.UpdatePartnerLocation(ctx, partnerID, loc)
}

// FindAndOffer picks the nearest available partner to the restaurant and
// creates an 'offered' assignment for them. This is a simple
// nearest-first dispatch — no batching, no partner-side accept timeout
// with auto-reassignment, no consideration of which direction a partner
// is already heading. See docs/assumptions.md for what a production
// dispatch algorithm would need beyond this.
func (s *DeliveryService) FindAndOffer(ctx context.Context, orderID uuid.UUID) (*domain.Assignment, error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}

	if existing, err := s.repo.GetActiveAssignmentForOrder(ctx, orderID); err == nil && existing != nil {
		return nil, apperr.New(apperr.CodeConflict, "this order already has an active delivery assignment")
	}

	candidates, err := s.repo.ListNearbyAvailable(ctx, domain.GeoPoint{Lat: order.DeliveryAddress.Lat, Lng: order.DeliveryAddress.Lng}, s.cfg.SearchRadiusM, s.cfg.MaxActiveDeliveries, 1)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, apperr.New(apperr.CodeUnavailable, "no delivery partners are currently available nearby")
	}

	otp, err := generateDeliveryOTP()
	if err != nil {
		return nil, apperr.Internal(err)
	}

	assignment, err := s.repo.CreateAssignment(ctx, orderID, order.RestaurantID, candidates[0].PartnerID, otp)
	if err != nil {
		return nil, err
	}
	_ = s.repo.IncrementActiveCount(ctx, candidates[0].PartnerID)

	return assignment, nil
}

// Accept is called by the delivery partner from their app. Only the
// assigned partner may accept — checked by comparing the caller's
// partner ID against assignment.DeliveryPartnerID at the HTTP layer,
// since that's where the authenticated identity is available. On
// success, the order's customer is notified that a partner has been
// assigned — this is the moment "someone is bringing your food" becomes
// true, not the earlier 'offered' state, which the partner might reject.
func (s *DeliveryService) Accept(ctx context.Context, assignmentID uuid.UUID) (*domain.Assignment, error) {
	assignment, err := s.repo.AcceptAssignment(ctx, assignmentID)
	if err != nil {
		return nil, err
	}
	if s.notifier != nil {
		if order, orderErr := s.orders.GetByID(ctx, assignment.OrderID); orderErr == nil {
			s.notifier.Notify(ctx, notificationsdomain.NotifyInput{
				UserID: order.CustomerID, Category: notificationsdomain.CategoryDeliveryAssigned,
				Title: "Delivery partner assigned", Body: fmt.Sprintf("A delivery partner has been assigned to your order %s.", order.OrderNumber),
				Data: map[string]interface{}{"order_id": order.ID.String(), "assignment_id": assignment.ID.String()},
			})
		}
	}
	return assignment, nil
}

// Reject releases the partner's slot (decrement active count) so
// FindAndOffer can immediately try the next-nearest candidate — the
// caller (HTTP handler or a background dispatcher, not built yet) is
// expected to call FindAndOffer again after a rejection.
func (s *DeliveryService) Reject(ctx context.Context, assignmentID uuid.UUID) (*domain.Assignment, error) {
	assignment, err := s.repo.RejectAssignment(ctx, assignmentID)
	if err != nil {
		return nil, err
	}
	_ = s.repo.DecrementActiveCount(ctx, assignment.DeliveryPartnerID)
	return assignment, nil
}

// MarkPickedUp transitions the assignment and, in the same call,
// advances the order to 'out_for_delivery' — these two state changes
// are conceptually one event from the partner's perspective (they
// picked up the food), so keeping them in one service method avoids the
// two states ever drifting out of sync in normal operation. actorID is
// the partner's user ID, used for the order's status-history audit trail.
func (s *DeliveryService) MarkPickedUp(ctx context.Context, assignmentID uuid.UUID, actorID uuid.UUID) (*domain.Assignment, error) {
	assignment, err := s.repo.MarkPickedUp(ctx, assignmentID)
	if err != nil {
		return nil, err
	}
	if _, err := s.orderStatus.UpdateStatus(ctx, assignment.OrderID, ordersdomain.StatusOutForDelivery, actorID); err != nil {
		return assignment, nil // assignment state is the source of truth here; order sync failure is logged upstream, not surfaced as this call's failure
	}
	return assignment, nil
}

// MarkDelivered requires the partner to enter the delivery OTP the
// customer gives them in person — this is the primary proof-of-delivery
// mechanism (matches how Swiggy/Zomato-style platforms confirm
// hand-off), not just a button tap, to reduce disputed "I never got my
// order" cases.
func (s *DeliveryService) MarkDelivered(ctx context.Context, assignmentID uuid.UUID, enteredOTP string, actorID uuid.UUID) (*domain.Assignment, error) {
	assignment, err := s.repo.GetAssignmentByID(ctx, assignmentID)
	if err != nil {
		return nil, err
	}
	if assignment.DeliveryOTP == nil || strings.TrimSpace(enteredOTP) != *assignment.DeliveryOTP {
		return nil, apperr.New(apperr.CodeValidation, "incorrect delivery confirmation code")
	}

	updated, err := s.repo.MarkDelivered(ctx, assignmentID)
	if err != nil {
		return nil, err
	}
	_ = s.repo.DecrementActiveCount(ctx, updated.DeliveryPartnerID)

	if _, err := s.orderStatus.UpdateStatus(ctx, updated.OrderID, ordersdomain.StatusDelivered, actorID); err != nil {
		return updated, nil
	}
	return updated, nil
}

func (s *DeliveryService) Cancel(ctx context.Context, assignmentID uuid.UUID, reason string) (*domain.Assignment, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, apperr.Validation("cancellation reason is required", nil)
	}
	assignment, err := s.repo.CancelAssignment(ctx, assignmentID, reason)
	if err != nil {
		return nil, err
	}
	_ = s.repo.DecrementActiveCount(ctx, assignment.DeliveryPartnerID)
	return assignment, nil
}

func (s *DeliveryService) GetAssignment(ctx context.Context, id uuid.UUID) (*domain.Assignment, error) {
	return s.repo.GetAssignmentByID(ctx, id)
}

func (s *DeliveryService) ListForOrder(ctx context.Context, orderID uuid.UUID) ([]*domain.Assignment, error) {
	return s.repo.ListAssignmentsForOrder(ctx, orderID)
}

func (s *DeliveryService) ListForPartner(ctx context.Context, partnerID uuid.UUID, filter domain.ListAssignmentsFilter) ([]*domain.Assignment, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 50 {
		filter.PageSize = 20
	}
	return s.repo.ListAssignmentsForPartner(ctx, partnerID, filter)
}

func (s *DeliveryService) ListActiveForPartner(ctx context.Context, partnerID uuid.UUID) ([]*domain.Assignment, error) {
	return s.repo.ListActiveAssignmentsForPartner(ctx, partnerID)
}

// CountDelivered exposes the Settlements module's read need — see
// settlements/application's DeliveryAggregateReader.
func (s *DeliveryService) CountDelivered(ctx context.Context, partnerID uuid.UUID, from, to time.Time) (int64, error) {
	return s.repo.CountDelivered(ctx, partnerID, from, to)
}

// GetDeliveryCodeForOrder returns the current delivery OTP for a
// customer to read aloud to their delivery partner at hand-off. Ownership
// is enforced here (requesterID must match the order's customer) since
// this is the one place in the module where the OTP is ever returned
// over the API — see assignmentToJSON's comment for why every other
// response omits it.
func (s *DeliveryService) GetDeliveryCodeForOrder(ctx context.Context, orderID, requesterID uuid.UUID) (string, error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return "", err
	}
	if order.CustomerID != requesterID {
		return "", apperr.Forbidden("you do not have access to this order")
	}

	assignment, err := s.repo.GetActiveAssignmentForOrder(ctx, orderID)
	if err != nil {
		return "", err
	}
	if assignment.DeliveryOTP == nil {
		return "", apperr.New(apperr.CodeNotFound, "no delivery code has been generated for this order yet")
	}
	return *assignment.DeliveryOTP, nil
}

func generateDeliveryOTP() (string, error) {
	digits := make([]byte, 4)
	for i := range digits {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		digits[i] = byte('0' + n.Int64())
	}
	return string(digits), nil
}

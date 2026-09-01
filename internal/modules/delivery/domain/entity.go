package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type VehicleType string

const (
	VehicleBike    VehicleType = "bike"
	VehicleScooter VehicleType = "scooter"
	VehicleBicycle VehicleType = "bicycle"
	VehicleCar     VehicleType = "car"
	VehicleOnFoot  VehicleType = "on_foot"
)

type KYCStatus string

const (
	KYCPending     KYCStatus = "pending"
	KYCUnderReview KYCStatus = "under_review"
	KYCVerified    KYCStatus = "verified"
	KYCRejected    KYCStatus = "rejected"
)

type AssignmentStatus string

const (
	AssignmentOffered   AssignmentStatus = "offered"
	AssignmentAccepted  AssignmentStatus = "accepted"
	AssignmentRejected  AssignmentStatus = "rejected"
	AssignmentPickedUp  AssignmentStatus = "picked_up"
	AssignmentDelivered AssignmentStatus = "delivered"
	AssignmentCancelled AssignmentStatus = "cancelled"
)

// allowedTransitions mirrors the pattern established in orders/domain —
// see that package's comment for the rationale of enforcing this in the
// domain layer rather than scattering checks through application code.
var allowedTransitions = map[AssignmentStatus][]AssignmentStatus{
	AssignmentOffered:   {AssignmentAccepted, AssignmentRejected, AssignmentCancelled},
	AssignmentAccepted:  {AssignmentPickedUp, AssignmentCancelled},
	AssignmentPickedUp:  {AssignmentDelivered, AssignmentCancelled},
	AssignmentDelivered: {},
	AssignmentRejected:  {},
	AssignmentCancelled: {},
}

func (s AssignmentStatus) CanTransitionTo(target AssignmentStatus) bool {
	for _, allowed := range allowedTransitions[s] {
		if allowed == target {
			return true
		}
	}
	return false
}

func (s AssignmentStatus) IsTerminal() bool {
	return s == AssignmentDelivered || s == AssignmentRejected || s == AssignmentCancelled
}

type GeoPoint struct {
	Lat float64
	Lng float64
}

type Partner struct {
	ID                     uuid.UUID
	UserID                 uuid.UUID
	VehicleType            VehicleType
	VehicleNumber          *string
	LicenseNumber          *string
	KYCStatus              KYCStatus
	IsOnline               bool
	CurrentLocation        *GeoPoint
	LastLocationUpdateAt   *time.Time
	RatingAvg              float64
	RatingCount            int
	ActiveAssignmentCount  int
}

func (p *Partner) IsAvailable(maxActive int) bool {
	return p.KYCStatus == KYCVerified && p.IsOnline && p.ActiveAssignmentCount < maxActive
}

type NearbyPartner struct {
	PartnerID  uuid.UUID
	UserID     uuid.UUID
	DistanceKM float64
}

type Assignment struct {
	ID                  uuid.UUID
	OrderID             uuid.UUID
	RestaurantID        uuid.UUID
	DeliveryPartnerID   uuid.UUID
	Status              AssignmentStatus
	DeliveryOTP         *string
	OfferedAt           time.Time
	AcceptedAt          *time.Time
	RejectedAt          *time.Time
	PickedUpAt          *time.Time
	DeliveredAt         *time.Time
	CancelledAt         *time.Time
	CancellationReason  *string
}

type ListAssignmentsFilter struct {
	Status   *AssignmentStatus
	Page     int
	PageSize int
}

// Repository is the persistence port for the Delivery module.
type Repository interface {
	CreatePartner(ctx context.Context, p *Partner) (*Partner, error)
	GetPartnerByID(ctx context.Context, id uuid.UUID) (*Partner, error)
	GetPartnerByUserID(ctx context.Context, userID uuid.UUID) (*Partner, error)
	SetPartnerKYCStatus(ctx context.Context, id uuid.UUID, status KYCStatus) error
	SetPartnerOnline(ctx context.Context, id uuid.UUID, online bool) error
	UpdatePartnerLocation(ctx context.Context, id uuid.UUID, loc GeoPoint) error
	IncrementActiveCount(ctx context.Context, id uuid.UUID) error
	DecrementActiveCount(ctx context.Context, id uuid.UUID) error
	ListNearbyAvailable(ctx context.Context, loc GeoPoint, radiusM float64, maxActive, limit int) ([]*NearbyPartner, error)

	CreateAssignment(ctx context.Context, orderID, restaurantID, partnerID uuid.UUID, otp string) (*Assignment, error)
	GetAssignmentByID(ctx context.Context, id uuid.UUID) (*Assignment, error)
	GetActiveAssignmentForOrder(ctx context.Context, orderID uuid.UUID) (*Assignment, error)
	ListAssignmentsForOrder(ctx context.Context, orderID uuid.UUID) ([]*Assignment, error)
	ListAssignmentsForPartner(ctx context.Context, partnerID uuid.UUID, filter ListAssignmentsFilter) ([]*Assignment, error)
	ListActiveAssignmentsForPartner(ctx context.Context, partnerID uuid.UUID) ([]*Assignment, error)
	AcceptAssignment(ctx context.Context, id uuid.UUID) (*Assignment, error)
	RejectAssignment(ctx context.Context, id uuid.UUID) (*Assignment, error)
	MarkPickedUp(ctx context.Context, id uuid.UUID) (*Assignment, error)
	MarkDelivered(ctx context.Context, id uuid.UUID) (*Assignment, error)
	CancelAssignment(ctx context.Context, id uuid.UUID, reason string) (*Assignment, error)

	// CountDelivered supports the Settlements module — see
	// db/queries/delivery.sql's CountDeliveredForPartner comment.
	CountDelivered(ctx context.Context, partnerID uuid.UUID, from, to time.Time) (int64, error)
}

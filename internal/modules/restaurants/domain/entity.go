package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusDraft            Status = "draft"
	StatusPendingApproval  Status = "pending_approval"
	StatusApproved         Status = "approved"
	StatusRejected         Status = "rejected"
	StatusSuspended        Status = "suspended"
	StatusClosed           Status = "closed"
)

type KYCStatus string

const (
	KYCPending     KYCStatus = "pending"
	KYCUnderReview KYCStatus = "under_review"
	KYCVerified    KYCStatus = "verified"
	KYCRejected    KYCStatus = "rejected"
)

type DocumentType string

const (
	DocFSSAILicense        DocumentType = "fssai_license"
	DocGSTCertificate      DocumentType = "gst_certificate"
	DocPANCard             DocumentType = "pan_card"
	DocBusinessRegistration DocumentType = "business_registration"
	DocBankAccountProof    DocumentType = "bank_account_proof"
	DocOwnerIdentityProof  DocumentType = "owner_identity_proof"
)

type StaffPermission string

const (
	PermManageMenu   StaffPermission = "manage_menu"
	PermManageOrders StaffPermission = "manage_orders"
	PermViewEarnings StaffPermission = "view_earnings"
	PermManageHours  StaffPermission = "manage_hours"
	PermManageStaff  StaffPermission = "manage_staff"
)

type GeoPoint struct {
	Lat float64
	Lng float64
}

type Restaurant struct {
	ID                uuid.UUID
	OwnerUserID       uuid.UUID
	Name              string
	Slug              string
	Description       *string
	CuisineTags       []string
	Status            Status
	KYCStatus         KYCStatus
	IsVegOnly         bool
	AvgPrepTimeMins   int
	MinOrderAmount    float64
	CommissionPct     float64
	LogoURL           *string
	BannerURL         *string
	AddressLine1      string
	AddressLine2      *string
	City              string
	State             string
	PostalCode        string
	Country           string
	Location          GeoPoint
	RatingAvg         float64
	RatingCount       int
	IsAcceptingOrders bool
	ApprovedAt        *time.Time
	ApprovedBy        *uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DistanceKM        *float64 // populated only on nearby-search results
}

func (r *Restaurant) IsLive() bool {
	return r.Status == StatusApproved && r.IsAcceptingOrders
}

type CreateRestaurantInput struct {
	OwnerUserID  uuid.UUID
	Name         string
	Description  *string
	CuisineTags  []string
	IsVegOnly    bool
	AddressLine1 string
	AddressLine2 *string
	City         string
	State        string
	PostalCode   string
	Country      string
	Location     GeoPoint
}

type UpdateRestaurantInput struct {
	Name            *string
	Description     *string
	CuisineTags     []string
	LogoURL         *string
	BannerURL       *string
	MinOrderAmount  *float64
	AvgPrepTimeMins *int
}

type NearbySearchInput struct {
	Location       GeoPoint
	SearchRadiusM  float64
	Page           int
	PageSize       int
}

type AdminListFilter struct {
	Status   *Status
	Page     int
	PageSize int
}

type OperatingHours struct {
	ID          uuid.UUID
	RestaurantID uuid.UUID
	DayOfWeek   int // 0=Sunday
	OpenTime    string // "HH:MM:SS"
	CloseTime   string
	IsClosed    bool
}

type ServiceArea struct {
	ID           uuid.UUID
	RestaurantID uuid.UUID
	RadiusKM     float64
	IsActive     bool
}

type Document struct {
	ID               uuid.UUID
	RestaurantID     uuid.UUID
	DocumentType     DocumentType
	FileURL          string
	DocumentNumber   *string
	Status           KYCStatus
	RejectionReason  *string
	ReviewedBy       *uuid.UUID
	ReviewedAt       *time.Time
	ExpiresAt        *time.Time
	CreatedAt        time.Time
}

type StaffMember struct {
	ID           uuid.UUID
	RestaurantID uuid.UUID
	UserID       uuid.UUID
	Permissions  []StaffPermission
	InvitedBy    uuid.UUID
	Status       string
	CreatedAt    time.Time
}

// Repository is the persistence port for the Restaurants module.
type Repository interface {
	Create(ctx context.Context, in CreateRestaurantInput, slug string) (*Restaurant, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Restaurant, error)
	GetBySlug(ctx context.Context, slug string) (*Restaurant, error)
	ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]*Restaurant, error)
	ListNearby(ctx context.Context, in NearbySearchInput) ([]*Restaurant, error)
	ListForAdmin(ctx context.Context, filter AdminListFilter) ([]*Restaurant, int64, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, in UpdateRestaurantInput) (*Restaurant, error)
	SetStatus(ctx context.Context, id uuid.UUID, status Status, approvedBy *uuid.UUID) error
	SetAcceptingOrders(ctx context.Context, id uuid.UUID, accepting bool) error
	SetKYCStatus(ctx context.Context, id uuid.UUID, status KYCStatus) error
	SoftDelete(ctx context.Context, id uuid.UUID) error

	UpsertOperatingHours(ctx context.Context, h *OperatingHours) (*OperatingHours, error)
	ListOperatingHours(ctx context.Context, restaurantID uuid.UUID) ([]*OperatingHours, error)

	UpsertServiceArea(ctx context.Context, restaurantID uuid.UUID, radiusKM float64, isActive bool) (*ServiceArea, error)

	UpsertDocument(ctx context.Context, d *Document) (*Document, error)
	ListDocuments(ctx context.Context, restaurantID uuid.UUID) ([]*Document, error)
	ReviewDocument(ctx context.Context, id uuid.UUID, status KYCStatus, rejectionReason *string, reviewedBy uuid.UUID) (*Document, error)

	AddStaff(ctx context.Context, s *StaffMember) (*StaffMember, error)
	ListStaff(ctx context.Context, restaurantID uuid.UUID) ([]*StaffMember, error)
	RevokeStaff(ctx context.Context, id uuid.UUID) error
}

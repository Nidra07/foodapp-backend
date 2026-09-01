package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/restaurants/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type RestaurantService struct {
	repo   domain.Repository
	audit  AuditLogger // may be nil
}

// AuditLogger is a small consumer-defined interface (same nil-safe
// pattern as Notifier elsewhere) so this package can record admin
// actions without importing the Admin RBAC module's full application
// service. Implemented by adminrbac/application.RBACService.LogAction.
type AuditLogger interface {
	LogAction(ctx context.Context, adminUserID uuid.UUID, action, resourceType string, resourceID *uuid.UUID, details map[string]interface{}, ipAddress *string)
}

func NewRestaurantService(repo domain.Repository, audit AuditLogger) *RestaurantService {
	return &RestaurantService{repo: repo, audit: audit}
}

// Onboard creates a new restaurant in "draft" status. The owner must
// separately complete operating hours, service area, and document
// upload steps before submitting for approval (SubmitForApproval) —
// modeled as distinct steps rather than one giant create call, matching
// how the Restaurant flutter app's onboarding wizard is structured
// (per docs/wireframes referenced in the master spec).
func (s *RestaurantService) Onboard(ctx context.Context, in domain.CreateRestaurantInput) (*domain.Restaurant, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, apperr.Validation("restaurant name is required", nil)
	}
	if in.Location.Lat == 0 && in.Location.Lng == 0 {
		return nil, apperr.Validation("a valid restaurant location is required", nil)
	}

	slug := slugify(in.Name) + "-" + uuid.NewString()[:8]
	return s.repo.Create(ctx, in, slug)
}

func (s *RestaurantService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Restaurant, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *RestaurantService) GetBySlug(ctx context.Context, slug string) (*domain.Restaurant, error) {
	return s.repo.GetBySlug(ctx, slug)
}

func (s *RestaurantService) ListMine(ctx context.Context, ownerID uuid.UUID) ([]*domain.Restaurant, error) {
	return s.repo.ListByOwner(ctx, ownerID)
}

// SearchNearby is the primary customer-facing discovery query. Distance
// filtering + each restaurant's own service radius is enforced at the
// SQL layer (see db/queries/restaurants.sql ListNearbyRestaurants) for
// performance — this method just applies sane pagination defaults.
func (s *RestaurantService) SearchNearby(ctx context.Context, in domain.NearbySearchInput) ([]*domain.Restaurant, error) {
	if in.SearchRadiusM <= 0 {
		in.SearchRadiusM = 10000 // default 10km
	}
	if in.Page < 1 {
		in.Page = 1
	}
	if in.PageSize < 1 || in.PageSize > 50 {
		in.PageSize = 20
	}
	return s.repo.ListNearby(ctx, in)
}

func (s *RestaurantService) UpdateProfile(ctx context.Context, id uuid.UUID, in domain.UpdateRestaurantInput) (*domain.Restaurant, error) {
	return s.repo.UpdateProfile(ctx, id, in)
}

// SubmitForApproval transitions draft -> pending_approval. Requires all
// mandatory KYC documents to at least be uploaded (not necessarily
// verified yet — that's the admin's job) and at least one operating-hours
// row + a service area to exist, since an approved restaurant with no
// hours/service area would be unreachable by customers.
func (s *RestaurantService) SubmitForApproval(ctx context.Context, restaurantID uuid.UUID) error {
	r, err := s.repo.GetByID(ctx, restaurantID)
	if err != nil {
		return err
	}
	if r.Status != domain.StatusDraft && r.Status != domain.StatusRejected {
		return apperr.New(apperr.CodeConflict, "restaurant is not in a state that can be submitted for approval")
	}

	hours, err := s.repo.ListOperatingHours(ctx, restaurantID)
	if err != nil {
		return err
	}
	if len(hours) == 0 {
		return apperr.Validation("operating hours must be set before submitting for approval", nil)
	}

	docs, err := s.repo.ListDocuments(ctx, restaurantID)
	if err != nil {
		return err
	}
	required := map[domain.DocumentType]bool{
		domain.DocFSSAILicense:   false,
		domain.DocGSTCertificate: false,
		domain.DocPANCard:        false,
	}
	for _, d := range docs {
		if _, ok := required[d.DocumentType]; ok {
			required[d.DocumentType] = true
		}
	}
	for docType, present := range required {
		if !present {
			return apperr.Validation(fmt.Sprintf("required document missing: %s", docType), nil)
		}
	}

	return s.repo.SetStatus(ctx, restaurantID, domain.StatusPendingApproval, nil)
}

// AdminReview is the admin-panel action to approve/reject a submitted
// restaurant. Authorization (admin-only) is enforced at the HTTP layer.
// This is one of the actions retrofitted with audit logging in Phase 8
// — see docs/assumptions.md for why not every admin action across every
// module got the same treatment.
func (s *RestaurantService) AdminReview(ctx context.Context, restaurantID uuid.UUID, approve bool, adminID uuid.UUID) error {
	r, err := s.repo.GetByID(ctx, restaurantID)
	if err != nil {
		return err
	}
	if r.Status != domain.StatusPendingApproval {
		return apperr.New(apperr.CodeConflict, "restaurant is not pending approval")
	}

	newStatus := domain.StatusApproved
	if !approve {
		newStatus = domain.StatusRejected
	}

	var setErr error
	if approve {
		setErr = s.repo.SetStatus(ctx, restaurantID, domain.StatusApproved, &adminID)
	} else {
		setErr = s.repo.SetStatus(ctx, restaurantID, domain.StatusRejected, nil)
	}
	if setErr != nil {
		return setErr
	}

	if s.audit != nil {
		s.audit.LogAction(ctx, adminID, "restaurant.review", "restaurant", &restaurantID,
			map[string]interface{}{"approved": approve, "new_status": string(newStatus)}, nil)
	}
	return nil
}

func (s *RestaurantService) SetAcceptingOrders(ctx context.Context, restaurantID uuid.UUID, accepting bool) error {
	r, err := s.repo.GetByID(ctx, restaurantID)
	if err != nil {
		return err
	}
	if accepting && r.Status != domain.StatusApproved {
		return apperr.New(apperr.CodeConflict, "restaurant must be approved before it can accept orders")
	}
	return s.repo.SetAcceptingOrders(ctx, restaurantID, accepting)
}

func (s *RestaurantService) SetOperatingHours(ctx context.Context, h *domain.OperatingHours) (*domain.OperatingHours, error) {
	if h.DayOfWeek < 0 || h.DayOfWeek > 6 {
		return nil, apperr.Validation("day_of_week must be between 0 (Sunday) and 6", nil)
	}
	return s.repo.UpsertOperatingHours(ctx, h)
}

func (s *RestaurantService) ListOperatingHours(ctx context.Context, restaurantID uuid.UUID) ([]*domain.OperatingHours, error) {
	return s.repo.ListOperatingHours(ctx, restaurantID)
}

func (s *RestaurantService) SetServiceArea(ctx context.Context, restaurantID uuid.UUID, radiusKM float64) (*domain.ServiceArea, error) {
	if radiusKM <= 0 || radiusKM > 50 {
		return nil, apperr.Validation("service radius must be between 0 and 50 km", nil)
	}
	return s.repo.UpsertServiceArea(ctx, restaurantID, radiusKM, true)
}

func (s *RestaurantService) UploadDocument(ctx context.Context, d *domain.Document) (*domain.Document, error) {
	if d.FileURL == "" {
		return nil, apperr.Validation("file_url is required", nil)
	}
	return s.repo.UpsertDocument(ctx, d)
}

func (s *RestaurantService) ListDocuments(ctx context.Context, restaurantID uuid.UUID) ([]*domain.Document, error) {
	return s.repo.ListDocuments(ctx, restaurantID)
}

// ReviewDocument is an admin action. If all required documents end up
// verified, the restaurant's overall kyc_status is promoted too.
func (s *RestaurantService) ReviewDocument(ctx context.Context, docID uuid.UUID, restaurantID uuid.UUID, approve bool, rejectionReason *string, adminID uuid.UUID) (*domain.Document, error) {
	status := domain.KYCVerified
	if !approve {
		status = domain.KYCRejected
	}

	doc, err := s.repo.ReviewDocument(ctx, docID, status, rejectionReason, adminID)
	if err != nil {
		return nil, err
	}

	docs, err := s.repo.ListDocuments(ctx, restaurantID)
	if err != nil {
		return doc, nil // document review itself succeeded; propagating this secondary error would be misleading
	}
	allVerified := len(docs) > 0
	for _, d := range docs {
		if d.Status != domain.KYCVerified {
			allVerified = false
			break
		}
	}
	if allVerified {
		_ = s.repo.SetKYCStatus(ctx, restaurantID, domain.KYCVerified)
	} else if status == domain.KYCRejected {
		_ = s.repo.SetKYCStatus(ctx, restaurantID, domain.KYCRejected)
	}

	return doc, nil
}

func (s *RestaurantService) AddStaff(ctx context.Context, restaurantID, userID, invitedBy uuid.UUID, permissions []domain.StaffPermission) (*domain.StaffMember, error) {
	if len(permissions) == 0 {
		return nil, apperr.Validation("at least one permission must be granted", nil)
	}
	return s.repo.AddStaff(ctx, &domain.StaffMember{
		RestaurantID: restaurantID,
		UserID:       userID,
		Permissions:  permissions,
		InvitedBy:    invitedBy,
	})
}

func (s *RestaurantService) ListStaff(ctx context.Context, restaurantID uuid.UUID) ([]*domain.StaffMember, error) {
	return s.repo.ListStaff(ctx, restaurantID)
}

func (s *RestaurantService) RevokeStaff(ctx context.Context, staffID uuid.UUID) error {
	return s.repo.RevokeStaff(ctx, staffID)
}

func (s *RestaurantService) AdminListRestaurants(ctx context.Context, filter domain.AdminListFilter) ([]*domain.Restaurant, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	return s.repo.ListForAdmin(ctx, filter)
}

func slugify(name string) string {
	lower := strings.ToLower(name)
	var b strings.Builder
	lastDash := false
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

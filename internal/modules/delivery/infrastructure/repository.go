package infrastructure

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/foodapp/backend/internal/modules/delivery/domain"
	sqlcgen "github.com/foodapp/backend/internal/platform/db/sqlc"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type Repository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) *Repository {
	return &Repository{q: q}
}

func (r *Repository) CreatePartner(ctx context.Context, p *domain.Partner) (*domain.Partner, error) {
	row, err := r.q.CreateDeliveryPartner(ctx, sqlcgen.CreateDeliveryPartnerParams{
		UserID: p.UserID, VehicleType: sqlcgen.VehicleType(p.VehicleType),
		VehicleNumber: toText(p.VehicleNumber), LicenseNumber: toText(p.LicenseNumber),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to register delivery partner", err)
	}
	return mapPartner(row), nil
}

func (r *Repository) GetPartnerByID(ctx context.Context, id uuid.UUID) (*domain.Partner, error) {
	row, err := r.q.GetDeliveryPartnerByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("delivery partner")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch delivery partner", err)
	}
	return mapPartner(row), nil
}

func (r *Repository) GetPartnerByUserID(ctx context.Context, userID uuid.UUID) (*domain.Partner, error) {
	row, err := r.q.GetDeliveryPartnerByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("delivery partner")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch delivery partner", err)
	}
	return mapPartner(row), nil
}

func (r *Repository) SetPartnerKYCStatus(ctx context.Context, id uuid.UUID, status domain.KYCStatus) error {
	if err := r.q.SetDeliveryPartnerKYCStatus(ctx, sqlcgen.SetDeliveryPartnerKYCStatusParams{ID: id, KycStatus: sqlcgen.KycStatus(status)}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update KYC status", err)
	}
	return nil
}

func (r *Repository) SetPartnerOnline(ctx context.Context, id uuid.UUID, online bool) error {
	if err := r.q.SetDeliveryPartnerOnline(ctx, sqlcgen.SetDeliveryPartnerOnlineParams{ID: id, IsOnline: online}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update availability", err)
	}
	return nil
}

func (r *Repository) UpdatePartnerLocation(ctx context.Context, id uuid.UUID, loc domain.GeoPoint) error {
	if err := r.q.UpdateDeliveryPartnerLocation(ctx, sqlcgen.UpdateDeliveryPartnerLocationParams{ID: id, Lng: loc.Lng, Lat: loc.Lat}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update location", err)
	}
	return nil
}

func (r *Repository) IncrementActiveCount(ctx context.Context, id uuid.UUID) error {
	if err := r.q.IncrementActiveAssignmentCount(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update assignment count", err)
	}
	return nil
}

func (r *Repository) DecrementActiveCount(ctx context.Context, id uuid.UUID) error {
	if err := r.q.DecrementActiveAssignmentCount(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update assignment count", err)
	}
	return nil
}

func (r *Repository) ListNearbyAvailable(ctx context.Context, loc domain.GeoPoint, radiusM float64, maxActive, limit int) ([]*domain.NearbyPartner, error) {
	rows, err := r.q.ListNearbyAvailablePartners(ctx, sqlcgen.ListNearbyAvailablePartnersParams{
		Lng: loc.Lng, Lat: loc.Lat, MaxActive: int16(maxActive), RadiusM: radiusM, Limit: int32(limit),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to search nearby partners", err)
	}
	out := make([]*domain.NearbyPartner, len(rows))
	for i, row := range rows {
		out[i] = &domain.NearbyPartner{PartnerID: row.ID, UserID: row.UserID, DistanceKM: row.DistanceKm}
	}
	return out, nil
}

func (r *Repository) CreateAssignment(ctx context.Context, orderID, restaurantID, partnerID uuid.UUID, otp string) (*domain.Assignment, error) {
	row, err := r.q.CreateDeliveryAssignment(ctx, sqlcgen.CreateDeliveryAssignmentParams{
		OrderID: orderID, RestaurantID: restaurantID, DeliveryPartnerID: partnerID, DeliveryOtp: toText(&otp),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create delivery assignment", err)
	}
	return mapAssignment(row), nil
}

func (r *Repository) GetAssignmentByID(ctx context.Context, id uuid.UUID) (*domain.Assignment, error) {
	row, err := r.q.GetDeliveryAssignmentByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("delivery assignment")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch delivery assignment", err)
	}
	return mapAssignment(row), nil
}

func (r *Repository) GetActiveAssignmentForOrder(ctx context.Context, orderID uuid.UUID) (*domain.Assignment, error) {
	row, err := r.q.GetActiveAssignmentForOrder(ctx, orderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("delivery assignment")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch delivery assignment", err)
	}
	return mapAssignment(row), nil
}

func (r *Repository) ListAssignmentsForOrder(ctx context.Context, orderID uuid.UUID) ([]*domain.Assignment, error) {
	rows, err := r.q.ListAssignmentsForOrder(ctx, orderID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list delivery assignments", err)
	}
	out := make([]*domain.Assignment, len(rows))
	for i, row := range rows {
		out[i] = mapAssignment(row)
	}
	return out, nil
}

func (r *Repository) ListAssignmentsForPartner(ctx context.Context, partnerID uuid.UUID, filter domain.ListAssignmentsFilter) ([]*domain.Assignment, error) {
	offset := (filter.Page - 1) * filter.PageSize
	var statusParam sqlcgen.NullAssignmentStatus
	if filter.Status != nil {
		statusParam = sqlcgen.NullAssignmentStatus{AssignmentStatus: sqlcgen.AssignmentStatus(*filter.Status), Valid: true}
	}
	rows, err := r.q.ListAssignmentsForPartner(ctx, sqlcgen.ListAssignmentsForPartnerParams{
		DeliveryPartnerID: partnerID, Status: statusParam, Limit: int32(filter.PageSize), Offset: int32(offset),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list delivery assignments", err)
	}
	out := make([]*domain.Assignment, len(rows))
	for i, row := range rows {
		out[i] = mapAssignment(row)
	}
	return out, nil
}

func (r *Repository) ListActiveAssignmentsForPartner(ctx context.Context, partnerID uuid.UUID) ([]*domain.Assignment, error) {
	rows, err := r.q.ListActiveAssignmentsForPartner(ctx, partnerID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list active assignments", err)
	}
	out := make([]*domain.Assignment, len(rows))
	for i, row := range rows {
		out[i] = mapAssignment(row)
	}
	return out, nil
}

func (r *Repository) AcceptAssignment(ctx context.Context, id uuid.UUID) (*domain.Assignment, error) {
	row, err := r.q.AcceptAssignment(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.New(apperr.CodeConflict, "assignment is not in a state that can be accepted")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to accept assignment", err)
	}
	return mapAssignment(row), nil
}

func (r *Repository) RejectAssignment(ctx context.Context, id uuid.UUID) (*domain.Assignment, error) {
	row, err := r.q.RejectAssignment(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.New(apperr.CodeConflict, "assignment is not in a state that can be rejected")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to reject assignment", err)
	}
	return mapAssignment(row), nil
}

func (r *Repository) MarkPickedUp(ctx context.Context, id uuid.UUID) (*domain.Assignment, error) {
	row, err := r.q.MarkPickedUp(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.New(apperr.CodeConflict, "assignment must be accepted before it can be marked picked up")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to mark picked up", err)
	}
	return mapAssignment(row), nil
}

func (r *Repository) MarkDelivered(ctx context.Context, id uuid.UUID) (*domain.Assignment, error) {
	row, err := r.q.MarkDelivered(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.New(apperr.CodeConflict, "assignment must be picked up before it can be marked delivered")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to mark delivered", err)
	}
	return mapAssignment(row), nil
}

func (r *Repository) CancelAssignment(ctx context.Context, id uuid.UUID, reason string) (*domain.Assignment, error) {
	row, err := r.q.CancelAssignment(ctx, sqlcgen.CancelAssignmentParams{ID: id, CancellationReason: toText(&reason)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to cancel assignment", err)
	}
	return mapAssignment(row), nil
}

func (r *Repository) CountDelivered(ctx context.Context, partnerID uuid.UUID, from, to time.Time) (int64, error) {
	count, err := r.q.CountDeliveredForPartner(ctx, sqlcgen.CountDeliveredForPartnerParams{
		DeliveryPartnerID: partnerID, FromTs: from, ToTs: to,
	})
	if err != nil {
		return 0, apperr.Wrap(apperr.CodeInternal, "failed to count deliveries", err)
	}
	return count, nil
}

// --- mapping helpers ---

func mapPartner(row sqlcgen.DeliveryPartner) *domain.Partner {
	ratingAvg, _ := row.RatingAvg.Float64Value()
	p := &domain.Partner{
		ID: row.ID, UserID: row.UserID, VehicleType: domain.VehicleType(row.VehicleType),
		KYCStatus: domain.KYCStatus(row.KycStatus), IsOnline: row.IsOnline,
		RatingAvg: ratingAvg.Float64, RatingCount: int(row.RatingCount), ActiveAssignmentCount: int(row.ActiveAssignmentCount),
	}
	if row.VehicleNumber.Valid {
		p.VehicleNumber = &row.VehicleNumber.String
	}
	if row.LicenseNumber.Valid {
		p.LicenseNumber = &row.LicenseNumber.String
	}
	if row.LastLocationUpdateAt.Valid {
		t := row.LastLocationUpdateAt.Time
		p.LastLocationUpdateAt = &t
	}
	// NOTE: row.CurrentLocation (PostGIS geography) intentionally not
	// decoded — same documented limitation as restaurants.location, see
	// docs/assumptions.md Phase 2 section. Distance/nearby queries already
	// return computed distance_km separately, so raw coordinates aren't
	// needed for any Phase 5 flow yet.
	return p
}

func mapAssignment(row sqlcgen.DeliveryAssignment) *domain.Assignment {
	a := &domain.Assignment{
		ID: row.ID, OrderID: row.OrderID, RestaurantID: row.RestaurantID, DeliveryPartnerID: row.DeliveryPartnerID,
		Status: domain.AssignmentStatus(row.Status), OfferedAt: row.OfferedAt,
	}
	if row.DeliveryOtp.Valid {
		a.DeliveryOTP = &row.DeliveryOtp.String
	}
	if row.AcceptedAt.Valid {
		t := row.AcceptedAt.Time
		a.AcceptedAt = &t
	}
	if row.RejectedAt.Valid {
		t := row.RejectedAt.Time
		a.RejectedAt = &t
	}
	if row.PickedUpAt.Valid {
		t := row.PickedUpAt.Time
		a.PickedUpAt = &t
	}
	if row.DeliveredAt.Valid {
		t := row.DeliveredAt.Time
		a.DeliveredAt = &t
	}
	if row.CancelledAt.Valid {
		t := row.CancelledAt.Time
		a.CancelledAt = &t
	}
	if row.CancellationReason.Valid {
		a.CancellationReason = &row.CancellationReason.String
	}
	return a
}

func toText(s *string) pgtype.Text {
	if s == nil || *s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

var _ domain.Repository = (*Repository)(nil)

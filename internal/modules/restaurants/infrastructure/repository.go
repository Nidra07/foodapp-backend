package infrastructure

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/foodapp/backend/internal/modules/restaurants/domain"
	sqlcgen "github.com/foodapp/backend/internal/platform/db/sqlc"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type Repository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) *Repository {
	return &Repository{q: q}
}

func (r *Repository) Create(ctx context.Context, in domain.CreateRestaurantInput, slug string) (*domain.Restaurant, error) {
	row, err := r.q.CreateRestaurant(ctx, sqlcgen.CreateRestaurantParams{
		OwnerUserID:  in.OwnerUserID,
		Name:         in.Name,
		Slug:         slug,
		Description:  toText(in.Description),
		CuisineTags:  in.CuisineTags,
		IsVegOnly:    in.IsVegOnly,
		AddressLine1: in.AddressLine1,
		AddressLine2: toText(in.AddressLine2),
		City:         in.City,
		State:        in.State,
		PostalCode:   in.PostalCode,
		Country:      orDefault(in.Country, "IN"),
		Lng:          in.Location.Lng,
		Lat:          in.Location.Lat,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create restaurant", err)
	}
	return mapRestaurant(row), nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Restaurant, error) {
	row, err := r.q.GetRestaurantByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("restaurant")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch restaurant", err)
	}
	return mapRestaurant(row), nil
}

func (r *Repository) GetBySlug(ctx context.Context, slug string) (*domain.Restaurant, error) {
	row, err := r.q.GetRestaurantBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("restaurant")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch restaurant", err)
	}
	return mapRestaurant(row), nil
}

func (r *Repository) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]*domain.Restaurant, error) {
	rows, err := r.q.ListRestaurantsByOwner(ctx, ownerID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list restaurants", err)
	}
	out := make([]*domain.Restaurant, len(rows))
	for i, row := range rows {
		out[i] = mapRestaurant(row)
	}
	return out, nil
}

func (r *Repository) ListNearby(ctx context.Context, in domain.NearbySearchInput) ([]*domain.Restaurant, error) {
	offset := (in.Page - 1) * in.PageSize
	rows, err := r.q.ListNearbyRestaurants(ctx, sqlcgen.ListNearbyRestaurantsParams{
		Lng:            in.Location.Lng,
		Lat:            in.Location.Lat,
		SearchRadiusM:  in.SearchRadiusM,
		Limit:          int32(in.PageSize),
		Offset:         int32(offset),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to search nearby restaurants", err)
	}
	out := make([]*domain.Restaurant, len(rows))
	for i, row := range rows {
		rest := mapRestaurant(row.Restaurant)
		dist := row.DistanceKm
		rest.DistanceKM = &dist
		out[i] = rest
	}
	return out, nil
}

func (r *Repository) ListForAdmin(ctx context.Context, filter domain.AdminListFilter) ([]*domain.Restaurant, int64, error) {
	offset := (filter.Page - 1) * filter.PageSize
	var statusParam sqlcgen.NullRestaurantStatus
	if filter.Status != nil {
		statusParam = sqlcgen.NullRestaurantStatus{RestaurantStatus: sqlcgen.RestaurantStatus(*filter.Status), Valid: true}
	}

	rows, err := r.q.ListRestaurantsForAdmin(ctx, sqlcgen.ListRestaurantsForAdminParams{
		Status: statusParam,
		Limit:  int32(filter.PageSize),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, 0, apperr.Wrap(apperr.CodeInternal, "failed to list restaurants", err)
	}

	total, err := r.q.CountRestaurantsForAdmin(ctx, statusParam)
	if err != nil {
		return nil, 0, apperr.Wrap(apperr.CodeInternal, "failed to count restaurants", err)
	}

	out := make([]*domain.Restaurant, len(rows))
	for i, row := range rows {
		out[i] = mapRestaurant(row)
	}
	return out, total, nil
}

func (r *Repository) UpdateProfile(ctx context.Context, id uuid.UUID, in domain.UpdateRestaurantInput) (*domain.Restaurant, error) {
	var minOrder pgtype.Numeric
	if in.MinOrderAmount != nil {
		_ = minOrder.Scan(*in.MinOrderAmount)
	}
	var avgPrep pgtype.Int2
	if in.AvgPrepTimeMins != nil {
		avgPrep = pgtype.Int2{Int16: int16(*in.AvgPrepTimeMins), Valid: true}
	}

	row, err := r.q.UpdateRestaurantProfile(ctx, sqlcgen.UpdateRestaurantProfileParams{
		ID:              id,
		Name:            toText(in.Name),
		Description:     toText(in.Description),
		CuisineTags:     in.CuisineTags,
		LogoUrl:         toText(in.LogoURL),
		BannerUrl:       toText(in.BannerURL),
		MinOrderAmount:  minOrder,
		AvgPrepTimeMins: avgPrep,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to update restaurant profile", err)
	}
	return mapRestaurant(row), nil
}

func (r *Repository) SetStatus(ctx context.Context, id uuid.UUID, status domain.Status, approvedBy *uuid.UUID) error {
	var approver pgtype.UUID
	if approvedBy != nil {
		approver = pgtype.UUID{Bytes: *approvedBy, Valid: true}
	}
	if err := r.q.SetRestaurantStatus(ctx, sqlcgen.SetRestaurantStatusParams{
		ID: id, Status: sqlcgen.RestaurantStatus(status), ApprovedBy: approver,
	}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to set restaurant status", err)
	}
	return nil
}

func (r *Repository) SetAcceptingOrders(ctx context.Context, id uuid.UUID, accepting bool) error {
	if err := r.q.SetRestaurantAcceptingOrders(ctx, sqlcgen.SetRestaurantAcceptingOrdersParams{ID: id, IsAcceptingOrders: accepting}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update accepting-orders flag", err)
	}
	return nil
}

func (r *Repository) SetKYCStatus(ctx context.Context, id uuid.UUID, status domain.KYCStatus) error {
	if err := r.q.SetRestaurantKYCStatus(ctx, sqlcgen.SetRestaurantKYCStatusParams{ID: id, KycStatus: sqlcgen.KycStatus(status)}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update KYC status", err)
	}
	return nil
}

func (r *Repository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	if err := r.q.SoftDeleteRestaurant(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to close restaurant", err)
	}
	return nil
}

func (r *Repository) UpsertOperatingHours(ctx context.Context, h *domain.OperatingHours) (*domain.OperatingHours, error) {
	openT, err := parseTimeOfDay(h.OpenTime)
	if err != nil {
		return nil, apperr.Validation("invalid open_time format, expected HH:MM", nil)
	}
	closeT, err := parseTimeOfDay(h.CloseTime)
	if err != nil {
		return nil, apperr.Validation("invalid close_time format, expected HH:MM", nil)
	}

	if err := r.q.DeleteOperatingHoursForDay(ctx, sqlcgen.DeleteOperatingHoursForDayParams{RestaurantID: h.RestaurantID, DayOfWeek: int16(h.DayOfWeek)}); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to clear existing hours", err)
	}

	row, err := r.q.UpsertOperatingHours(ctx, sqlcgen.UpsertOperatingHoursParams{
		RestaurantID: h.RestaurantID,
		DayOfWeek:    int16(h.DayOfWeek),
		OpenTime:     openT,
		CloseTime:    closeT,
		IsClosed:     h.IsClosed,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to set operating hours", err)
	}
	return mapOperatingHours(row), nil
}

func (r *Repository) ListOperatingHours(ctx context.Context, restaurantID uuid.UUID) ([]*domain.OperatingHours, error) {
	rows, err := r.q.ListOperatingHours(ctx, restaurantID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list operating hours", err)
	}
	out := make([]*domain.OperatingHours, len(rows))
	for i, row := range rows {
		out[i] = mapOperatingHours(row)
	}
	return out, nil
}

func (r *Repository) UpsertServiceArea(ctx context.Context, restaurantID uuid.UUID, radiusKM float64, isActive bool) (*domain.ServiceArea, error) {
	var radius pgtype.Numeric
	_ = radius.Scan(radiusKM)

	row, err := r.q.UpsertServiceArea(ctx, sqlcgen.UpsertServiceAreaParams{RestaurantID: restaurantID, RadiusKm: radius, IsActive: isActive})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to set service area", err)
	}
	radiusF, _ := row.RadiusKm.Float64Value()
	return &domain.ServiceArea{ID: row.ID, RestaurantID: row.RestaurantID, RadiusKM: radiusF.Float64, IsActive: row.IsActive}, nil
}

func (r *Repository) UpsertDocument(ctx context.Context, d *domain.Document) (*domain.Document, error) {
	var expiresAt pgtype.Timestamptz
	if d.ExpiresAt != nil {
		expiresAt = pgtype.Timestamptz{Time: *d.ExpiresAt, Valid: true}
	}

	row, err := r.q.UpsertRestaurantDocument(ctx, sqlcgen.UpsertRestaurantDocumentParams{
		RestaurantID:   d.RestaurantID,
		DocumentType:   sqlcgen.DocumentType(d.DocumentType),
		FileUrl:        d.FileURL,
		DocumentNumber: toText(d.DocumentNumber),
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to upload document", err)
	}
	return mapDocument(row), nil
}

func (r *Repository) ListDocuments(ctx context.Context, restaurantID uuid.UUID) ([]*domain.Document, error) {
	rows, err := r.q.ListRestaurantDocuments(ctx, restaurantID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list documents", err)
	}
	out := make([]*domain.Document, len(rows))
	for i, row := range rows {
		out[i] = mapDocument(row)
	}
	return out, nil
}

func (r *Repository) ReviewDocument(ctx context.Context, id uuid.UUID, status domain.KYCStatus, rejectionReason *string, reviewedBy uuid.UUID) (*domain.Document, error) {
	row, err := r.q.ReviewRestaurantDocument(ctx, sqlcgen.ReviewRestaurantDocumentParams{
		ID: id, Status: sqlcgen.KycStatus(status), RejectionReason: toText(rejectionReason), ReviewedBy: pgtype.UUID{Bytes: reviewedBy, Valid: true},
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to review document", err)
	}
	return mapDocument(row), nil
}

func (r *Repository) AddStaff(ctx context.Context, s *domain.StaffMember) (*domain.StaffMember, error) {
	perms := make([]sqlcgen.StaffPermission, len(s.Permissions))
	for i, p := range s.Permissions {
		perms[i] = sqlcgen.StaffPermission(p)
	}
	row, err := r.q.AddRestaurantStaff(ctx, sqlcgen.AddRestaurantStaffParams{
		RestaurantID: s.RestaurantID, UserID: s.UserID, Permissions: perms, InvitedBy: s.InvitedBy,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to add staff member", err)
	}
	return mapStaff(row), nil
}

func (r *Repository) ListStaff(ctx context.Context, restaurantID uuid.UUID) ([]*domain.StaffMember, error) {
	rows, err := r.q.ListRestaurantStaff(ctx, restaurantID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list staff", err)
	}
	out := make([]*domain.StaffMember, len(rows))
	for i, row := range rows {
		out[i] = mapStaff(row)
	}
	return out, nil
}

func (r *Repository) RevokeStaff(ctx context.Context, id uuid.UUID) error {
	if err := r.q.RevokeRestaurantStaff(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to revoke staff", err)
	}
	return nil
}

// --- mapping helpers ---

func mapRestaurant(row sqlcgen.Restaurant) *domain.Restaurant {
	minOrder, _ := row.MinOrderAmount.Float64Value()
	commission, _ := row.CommissionPct.Float64Value()
	ratingAvg, _ := row.RatingAvg.Float64Value()

	rest := &domain.Restaurant{
		ID:                row.ID,
		OwnerUserID:       row.OwnerUserID,
		Name:              row.Name,
		Slug:              row.Slug,
		CuisineTags:       row.CuisineTags,
		Status:            domain.Status(row.Status),
		KYCStatus:         domain.KYCStatus(row.KycStatus),
		IsVegOnly:         row.IsVegOnly,
		AvgPrepTimeMins:   int(row.AvgPrepTimeMins),
		MinOrderAmount:    minOrder.Float64,
		CommissionPct:     commission.Float64,
		AddressLine1:      row.AddressLine1,
		City:              row.City,
		State:             row.State,
		PostalCode:        row.PostalCode,
		Country:           row.Country,
		RatingAvg:         ratingAvg.Float64,
		RatingCount:       int(row.RatingCount),
		IsAcceptingOrders: row.IsAcceptingOrders,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
	if row.Description.Valid {
		rest.Description = &row.Description.String
	}
	if row.LogoUrl.Valid {
		rest.LogoURL = &row.LogoUrl.String
	}
	if row.BannerUrl.Valid {
		rest.BannerURL = &row.BannerUrl.String
	}
	if row.AddressLine2.Valid {
		rest.AddressLine2 = &row.AddressLine2.String
	}
	if row.ApprovedAt.Valid {
		t := row.ApprovedAt.Time
		rest.ApprovedAt = &t
	}
	if row.ApprovedBy.Valid {
		id := uuid.UUID(row.ApprovedBy.Bytes)
		rest.ApprovedBy = &id
	}
	// NOTE: row.Location (PostGIS geography) intentionally not decoded
	// back into GeoPoint here — reads that need lat/lng should use the
	// dedicated ST_X/ST_Y projection in a future query rather than
	// parsing WKB in application code. Not needed for Phase 2 flows
	// (search already returns distance; detail views don't need raw
	// coordinates yet).
	return rest
}

func mapOperatingHours(row sqlcgen.RestaurantOperatingHour) *domain.OperatingHours {
	return &domain.OperatingHours{
		ID:           row.ID,
		RestaurantID: row.RestaurantID,
		DayOfWeek:    int(row.DayOfWeek),
		OpenTime:     formatPgTime(row.OpenTime),
		CloseTime:    formatPgTime(row.CloseTime),
		IsClosed:     row.IsClosed,
	}
}

func mapDocument(row sqlcgen.RestaurantDocument) *domain.Document {
	d := &domain.Document{
		ID:           row.ID,
		RestaurantID: row.RestaurantID,
		DocumentType: domain.DocumentType(row.DocumentType),
		FileURL:      row.FileUrl,
		Status:       domain.KYCStatus(row.Status),
		CreatedAt:    row.CreatedAt,
	}
	if row.DocumentNumber.Valid {
		d.DocumentNumber = &row.DocumentNumber.String
	}
	if row.RejectionReason.Valid {
		d.RejectionReason = &row.RejectionReason.String
	}
	if row.ReviewedBy.Valid {
		id := uuid.UUID(row.ReviewedBy.Bytes)
		d.ReviewedBy = &id
	}
	if row.ReviewedAt.Valid {
		t := row.ReviewedAt.Time
		d.ReviewedAt = &t
	}
	if row.ExpiresAt.Valid {
		t := row.ExpiresAt.Time
		d.ExpiresAt = &t
	}
	return d
}

func mapStaff(row sqlcgen.RestaurantStaff) *domain.StaffMember {
	perms := make([]domain.StaffPermission, len(row.Permissions))
	for i, p := range row.Permissions {
		perms[i] = domain.StaffPermission(p)
	}
	return &domain.StaffMember{
		ID:           row.ID,
		RestaurantID: row.RestaurantID,
		UserID:       row.UserID,
		Permissions:  perms,
		InvitedBy:    row.InvitedBy,
		Status:       row.Status,
		CreatedAt:    row.CreatedAt,
	}
}

func toText(s *string) pgtype.Text {
	if s == nil || *s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func parseTimeOfDay(s string) (pgtype.Time, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		t, err = time.Parse("15:04:05", s)
		if err != nil {
			return pgtype.Time{}, err
		}
	}
	micros := int64(t.Hour())*3600_000_000 + int64(t.Minute())*60_000_000 + int64(t.Second())*1_000_000
	return pgtype.Time{Microseconds: micros, Valid: true}, nil
}

func formatPgTime(t pgtype.Time) string {
	if !t.Valid {
		return ""
	}
	total := t.Microseconds / 1_000_000
	h := total / 3600
	m := (total % 3600) / 60
	sec := total % 60
	return time.Date(0, 1, 1, int(h), int(m), int(sec), 0, time.UTC).Format("15:04:05")
}

var _ domain.Repository = (*Repository)(nil)

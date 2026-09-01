package infrastructure

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/foodapp/backend/internal/modules/settlements/domain"
	sqlcgen "github.com/foodapp/backend/internal/platform/db/sqlc"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type Repository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) *Repository {
	return &Repository{q: q}
}

func (r *Repository) UpsertPayoutAccount(ctx context.Context, a *domain.PayoutAccount) (*domain.PayoutAccount, error) {
	row, err := r.q.CreatePayoutAccount(ctx, sqlcgen.CreatePayoutAccountParams{
		OwnerType: sqlcgen.PayoutOwnerType(a.OwnerType), OwnerID: a.OwnerID, AccountHolderName: a.AccountHolderName,
		AccountNumberLast4: a.AccountNumberLast4, AccountToken: a.AccountToken, IfscCode: a.IFSCCode, BankName: toText(a.BankName),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to save payout account", err)
	}
	return mapPayoutAccount(row), nil
}

func (r *Repository) GetPayoutAccount(ctx context.Context, ownerType domain.OwnerType, ownerID uuid.UUID) (*domain.PayoutAccount, error) {
	row, err := r.q.GetPayoutAccount(ctx, sqlcgen.GetPayoutAccountParams{OwnerType: sqlcgen.PayoutOwnerType(ownerType), OwnerID: ownerID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("payout account")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch payout account", err)
	}
	return mapPayoutAccount(row), nil
}

func (r *Repository) SetPayoutAccountVerified(ctx context.Context, id uuid.UUID, verified bool) error {
	if err := r.q.SetPayoutAccountVerified(ctx, sqlcgen.SetPayoutAccountVerifiedParams{ID: id, IsVerified: verified}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update payout account", err)
	}
	return nil
}

func (r *Repository) CreateCycle(ctx context.Context, start, end time.Time) (*domain.Cycle, error) {
	row, err := r.q.CreateSettlementCycle(ctx, sqlcgen.CreateSettlementCycleParams{
		CycleStart: pgtype.Date{Time: start, Valid: true}, CycleEnd: pgtype.Date{Time: end, Valid: true},
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create settlement cycle", err)
	}
	return mapCycle(row), nil
}

func (r *Repository) GetCycle(ctx context.Context, id uuid.UUID) (*domain.Cycle, error) {
	row, err := r.q.GetSettlementCycle(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("settlement cycle")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch settlement cycle", err)
	}
	return mapCycle(row), nil
}

func (r *Repository) ListCycles(ctx context.Context, page, pageSize int) ([]*domain.Cycle, error) {
	offset := (page - 1) * pageSize
	rows, err := r.q.ListSettlementCycles(ctx, sqlcgen.ListSettlementCyclesParams{Limit: int32(pageSize), Offset: int32(offset)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list settlement cycles", err)
	}
	out := make([]*domain.Cycle, len(rows))
	for i, row := range rows {
		out[i] = mapCycle(row)
	}
	return out, nil
}

func (r *Repository) SetCycleStatus(ctx context.Context, id uuid.UUID, status domain.CycleStatus) error {
	if err := r.q.SetCycleStatus(ctx, sqlcgen.SetCycleStatusParams{ID: id, Status: sqlcgen.CycleStatus(status)}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update cycle status", err)
	}
	return nil
}

func (r *Repository) ListRestaurantsWithActivity(ctx context.Context, from, to time.Time) ([]uuid.UUID, error) {
	rows, err := r.q.ListDistinctRestaurantsWithDeliveredOrders(ctx, sqlcgen.ListDistinctRestaurantsWithDeliveredOrdersParams{FromTs: from, ToTs: to})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list active restaurants", err)
	}
	return rows, nil
}

func (r *Repository) ListPartnersWithActivity(ctx context.Context, from, to time.Time) ([]uuid.UUID, error) {
	rows, err := r.q.ListDistinctPartnersWithDeliveries(ctx, sqlcgen.ListDistinctPartnersWithDeliveriesParams{FromTs: from, ToTs: to})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list active delivery partners", err)
	}
	return rows, nil
}

func (r *Repository) UpsertRestaurantSettlement(ctx context.Context, s *domain.RestaurantSettlement) (*domain.RestaurantSettlement, error) {
	var gross, commission, net pgtype.Numeric
	_ = gross.Scan(s.GrossSubtotal)
	_ = commission.Scan(s.CommissionAmount)
	_ = net.Scan(s.NetPayable)

	row, err := r.q.UpsertRestaurantSettlement(ctx, sqlcgen.UpsertRestaurantSettlementParams{
		CycleID: s.CycleID, RestaurantID: s.RestaurantID, OrderCount: int32(s.OrderCount),
		GrossSubtotal: gross, CommissionAmount: commission, NetPayable: net,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to save restaurant settlement", err)
	}
	return mapRestaurantSettlement(row), nil
}

func (r *Repository) GetRestaurantSettlement(ctx context.Context, id uuid.UUID) (*domain.RestaurantSettlement, error) {
	row, err := r.q.GetRestaurantSettlement(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("restaurant settlement")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch restaurant settlement", err)
	}
	return mapRestaurantSettlement(row), nil
}

func (r *Repository) ListRestaurantSettlementsForRestaurant(ctx context.Context, restaurantID uuid.UUID, page, pageSize int) ([]*domain.RestaurantSettlement, error) {
	offset := (page - 1) * pageSize
	rows, err := r.q.ListRestaurantSettlementsForRestaurant(ctx, sqlcgen.ListRestaurantSettlementsForRestaurantParams{RestaurantID: restaurantID, Limit: int32(pageSize), Offset: int32(offset)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list restaurant settlements", err)
	}
	out := make([]*domain.RestaurantSettlement, len(rows))
	for i, row := range rows {
		out[i] = mapRestaurantSettlement(row)
	}
	return out, nil
}

func (r *Repository) ListRestaurantSettlementsForCycle(ctx context.Context, cycleID uuid.UUID) ([]*domain.RestaurantSettlement, error) {
	rows, err := r.q.ListRestaurantSettlementsForCycle(ctx, cycleID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list restaurant settlements", err)
	}
	out := make([]*domain.RestaurantSettlement, len(rows))
	for i, row := range rows {
		out[i] = mapRestaurantSettlement(row)
	}
	return out, nil
}

func (r *Repository) MarkRestaurantSettlementPaid(ctx context.Context, id uuid.UUID, reference string, payoutAccountID uuid.UUID) (*domain.RestaurantSettlement, error) {
	row, err := r.q.MarkRestaurantSettlementPaid(ctx, sqlcgen.MarkRestaurantSettlementPaidParams{
		ID: id, PayoutReference: toText(&reference), PayoutAccountID: pgtype.UUID{Bytes: payoutAccountID, Valid: true},
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to mark settlement paid", err)
	}
	return mapRestaurantSettlement(row), nil
}

func (r *Repository) MarkRestaurantSettlementFailed(ctx context.Context, id uuid.UUID, reason string) (*domain.RestaurantSettlement, error) {
	row, err := r.q.MarkRestaurantSettlementFailed(ctx, sqlcgen.MarkRestaurantSettlementFailedParams{ID: id, FailureReason: toText(&reason)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to mark settlement failed", err)
	}
	return mapRestaurantSettlement(row), nil
}

func (r *Repository) UpsertDeliverySettlement(ctx context.Context, s *domain.DeliverySettlement) (*domain.DeliverySettlement, error) {
	var gross, incentive, net pgtype.Numeric
	_ = gross.Scan(s.GrossEarnings)
	_ = incentive.Scan(s.IncentiveAmount)
	_ = net.Scan(s.NetPayable)

	row, err := r.q.UpsertDeliverySettlement(ctx, sqlcgen.UpsertDeliverySettlementParams{
		CycleID: s.CycleID, DeliveryPartnerID: s.DeliveryPartnerID, DeliveryCount: int32(s.DeliveryCount),
		GrossEarnings: gross, IncentiveAmount: incentive, NetPayable: net,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to save delivery settlement", err)
	}
	return mapDeliverySettlement(row), nil
}

func (r *Repository) GetDeliverySettlement(ctx context.Context, id uuid.UUID) (*domain.DeliverySettlement, error) {
	row, err := r.q.GetDeliverySettlement(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("delivery settlement")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch delivery settlement", err)
	}
	return mapDeliverySettlement(row), nil
}

func (r *Repository) ListDeliverySettlementsForPartner(ctx context.Context, partnerID uuid.UUID, page, pageSize int) ([]*domain.DeliverySettlement, error) {
	offset := (page - 1) * pageSize
	rows, err := r.q.ListDeliverySettlementsForPartner(ctx, sqlcgen.ListDeliverySettlementsForPartnerParams{DeliveryPartnerID: partnerID, Limit: int32(pageSize), Offset: int32(offset)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list delivery settlements", err)
	}
	out := make([]*domain.DeliverySettlement, len(rows))
	for i, row := range rows {
		out[i] = mapDeliverySettlement(row)
	}
	return out, nil
}

func (r *Repository) ListDeliverySettlementsForCycle(ctx context.Context, cycleID uuid.UUID) ([]*domain.DeliverySettlement, error) {
	rows, err := r.q.ListDeliverySettlementsForCycle(ctx, cycleID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list delivery settlements", err)
	}
	out := make([]*domain.DeliverySettlement, len(rows))
	for i, row := range rows {
		out[i] = mapDeliverySettlement(row)
	}
	return out, nil
}

func (r *Repository) MarkDeliverySettlementPaid(ctx context.Context, id uuid.UUID, reference string, payoutAccountID uuid.UUID) (*domain.DeliverySettlement, error) {
	row, err := r.q.MarkDeliverySettlementPaid(ctx, sqlcgen.MarkDeliverySettlementPaidParams{
		ID: id, PayoutReference: toText(&reference), PayoutAccountID: pgtype.UUID{Bytes: payoutAccountID, Valid: true},
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to mark settlement paid", err)
	}
	return mapDeliverySettlement(row), nil
}

func (r *Repository) MarkDeliverySettlementFailed(ctx context.Context, id uuid.UUID, reason string) (*domain.DeliverySettlement, error) {
	row, err := r.q.MarkDeliverySettlementFailed(ctx, sqlcgen.MarkDeliverySettlementFailedParams{ID: id, FailureReason: toText(&reason)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to mark settlement failed", err)
	}
	return mapDeliverySettlement(row), nil
}

// --- mapping helpers ---

func mapPayoutAccount(row sqlcgen.PayoutAccount) *domain.PayoutAccount {
	a := &domain.PayoutAccount{
		ID: row.ID, OwnerType: domain.OwnerType(row.OwnerType), OwnerID: row.OwnerID,
		AccountHolderName: row.AccountHolderName, AccountNumberLast4: row.AccountNumberLast4,
		AccountToken: row.AccountToken, IFSCCode: row.IfscCode, IsVerified: row.IsVerified, CreatedAt: row.CreatedAt,
	}
	if row.BankName.Valid {
		a.BankName = &row.BankName.String
	}
	return a
}

func mapCycle(row sqlcgen.SettlementCycle) *domain.Cycle {
	c := &domain.Cycle{
		ID: row.ID, CycleStart: row.CycleStart.Time, CycleEnd: row.CycleEnd.Time,
		Status: domain.CycleStatus(row.Status), CreatedAt: row.CreatedAt,
	}
	if row.ProcessedAt.Valid {
		t := row.ProcessedAt.Time
		c.ProcessedAt = &t
	}
	return c
}

func mapRestaurantSettlement(row sqlcgen.RestaurantSettlement) *domain.RestaurantSettlement {
	gross, _ := row.GrossSubtotal.Float64Value()
	commission, _ := row.CommissionAmount.Float64Value()
	net, _ := row.NetPayable.Float64Value()
	s := &domain.RestaurantSettlement{
		ID: row.ID, CycleID: row.CycleID, RestaurantID: row.RestaurantID, OrderCount: int(row.OrderCount),
		GrossSubtotal: gross.Float64, CommissionAmount: commission.Float64, NetPayable: net.Float64,
		Status: domain.SettlementStatus(row.Status), CreatedAt: row.CreatedAt,
	}
	if row.PayoutAccountID.Valid {
		id := uuid.UUID(row.PayoutAccountID.Bytes)
		s.PayoutAccountID = &id
	}
	if row.PayoutReference.Valid {
		s.PayoutReference = &row.PayoutReference.String
	}
	if row.FailureReason.Valid {
		s.FailureReason = &row.FailureReason.String
	}
	if row.PaidAt.Valid {
		t := row.PaidAt.Time
		s.PaidAt = &t
	}
	return s
}

func mapDeliverySettlement(row sqlcgen.DeliverySettlement) *domain.DeliverySettlement {
	gross, _ := row.GrossEarnings.Float64Value()
	incentive, _ := row.IncentiveAmount.Float64Value()
	net, _ := row.NetPayable.Float64Value()
	s := &domain.DeliverySettlement{
		ID: row.ID, CycleID: row.CycleID, DeliveryPartnerID: row.DeliveryPartnerID, DeliveryCount: int(row.DeliveryCount),
		GrossEarnings: gross.Float64, IncentiveAmount: incentive.Float64, NetPayable: net.Float64,
		Status: domain.SettlementStatus(row.Status), CreatedAt: row.CreatedAt,
	}
	if row.PayoutAccountID.Valid {
		id := uuid.UUID(row.PayoutAccountID.Bytes)
		s.PayoutAccountID = &id
	}
	if row.PayoutReference.Valid {
		s.PayoutReference = &row.PayoutReference.String
	}
	if row.FailureReason.Valid {
		s.FailureReason = &row.FailureReason.String
	}
	if row.PaidAt.Valid {
		t := row.PaidAt.Time
		s.PaidAt = &t
	}
	return s
}

func toText(s *string) pgtype.Text {
	if s == nil || *s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

var _ domain.Repository = (*Repository)(nil)

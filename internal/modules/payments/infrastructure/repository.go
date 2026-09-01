package infrastructure

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/foodapp/backend/internal/modules/payments/domain"
	sqlcgen "github.com/foodapp/backend/internal/platform/db/sqlc"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type Repository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) *Repository {
	return &Repository{q: q}
}

func (r *Repository) CreateTransaction(ctx context.Context, t *domain.Transaction) (*domain.Transaction, error) {
	var amount pgtype.Numeric
	_ = amount.Scan(t.Amount)
	row, err := r.q.CreatePaymentTransaction(ctx, sqlcgen.CreatePaymentTransactionParams{
		OrderID: t.OrderID, CustomerID: t.CustomerID, Amount: amount, Currency: t.Currency,
		Method: sqlcgen.PaymentMethod(t.Method), Gateway: t.Gateway, GatewayOrderID: toText(t.GatewayOrderID),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create payment transaction", err)
	}
	return mapTransaction(row), nil
}

func (r *Repository) GetTransactionByID(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	row, err := r.q.GetPaymentTransactionByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("payment transaction")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch payment transaction", err)
	}
	return mapTransaction(row), nil
}

func (r *Repository) GetTransactionByGatewayOrderID(ctx context.Context, gateway, gatewayOrderID string) (*domain.Transaction, error) {
	row, err := r.q.GetPaymentTransactionByGatewayOrderID(ctx, sqlcgen.GetPaymentTransactionByGatewayOrderIDParams{
		Gateway: gateway, GatewayOrderID: toText(&gatewayOrderID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("payment transaction")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch payment transaction", err)
	}
	return mapTransaction(row), nil
}

func (r *Repository) ListTransactionsByOrder(ctx context.Context, orderID uuid.UUID) ([]*domain.Transaction, error) {
	rows, err := r.q.ListPaymentTransactionsByOrder(ctx, orderID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list payment transactions", err)
	}
	out := make([]*domain.Transaction, len(rows))
	for i, row := range rows {
		out[i] = mapTransaction(row)
	}
	return out, nil
}

func (r *Repository) GetLatestTransactionForOrder(ctx context.Context, orderID uuid.UUID) (*domain.Transaction, error) {
	row, err := r.q.GetLatestPaymentTransactionForOrder(ctx, orderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("payment transaction")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch payment transaction", err)
	}
	return mapTransaction(row), nil
}

func (r *Repository) MarkCaptured(ctx context.Context, id uuid.UUID, gatewayPaymentID, signature string) (*domain.Transaction, error) {
	row, err := r.q.MarkPaymentCaptured(ctx, sqlcgen.MarkPaymentCapturedParams{
		ID: id, GatewayPaymentID: toText(&gatewayPaymentID), GatewaySignature: toText(&signature),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to mark payment captured", err)
	}
	return mapTransaction(row), nil
}

func (r *Repository) MarkFailed(ctx context.Context, id uuid.UUID, reason string) (*domain.Transaction, error) {
	row, err := r.q.MarkPaymentFailed(ctx, sqlcgen.MarkPaymentFailedParams{ID: id, FailureReason: toText(&reason)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to mark payment failed", err)
	}
	return mapTransaction(row), nil
}

func (r *Repository) MarkAuthorized(ctx context.Context, id uuid.UUID, gatewayPaymentID string) (*domain.Transaction, error) {
	row, err := r.q.MarkPaymentAuthorized(ctx, sqlcgen.MarkPaymentAuthorizedParams{ID: id, GatewayPaymentID: toText(&gatewayPaymentID)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to mark payment authorized", err)
	}
	return mapTransaction(row), nil
}

func (r *Repository) SetStatus(ctx context.Context, id uuid.UUID, status domain.TransactionStatus) error {
	if err := r.q.SetPaymentRefundStatus(ctx, sqlcgen.SetPaymentRefundStatusParams{ID: id, Status: sqlcgen.PaymentTransactionStatus(status)}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update payment status", err)
	}
	return nil
}

func (r *Repository) CreateSavedMethod(ctx context.Context, m *domain.SavedMethod) (*domain.SavedMethod, error) {
	row, err := r.q.CreateSavedPaymentMethod(ctx, sqlcgen.CreateSavedPaymentMethodParams{
		CustomerID: m.CustomerID, Method: sqlcgen.PaymentMethod(m.Method), Gateway: m.Gateway,
		GatewayToken: m.GatewayToken, DisplayLabel: m.DisplayLabel, IsDefault: m.IsDefault,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to save payment method", err)
	}
	return mapSavedMethod(row), nil
}

func (r *Repository) ListSavedMethods(ctx context.Context, customerID uuid.UUID) ([]*domain.SavedMethod, error) {
	rows, err := r.q.ListSavedPaymentMethods(ctx, customerID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list saved payment methods", err)
	}
	out := make([]*domain.SavedMethod, len(rows))
	for i, row := range rows {
		out[i] = mapSavedMethod(row)
	}
	return out, nil
}

func (r *Repository) UnsetDefaultMethods(ctx context.Context, customerID uuid.UUID) error {
	if err := r.q.UnsetDefaultPaymentMethods(ctx, customerID); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update default payment method", err)
	}
	return nil
}

func (r *Repository) DeleteSavedMethod(ctx context.Context, id, customerID uuid.UUID) error {
	if err := r.q.DeleteSavedPaymentMethod(ctx, sqlcgen.DeleteSavedPaymentMethodParams{ID: id, CustomerID: customerID}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to delete saved payment method", err)
	}
	return nil
}

func (r *Repository) CreateRefund(ctx context.Context, ref *domain.Refund) (*domain.Refund, error) {
	var amount pgtype.Numeric
	_ = amount.Scan(ref.Amount)
	var initiatedBy pgtype.UUID
	if ref.InitiatedBy != nil {
		initiatedBy = pgtype.UUID{Bytes: *ref.InitiatedBy, Valid: true}
	}
	row, err := r.q.CreateRefund(ctx, sqlcgen.CreateRefundParams{
		PaymentTransactionID: ref.PaymentTransactionID, OrderID: ref.OrderID, Amount: amount, Reason: ref.Reason, InitiatedBy: initiatedBy,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create refund", err)
	}
	return mapRefund(row), nil
}

func (r *Repository) GetRefundByID(ctx context.Context, id uuid.UUID) (*domain.Refund, error) {
	row, err := r.q.GetRefundByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("refund")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch refund", err)
	}
	return mapRefund(row), nil
}

func (r *Repository) ListRefundsByOrder(ctx context.Context, orderID uuid.UUID) ([]*domain.Refund, error) {
	rows, err := r.q.ListRefundsByOrder(ctx, orderID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list refunds", err)
	}
	out := make([]*domain.Refund, len(rows))
	for i, row := range rows {
		out[i] = mapRefund(row)
	}
	return out, nil
}

func (r *Repository) MarkRefundCompleted(ctx context.Context, id uuid.UUID, gatewayRefundID string) (*domain.Refund, error) {
	row, err := r.q.MarkRefundCompleted(ctx, sqlcgen.MarkRefundCompletedParams{ID: id, GatewayRefundID: toText(&gatewayRefundID)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to mark refund completed", err)
	}
	return mapRefund(row), nil
}

func (r *Repository) MarkRefundFailed(ctx context.Context, id uuid.UUID, reason string) (*domain.Refund, error) {
	row, err := r.q.MarkRefundFailed(ctx, sqlcgen.MarkRefundFailedParams{ID: id, FailureReason: toText(&reason)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to mark refund failed", err)
	}
	return mapRefund(row), nil
}

func (r *Repository) SumCompletedRefundsForOrder(ctx context.Context, orderID uuid.UUID) (float64, error) {
	total, err := r.q.SumCompletedRefundsForOrder(ctx, orderID)
	if err != nil {
		return 0, apperr.Wrap(apperr.CodeInternal, "failed to sum refunds", err)
	}
	f, _ := total.Float64Value()
	return f.Float64, nil
}

// --- mapping helpers ---

func mapTransaction(row sqlcgen.PaymentTransaction) *domain.Transaction {
	amount, _ := row.Amount.Float64Value()
	t := &domain.Transaction{
		ID: row.ID, OrderID: row.OrderID, CustomerID: row.CustomerID, Amount: amount.Float64,
		Currency: row.Currency, Method: domain.Method(row.Method), Status: domain.TransactionStatus(row.Status),
		Gateway: row.Gateway, InitiatedAt: row.InitiatedAt,
	}
	if row.GatewayOrderID.Valid {
		t.GatewayOrderID = &row.GatewayOrderID.String
	}
	if row.GatewayPaymentID.Valid {
		t.GatewayPaymentID = &row.GatewayPaymentID.String
	}
	if row.FailureReason.Valid {
		t.FailureReason = &row.FailureReason.String
	}
	if row.CompletedAt.Valid {
		ct := row.CompletedAt.Time
		t.CompletedAt = &ct
	}
	return t
}

func mapSavedMethod(row sqlcgen.SavedPaymentMethod) *domain.SavedMethod {
	return &domain.SavedMethod{
		ID: row.ID, CustomerID: row.CustomerID, Method: domain.Method(row.Method), Gateway: row.Gateway,
		GatewayToken: row.GatewayToken, DisplayLabel: row.DisplayLabel, IsDefault: row.IsDefault, CreatedAt: row.CreatedAt,
	}
}

func mapRefund(row sqlcgen.Refund) *domain.Refund {
	amount, _ := row.Amount.Float64Value()
	ref := &domain.Refund{
		ID: row.ID, PaymentTransactionID: row.PaymentTransactionID, OrderID: row.OrderID, Amount: amount.Float64,
		Reason: row.Reason, Status: domain.RefundStatus(row.Status), InitiatedAt: row.InitiatedAt,
	}
	if row.GatewayRefundID.Valid {
		ref.GatewayRefundID = &row.GatewayRefundID.String
	}
	if row.InitiatedBy.Valid {
		id := uuid.UUID(row.InitiatedBy.Bytes)
		ref.InitiatedBy = &id
	}
	if row.FailureReason.Valid {
		ref.FailureReason = &row.FailureReason.String
	}
	if row.CompletedAt.Valid {
		ct := row.CompletedAt.Time
		ref.CompletedAt = &ct
	}
	return ref
}

func toText(s *string) pgtype.Text {
	if s == nil || *s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

var _ domain.Repository = (*Repository)(nil)

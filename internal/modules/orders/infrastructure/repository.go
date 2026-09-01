package infrastructure

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/foodapp/backend/internal/modules/orders/domain"
	sqlcgen "github.com/foodapp/backend/internal/platform/db/sqlc"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type Repository struct {
	pool *pgxpool.Pool // used only to open the transaction Create runs in; every other method uses q directly
	q    *sqlcgen.Queries
}

func NewRepository(pool *pgxpool.Pool, q *sqlcgen.Queries) *Repository {
	return &Repository{pool: pool, q: q}
}

// Create persists the order header, all order_items, and each item's
// order_item_addons inside a single database transaction — a failure at
// any point rolls back everything, so an order can never exist with
// missing items or vice versa. This closes a gap flagged since Phase 3
// (see docs/assumptions.md, "Order creation is NOT wrapped in db.WithTx
// yet"): the fix is to open the transaction here and derive a
// transaction-scoped *sqlcgen.Queries via q.WithTx(tx) — sqlc generates
// WithTx automatically for the pgx/v5 driver since pgx.Tx satisfies the
// same DBTX interface the pool does.
func (r *Repository) Create(ctx context.Context, in domain.PlaceOrderInput, orderNumber string) (*domain.Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to begin transaction", err)
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit; rolls back on any early return

	qtx := r.q.WithTx(tx)

	var subtotal, tax, deliveryFee, discount, total pgtype.Numeric
	_ = subtotal.Scan(in.Subtotal)
	_ = tax.Scan(in.TaxAmount)
	_ = deliveryFee.Scan(in.DeliveryFee)
	_ = discount.Scan(in.DiscountAmount)
	_ = total.Scan(in.TotalAmount)

	row, err := qtx.CreateOrder(ctx, sqlcgen.CreateOrderParams{
		OrderNumber: orderNumber, CustomerID: in.CustomerID, RestaurantID: in.RestaurantID,
		Subtotal: subtotal, TaxAmount: tax, DeliveryFee: deliveryFee, DiscountAmount: discount, TotalAmount: total,
		PaymentMethod:         sqlcgen.PaymentMethod(in.PaymentMethod),
		DeliveryAddressLine1:  in.DeliveryAddress.Line1,
		DeliveryAddressLine2:  toText(in.DeliveryAddress.Line2),
		DeliveryCity:          in.DeliveryAddress.City,
		DeliveryState:         in.DeliveryAddress.State,
		DeliveryPostalCode:    in.DeliveryAddress.PostalCode,
		DeliveryLat:           in.DeliveryAddress.Lat,
		DeliveryLng:           in.DeliveryAddress.Lng,
		ContactPhone:          in.DeliveryAddress.Phone,
		SpecialInstructions:   toText(in.SpecialInstructions),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create order", err)
	}

	for _, item := range in.Items {
		var menuItemID pgtype.UUID
		if item.MenuItemID != nil {
			menuItemID = pgtype.UUID{Bytes: *item.MenuItemID, Valid: true}
		}
		var unitPrice, lineTotal pgtype.Numeric
		_ = unitPrice.Scan(item.UnitPrice)
		_ = lineTotal.Scan(item.LineTotal)

		itemRow, err := qtx.CreateOrderItem(ctx, sqlcgen.CreateOrderItemParams{
			OrderID: row.ID, MenuItemID: menuItemID, ItemName: item.ItemName, VariantName: toText(item.VariantName),
			UnitPrice: unitPrice, Quantity: int16(item.Quantity), LineTotal: lineTotal,
			SpecialInstructions: toText(item.SpecialInstructions),
		})
		if err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "failed to create order item", err)
		}

		for _, addon := range item.Addons {
			var addonPrice pgtype.Numeric
			_ = addonPrice.Scan(addon.Price)
			if _, err := qtx.CreateOrderItemAddon(ctx, sqlcgen.CreateOrderItemAddonParams{
				OrderItemID: itemRow.ID, AddonName: addon.Name, AddonPrice: addonPrice,
			}); err != nil {
				return nil, apperr.Wrap(apperr.CodeInternal, "failed to create order item add-on", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to commit order creation", err)
	}

	return mapOrder(row), nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	row, err := r.q.GetOrderByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("order")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch order", err)
	}
	return mapOrder(row), nil
}

func (r *Repository) GetByNumber(ctx context.Context, orderNumber string) (*domain.Order, error) {
	row, err := r.q.GetOrderByNumber(ctx, orderNumber)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("order")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch order", err)
	}
	return mapOrder(row), nil
}

func (r *Repository) ListByCustomer(ctx context.Context, customerID uuid.UUID, page, pageSize int) ([]*domain.Order, int64, error) {
	offset := (page - 1) * pageSize
	rows, err := r.q.ListOrdersByCustomer(ctx, sqlcgen.ListOrdersByCustomerParams{CustomerID: customerID, Limit: int32(pageSize), Offset: int32(offset)})
	if err != nil {
		return nil, 0, apperr.Wrap(apperr.CodeInternal, "failed to list orders", err)
	}
	total, err := r.q.CountOrdersByCustomer(ctx, customerID)
	if err != nil {
		return nil, 0, apperr.Wrap(apperr.CodeInternal, "failed to count orders", err)
	}
	out := make([]*domain.Order, len(rows))
	for i, row := range rows {
		out[i] = mapOrder(row)
	}
	return out, total, nil
}

func (r *Repository) ListByRestaurant(ctx context.Context, restaurantID uuid.UUID, filter domain.ListFilter) ([]*domain.Order, int64, error) {
	offset := (filter.Page - 1) * filter.PageSize
	var statusParam sqlcgen.NullOrderStatus
	if filter.Status != nil {
		statusParam = sqlcgen.NullOrderStatus{OrderStatus: sqlcgen.OrderStatus(*filter.Status), Valid: true}
	}

	rows, err := r.q.ListOrdersByRestaurant(ctx, sqlcgen.ListOrdersByRestaurantParams{
		RestaurantID: restaurantID, Status: statusParam, Limit: int32(filter.PageSize), Offset: int32(offset),
	})
	if err != nil {
		return nil, 0, apperr.Wrap(apperr.CodeInternal, "failed to list orders", err)
	}
	total, err := r.q.CountOrdersByRestaurant(ctx, sqlcgen.CountOrdersByRestaurantParams{RestaurantID: restaurantID, Status: statusParam})
	if err != nil {
		return nil, 0, apperr.Wrap(apperr.CodeInternal, "failed to count orders", err)
	}
	out := make([]*domain.Order, len(rows))
	for i, row := range rows {
		out[i] = mapOrder(row)
	}
	return out, total, nil
}

func (r *Repository) ListActiveByRestaurant(ctx context.Context, restaurantID uuid.UUID) ([]*domain.Order, error) {
	rows, err := r.q.ListActiveOrdersByRestaurant(ctx, restaurantID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list active orders", err)
	}
	out := make([]*domain.Order, len(rows))
	for i, row := range rows {
		out[i] = mapOrder(row)
	}
	return out, nil
}

func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.Status) (*domain.Order, error) {
	row, err := r.q.UpdateOrderStatus(ctx, sqlcgen.UpdateOrderStatusParams{ID: id, Status: sqlcgen.OrderStatus(status)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to update order status", err)
	}
	return mapOrder(row), nil
}

func (r *Repository) Cancel(ctx context.Context, id uuid.UUID, reason string, cancelledBy uuid.UUID) (*domain.Order, error) {
	row, err := r.q.CancelOrder(ctx, sqlcgen.CancelOrderParams{
		ID: id, CancellationReason: toText(&reason), CancelledBy: pgtype.UUID{Bytes: cancelledBy, Valid: true},
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to cancel order", err)
	}
	return mapOrder(row), nil
}

func (r *Repository) SetPaymentStatus(ctx context.Context, id uuid.UUID, status domain.PaymentStatus) error {
	if err := r.q.SetOrderPaymentStatus(ctx, sqlcgen.SetOrderPaymentStatusParams{ID: id, PaymentStatus: sqlcgen.PaymentStatus(status)}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update payment status", err)
	}
	return nil
}

func (r *Repository) ListItems(ctx context.Context, orderID uuid.UUID) ([]*domain.OrderItem, error) {
	rows, err := r.q.ListOrderItems(ctx, orderID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list order items", err)
	}

	addonRows, err := r.q.ListOrderItemAddonsByOrder(ctx, orderID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list order item add-ons", err)
	}
	addonsByItem := make(map[uuid.UUID][]domain.OrderItemAddon)
	for _, ar := range addonRows {
		price, _ := ar.AddonPrice.Float64Value()
		addonsByItem[ar.OrderItemID] = append(addonsByItem[ar.OrderItemID], domain.OrderItemAddon{
			ID: ar.ID, OrderItemID: ar.OrderItemID, AddonName: ar.AddonName, AddonPrice: price.Float64,
		})
	}

	out := make([]*domain.OrderItem, len(rows))
	for i, row := range rows {
		item := mapOrderItem(row)
		item.Addons = addonsByItem[item.ID]
		out[i] = item
	}
	return out, nil
}

func (r *Repository) RecordStatusChange(ctx context.Context, orderID uuid.UUID, status domain.Status, changedBy *uuid.UUID, notes *string) error {
	var changedByParam pgtype.UUID
	if changedBy != nil {
		changedByParam = pgtype.UUID{Bytes: *changedBy, Valid: true}
	}
	if _, err := r.q.CreateOrderStatusHistory(ctx, sqlcgen.CreateOrderStatusHistoryParams{
		OrderID: orderID, Status: sqlcgen.OrderStatus(status), ChangedBy: changedByParam, Notes: toText(notes),
	}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to record status history", err)
	}
	return nil
}

func (r *Repository) ListStatusHistory(ctx context.Context, orderID uuid.UUID) ([]*domain.StatusHistoryEntry, error) {
	rows, err := r.q.ListOrderStatusHistory(ctx, orderID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list status history", err)
	}
	out := make([]*domain.StatusHistoryEntry, len(rows))
	for i, row := range rows {
		entry := &domain.StatusHistoryEntry{ID: row.ID, OrderID: row.OrderID, Status: domain.Status(row.Status), CreatedAt: row.CreatedAt}
		if row.ChangedBy.Valid {
			id := uuid.UUID(row.ChangedBy.Bytes)
			entry.ChangedBy = &id
		}
		if row.Notes.Valid {
			entry.Notes = &row.Notes.String
		}
		out[i] = entry
	}
	return out, nil
}

func (r *Repository) SumSettlementData(ctx context.Context, restaurantID uuid.UUID, from, to time.Time) (int64, float64, error) {
	row, err := r.q.SumSettlementDataForRestaurant(ctx, sqlcgen.SumSettlementDataForRestaurantParams{
		RestaurantID: restaurantID, FromTs: from, ToTs: to,
	})
	if err != nil {
		return 0, 0, apperr.Wrap(apperr.CodeInternal, "failed to compute settlement data", err)
	}
	subtotal, _ := row.GrossSubtotal.Float64Value()
	return row.OrderCount, subtotal.Float64, nil
}

// --- mapping helpers ---

func mapOrder(row sqlcgen.Order) *domain.Order {
	subtotal, _ := row.Subtotal.Float64Value()
	tax, _ := row.TaxAmount.Float64Value()
	deliveryFee, _ := row.DeliveryFee.Float64Value()
	discount, _ := row.DiscountAmount.Float64Value()
	total, _ := row.TotalAmount.Float64Value()

	o := &domain.Order{
		ID: row.ID, OrderNumber: row.OrderNumber, CustomerID: row.CustomerID, RestaurantID: row.RestaurantID,
		Status: domain.Status(row.Status), Subtotal: subtotal.Float64, TaxAmount: tax.Float64,
		DeliveryFee: deliveryFee.Float64, DiscountAmount: discount.Float64, TotalAmount: total.Float64,
		PaymentStatus: domain.PaymentStatus(row.PaymentStatus), PaymentMethod: domain.PaymentMethod(row.PaymentMethod),
		DeliveryAddress: domain.DeliveryAddress{
			Line1: row.DeliveryAddressLine1, City: row.DeliveryCity, State: row.DeliveryState,
			PostalCode: row.DeliveryPostalCode, Lat: row.DeliveryLat, Lng: row.DeliveryLng, Phone: row.ContactPhone,
		},
		PlacedAt: row.PlacedAt, CreatedAt: row.CreatedAt,
	}
	if row.DeliveryAddressLine2.Valid {
		o.DeliveryAddress.Line2 = &row.DeliveryAddressLine2.String
	}
	if row.SpecialInstructions.Valid {
		o.SpecialInstructions = &row.SpecialInstructions.String
	}
	if row.CancellationReason.Valid {
		o.CancellationReason = &row.CancellationReason.String
	}
	if row.CancelledBy.Valid {
		id := uuid.UUID(row.CancelledBy.Bytes)
		o.CancelledBy = &id
	}
	if row.ConfirmedAt.Valid {
		t := row.ConfirmedAt.Time
		o.ConfirmedAt = &t
	}
	if row.ReadyAt.Valid {
		t := row.ReadyAt.Time
		o.ReadyAt = &t
	}
	if row.PickedUpAt.Valid {
		t := row.PickedUpAt.Time
		o.PickedUpAt = &t
	}
	if row.DeliveredAt.Valid {
		t := row.DeliveredAt.Time
		o.DeliveredAt = &t
	}
	if row.CancelledAt.Valid {
		t := row.CancelledAt.Time
		o.CancelledAt = &t
	}
	if row.EstimatedDeliveryAt.Valid {
		t := row.EstimatedDeliveryAt.Time
		o.EstimatedDeliveryAt = &t
	}
	return o
}

func mapOrderItem(row sqlcgen.OrderItem) *domain.OrderItem {
	unitPrice, _ := row.UnitPrice.Float64Value()
	lineTotal, _ := row.LineTotal.Float64Value()
	item := &domain.OrderItem{
		ID: row.ID, OrderID: row.OrderID, ItemName: row.ItemName, UnitPrice: unitPrice.Float64,
		Quantity: int(row.Quantity), LineTotal: lineTotal.Float64,
	}
	if row.MenuItemID.Valid {
		id := uuid.UUID(row.MenuItemID.Bytes)
		item.MenuItemID = &id
	}
	if row.VariantName.Valid {
		item.VariantName = &row.VariantName.String
	}
	if row.SpecialInstructions.Valid {
		item.SpecialInstructions = &row.SpecialInstructions.String
	}
	return item
}

func toText(s *string) pgtype.Text {
	if s == nil || *s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

var _ domain.Repository = (*Repository)(nil)

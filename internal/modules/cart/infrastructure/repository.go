package infrastructure

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/foodapp/backend/internal/modules/cart/domain"
	sqlcgen "github.com/foodapp/backend/internal/platform/db/sqlc"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type Repository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) *Repository {
	return &Repository{q: q}
}

func (r *Repository) GetByCustomer(ctx context.Context, customerID uuid.UUID) (*domain.Cart, error) {
	row, err := r.q.GetCartByCustomer(ctx, customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("cart")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch cart", err)
	}
	return mapCart(row), nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Cart, error) {
	row, err := r.q.GetCartByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("cart")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch cart", err)
	}
	return mapCart(row), nil
}

func (r *Repository) Create(ctx context.Context, customerID, restaurantID uuid.UUID) (*domain.Cart, error) {
	row, err := r.q.CreateCart(ctx, sqlcgen.CreateCartParams{CustomerID: customerID, RestaurantID: restaurantID})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create cart", err)
	}
	return mapCart(row), nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.q.DeleteCart(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to delete cart", err)
	}
	return nil
}

func (r *Repository) Touch(ctx context.Context, id uuid.UUID) error {
	if err := r.q.TouchCart(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update cart", err)
	}
	return nil
}

func (r *Repository) AddItem(ctx context.Context, in *domain.Item) (*domain.Item, error) {
	var variantID pgtype.UUID
	if in.VariantID != nil {
		variantID = pgtype.UUID{Bytes: *in.VariantID, Valid: true}
	}
	row, err := r.q.AddCartItem(ctx, sqlcgen.AddCartItemParams{
		CartID: in.CartID, MenuItemID: in.MenuItemID, VariantID: variantID,
		Quantity: int16(in.Quantity), SpecialInstructions: toText(in.SpecialInstructions),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to add cart item", err)
	}
	return mapItem(row), nil
}

func (r *Repository) GetItemByID(ctx context.Context, id uuid.UUID) (*domain.Item, error) {
	row, err := r.q.GetCartItemByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("cart item")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch cart item", err)
	}
	return mapItem(row), nil
}

func (r *Repository) ListItems(ctx context.Context, cartID uuid.UUID) ([]*domain.Item, error) {
	rows, err := r.q.ListCartItems(ctx, cartID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list cart items", err)
	}
	out := make([]*domain.Item, len(rows))
	for i, row := range rows {
		out[i] = mapItem(row)
	}
	return out, nil
}

func (r *Repository) UpdateItemQuantity(ctx context.Context, id uuid.UUID, quantity int) (*domain.Item, error) {
	row, err := r.q.UpdateCartItemQuantity(ctx, sqlcgen.UpdateCartItemQuantityParams{ID: id, Quantity: int16(quantity)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to update cart item", err)
	}
	return mapItem(row), nil
}

func (r *Repository) DeleteItem(ctx context.Context, id uuid.UUID) error {
	if err := r.q.DeleteCartItem(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to remove cart item", err)
	}
	return nil
}

func (r *Repository) ClearItems(ctx context.Context, cartID uuid.UUID) error {
	if err := r.q.ClearCartItems(ctx, cartID); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to clear cart", err)
	}
	return nil
}

func (r *Repository) AddItemAddon(ctx context.Context, cartItemID, addonID uuid.UUID) error {
	if _, err := r.q.AddCartItemAddon(ctx, sqlcgen.AddCartItemAddonParams{CartItemID: cartItemID, AddonID: addonID}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to add cart item add-on", err)
	}
	return nil
}

func (r *Repository) ListItemAddons(ctx context.Context, cartItemID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.q.ListCartItemAddons(ctx, cartItemID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list cart item add-ons", err)
	}
	out := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		out[i] = row.AddonID
	}
	return out, nil
}

func (r *Repository) ClearItemAddons(ctx context.Context, cartItemID uuid.UUID) error {
	if err := r.q.ClearCartItemAddons(ctx, cartItemID); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to clear cart item add-ons", err)
	}
	return nil
}

func (r *Repository) ListPricedItems(ctx context.Context, cartID uuid.UUID) ([]*domain.PricedItem, error) {
	rows, err := r.q.ListCartItemsWithPricing(ctx, cartID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to price cart items", err)
	}

	out := make([]*domain.PricedItem, len(rows))
	for i, row := range rows {
		var variantID *uuid.UUID
		if row.VariantID.Valid {
			id := uuid.UUID(row.VariantID.Bytes)
			variantID = &id
		}

		basePrice, _ := row.ItemBasePrice.Float64Value()
		unitPrice := basePrice.Float64
		var variantName *string
		var variantAvailable *bool
		if row.VariantName.Valid {
			variantName = &row.VariantName.String
			vp, _ := row.VariantPrice.Float64Value()
			unitPrice = vp.Float64 // variant price REPLACES base price, per menu pricing model
			avail := row.VariantIsAvailable.Bool
			variantAvailable = &avail
		}

		addonRows, err := r.q.ListAddonsForCartItem(ctx, row.ID)
		if err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch item add-ons", err)
		}
		addons := make([]domain.PricedAddon, len(addonRows))
		var addonTotal float64
		for j, ar := range addonRows {
			price, _ := ar.AddonPrice.Float64Value()
			addons[j] = domain.PricedAddon{AddonID: ar.AddonID, Name: ar.AddonName, Price: price.Float64, IsAvailable: ar.IsAvailable}
			addonTotal += price.Float64
		}

		quantity := int(row.Quantity)
		out[i] = &domain.PricedItem{
			Item: &domain.Item{
				ID: row.ID, CartID: row.CartID, MenuItemID: row.MenuItemID, VariantID: variantID,
				Quantity: quantity, SpecialInstructions: textPtr(row.SpecialInstructions),
			},
			ItemName:           row.ItemName,
			ItemIsAvailable:    row.ItemIsAvailable,
			VariantName:        variantName,
			VariantIsAvailable: variantAvailable,
			UnitPrice:          unitPrice,
			Addons:             addons,
			LineTotal:          (unitPrice + addonTotal) * float64(quantity),
		}
	}
	return out, nil
}

// --- mapping helpers ---

func mapCart(row sqlcgen.Cart) *domain.Cart {
	return &domain.Cart{ID: row.ID, CustomerID: row.CustomerID, RestaurantID: row.RestaurantID}
}

func mapItem(row sqlcgen.CartItem) *domain.Item {
	var variantID *uuid.UUID
	if row.VariantID.Valid {
		id := uuid.UUID(row.VariantID.Bytes)
		variantID = &id
	}
	return &domain.Item{
		ID: row.ID, CartID: row.CartID, MenuItemID: row.MenuItemID, VariantID: variantID,
		Quantity: int(row.Quantity), SpecialInstructions: textPtr(row.SpecialInstructions),
	}
}

func toText(s *string) pgtype.Text {
	if s == nil || *s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

var _ domain.Repository = (*Repository)(nil)

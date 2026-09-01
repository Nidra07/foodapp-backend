package domain

import (
	"context"

	"github.com/google/uuid"
)

type Cart struct {
	ID           uuid.UUID
	CustomerID   uuid.UUID
	RestaurantID uuid.UUID
}

type Item struct {
	ID                   uuid.UUID
	CartID               uuid.UUID
	MenuItemID           uuid.UUID
	VariantID            *uuid.UUID
	Quantity             int
	SpecialInstructions  *string
}

type ItemAddon struct {
	ID         uuid.UUID
	CartItemID uuid.UUID
	AddonID    uuid.UUID
}

// PricedItem is a cart item enriched with live menu pricing/availability,
// used to compute totals and to flag items that became unavailable or
// changed price since being added — the client needs to show the
// customer a warning before checkout in that case.
type PricedItem struct {
	Item              *Item
	ItemName          string
	ItemIsAvailable   bool
	VariantName       *string
	VariantIsAvailable *bool
	UnitPrice         float64 // base_price, or variant price if a variant is selected
	Addons            []PricedAddon
	LineTotal         float64 // (UnitPrice + sum(addon prices)) * Quantity
}

type PricedAddon struct {
	AddonID     uuid.UUID
	Name        string
	Price       float64
	IsAvailable bool
}

type CartSummary struct {
	Cart            *Cart
	Items           []*PricedItem
	Subtotal        float64
	HasUnavailable  bool // true if any item/variant/addon in the cart is currently unavailable
}

type AddItemInput struct {
	MenuItemID          uuid.UUID
	VariantID           *uuid.UUID
	AddonIDs            []uuid.UUID
	Quantity            int
	SpecialInstructions *string
}

// Repository is the persistence port for the Cart module.
type Repository interface {
	GetByCustomer(ctx context.Context, customerID uuid.UUID) (*Cart, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Cart, error)
	Create(ctx context.Context, customerID, restaurantID uuid.UUID) (*Cart, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Touch(ctx context.Context, id uuid.UUID) error

	AddItem(ctx context.Context, in *Item) (*Item, error)
	GetItemByID(ctx context.Context, id uuid.UUID) (*Item, error)
	ListItems(ctx context.Context, cartID uuid.UUID) ([]*Item, error)
	UpdateItemQuantity(ctx context.Context, id uuid.UUID, quantity int) (*Item, error)
	DeleteItem(ctx context.Context, id uuid.UUID) error
	ClearItems(ctx context.Context, cartID uuid.UUID) error

	AddItemAddon(ctx context.Context, cartItemID, addonID uuid.UUID) error
	ListItemAddons(ctx context.Context, cartItemID uuid.UUID) ([]uuid.UUID, error)
	ClearItemAddons(ctx context.Context, cartItemID uuid.UUID) error

	// ListPricedItems returns every cart item joined against live menu
	// pricing/availability, used for both the "view cart" response and
	// the checkout total calculation, so the two can never disagree.
	ListPricedItems(ctx context.Context, cartID uuid.UUID) ([]*PricedItem, error)
}

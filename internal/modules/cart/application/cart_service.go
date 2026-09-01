package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/cart/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type CartService struct {
	repo domain.Repository
}

func NewCartService(repo domain.Repository) *CartService {
	return &CartService{repo: repo}
}

// AddItem adds an item to the customer's cart. If the customer already
// has a cart for a DIFFERENT restaurant, that cart (and its items) is
// cleared first and replaced — matches standard "one restaurant per
// cart" behavior. If a cart for the SAME restaurant exists, the item is
// just appended to it.
func (s *CartService) AddItem(ctx context.Context, customerID, restaurantID uuid.UUID, in domain.AddItemInput) (*domain.CartSummary, error) {
	if in.Quantity < 1 {
		in.Quantity = 1
	}

	cart, err := s.repo.GetByCustomer(ctx, customerID)
	if err != nil {
		if !isNotFound(err) {
			return nil, err
		}
		cart, err = s.repo.Create(ctx, customerID, restaurantID)
		if err != nil {
			return nil, err
		}
	} else if cart.RestaurantID != restaurantID {
		// Switching restaurants: clear the old cart's items and re-point it,
		// rather than deleting/recreating the cart row (keeps cart.id stable
		// for the customer app's local state).
		if err := s.repo.ClearItems(ctx, cart.ID); err != nil {
			return nil, err
		}
		if err := s.repo.Delete(ctx, cart.ID); err != nil {
			return nil, err
		}
		cart, err = s.repo.Create(ctx, customerID, restaurantID)
		if err != nil {
			return nil, err
		}
	}

	item, err := s.repo.AddItem(ctx, &domain.Item{
		CartID: cart.ID, MenuItemID: in.MenuItemID, VariantID: in.VariantID,
		Quantity: in.Quantity, SpecialInstructions: in.SpecialInstructions,
	})
	if err != nil {
		return nil, err
	}

	for _, addonID := range in.AddonIDs {
		if err := s.repo.AddItemAddon(ctx, item.ID, addonID); err != nil {
			return nil, err
		}
	}

	return s.GetSummary(ctx, cart.ID)
}

func (s *CartService) UpdateItemQuantity(ctx context.Context, cartItemID uuid.UUID, quantity int) error {
	if quantity < 1 {
		return apperr.Validation("quantity must be at least 1; use the delete endpoint to remove an item", nil)
	}
	_, err := s.repo.UpdateItemQuantity(ctx, cartItemID, quantity)
	return err
}

func (s *CartService) RemoveItem(ctx context.Context, cartItemID uuid.UUID) error {
	return s.repo.DeleteItem(ctx, cartItemID)
}

func (s *CartService) GetMyCart(ctx context.Context, customerID uuid.UUID) (*domain.CartSummary, error) {
	cart, err := s.repo.GetByCustomer(ctx, customerID)
	if err != nil {
		if isNotFound(err) {
			return &domain.CartSummary{Items: []*domain.PricedItem{}}, nil
		}
		return nil, err
	}
	return s.GetSummary(ctx, cart.ID)
}

func (s *CartService) GetSummary(ctx context.Context, cartID uuid.UUID) (*domain.CartSummary, error) {
	cart, err := s.repo.GetByID(ctx, cartID)
	if err != nil {
		return nil, err
	}

	items, err := s.repo.ListPricedItems(ctx, cartID)
	if err != nil {
		return nil, err
	}

	var subtotal float64
	hasUnavailable := false
	for _, item := range items {
		subtotal += item.LineTotal
		if !item.ItemIsAvailable {
			hasUnavailable = true
		}
		if item.VariantIsAvailable != nil && !*item.VariantIsAvailable {
			hasUnavailable = true
		}
		for _, a := range item.Addons {
			if !a.IsAvailable {
				hasUnavailable = true
			}
		}
	}

	return &domain.CartSummary{Cart: cart, Items: items, Subtotal: subtotal, HasUnavailable: hasUnavailable}, nil
}

func (s *CartService) ClearCart(ctx context.Context, customerID uuid.UUID) error {
	cart, err := s.repo.GetByCustomer(ctx, customerID)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	return s.repo.Delete(ctx, cart.ID)
}

func isNotFound(err error) bool {
	ae, ok := apperr.As(err)
	return ok && ae.Code == apperr.CodeNotFound
}

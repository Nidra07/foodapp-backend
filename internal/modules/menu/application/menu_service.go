package application

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/menu/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type MenuService struct {
	repo domain.Repository
}

func NewMenuService(repo domain.Repository) *MenuService {
	return &MenuService{repo: repo}
}

func (s *MenuService) CreateCategory(ctx context.Context, restaurantID uuid.UUID, name string, description *string, displayOrder int) (*domain.Category, error) {
	if strings.TrimSpace(name) == "" {
		return nil, apperr.Validation("category name is required", nil)
	}
	return s.repo.CreateCategory(ctx, restaurantID, name, description, displayOrder)
}

func (s *MenuService) ListCategories(ctx context.Context, restaurantID uuid.UUID) ([]*domain.Category, error) {
	return s.repo.ListCategories(ctx, restaurantID)
}

func (s *MenuService) UpdateCategory(ctx context.Context, id uuid.UUID, name, description *string, displayOrder *int) (*domain.Category, error) {
	return s.repo.UpdateCategory(ctx, id, name, description, displayOrder)
}

func (s *MenuService) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteCategory(ctx, id)
}

func (s *MenuService) CreateItem(ctx context.Context, in domain.CreateItemInput) (*domain.Item, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, apperr.Validation("item name is required", nil)
	}
	if in.BasePrice < 0 {
		return nil, apperr.Validation("base_price cannot be negative", nil)
	}
	cat, err := s.repo.GetCategoryByID(ctx, in.CategoryID)
	if err != nil {
		return nil, err
	}
	if cat.RestaurantID != in.RestaurantID {
		return nil, apperr.Validation("category does not belong to this restaurant", nil)
	}
	return s.repo.CreateItem(ctx, in)
}

func (s *MenuService) GetItem(ctx context.Context, id uuid.UUID) (*domain.Item, error) {
	return s.repo.GetItemByID(ctx, id)
}

func (s *MenuService) UpdateItem(ctx context.Context, id uuid.UUID, in domain.UpdateItemInput) (*domain.Item, error) {
	if in.BasePrice != nil && *in.BasePrice < 0 {
		return nil, apperr.Validation("base_price cannot be negative", nil)
	}
	return s.repo.UpdateItem(ctx, id, in)
}

func (s *MenuService) SetItemAvailability(ctx context.Context, id uuid.UUID, available bool) error {
	return s.repo.SetItemAvailability(ctx, id, available)
}

func (s *MenuService) SetItemActive(ctx context.Context, id uuid.UUID, active bool) error {
	return s.repo.SetItemActive(ctx, id, active)
}

func (s *MenuService) DeleteItem(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteItem(ctx, id)
}

func (s *MenuService) CreateVariantGroup(ctx context.Context, g *domain.VariantGroup) (*domain.VariantGroup, error) {
	if strings.TrimSpace(g.Name) == "" {
		return nil, apperr.Validation("variant group name is required", nil)
	}
	if g.MinSelect > g.MaxSelect {
		return nil, apperr.Validation("min_select cannot exceed max_select", nil)
	}
	return s.repo.CreateVariantGroup(ctx, g)
}

func (s *MenuService) CreateVariant(ctx context.Context, v *domain.Variant) (*domain.Variant, error) {
	if strings.TrimSpace(v.Name) == "" {
		return nil, apperr.Validation("variant name is required", nil)
	}
	if v.Price < 0 {
		return nil, apperr.Validation("variant price cannot be negative", nil)
	}
	return s.repo.CreateVariant(ctx, v)
}

func (s *MenuService) CreateAddonGroup(ctx context.Context, g *domain.AddonGroup) (*domain.AddonGroup, error) {
	if strings.TrimSpace(g.Name) == "" {
		return nil, apperr.Validation("add-on group name is required", nil)
	}
	if g.MinSelect > g.MaxSelect {
		return nil, apperr.Validation("min_select cannot exceed max_select", nil)
	}
	return s.repo.CreateAddonGroup(ctx, g)
}

func (s *MenuService) CreateAddon(ctx context.Context, a *domain.Addon) (*domain.Addon, error) {
	if strings.TrimSpace(a.Name) == "" {
		return nil, apperr.Validation("add-on name is required", nil)
	}
	if a.Price < 0 {
		return nil, apperr.Validation("add-on price cannot be negative", nil)
	}
	return s.repo.CreateAddon(ctx, a)
}

// GetFullMenu assembles the entire customer-facing menu tree for a
// restaurant: categories -> items -> variant groups/variants + addon
// groups/addons. This is the single call the Customer app makes when
// opening a restaurant page.
func (s *MenuService) GetFullMenu(ctx context.Context, restaurantID uuid.UUID) (*domain.FullMenu, error) {
	categories, err := s.repo.ListCategories(ctx, restaurantID)
	if err != nil {
		return nil, err
	}

	items, err := s.repo.ListItemsByRestaurant(ctx, restaurantID)
	if err != nil {
		return nil, err
	}

	itemsByCategory := make(map[uuid.UUID][]*domain.Item)
	for _, item := range items {
		itemsByCategory[item.CategoryID] = append(itemsByCategory[item.CategoryID], item)
	}

	menu := &domain.FullMenu{}
	for _, cat := range categories {
		catItems := itemsByCategory[cat.ID]
		itemsWithOptions := make([]*domain.ItemWithOptions, 0, len(catItems))

		for _, item := range catItems {
			variantGroups, err := s.repo.ListVariantGroupsByItem(ctx, item.ID)
			if err != nil {
				return nil, err
			}
			vgWithVariants := make([]*domain.VariantGroupWithVariants, 0, len(variantGroups))
			for _, vg := range variantGroups {
				variants, err := s.repo.ListVariantsByGroup(ctx, vg.ID)
				if err != nil {
					return nil, err
				}
				vgWithVariants = append(vgWithVariants, &domain.VariantGroupWithVariants{Group: vg, Variants: variants})
			}

			addonGroups, err := s.repo.ListAddonGroupsByItem(ctx, item.ID)
			if err != nil {
				return nil, err
			}
			agWithAddons := make([]*domain.AddonGroupWithAddons, 0, len(addonGroups))
			for _, ag := range addonGroups {
				addons, err := s.repo.ListAddonsByGroup(ctx, ag.ID)
				if err != nil {
					return nil, err
				}
				agWithAddons = append(agWithAddons, &domain.AddonGroupWithAddons{Group: ag, Addons: addons})
			}

			itemsWithOptions = append(itemsWithOptions, &domain.ItemWithOptions{
				Item:          item,
				VariantGroups: vgWithVariants,
				AddonGroups:   agWithAddons,
			})
		}

		menu.Categories = append(menu.Categories, &domain.CategoryWithItems{Category: cat, Items: itemsWithOptions})
	}

	return menu, nil
}

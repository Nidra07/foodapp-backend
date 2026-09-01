package domain

import (
	"context"

	"github.com/google/uuid"
)

type FoodType string

const (
	FoodTypeVeg    FoodType = "veg"
	FoodTypeNonVeg FoodType = "non_veg"
	FoodTypeEgg    FoodType = "egg"
)

type Category struct {
	ID            uuid.UUID
	RestaurantID  uuid.UUID
	Name          string
	Description   *string
	DisplayOrder  int
	IsActive      bool
}

type Item struct {
	ID             uuid.UUID
	RestaurantID   uuid.UUID
	CategoryID     uuid.UUID
	Name           string
	Description    *string
	FoodType       FoodType
	BasePrice      float64
	ImageURL       *string
	IsAvailable    bool
	IsActive       bool
	DisplayOrder   int
	Calories       *int
	IsBestseller   bool
	SpiceLevel     *int
	PrepTimeMins   *int
}

type VariantGroup struct {
	ID           uuid.UUID
	MenuItemID   uuid.UUID
	Name         string
	IsRequired   bool
	MinSelect    int
	MaxSelect    int
	DisplayOrder int
}

type Variant struct {
	ID              uuid.UUID
	VariantGroupID  uuid.UUID
	Name            string
	Price           float64
	IsAvailable     bool
	DisplayOrder    int
}

type AddonGroup struct {
	ID           uuid.UUID
	MenuItemID   uuid.UUID
	Name         string
	IsRequired   bool
	MinSelect    int
	MaxSelect    int
	DisplayOrder int
}

type Addon struct {
	ID           uuid.UUID
	AddonGroupID uuid.UUID
	Name         string
	Price        float64
	IsAvailable  bool
	DisplayOrder int
}

// FullMenu is the assembled response shape for "get restaurant menu" —
// categories, each with their items, each item with its variant/addon
// groups nested. Built by the application layer from several repository
// calls rather than one giant SQL join, keeping each query simple and
// independently cacheable later (Redis cache-aside is a documented
// Phase 2 follow-up in docs/assumptions.md).
type FullMenu struct {
	Categories []*CategoryWithItems
}

type CategoryWithItems struct {
	Category *Category
	Items    []*ItemWithOptions
}

type ItemWithOptions struct {
	Item          *Item
	VariantGroups []*VariantGroupWithVariants
	AddonGroups   []*AddonGroupWithAddons
}

type VariantGroupWithVariants struct {
	Group    *VariantGroup
	Variants []*Variant
}

type AddonGroupWithAddons struct {
	Group  *AddonGroup
	Addons []*Addon
}

type CreateItemInput struct {
	RestaurantID uuid.UUID
	CategoryID   uuid.UUID
	Name         string
	Description  *string
	FoodType     FoodType
	BasePrice    float64
	ImageURL     *string
	Calories     *int
	SpiceLevel   *int
	PrepTimeMins *int
	DisplayOrder int
}

type UpdateItemInput struct {
	Name         *string
	Description  *string
	FoodType     *FoodType
	BasePrice    *float64
	ImageURL     *string
	Calories     *int
	SpiceLevel   *int
	PrepTimeMins *int
	CategoryID   *uuid.UUID
}

// Repository is the persistence port for the Menu module.
type Repository interface {
	CreateCategory(ctx context.Context, restaurantID uuid.UUID, name string, description *string, displayOrder int) (*Category, error)
	GetCategoryByID(ctx context.Context, id uuid.UUID) (*Category, error)
	ListCategories(ctx context.Context, restaurantID uuid.UUID) ([]*Category, error)
	UpdateCategory(ctx context.Context, id uuid.UUID, name, description *string, displayOrder *int) (*Category, error)
	SetCategoryActive(ctx context.Context, id uuid.UUID, active bool) error
	DeleteCategory(ctx context.Context, id uuid.UUID) error

	CreateItem(ctx context.Context, in CreateItemInput) (*Item, error)
	GetItemByID(ctx context.Context, id uuid.UUID) (*Item, error)
	ListItemsByRestaurant(ctx context.Context, restaurantID uuid.UUID) ([]*Item, error)
	ListItemsByCategory(ctx context.Context, categoryID uuid.UUID) ([]*Item, error)
	UpdateItem(ctx context.Context, id uuid.UUID, in UpdateItemInput) (*Item, error)
	SetItemAvailability(ctx context.Context, id uuid.UUID, available bool) error
	SetItemActive(ctx context.Context, id uuid.UUID, active bool) error
	DeleteItem(ctx context.Context, id uuid.UUID) error

	CreateVariantGroup(ctx context.Context, g *VariantGroup) (*VariantGroup, error)
	ListVariantGroupsByItem(ctx context.Context, itemID uuid.UUID) ([]*VariantGroup, error)
	DeleteVariantGroup(ctx context.Context, id uuid.UUID) error

	CreateVariant(ctx context.Context, v *Variant) (*Variant, error)
	ListVariantsByGroup(ctx context.Context, groupID uuid.UUID) ([]*Variant, error)
	ListVariantsByItem(ctx context.Context, itemID uuid.UUID) ([]*Variant, error)
	SetVariantAvailability(ctx context.Context, id uuid.UUID, available bool) error
	DeleteVariant(ctx context.Context, id uuid.UUID) error

	CreateAddonGroup(ctx context.Context, g *AddonGroup) (*AddonGroup, error)
	ListAddonGroupsByItem(ctx context.Context, itemID uuid.UUID) ([]*AddonGroup, error)
	DeleteAddonGroup(ctx context.Context, id uuid.UUID) error

	CreateAddon(ctx context.Context, a *Addon) (*Addon, error)
	ListAddonsByGroup(ctx context.Context, groupID uuid.UUID) ([]*Addon, error)
	ListAddonsByItem(ctx context.Context, itemID uuid.UUID) ([]*Addon, error)
	SetAddonAvailability(ctx context.Context, id uuid.UUID, available bool) error
	DeleteAddon(ctx context.Context, id uuid.UUID) error
}

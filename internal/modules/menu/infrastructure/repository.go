package infrastructure

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/foodapp/backend/internal/modules/menu/domain"
	sqlcgen "github.com/foodapp/backend/internal/platform/db/sqlc"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type Repository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) *Repository {
	return &Repository{q: q}
}

func (r *Repository) CreateCategory(ctx context.Context, restaurantID uuid.UUID, name string, description *string, displayOrder int) (*domain.Category, error) {
	row, err := r.q.CreateMenuCategory(ctx, sqlcgen.CreateMenuCategoryParams{
		RestaurantID: restaurantID, Name: name, Description: toText(description), DisplayOrder: int32(displayOrder),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create category", err)
	}
	return mapCategory(row), nil
}

func (r *Repository) GetCategoryByID(ctx context.Context, id uuid.UUID) (*domain.Category, error) {
	row, err := r.q.GetMenuCategoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("category")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch category", err)
	}
	return mapCategory(row), nil
}

func (r *Repository) ListCategories(ctx context.Context, restaurantID uuid.UUID) ([]*domain.Category, error) {
	rows, err := r.q.ListMenuCategories(ctx, restaurantID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list categories", err)
	}
	out := make([]*domain.Category, len(rows))
	for i, row := range rows {
		out[i] = mapCategory(row)
	}
	return out, nil
}

func (r *Repository) UpdateCategory(ctx context.Context, id uuid.UUID, name, description *string, displayOrder *int) (*domain.Category, error) {
	var order pgtype.Int4
	if displayOrder != nil {
		order = pgtype.Int4{Int32: int32(*displayOrder), Valid: true}
	}
	row, err := r.q.UpdateMenuCategory(ctx, sqlcgen.UpdateMenuCategoryParams{
		ID: id, Name: toText(name), Description: toText(description), DisplayOrder: order,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to update category", err)
	}
	return mapCategory(row), nil
}

func (r *Repository) SetCategoryActive(ctx context.Context, id uuid.UUID, active bool) error {
	if err := r.q.SetMenuCategoryActive(ctx, sqlcgen.SetMenuCategoryActiveParams{ID: id, IsActive: active}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update category", err)
	}
	return nil
}

func (r *Repository) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	if err := r.q.DeleteMenuCategory(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to delete category", err)
	}
	return nil
}

func (r *Repository) CreateItem(ctx context.Context, in domain.CreateItemInput) (*domain.Item, error) {
	var price pgtype.Numeric
	_ = price.Scan(in.BasePrice)

	var calories pgtype.Int4
	if in.Calories != nil {
		calories = pgtype.Int4{Int32: int32(*in.Calories), Valid: true}
	}
	var spiceLevel pgtype.Int2
	if in.SpiceLevel != nil {
		spiceLevel = pgtype.Int2{Int16: int16(*in.SpiceLevel), Valid: true}
	}
	var prepTime pgtype.Int2
	if in.PrepTimeMins != nil {
		prepTime = pgtype.Int2{Int16: int16(*in.PrepTimeMins), Valid: true}
	}

	row, err := r.q.CreateMenuItem(ctx, sqlcgen.CreateMenuItemParams{
		RestaurantID: in.RestaurantID,
		CategoryID:   in.CategoryID,
		Name:         in.Name,
		Description:  toText(in.Description),
		FoodType:     sqlcgen.FoodType(in.FoodType),
		BasePrice:    price,
		ImageUrl:     toText(in.ImageURL),
		Calories:     calories,
		SpiceLevel:   spiceLevel,
		PrepTimeMins: prepTime,
		DisplayOrder: int32(in.DisplayOrder),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create menu item", err)
	}
	return mapItem(row), nil
}

func (r *Repository) GetItemByID(ctx context.Context, id uuid.UUID) (*domain.Item, error) {
	row, err := r.q.GetMenuItemByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("menu item")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch menu item", err)
	}
	return mapItem(row), nil
}

func (r *Repository) ListItemsByRestaurant(ctx context.Context, restaurantID uuid.UUID) ([]*domain.Item, error) {
	rows, err := r.q.ListMenuItemsByRestaurant(ctx, restaurantID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list menu items", err)
	}
	out := make([]*domain.Item, len(rows))
	for i, row := range rows {
		out[i] = mapItem(row)
	}
	return out, nil
}

func (r *Repository) ListItemsByCategory(ctx context.Context, categoryID uuid.UUID) ([]*domain.Item, error) {
	rows, err := r.q.ListMenuItemsByCategory(ctx, categoryID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list menu items", err)
	}
	out := make([]*domain.Item, len(rows))
	for i, row := range rows {
		out[i] = mapItem(row)
	}
	return out, nil
}

func (r *Repository) UpdateItem(ctx context.Context, id uuid.UUID, in domain.UpdateItemInput) (*domain.Item, error) {
	var price pgtype.Numeric
	if in.BasePrice != nil {
		_ = price.Scan(*in.BasePrice)
	}
	var calories pgtype.Int4
	if in.Calories != nil {
		calories = pgtype.Int4{Int32: int32(*in.Calories), Valid: true}
	}
	var spiceLevel pgtype.Int2
	if in.SpiceLevel != nil {
		spiceLevel = pgtype.Int2{Int16: int16(*in.SpiceLevel), Valid: true}
	}
	var prepTime pgtype.Int2
	if in.PrepTimeMins != nil {
		prepTime = pgtype.Int2{Int16: int16(*in.PrepTimeMins), Valid: true}
	}
	var foodType sqlcgen.NullFoodType
	if in.FoodType != nil {
		foodType = sqlcgen.NullFoodType{FoodType: sqlcgen.FoodType(*in.FoodType), Valid: true}
	}
	var categoryID pgtype.UUID
	if in.CategoryID != nil {
		categoryID = pgtype.UUID{Bytes: *in.CategoryID, Valid: true}
	}

	row, err := r.q.UpdateMenuItem(ctx, sqlcgen.UpdateMenuItemParams{
		ID: id, Name: toText(in.Name), Description: toText(in.Description), FoodType: foodType,
		BasePrice: price, ImageUrl: toText(in.ImageURL), Calories: calories, SpiceLevel: spiceLevel,
		PrepTimeMins: prepTime, CategoryID: categoryID,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to update menu item", err)
	}
	return mapItem(row), nil
}

func (r *Repository) SetItemAvailability(ctx context.Context, id uuid.UUID, available bool) error {
	if err := r.q.SetMenuItemAvailability(ctx, sqlcgen.SetMenuItemAvailabilityParams{ID: id, IsAvailable: available}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update item availability", err)
	}
	return nil
}

func (r *Repository) SetItemActive(ctx context.Context, id uuid.UUID, active bool) error {
	if err := r.q.SetMenuItemActive(ctx, sqlcgen.SetMenuItemActiveParams{ID: id, IsActive: active}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update item", err)
	}
	return nil
}

func (r *Repository) DeleteItem(ctx context.Context, id uuid.UUID) error {
	if err := r.q.SoftDeleteMenuItem(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to delete menu item", err)
	}
	return nil
}

func (r *Repository) CreateVariantGroup(ctx context.Context, g *domain.VariantGroup) (*domain.VariantGroup, error) {
	row, err := r.q.CreateVariantGroup(ctx, sqlcgen.CreateVariantGroupParams{
		MenuItemID: g.MenuItemID, Name: g.Name, IsRequired: g.IsRequired,
		MinSelect: int16(g.MinSelect), MaxSelect: int16(g.MaxSelect), DisplayOrder: int32(g.DisplayOrder),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create variant group", err)
	}
	return mapVariantGroup(row), nil
}

func (r *Repository) ListVariantGroupsByItem(ctx context.Context, itemID uuid.UUID) ([]*domain.VariantGroup, error) {
	rows, err := r.q.ListVariantGroupsByItem(ctx, itemID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list variant groups", err)
	}
	out := make([]*domain.VariantGroup, len(rows))
	for i, row := range rows {
		out[i] = mapVariantGroup(row)
	}
	return out, nil
}

func (r *Repository) DeleteVariantGroup(ctx context.Context, id uuid.UUID) error {
	if err := r.q.DeleteVariantGroup(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to delete variant group", err)
	}
	return nil
}

func (r *Repository) CreateVariant(ctx context.Context, v *domain.Variant) (*domain.Variant, error) {
	var price pgtype.Numeric
	_ = price.Scan(v.Price)
	row, err := r.q.CreateMenuItemVariant(ctx, sqlcgen.CreateMenuItemVariantParams{
		VariantGroupID: v.VariantGroupID, Name: v.Name, Price: price, DisplayOrder: int32(v.DisplayOrder),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create variant", err)
	}
	return mapVariant(row), nil
}

func (r *Repository) ListVariantsByGroup(ctx context.Context, groupID uuid.UUID) ([]*domain.Variant, error) {
	rows, err := r.q.ListVariantsByGroup(ctx, groupID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list variants", err)
	}
	out := make([]*domain.Variant, len(rows))
	for i, row := range rows {
		out[i] = mapVariant(row)
	}
	return out, nil
}

func (r *Repository) ListVariantsByItem(ctx context.Context, itemID uuid.UUID) ([]*domain.Variant, error) {
	rows, err := r.q.ListVariantsByItem(ctx, itemID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list variants", err)
	}
	out := make([]*domain.Variant, len(rows))
	for i, row := range rows {
		out[i] = mapVariant(row)
	}
	return out, nil
}

func (r *Repository) SetVariantAvailability(ctx context.Context, id uuid.UUID, available bool) error {
	if err := r.q.SetVariantAvailability(ctx, sqlcgen.SetVariantAvailabilityParams{ID: id, IsAvailable: available}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update variant", err)
	}
	return nil
}

func (r *Repository) DeleteVariant(ctx context.Context, id uuid.UUID) error {
	if err := r.q.DeleteMenuItemVariant(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to delete variant", err)
	}
	return nil
}

func (r *Repository) CreateAddonGroup(ctx context.Context, g *domain.AddonGroup) (*domain.AddonGroup, error) {
	row, err := r.q.CreateAddonGroup(ctx, sqlcgen.CreateAddonGroupParams{
		MenuItemID: g.MenuItemID, Name: g.Name, IsRequired: g.IsRequired,
		MinSelect: int16(g.MinSelect), MaxSelect: int16(g.MaxSelect), DisplayOrder: int32(g.DisplayOrder),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create add-on group", err)
	}
	return mapAddonGroup(row), nil
}

func (r *Repository) ListAddonGroupsByItem(ctx context.Context, itemID uuid.UUID) ([]*domain.AddonGroup, error) {
	rows, err := r.q.ListAddonGroupsByItem(ctx, itemID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list add-on groups", err)
	}
	out := make([]*domain.AddonGroup, len(rows))
	for i, row := range rows {
		out[i] = mapAddonGroup(row)
	}
	return out, nil
}

func (r *Repository) DeleteAddonGroup(ctx context.Context, id uuid.UUID) error {
	if err := r.q.DeleteAddonGroup(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to delete add-on group", err)
	}
	return nil
}

func (r *Repository) CreateAddon(ctx context.Context, a *domain.Addon) (*domain.Addon, error) {
	var price pgtype.Numeric
	_ = price.Scan(a.Price)
	row, err := r.q.CreateMenuAddon(ctx, sqlcgen.CreateMenuAddonParams{
		AddonGroupID: a.AddonGroupID, Name: a.Name, Price: price, DisplayOrder: int32(a.DisplayOrder),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create add-on", err)
	}
	return mapAddon(row), nil
}

func (r *Repository) ListAddonsByGroup(ctx context.Context, groupID uuid.UUID) ([]*domain.Addon, error) {
	rows, err := r.q.ListAddonsByGroup(ctx, groupID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list add-ons", err)
	}
	out := make([]*domain.Addon, len(rows))
	for i, row := range rows {
		out[i] = mapAddon(row)
	}
	return out, nil
}

func (r *Repository) ListAddonsByItem(ctx context.Context, itemID uuid.UUID) ([]*domain.Addon, error) {
	rows, err := r.q.ListAddonsByItem(ctx, itemID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list add-ons", err)
	}
	out := make([]*domain.Addon, len(rows))
	for i, row := range rows {
		out[i] = mapAddon(row)
	}
	return out, nil
}

func (r *Repository) SetAddonAvailability(ctx context.Context, id uuid.UUID, available bool) error {
	if err := r.q.SetAddonAvailability(ctx, sqlcgen.SetAddonAvailabilityParams{ID: id, IsAvailable: available}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update add-on", err)
	}
	return nil
}

func (r *Repository) DeleteAddon(ctx context.Context, id uuid.UUID) error {
	if err := r.q.DeleteMenuAddon(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to delete add-on", err)
	}
	return nil
}

// --- mapping helpers ---

func mapCategory(row sqlcgen.MenuCategory) *domain.Category {
	c := &domain.Category{ID: row.ID, RestaurantID: row.RestaurantID, Name: row.Name, DisplayOrder: int(row.DisplayOrder), IsActive: row.IsActive}
	if row.Description.Valid {
		c.Description = &row.Description.String
	}
	return c
}

func mapItem(row sqlcgen.MenuItem) *domain.Item {
	price, _ := row.BasePrice.Float64Value()
	item := &domain.Item{
		ID: row.ID, RestaurantID: row.RestaurantID, CategoryID: row.CategoryID, Name: row.Name,
		FoodType: domain.FoodType(row.FoodType), BasePrice: price.Float64,
		IsAvailable: row.IsAvailable, IsActive: row.IsActive, DisplayOrder: int(row.DisplayOrder),
		IsBestseller: row.IsBestseller,
	}
	if row.Description.Valid {
		item.Description = &row.Description.String
	}
	if row.ImageUrl.Valid {
		item.ImageURL = &row.ImageUrl.String
	}
	if row.Calories.Valid {
		c := int(row.Calories.Int32)
		item.Calories = &c
	}
	if row.SpiceLevel.Valid {
		sl := int(row.SpiceLevel.Int16)
		item.SpiceLevel = &sl
	}
	if row.PrepTimeMins.Valid {
		pt := int(row.PrepTimeMins.Int16)
		item.PrepTimeMins = &pt
	}
	return item
}

func mapVariantGroup(row sqlcgen.MenuItemVariantGroup) *domain.VariantGroup {
	return &domain.VariantGroup{
		ID: row.ID, MenuItemID: row.MenuItemID, Name: row.Name, IsRequired: row.IsRequired,
		MinSelect: int(row.MinSelect), MaxSelect: int(row.MaxSelect), DisplayOrder: int(row.DisplayOrder),
	}
}

func mapVariant(row sqlcgen.MenuItemVariant) *domain.Variant {
	price, _ := row.Price.Float64Value()
	return &domain.Variant{
		ID: row.ID, VariantGroupID: row.VariantGroupID, Name: row.Name, Price: price.Float64,
		IsAvailable: row.IsAvailable, DisplayOrder: int(row.DisplayOrder),
	}
}

func mapAddonGroup(row sqlcgen.MenuAddonGroup) *domain.AddonGroup {
	return &domain.AddonGroup{
		ID: row.ID, MenuItemID: row.MenuItemID, Name: row.Name, IsRequired: row.IsRequired,
		MinSelect: int(row.MinSelect), MaxSelect: int(row.MaxSelect), DisplayOrder: int(row.DisplayOrder),
	}
}

func mapAddon(row sqlcgen.MenuAddon) *domain.Addon {
	price, _ := row.Price.Float64Value()
	return &domain.Addon{
		ID: row.ID, AddonGroupID: row.AddonGroupID, Name: row.Name, Price: price.Float64,
		IsAvailable: row.IsAvailable, DisplayOrder: int(row.DisplayOrder),
	}
}

func toText(s *string) pgtype.Text {
	if s == nil || *s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

var _ domain.Repository = (*Repository)(nil)

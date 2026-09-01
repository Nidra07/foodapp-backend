package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/menu/application"
	"github.com/foodapp/backend/internal/modules/menu/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/response"
)

type MenuHandler struct {
	svc *application.MenuService
}

func NewMenuHandler(svc *application.MenuService) *MenuHandler {
	return &MenuHandler{svc: svc}
}

// RegisterRoutes mounts the public "view menu" route and owner-only
// management routes for categories/items/variants/add-ons.
func (h *MenuHandler) RegisterRoutes(rg *gin.RouterGroup, authMW, ownerOnly gin.HandlerFunc) {
	public := rg.Group("/restaurants/:restaurantId/menu")
	public.GET("", h.GetFullMenu)

	owner := rg.Group("/restaurants/:restaurantId/menu", authMW, ownerOnly)
	owner.POST("/categories", h.CreateCategory)
	owner.GET("/categories", h.ListCategories)
	owner.PATCH("/categories/:categoryId", h.UpdateCategory)
	owner.DELETE("/categories/:categoryId", h.DeleteCategory)

	owner.POST("/items", h.CreateItem)
	owner.GET("/items/:itemId", h.GetItem)
	owner.PATCH("/items/:itemId", h.UpdateItem)
	owner.PATCH("/items/:itemId/availability", h.SetItemAvailability)
	owner.DELETE("/items/:itemId", h.DeleteItem)

	owner.POST("/items/:itemId/variant-groups", h.CreateVariantGroup)
	owner.POST("/variant-groups/:groupId/variants", h.CreateVariant)

	owner.POST("/items/:itemId/addon-groups", h.CreateAddonGroup)
	owner.POST("/addon-groups/:groupId/addons", h.CreateAddon)
}

func (h *MenuHandler) GetFullMenu(c *gin.Context) {
	restaurantID, err := uuid.Parse(c.Param("restaurantId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	menu, err := h.svc.GetFullMenu(c.Request.Context(), restaurantID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, menuToJSON(menu))
}

type createCategoryBody struct {
	Name         string  `json:"name" binding:"required"`
	Description  *string `json:"description"`
	DisplayOrder int     `json:"display_order"`
}

func (h *MenuHandler) CreateCategory(c *gin.Context) {
	restaurantID, err := uuid.Parse(c.Param("restaurantId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	var body createCategoryBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	cat, err := h.svc.CreateCategory(c.Request.Context(), restaurantID, body.Name, body.Description, body.DisplayOrder)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, cat)
}

func (h *MenuHandler) ListCategories(c *gin.Context) {
	restaurantID, err := uuid.Parse(c.Param("restaurantId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	cats, err := h.svc.ListCategories(c.Request.Context(), restaurantID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, cats)
}

type updateCategoryBody struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	DisplayOrder *int    `json:"display_order"`
}

func (h *MenuHandler) UpdateCategory(c *gin.Context) {
	id, err := uuid.Parse(c.Param("categoryId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid category id", nil))
		return
	}
	var body updateCategoryBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", nil))
		return
	}
	cat, err := h.svc.UpdateCategory(c.Request.Context(), id, body.Name, body.Description, body.DisplayOrder)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, cat)
}

func (h *MenuHandler) DeleteCategory(c *gin.Context) {
	id, err := uuid.Parse(c.Param("categoryId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid category id", nil))
		return
	}
	if err := h.svc.DeleteCategory(c.Request.Context(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

type createItemBody struct {
	CategoryID   string  `json:"category_id" binding:"required"`
	Name         string  `json:"name" binding:"required"`
	Description  *string `json:"description"`
	FoodType     string  `json:"food_type" binding:"required,oneof=veg non_veg egg"`
	BasePrice    float64 `json:"base_price" binding:"required"`
	ImageURL     *string `json:"image_url"`
	Calories     *int    `json:"calories"`
	SpiceLevel   *int    `json:"spice_level"`
	PrepTimeMins *int    `json:"prep_time_mins"`
	DisplayOrder int     `json:"display_order"`
}

func (h *MenuHandler) CreateItem(c *gin.Context) {
	restaurantID, err := uuid.Parse(c.Param("restaurantId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	var body createItemBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	categoryID, err := uuid.Parse(body.CategoryID)
	if err != nil {
		response.Error(c, apperr.Validation("invalid category_id", nil))
		return
	}

	item, err := h.svc.CreateItem(c.Request.Context(), domain.CreateItemInput{
		RestaurantID: restaurantID, CategoryID: categoryID, Name: body.Name, Description: body.Description,
		FoodType: domain.FoodType(body.FoodType), BasePrice: body.BasePrice, ImageURL: body.ImageURL,
		Calories: body.Calories, SpiceLevel: body.SpiceLevel, PrepTimeMins: body.PrepTimeMins, DisplayOrder: body.DisplayOrder,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, itemToJSON(item))
}

func (h *MenuHandler) GetItem(c *gin.Context) {
	id, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid item id", nil))
		return
	}
	item, err := h.svc.GetItem(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, itemToJSON(item))
}

type updateItemBody struct {
	Name         *string  `json:"name"`
	Description  *string  `json:"description"`
	FoodType     *string  `json:"food_type" binding:"omitempty,oneof=veg non_veg egg"`
	BasePrice    *float64 `json:"base_price"`
	ImageURL     *string  `json:"image_url"`
	Calories     *int     `json:"calories"`
	SpiceLevel   *int     `json:"spice_level"`
	PrepTimeMins *int     `json:"prep_time_mins"`
	CategoryID   *string  `json:"category_id"`
}

func (h *MenuHandler) UpdateItem(c *gin.Context) {
	id, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid item id", nil))
		return
	}
	var body updateItemBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}

	in := domain.UpdateItemInput{
		Name: body.Name, Description: body.Description, BasePrice: body.BasePrice, ImageURL: body.ImageURL,
		Calories: body.Calories, SpiceLevel: body.SpiceLevel, PrepTimeMins: body.PrepTimeMins,
	}
	if body.FoodType != nil {
		ft := domain.FoodType(*body.FoodType)
		in.FoodType = &ft
	}
	if body.CategoryID != nil {
		catID, err := uuid.Parse(*body.CategoryID)
		if err != nil {
			response.Error(c, apperr.Validation("invalid category_id", nil))
			return
		}
		in.CategoryID = &catID
	}

	item, err := h.svc.UpdateItem(c.Request.Context(), id, in)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, itemToJSON(item))
}

type setAvailabilityBody struct {
	Available bool `json:"available"`
}

func (h *MenuHandler) SetItemAvailability(c *gin.Context) {
	id, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid item id", nil))
		return
	}
	var body setAvailabilityBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", nil))
		return
	}
	if err := h.svc.SetItemAvailability(c.Request.Context(), id, body.Available); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"is_available": body.Available})
}

func (h *MenuHandler) DeleteItem(c *gin.Context) {
	id, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid item id", nil))
		return
	}
	if err := h.svc.DeleteItem(c.Request.Context(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

type createVariantGroupBody struct {
	Name         string `json:"name" binding:"required"`
	IsRequired   bool   `json:"is_required"`
	MinSelect    int    `json:"min_select"`
	MaxSelect    int    `json:"max_select"`
	DisplayOrder int    `json:"display_order"`
}

func (h *MenuHandler) CreateVariantGroup(c *gin.Context) {
	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid item id", nil))
		return
	}
	var body createVariantGroupBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	if body.MaxSelect == 0 {
		body.MaxSelect = 1
	}
	group, err := h.svc.CreateVariantGroup(c.Request.Context(), &domain.VariantGroup{
		MenuItemID: itemID, Name: body.Name, IsRequired: body.IsRequired,
		MinSelect: body.MinSelect, MaxSelect: body.MaxSelect, DisplayOrder: body.DisplayOrder,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, group)
}

type createVariantBody struct {
	Name         string  `json:"name" binding:"required"`
	Price        float64 `json:"price" binding:"required"`
	DisplayOrder int     `json:"display_order"`
}

func (h *MenuHandler) CreateVariant(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid group id", nil))
		return
	}
	var body createVariantBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	variant, err := h.svc.CreateVariant(c.Request.Context(), &domain.Variant{
		VariantGroupID: groupID, Name: body.Name, Price: body.Price, DisplayOrder: body.DisplayOrder,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, variant)
}

type createAddonGroupBody struct {
	Name         string `json:"name" binding:"required"`
	IsRequired   bool   `json:"is_required"`
	MinSelect    int    `json:"min_select"`
	MaxSelect    int    `json:"max_select"`
	DisplayOrder int    `json:"display_order"`
}

func (h *MenuHandler) CreateAddonGroup(c *gin.Context) {
	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid item id", nil))
		return
	}
	var body createAddonGroupBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	if body.MaxSelect == 0 {
		body.MaxSelect = 1
	}
	group, err := h.svc.CreateAddonGroup(c.Request.Context(), &domain.AddonGroup{
		MenuItemID: itemID, Name: body.Name, IsRequired: body.IsRequired,
		MinSelect: body.MinSelect, MaxSelect: body.MaxSelect, DisplayOrder: body.DisplayOrder,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, group)
}

type createAddonBody struct {
	Name         string  `json:"name" binding:"required"`
	Price        float64 `json:"price"`
	DisplayOrder int     `json:"display_order"`
}

func (h *MenuHandler) CreateAddon(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid group id", nil))
		return
	}
	var body createAddonBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	addon, err := h.svc.CreateAddon(c.Request.Context(), &domain.Addon{
		AddonGroupID: groupID, Name: body.Name, Price: body.Price, DisplayOrder: body.DisplayOrder,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, addon)
}

// --- JSON shaping helpers ---

func itemToJSON(item *domain.Item) gin.H {
	return gin.H{
		"id": item.ID, "category_id": item.CategoryID, "name": item.Name, "description": item.Description,
		"food_type": item.FoodType, "base_price": item.BasePrice, "image_url": item.ImageURL,
		"is_available": item.IsAvailable, "is_active": item.IsActive, "is_bestseller": item.IsBestseller,
		"calories": item.Calories, "spice_level": item.SpiceLevel, "prep_time_mins": item.PrepTimeMins,
	}
}

func menuToJSON(menu *domain.FullMenu) gin.H {
	categories := make([]gin.H, len(menu.Categories))
	for i, cwi := range menu.Categories {
		items := make([]gin.H, len(cwi.Items))
		for j, iwo := range cwi.Items {
			variantGroups := make([]gin.H, len(iwo.VariantGroups))
			for k, vg := range iwo.VariantGroups {
				variantGroups[k] = gin.H{"id": vg.Group.ID, "name": vg.Group.Name, "is_required": vg.Group.IsRequired,
					"min_select": vg.Group.MinSelect, "max_select": vg.Group.MaxSelect, "variants": vg.Variants}
			}
			addonGroups := make([]gin.H, len(iwo.AddonGroups))
			for k, ag := range iwo.AddonGroups {
				addonGroups[k] = gin.H{"id": ag.Group.ID, "name": ag.Group.Name, "is_required": ag.Group.IsRequired,
					"min_select": ag.Group.MinSelect, "max_select": ag.Group.MaxSelect, "addons": ag.Addons}
			}
			item := itemToJSON(iwo.Item)
			item["variant_groups"] = variantGroups
			item["addon_groups"] = addonGroups
			items[j] = item
		}
		categories[i] = gin.H{"id": cwi.Category.ID, "name": cwi.Category.Name, "description": cwi.Category.Description, "items": items}
	}
	return gin.H{"categories": categories}
}

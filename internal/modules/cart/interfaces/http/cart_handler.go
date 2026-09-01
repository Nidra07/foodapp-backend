package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/cart/application"
	"github.com/foodapp/backend/internal/modules/cart/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/middleware"
	"github.com/foodapp/backend/internal/platform/response"
)

type CartHandler struct {
	svc *application.CartService
}

func NewCartHandler(svc *application.CartService) *CartHandler {
	return &CartHandler{svc: svc}
}

func (h *CartHandler) RegisterRoutes(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	cart := rg.Group("/cart", authMW)
	cart.GET("", h.GetMyCart)
	cart.POST("/items", h.AddItem)
	cart.PATCH("/items/:itemId", h.UpdateItemQuantity)
	cart.DELETE("/items/:itemId", h.RemoveItem)
	cart.DELETE("", h.ClearCart)
}

func (h *CartHandler) GetMyCart(c *gin.Context) {
	customerID, _ := middleware.UserIDFromContext(c)
	id, _ := uuid.Parse(customerID)

	summary, err := h.svc.GetMyCart(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, summaryToJSON(summary))
}

type addItemBody struct {
	RestaurantID        string   `json:"restaurant_id" binding:"required"`
	MenuItemID          string   `json:"menu_item_id" binding:"required"`
	VariantID           *string  `json:"variant_id"`
	AddonIDs            []string `json:"addon_ids"`
	Quantity            int      `json:"quantity"`
	SpecialInstructions *string  `json:"special_instructions"`
}

func (h *CartHandler) AddItem(c *gin.Context) {
	var body addItemBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}

	restaurantID, err := uuid.Parse(body.RestaurantID)
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant_id", nil))
		return
	}
	menuItemID, err := uuid.Parse(body.MenuItemID)
	if err != nil {
		response.Error(c, apperr.Validation("invalid menu_item_id", nil))
		return
	}
	var variantID *uuid.UUID
	if body.VariantID != nil {
		vid, err := uuid.Parse(*body.VariantID)
		if err != nil {
			response.Error(c, apperr.Validation("invalid variant_id", nil))
			return
		}
		variantID = &vid
	}
	addonIDs := make([]uuid.UUID, 0, len(body.AddonIDs))
	for _, a := range body.AddonIDs {
		aid, err := uuid.Parse(a)
		if err != nil {
			response.Error(c, apperr.Validation("invalid addon id in addon_ids", nil))
			return
		}
		addonIDs = append(addonIDs, aid)
	}

	customerID, _ := middleware.UserIDFromContext(c)
	custUUID, _ := uuid.Parse(customerID)

	summary, err := h.svc.AddItem(c.Request.Context(), custUUID, restaurantID, domain.AddItemInput{
		MenuItemID: menuItemID, VariantID: variantID, AddonIDs: addonIDs,
		Quantity: body.Quantity, SpecialInstructions: body.SpecialInstructions,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, summaryToJSON(summary))
}

type updateQuantityBody struct {
	Quantity int `json:"quantity" binding:"required"`
}

func (h *CartHandler) UpdateItemQuantity(c *gin.Context) {
	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid cart item id", nil))
		return
	}
	var body updateQuantityBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", nil))
		return
	}
	if err := h.svc.UpdateItemQuantity(c.Request.Context(), itemID, body.Quantity); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"message": "quantity updated"})
}

func (h *CartHandler) RemoveItem(c *gin.Context) {
	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid cart item id", nil))
		return
	}
	if err := h.svc.RemoveItem(c.Request.Context(), itemID); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

func (h *CartHandler) ClearCart(c *gin.Context) {
	customerID, _ := middleware.UserIDFromContext(c)
	id, _ := uuid.Parse(customerID)
	if err := h.svc.ClearCart(c.Request.Context(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

func summaryToJSON(s *domain.CartSummary) gin.H {
	items := make([]gin.H, len(s.Items))
	for i, pi := range s.Items {
		addons := make([]gin.H, len(pi.Addons))
		for j, a := range pi.Addons {
			addons[j] = gin.H{"addon_id": a.AddonID, "name": a.Name, "price": a.Price, "is_available": a.IsAvailable}
		}
		items[i] = gin.H{
			"id": pi.Item.ID, "menu_item_id": pi.Item.MenuItemID, "variant_id": pi.Item.VariantID,
			"item_name": pi.ItemName, "item_is_available": pi.ItemIsAvailable,
			"variant_name": pi.VariantName, "variant_is_available": pi.VariantIsAvailable,
			"unit_price": pi.UnitPrice, "quantity": pi.Item.Quantity, "addons": addons,
			"special_instructions": pi.Item.SpecialInstructions, "line_total": pi.LineTotal,
		}
	}

	out := gin.H{"items": items, "subtotal": s.Subtotal, "has_unavailable_items": s.HasUnavailable}
	if s.Cart != nil {
		out["cart_id"] = s.Cart.ID
		out["restaurant_id"] = s.Cart.RestaurantID
	}
	return out
}

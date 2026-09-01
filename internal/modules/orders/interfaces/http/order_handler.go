package http

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/orders/application"
	"github.com/foodapp/backend/internal/modules/orders/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/middleware"
	"github.com/foodapp/backend/internal/platform/response"
)

type OrderHandler struct {
	svc *application.OrderService
}

func NewOrderHandler(svc *application.OrderService) *OrderHandler {
	return &OrderHandler{svc: svc}
}

// RegisterRoutes mounts customer-facing checkout/order-history routes
// and restaurant-facing order-queue/status-update routes. All require
// auth; role-based access (customer vs restaurant_owner/staff vs admin)
// is enforced per-route via the passed-in middleware.
func (h *OrderHandler) RegisterRoutes(rg *gin.RouterGroup, authMW, customerOnly, restaurantOnly gin.HandlerFunc) {
	customer := rg.Group("/orders", authMW, customerOnly)
	customer.POST("/checkout", h.Checkout)
	customer.GET("/mine", h.ListMyOrders)
	customer.GET("/:id", h.GetByID)
	customer.GET("/:id/history", h.GetStatusHistory)
	customer.POST("/:id/cancel", h.Cancel)
	customer.GET("/track/:orderNumber", h.TrackByNumber)

	restaurant := rg.Group("/restaurants/:restaurantId/orders", authMW, restaurantOnly)
	restaurant.GET("", h.ListRestaurantOrders)
	restaurant.GET("/queue", h.ListActiveQueue)
	restaurant.PATCH("/:id/status", h.UpdateStatus)
}

type checkoutBody struct {
	PaymentMethod       string  `json:"payment_method" binding:"required,oneof=cod upi card wallet"`
	AddressLine1        string  `json:"address_line1" binding:"required"`
	AddressLine2        *string `json:"address_line2"`
	City                string  `json:"city" binding:"required"`
	State               string  `json:"state" binding:"required"`
	PostalCode          string  `json:"postal_code" binding:"required"`
	Lat                 float64 `json:"lat" binding:"required"`
	Lng                 float64 `json:"lng" binding:"required"`
	ContactPhone        string  `json:"contact_phone" binding:"required"`
	SpecialInstructions *string `json:"special_instructions"`
}

func (h *OrderHandler) Checkout(c *gin.Context) {
	var body checkoutBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}

	customerID, _ := middleware.UserIDFromContext(c)
	custUUID, _ := uuid.Parse(customerID)

	order, err := h.svc.Checkout(c.Request.Context(), custUUID, domain.PaymentMethod(body.PaymentMethod), domain.DeliveryAddress{
		Line1: body.AddressLine1, Line2: body.AddressLine2, City: body.City, State: body.State,
		PostalCode: body.PostalCode, Lat: body.Lat, Lng: body.Lng, Phone: body.ContactPhone,
	}, body.SpecialInstructions)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, orderToJSON(order))
}

func (h *OrderHandler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid order id", nil))
		return
	}
	order, items, err := h.svc.GetWithItems(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	if !h.canAccessOrder(c, order.CustomerID) {
		response.Error(c, apperr.Forbidden("you do not have access to this order"))
		return
	}
	out := orderToJSON(order)
	out["items"] = itemsToJSON(items)
	response.OK(c, out)
}

// canAccessOrder allows the order's own customer or an admin; every
// customer-facing order route (GetByID, Cancel, GetStatusHistory) must
// call this before returning data — otherwise any authenticated customer
// could read/cancel any order by guessing/enumerating IDs.
func (h *OrderHandler) canAccessOrder(c *gin.Context, orderCustomerID uuid.UUID) bool {
	requesterID, _ := middleware.UserIDFromContext(c)
	requesterUUID, _ := uuid.Parse(requesterID)
	if requesterUUID == orderCustomerID {
		return true
	}
	role, _ := c.Get(string(middleware.CtxRole))
	roleStr, _ := role.(string)
	return roleStr == "admin"
}

func (h *OrderHandler) TrackByNumber(c *gin.Context) {
	order, err := h.svc.GetByNumber(c.Request.Context(), c.Param("orderNumber"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, orderToJSON(order))
}

func (h *OrderHandler) ListMyOrders(c *gin.Context) {
	customerID, _ := middleware.UserIDFromContext(c)
	custUUID, _ := uuid.Parse(customerID)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	orders, total, err := h.svc.ListMyOrders(c.Request.Context(), custUUID, page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(orders))
	for i, o := range orders {
		out[i] = orderToJSON(o)
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	response.Paginated(c, out, response.Meta{Page: page, PageSize: pageSize, TotalCount: total, TotalPages: totalPages})
}

func (h *OrderHandler) GetStatusHistory(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid order id", nil))
		return
	}
	order, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	if !h.canAccessOrder(c, order.CustomerID) {
		response.Error(c, apperr.Forbidden("you do not have access to this order"))
		return
	}
	history, err := h.svc.GetStatusHistory(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, history)
}

type cancelBody struct {
	Reason string `json:"reason" binding:"required"`
}

func (h *OrderHandler) Cancel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid order id", nil))
		return
	}
	var body cancelBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}

	existing, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	if !h.canAccessOrder(c, existing.CustomerID) {
		response.Error(c, apperr.Forbidden("you do not have access to this order"))
		return
	}

	actorID, _ := middleware.UserIDFromContext(c)
	actorUUID, _ := uuid.Parse(actorID)

	order, err := h.svc.Cancel(c.Request.Context(), id, body.Reason, actorUUID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, orderToJSON(order))
}

func (h *OrderHandler) ListRestaurantOrders(c *gin.Context) {
	restaurantID, err := uuid.Parse(c.Param("restaurantId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	var status *domain.Status
	if s := c.Query("status"); s != "" {
		st := domain.Status(s)
		status = &st
	}

	orders, total, err := h.svc.ListRestaurantOrders(c.Request.Context(), restaurantID, domain.ListFilter{Status: status, Page: page, PageSize: pageSize})
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(orders))
	for i, o := range orders {
		out[i] = orderToJSON(o)
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	response.Paginated(c, out, response.Meta{Page: page, PageSize: pageSize, TotalCount: total, TotalPages: totalPages})
}

func (h *OrderHandler) ListActiveQueue(c *gin.Context) {
	restaurantID, err := uuid.Parse(c.Param("restaurantId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	orders, err := h.svc.ListActiveQueue(c.Request.Context(), restaurantID)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(orders))
	for i, o := range orders {
		out[i] = orderToJSON(o)
	}
	response.OK(c, out)
}

type updateStatusBody struct {
	Status string `json:"status" binding:"required"`
}

func (h *OrderHandler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid order id", nil))
		return
	}
	var body updateStatusBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", nil))
		return
	}

	actorID, _ := middleware.UserIDFromContext(c)
	actorUUID, _ := uuid.Parse(actorID)

	order, err := h.svc.UpdateStatus(c.Request.Context(), id, domain.Status(body.Status), actorUUID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, orderToJSON(order))
}

func orderToJSON(o *domain.Order) gin.H {
	return gin.H{
		"id": o.ID, "order_number": o.OrderNumber, "customer_id": o.CustomerID, "restaurant_id": o.RestaurantID,
		"status": o.Status, "subtotal": o.Subtotal, "tax_amount": o.TaxAmount, "delivery_fee": o.DeliveryFee,
		"discount_amount": o.DiscountAmount, "total_amount": o.TotalAmount,
		"payment_status": o.PaymentStatus, "payment_method": o.PaymentMethod,
		"delivery_address": gin.H{
			"line1": o.DeliveryAddress.Line1, "line2": o.DeliveryAddress.Line2, "city": o.DeliveryAddress.City,
			"state": o.DeliveryAddress.State, "postal_code": o.DeliveryAddress.PostalCode,
			"lat": o.DeliveryAddress.Lat, "lng": o.DeliveryAddress.Lng, "phone": o.DeliveryAddress.Phone,
		},
		"special_instructions": o.SpecialInstructions,
		"placed_at":            o.PlacedAt,
		"estimated_delivery_at": o.EstimatedDeliveryAt,
	}
}

func itemsToJSON(items []*domain.OrderItem) []gin.H {
	out := make([]gin.H, len(items))
	for i, item := range items {
		addons := make([]gin.H, len(item.Addons))
		for j, a := range item.Addons {
			addons[j] = gin.H{"name": a.AddonName, "price": a.AddonPrice}
		}
		out[i] = gin.H{
			"id": item.ID, "menu_item_id": item.MenuItemID, "item_name": item.ItemName, "variant_name": item.VariantName,
			"unit_price": item.UnitPrice, "quantity": item.Quantity, "line_total": item.LineTotal,
			"special_instructions": item.SpecialInstructions, "addons": addons,
		}
	}
	return out
}

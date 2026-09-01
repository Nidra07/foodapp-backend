package http

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/settlements/application"
	"github.com/foodapp/backend/internal/modules/settlements/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/middleware"
	"github.com/foodapp/backend/internal/platform/response"
)

type SettlementHandler struct {
	svc *application.SettlementService
}

func NewSettlementHandler(svc *application.SettlementService) *SettlementHandler {
	return &SettlementHandler{svc: svc}
}

// RegisterRoutes mounts admin-only cycle management + payout actions,
// plus self-service routes for restaurant owners and delivery partners
// to view their own settlements and register a payout account.
func (h *SettlementHandler) RegisterRoutes(rg *gin.RouterGroup, authMW, adminOnly, ownerOnly, partnerOnly gin.HandlerFunc) {
	admin := rg.Group("/admin/settlements", authMW, adminOnly)
	admin.POST("/cycles", h.OpenCycle)
	admin.GET("/cycles", h.ListCycles)
	admin.GET("/cycles/:id", h.GetCycle)
	admin.POST("/cycles/:id/process", h.ProcessCycle)
	admin.GET("/cycles/:id/restaurants", h.ListRestaurantSettlementsForCycle)
	admin.GET("/cycles/:id/partners", h.ListDeliverySettlementsForCycle)
	admin.POST("/restaurant-settlements/:id/pay", h.PayRestaurantSettlement)
	admin.POST("/delivery-settlements/:id/pay", h.PayDeliverySettlement)

	owner := rg.Group("/restaurants/:restaurantId/settlements", authMW, ownerOnly)
	owner.GET("", h.ListMyRestaurantSettlements)
	owner.PUT("/payout-account", h.RegisterRestaurantPayoutAccount)

	partner := rg.Group("/delivery/partner/settlements", authMW, partnerOnly)
	partner.GET("", h.ListMyDeliverySettlements)
	partner.PUT("/payout-account", h.RegisterPartnerPayoutAccount)
}

type openCycleBody struct {
	CycleStart string `json:"cycle_start" binding:"required"` // YYYY-MM-DD
	CycleEnd   string `json:"cycle_end" binding:"required"`
}

func (h *SettlementHandler) OpenCycle(c *gin.Context) {
	var body openCycleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	start, err1 := time.Parse("2006-01-02", body.CycleStart)
	end, err2 := time.Parse("2006-01-02", body.CycleEnd)
	if err1 != nil || err2 != nil {
		response.Error(c, apperr.Validation("cycle_start and cycle_end must be in YYYY-MM-DD format", nil))
		return
	}

	cycle, err := h.svc.OpenCycle(c.Request.Context(), start, end)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, cycleToJSON(cycle))
}

func (h *SettlementHandler) ListCycles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	cycles, err := h.svc.ListCycles(c.Request.Context(), page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(cycles))
	for i, cy := range cycles {
		out[i] = cycleToJSON(cy)
	}
	response.OK(c, out)
}

func (h *SettlementHandler) GetCycle(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid cycle id", nil))
		return
	}
	cycle, err := h.svc.GetCycle(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, cycleToJSON(cycle))
}

func (h *SettlementHandler) ProcessCycle(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid cycle id", nil))
		return
	}
	if err := h.svc.ProcessCycle(c.Request.Context(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"message": "cycle processed"})
}

func (h *SettlementHandler) ListRestaurantSettlementsForCycle(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid cycle id", nil))
		return
	}
	settlements, err := h.svc.ListRestaurantSettlementsForCycle(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(settlements))
	for i, s := range settlements {
		out[i] = restaurantSettlementToJSON(s)
	}
	response.OK(c, out)
}

func (h *SettlementHandler) ListDeliverySettlementsForCycle(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid cycle id", nil))
		return
	}
	settlements, err := h.svc.ListDeliverySettlementsForCycle(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(settlements))
	for i, s := range settlements {
		out[i] = deliverySettlementToJSON(s)
	}
	response.OK(c, out)
}

type payBody struct {
	Reference string `json:"reference" binding:"required"`
}

func (h *SettlementHandler) PayRestaurantSettlement(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid settlement id", nil))
		return
	}
	var body payBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	adminID, _ := middleware.UserIDFromContext(c)
	adminUUID, _ := uuid.Parse(adminID)

	settlement, err := h.svc.PayRestaurantSettlement(c.Request.Context(), id, body.Reference, adminUUID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, restaurantSettlementToJSON(settlement))
}

func (h *SettlementHandler) PayDeliverySettlement(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid settlement id", nil))
		return
	}
	var body payBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	adminID, _ := middleware.UserIDFromContext(c)
	adminUUID, _ := uuid.Parse(adminID)

	settlement, err := h.svc.PayDeliverySettlement(c.Request.Context(), id, body.Reference, adminUUID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, deliverySettlementToJSON(settlement))
}

func (h *SettlementHandler) ListMyRestaurantSettlements(c *gin.Context) {
	restaurantID, err := uuid.Parse(c.Param("restaurantId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	settlements, err := h.svc.ListMyRestaurantSettlements(c.Request.Context(), restaurantID, page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(settlements))
	for i, s := range settlements {
		out[i] = restaurantSettlementToJSON(s)
	}
	response.OK(c, out)
}

type registerPayoutAccountBody struct {
	AccountHolderName string `json:"account_holder_name" binding:"required"`
	AccountNumber     string `json:"account_number" binding:"required"`
	IFSCCode          string `json:"ifsc_code" binding:"required"`
	BankName          string `json:"bank_name"`
}

func (h *SettlementHandler) RegisterRestaurantPayoutAccount(c *gin.Context) {
	restaurantID, err := uuid.Parse(c.Param("restaurantId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	var body registerPayoutAccountBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	account, err := h.svc.RegisterPayoutAccount(c.Request.Context(), domain.OwnerRestaurant, restaurantID, body.AccountHolderName, body.AccountNumber, body.IFSCCode, body.BankName)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, payoutAccountToJSON(account))
}

func (h *SettlementHandler) ListMyDeliverySettlements(c *gin.Context) {
	userID, _ := middleware.UserIDFromContext(c)
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		response.Error(c, apperr.Unauthorized(""))
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	settlements, err := h.svc.ListMyDeliverySettlementsForUser(c.Request.Context(), userUUID, page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(settlements))
	for i, s := range settlements {
		out[i] = deliverySettlementToJSON(s)
	}
	response.OK(c, out)
}

func (h *SettlementHandler) RegisterPartnerPayoutAccount(c *gin.Context) {
	var body registerPayoutAccountBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	userID, _ := middleware.UserIDFromContext(c)
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		response.Error(c, apperr.Unauthorized(""))
		return
	}

	account, err := h.svc.RegisterPartnerPayoutAccountForUser(c.Request.Context(), userUUID, body.AccountHolderName, body.AccountNumber, body.IFSCCode, body.BankName)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, payoutAccountToJSON(account))
}

func cycleToJSON(c *domain.Cycle) gin.H {
	return gin.H{"id": c.ID, "cycle_start": c.CycleStart.Format("2006-01-02"), "cycle_end": c.CycleEnd.Format("2006-01-02"), "status": c.Status, "processed_at": c.ProcessedAt}
}

func restaurantSettlementToJSON(s *domain.RestaurantSettlement) gin.H {
	return gin.H{
		"id": s.ID, "cycle_id": s.CycleID, "restaurant_id": s.RestaurantID, "order_count": s.OrderCount,
		"gross_subtotal": s.GrossSubtotal, "commission_amount": s.CommissionAmount, "net_payable": s.NetPayable,
		"status": s.Status, "payout_reference": s.PayoutReference, "paid_at": s.PaidAt,
	}
}

func deliverySettlementToJSON(s *domain.DeliverySettlement) gin.H {
	return gin.H{
		"id": s.ID, "cycle_id": s.CycleID, "delivery_partner_id": s.DeliveryPartnerID, "delivery_count": s.DeliveryCount,
		"gross_earnings": s.GrossEarnings, "incentive_amount": s.IncentiveAmount, "net_payable": s.NetPayable,
		"status": s.Status, "payout_reference": s.PayoutReference, "paid_at": s.PaidAt,
	}
}

func payoutAccountToJSON(a *domain.PayoutAccount) gin.H {
	return gin.H{
		"id": a.ID, "account_holder_name": a.AccountHolderName, "account_number_last4": a.AccountNumberLast4,
		"ifsc_code": a.IFSCCode, "bank_name": a.BankName, "is_verified": a.IsVerified,
	}
}

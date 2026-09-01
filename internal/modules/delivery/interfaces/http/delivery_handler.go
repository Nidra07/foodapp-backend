package http

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/delivery/application"
	"github.com/foodapp/backend/internal/modules/delivery/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/middleware"
	"github.com/foodapp/backend/internal/platform/response"
)

type DeliveryHandler struct {
	svc *application.DeliveryService
}

func NewDeliveryHandler(svc *application.DeliveryService) *DeliveryHandler {
	return &DeliveryHandler{svc: svc}
}

// RegisterRoutes mounts partner self-service routes (register, go
// online/offline, location ping, accept/reject/pickup/deliver) and
// restaurant/admin dispatch routes.
func (h *DeliveryHandler) RegisterRoutes(rg *gin.RouterGroup, authMW, partnerOnly, dispatchOnly gin.HandlerFunc) {
	partner := rg.Group("/delivery/partner", authMW, partnerOnly)
	partner.POST("/register", h.RegisterPartner)
	partner.GET("/me", h.GetMyPartnerProfile)
	partner.PATCH("/online", h.SetOnline)
	partner.PATCH("/location", h.UpdateLocation)
	partner.GET("/assignments", h.ListMyAssignments)
	partner.GET("/assignments/active", h.ListMyActiveAssignments)
	partner.POST("/assignments/:id/accept", h.Accept)
	partner.POST("/assignments/:id/reject", h.Reject)
	partner.POST("/assignments/:id/pickup", h.MarkPickedUp)
	partner.POST("/assignments/:id/deliver", h.MarkDelivered)

	dispatch := rg.Group("/delivery", authMW, dispatchOnly)
	dispatch.POST("/orders/:orderId/dispatch", h.Dispatch)
	dispatch.GET("/orders/:orderId/assignments", h.ListForOrder)
	dispatch.POST("/assignments/:id/cancel", h.Cancel)
	dispatch.POST("/partners/:id/kyc-review", h.AdminReviewKYC)

	customer := rg.Group("/delivery", authMW)
	customer.GET("/orders/:orderId/code", h.GetDeliveryCode)
}

// GetDeliveryCode returns the delivery OTP for the order's own customer
// to read aloud to the partner at hand-off. Ownership is enforced in
// DeliveryService.GetDeliveryCodeForOrder.
func (h *DeliveryHandler) GetDeliveryCode(c *gin.Context) {
	orderID, err := uuid.Parse(c.Param("orderId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid order id", nil))
		return
	}
	userID, _ := middleware.UserIDFromContext(c)
	userUUID, _ := uuid.Parse(userID)

	code, err := h.svc.GetDeliveryCodeForOrder(c.Request.Context(), orderID, userUUID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"delivery_code": code})
}

type registerPartnerBody struct {
	VehicleType    string  `json:"vehicle_type" binding:"required,oneof=bike scooter bicycle car on_foot"`
	VehicleNumber  *string `json:"vehicle_number"`
	LicenseNumber  *string `json:"license_number"`
}

func (h *DeliveryHandler) RegisterPartner(c *gin.Context) {
	var body registerPartnerBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	userID, _ := middleware.UserIDFromContext(c)
	userUUID, _ := uuid.Parse(userID)

	partner, err := h.svc.RegisterPartner(c.Request.Context(), userUUID, domain.VehicleType(body.VehicleType), body.VehicleNumber, body.LicenseNumber)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, partnerToJSON(partner))
}

func (h *DeliveryHandler) GetMyPartnerProfile(c *gin.Context) {
	userID, _ := middleware.UserIDFromContext(c)
	userUUID, _ := uuid.Parse(userID)

	partner, err := h.svc.GetPartnerByUserID(c.Request.Context(), userUUID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, partnerToJSON(partner))
}

type setOnlineBody struct {
	Online bool `json:"online"`
}

func (h *DeliveryHandler) SetOnline(c *gin.Context) {
	partner, err := h.currentPartner(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body setOnlineBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", nil))
		return
	}
	if err := h.svc.SetOnline(c.Request.Context(), partner.ID, body.Online); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"is_online": body.Online})
}

type updateLocationBody struct {
	Lat float64 `json:"lat" binding:"required"`
	Lng float64 `json:"lng" binding:"required"`
}

func (h *DeliveryHandler) UpdateLocation(c *gin.Context) {
	partner, err := h.currentPartner(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body updateLocationBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", nil))
		return
	}
	if err := h.svc.UpdateLocation(c.Request.Context(), partner.ID, domain.GeoPoint{Lat: body.Lat, Lng: body.Lng}); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"message": "location updated"})
}

func (h *DeliveryHandler) ListMyAssignments(c *gin.Context) {
	partner, err := h.currentPartner(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	var status *domain.AssignmentStatus
	if s := c.Query("status"); s != "" {
		st := domain.AssignmentStatus(s)
		status = &st
	}

	assignments, err := h.svc.ListForPartner(c.Request.Context(), partner.ID, domain.ListAssignmentsFilter{Status: status, Page: page, PageSize: pageSize})
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(assignments))
	for i, a := range assignments {
		out[i] = assignmentToJSON(a)
	}
	response.OK(c, out)
}

func (h *DeliveryHandler) ListMyActiveAssignments(c *gin.Context) {
	partner, err := h.currentPartner(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	assignments, err := h.svc.ListActiveForPartner(c.Request.Context(), partner.ID)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(assignments))
	for i, a := range assignments {
		out[i] = assignmentToJSON(a)
	}
	response.OK(c, out)
}

// assignmentAction is shared plumbing for accept/reject/pickup/deliver:
// resolve the caller's partner profile, load the assignment, and verify
// it actually belongs to them before letting the service mutate it —
// without this check any authenticated partner could act on any other
// partner's assignment by guessing an ID.
func (h *DeliveryHandler) assignmentAction(c *gin.Context) (*domain.Partner, uuid.UUID, bool) {
	partner, err := h.currentPartner(c)
	if err != nil {
		response.Error(c, err)
		return nil, uuid.Nil, false
	}
	assignmentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid assignment id", nil))
		return nil, uuid.Nil, false
	}
	assignment, err := h.svc.GetAssignment(c.Request.Context(), assignmentID)
	if err != nil {
		response.Error(c, err)
		return nil, uuid.Nil, false
	}
	if assignment.DeliveryPartnerID != partner.ID {
		response.Error(c, apperr.Forbidden("this assignment does not belong to you"))
		return nil, uuid.Nil, false
	}
	return partner, assignmentID, true
}

func (h *DeliveryHandler) Accept(c *gin.Context) {
	_, id, ok := h.assignmentAction(c)
	if !ok {
		return
	}
	assignment, err := h.svc.Accept(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, assignmentToJSON(assignment))
}

func (h *DeliveryHandler) Reject(c *gin.Context) {
	_, id, ok := h.assignmentAction(c)
	if !ok {
		return
	}
	assignment, err := h.svc.Reject(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, assignmentToJSON(assignment))
}

func (h *DeliveryHandler) MarkPickedUp(c *gin.Context) {
	_, id, ok := h.assignmentAction(c)
	if !ok {
		return
	}
	userID, _ := middleware.UserIDFromContext(c)
	userUUID, _ := uuid.Parse(userID)

	assignment, err := h.svc.MarkPickedUp(c.Request.Context(), id, userUUID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, assignmentToJSON(assignment))
}

type deliverBody struct {
	OTP string `json:"otp" binding:"required"`
}

func (h *DeliveryHandler) MarkDelivered(c *gin.Context) {
	_, id, ok := h.assignmentAction(c)
	if !ok {
		return
	}
	var body deliverBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", nil))
		return
	}
	userID, _ := middleware.UserIDFromContext(c)
	userUUID, _ := uuid.Parse(userID)

	assignment, err := h.svc.MarkDelivered(c.Request.Context(), id, body.OTP, userUUID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, assignmentToJSON(assignment))
}

func (h *DeliveryHandler) currentPartner(c *gin.Context) (*domain.Partner, error) {
	userID, _ := middleware.UserIDFromContext(c)
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, apperr.Unauthorized("")
	}
	return h.svc.GetPartnerByUserID(c.Request.Context(), userUUID)
}

func (h *DeliveryHandler) Dispatch(c *gin.Context) {
	orderID, err := uuid.Parse(c.Param("orderId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid order id", nil))
		return
	}
	assignment, err := h.svc.FindAndOffer(c.Request.Context(), orderID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, assignmentToJSON(assignment))
}

func (h *DeliveryHandler) ListForOrder(c *gin.Context) {
	orderID, err := uuid.Parse(c.Param("orderId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid order id", nil))
		return
	}
	assignments, err := h.svc.ListForOrder(c.Request.Context(), orderID)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(assignments))
	for i, a := range assignments {
		out[i] = assignmentToJSON(a)
	}
	response.OK(c, out)
}

type cancelAssignmentBody struct {
	Reason string `json:"reason" binding:"required"`
}

func (h *DeliveryHandler) Cancel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid assignment id", nil))
		return
	}
	var body cancelAssignmentBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	assignment, err := h.svc.Cancel(c.Request.Context(), id, body.Reason)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, assignmentToJSON(assignment))
}

type kycReviewBody struct {
	Approve bool `json:"approve"`
}

func (h *DeliveryHandler) AdminReviewKYC(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid partner id", nil))
		return
	}
	var body kycReviewBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", nil))
		return
	}
	if err := h.svc.AdminReviewKYC(c.Request.Context(), id, body.Approve); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"message": "review recorded"})
}

func partnerToJSON(p *domain.Partner) gin.H {
	return gin.H{
		"id": p.ID, "user_id": p.UserID, "vehicle_type": p.VehicleType, "vehicle_number": p.VehicleNumber,
		"kyc_status": p.KYCStatus, "is_online": p.IsOnline, "rating_avg": p.RatingAvg, "rating_count": p.RatingCount,
		"active_assignment_count": p.ActiveAssignmentCount,
	}
}

// assignmentToJSON deliberately never includes the delivery OTP. The OTP
// is a proof-of-delivery secret meant to be read aloud by the CUSTOMER
// to the partner at hand-off — a partner who could look it up via their
// own API response could self-confirm a delivery that never happened.
// See GetDeliveryCode for the one endpoint that does expose it, gated to
// the order's own customer.
func assignmentToJSON(a *domain.Assignment) gin.H {
	return gin.H{
		"id": a.ID, "order_id": a.OrderID, "restaurant_id": a.RestaurantID, "delivery_partner_id": a.DeliveryPartnerID,
		"status": a.Status, "offered_at": a.OfferedAt, "accepted_at": a.AcceptedAt,
		"picked_up_at": a.PickedUpAt, "delivered_at": a.DeliveredAt,
	}
}

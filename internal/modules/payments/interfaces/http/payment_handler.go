package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/payments/application"
	"github.com/foodapp/backend/internal/modules/payments/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/middleware"
	"github.com/foodapp/backend/internal/platform/response"
)

type PaymentHandler struct {
	svc *application.PaymentService
}

func NewPaymentHandler(svc *application.PaymentService) *PaymentHandler {
	return &PaymentHandler{svc: svc}
}

// RegisterRoutes mounts customer-facing payment routes and a public
// webhook endpoint. The webhook route is intentionally NOT behind
// authMW — gateways call it directly with their own signature scheme,
// which PaymentService.Capture verifies via domain.PaymentGateway
// (same verification used for the client-side callback).
func (h *PaymentHandler) RegisterRoutes(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	payments := rg.Group("/payments", authMW)
	payments.POST("/initiate", h.Initiate)
	payments.POST("/capture", h.Capture)
	payments.GET("/orders/:orderId", h.ListForOrder)
	payments.GET("/methods", h.ListSavedMethods)
	payments.POST("/methods", h.SaveMethod)
	payments.DELETE("/methods/:id", h.DeleteSavedMethod)

	rg.POST("/payments/webhook", h.Webhook)
}

// RegisterAdminRoutes mounts the admin-only refund action, kept separate
// from RegisterRoutes so main.go can gate it behind an admin-only
// middleware distinct from the regular authenticated-customer routes.
func (h *PaymentHandler) RegisterAdminRoutes(rg *gin.RouterGroup, authMW, adminOnly gin.HandlerFunc) {
	admin := rg.Group("/admin/payments", authMW, adminOnly)
	admin.POST("/refund", h.Refund)
	admin.GET("/orders/:orderId/refunds", h.ListRefunds)
}

type refundBody struct {
	OrderID string  `json:"order_id" binding:"required"`
	Amount  float64 `json:"amount" binding:"required"`
	Reason  string  `json:"reason" binding:"required"`
}

func (h *PaymentHandler) Refund(c *gin.Context) {
	var body refundBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	orderID, err := uuid.Parse(body.OrderID)
	if err != nil {
		response.Error(c, apperr.Validation("invalid order_id", nil))
		return
	}

	adminID, _ := middleware.UserIDFromContext(c)
	adminUUID, _ := uuid.Parse(adminID)

	refund, err := h.svc.Refund(c.Request.Context(), orderID, body.Amount, body.Reason, &adminUUID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, refund)
}

func (h *PaymentHandler) ListRefunds(c *gin.Context) {
	orderID, err := uuid.Parse(c.Param("orderId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid order id", nil))
		return
	}
	refunds, err := h.svc.ListRefunds(c.Request.Context(), orderID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, refunds)
}

type initiateBody struct {
	OrderID string `json:"order_id" binding:"required"`
	Method  string `json:"method" binding:"required,oneof=upi card wallet"`
}

func (h *PaymentHandler) Initiate(c *gin.Context) {
	var body initiateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	orderID, err := uuid.Parse(body.OrderID)
	if err != nil {
		response.Error(c, apperr.Validation("invalid order_id", nil))
		return
	}

	customerID, _ := middleware.UserIDFromContext(c)
	custUUID, _ := uuid.Parse(customerID)

	result, err := h.svc.Initiate(c.Request.Context(), orderID, custUUID, domain.Method(body.Method))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{
		"transaction_id":   result.Transaction.ID,
		"gateway":          result.Transaction.Gateway,
		"gateway_order_id": result.GatewayOrderID,
		"gateway_key":      result.GatewayKey,
		"amount":           result.Amount,
		"currency":         result.Currency,
	})
}

type captureBody struct {
	GatewayOrderID   string `json:"gateway_order_id" binding:"required"`
	GatewayPaymentID string `json:"gateway_payment_id" binding:"required"`
	GatewaySignature string `json:"gateway_signature" binding:"required"`
}

func (h *PaymentHandler) Capture(c *gin.Context) {
	var body captureBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}

	tx, err := h.svc.Capture(c.Request.Context(), domain.CaptureCallback{
		GatewayOrderID: body.GatewayOrderID, GatewayPaymentID: body.GatewayPaymentID, GatewaySignature: body.GatewaySignature,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, transactionToJSON(tx))
}

// Webhook is a minimal placeholder: real gateways (Razorpay/Stripe) POST
// event payloads with their own envelope shape and signature header,
// which would need per-gateway parsing before calling into
// PaymentService. This handler demonstrates the intended shape (capture
// event -> Capture(), failure event -> MarkFailed()) using the same
// generic body as the client-side /capture endpoint; swap the parsing
// logic when a real gateway is wired in — see docs/assumptions.md.
type webhookBody struct {
	Event            string `json:"event" binding:"required"` // e.g. "payment.captured", "payment.failed"
	GatewayOrderID   string `json:"gateway_order_id" binding:"required"`
	GatewayPaymentID string `json:"gateway_payment_id"`
	GatewaySignature string `json:"gateway_signature"`
	FailureReason    string `json:"failure_reason"`
}

func (h *PaymentHandler) Webhook(c *gin.Context) {
	var body webhookBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid webhook payload", nil))
		return
	}

	switch body.Event {
	case "payment.captured":
		if _, err := h.svc.Capture(c.Request.Context(), domain.CaptureCallback{
			GatewayOrderID: body.GatewayOrderID, GatewayPaymentID: body.GatewayPaymentID, GatewaySignature: body.GatewaySignature,
		}); err != nil {
			response.Error(c, err)
			return
		}
	case "payment.failed":
		if _, err := h.svc.MarkFailed(c.Request.Context(), body.GatewayOrderID, body.FailureReason); err != nil {
			response.Error(c, err)
			return
		}
	default:
		// Unrecognized event types are acknowledged (200) rather than
		// erroring, so the gateway doesn't retry-storm us for events we
		// intentionally don't handle yet (e.g. refund.processed webhooks —
		// refunds are currently only tracked via the synchronous Refund()
		// call, not webhook-driven).
	}

	c.Status(200)
}

func (h *PaymentHandler) ListForOrder(c *gin.Context) {
	orderID, err := uuid.Parse(c.Param("orderId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid order id", nil))
		return
	}
	txs, err := h.svc.ListForOrder(c.Request.Context(), orderID)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(txs))
	for i, t := range txs {
		out[i] = transactionToJSON(t)
	}
	response.OK(c, out)
}

type saveMethodBody struct {
	Method       string `json:"method" binding:"required,oneof=upi card wallet"`
	Gateway      string `json:"gateway" binding:"required"`
	GatewayToken string `json:"gateway_token" binding:"required"`
	DisplayLabel string `json:"display_label" binding:"required"`
	IsDefault    bool   `json:"is_default"`
}

func (h *PaymentHandler) SaveMethod(c *gin.Context) {
	var body saveMethodBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}

	customerID, _ := middleware.UserIDFromContext(c)
	custUUID, _ := uuid.Parse(customerID)

	m, err := h.svc.SaveMethod(c.Request.Context(), custUUID, domain.Method(body.Method), body.Gateway, body.GatewayToken, body.DisplayLabel, body.IsDefault)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, m)
}

func (h *PaymentHandler) ListSavedMethods(c *gin.Context) {
	customerID, _ := middleware.UserIDFromContext(c)
	custUUID, _ := uuid.Parse(customerID)

	methods, err := h.svc.ListSavedMethods(c.Request.Context(), custUUID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, methods)
}

func (h *PaymentHandler) DeleteSavedMethod(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid id", nil))
		return
	}
	customerID, _ := middleware.UserIDFromContext(c)
	custUUID, _ := uuid.Parse(customerID)

	if err := h.svc.DeleteSavedMethod(c.Request.Context(), id, custUUID); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

func transactionToJSON(t *domain.Transaction) gin.H {
	return gin.H{
		"id": t.ID, "order_id": t.OrderID, "amount": t.Amount, "currency": t.Currency,
		"method": t.Method, "status": t.Status, "gateway": t.Gateway,
		"initiated_at": t.InitiatedAt, "completed_at": t.CompletedAt,
	}
}

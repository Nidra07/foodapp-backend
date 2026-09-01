package http

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/notifications/application"
	"github.com/foodapp/backend/internal/modules/notifications/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/middleware"
	"github.com/foodapp/backend/internal/platform/response"
)

type NotificationHandler struct {
	svc *application.NotificationService
}

func NewNotificationHandler(svc *application.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

func (h *NotificationHandler) RegisterRoutes(rg *gin.RouterGroup, authMW gin.HandlerFunc) {
	n := rg.Group("/notifications", authMW)
	n.GET("", h.List)
	n.GET("/unread-count", h.UnreadCount)
	n.POST("/:id/read", h.MarkRead)
	n.POST("/read-all", h.MarkAllRead)
	n.GET("/preferences", h.ListPreferences)
	n.PUT("/preferences", h.SetPreference)
}

func (h *NotificationHandler) List(c *gin.Context) {
	userID, _ := middleware.UserIDFromContext(c)
	userUUID, _ := uuid.Parse(userID)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))

	notifications, err := h.svc.ListForUser(c.Request.Context(), userUUID, page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(notifications))
	for i, n := range notifications {
		out[i] = notificationToJSON(n)
	}
	response.OK(c, out)
}

func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	userID, _ := middleware.UserIDFromContext(c)
	userUUID, _ := uuid.Parse(userID)

	count, err := h.svc.CountUnread(c.Request.Context(), userUUID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"unread_count": count})
}

func (h *NotificationHandler) MarkRead(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid notification id", nil))
		return
	}
	userID, _ := middleware.UserIDFromContext(c)
	userUUID, _ := uuid.Parse(userID)

	if err := h.svc.MarkRead(c.Request.Context(), id, userUUID); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"message": "marked read"})
}

func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	userID, _ := middleware.UserIDFromContext(c)
	userUUID, _ := uuid.Parse(userID)

	if err := h.svc.MarkAllRead(c.Request.Context(), userUUID); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"message": "all notifications marked read"})
}

func (h *NotificationHandler) ListPreferences(c *gin.Context) {
	userID, _ := middleware.UserIDFromContext(c)
	userUUID, _ := uuid.Parse(userID)

	prefs, err := h.svc.ListPreferences(c.Request.Context(), userUUID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, prefs)
}

type setPreferenceBody struct {
	Category string `json:"category" binding:"required"`
	Channel  string `json:"channel" binding:"required,oneof=push sms email in_app"`
	Enabled  bool   `json:"enabled"`
}

func (h *NotificationHandler) SetPreference(c *gin.Context) {
	var body setPreferenceBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	userID, _ := middleware.UserIDFromContext(c)
	userUUID, _ := uuid.Parse(userID)

	pref, err := h.svc.SetPreference(c.Request.Context(), userUUID, domain.Category(body.Category), domain.Channel(body.Channel), body.Enabled)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, pref)
}

func notificationToJSON(n *domain.Notification) gin.H {
	return gin.H{
		"id": n.ID, "category": n.Category, "title": n.Title, "body": n.Body, "data": n.Data,
		"is_read": n.IsRead, "created_at": n.CreatedAt,
	}
}

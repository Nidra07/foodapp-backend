package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/foodapp/backend/internal/modules/identity/application"
	"github.com/foodapp/backend/internal/modules/identity/domain"
	usersdomain "github.com/foodapp/backend/internal/modules/users/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/response"
)

type AuthHandler struct {
	svc *application.AuthService
}

func NewAuthHandler(svc *application.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) RegisterRoutes(rg *gin.RouterGroup) {
	auth := rg.Group("/auth")
	auth.POST("/otp/request", h.RequestOTP)
	auth.POST("/otp/verify", h.VerifyOTP)
	auth.POST("/refresh", h.Refresh)
	auth.POST("/logout", h.Logout)
}

type requestOTPBody struct {
	Identifier string `json:"identifier" binding:"required"`
	Purpose    string `json:"purpose" binding:"required,oneof=login signup phone_verification email_verification password_reset"`
}

func (h *AuthHandler) RequestOTP(c *gin.Context) {
	var body requestOTPBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}

	if err := h.svc.RequestOTP(c.Request.Context(), body.Identifier, domain.OTPPurpose(body.Purpose)); err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, gin.H{"message": "verification code sent"})
}

type verifyOTPBody struct {
	Identifier string `json:"identifier" binding:"required"`
	Code       string `json:"code" binding:"required,len=6"`
	Purpose    string `json:"purpose" binding:"required"`
	Role       string `json:"role" binding:"required,oneof=customer restaurant_owner delivery_partner"`
	DeviceID   string `json:"device_id"`
	Platform   string `json:"platform" binding:"omitempty,oneof=ios android web"`
	FCMToken   string `json:"fcm_token"`
	AppVersion string `json:"app_version"`
}

func (h *AuthHandler) VerifyOTP(c *gin.Context) {
	var body verifyOTPBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}

	var device *domain.Device
	if body.DeviceID != "" {
		device = &domain.Device{
			DeviceID:   body.DeviceID,
			Platform:   domain.Platform(body.Platform),
			FCMToken:   body.FCMToken,
			AppVersion: body.AppVersion,
		}
	}

	result, err := h.svc.VerifyOTP(
		c.Request.Context(),
		body.Identifier,
		body.Code,
		domain.OTPPurpose(body.Purpose),
		usersdomain.Role(body.Role),
		device,
		c.ClientIP(),
		c.Request.UserAgent(),
	)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, gin.H{
		"user": gin.H{
			"id":           result.User.ID,
			"phone_number": result.User.PhoneNumber,
			"email":        result.User.Email,
			"full_name":    result.User.FullName,
			"role":         result.User.PrimaryRole,
			"status":       result.User.Status,
		},
		"tokens": gin.H{
			"access_token":  result.Tokens.AccessToken,
			"refresh_token": result.Tokens.RefreshToken,
			"expires_in":    result.Tokens.ExpiresIn,
			"token_type":    "Bearer",
		},
		"is_new_user": result.IsNewUser,
	})
}

type refreshBody struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var body refreshBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", nil))
		return
	}

	tokens, err := h.svc.RefreshTokens(c.Request.Context(), body.RefreshToken, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, gin.H{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_in":    tokens.ExpiresIn,
		"token_type":    "Bearer",
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var body refreshBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", nil))
		return
	}

	if err := h.svc.Logout(c.Request.Context(), body.RefreshToken); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

package http

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/restaurants/application"
	"github.com/foodapp/backend/internal/modules/restaurants/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/middleware"
	"github.com/foodapp/backend/internal/platform/response"
)

type RestaurantHandler struct {
	svc *application.RestaurantService
}

func NewRestaurantHandler(svc *application.RestaurantService) *RestaurantHandler {
	return &RestaurantHandler{svc: svc}
}

// RegisterRoutes mounts public discovery routes, owner-only management
// routes, and admin approval routes. authMW/ownerOnly/adminOnly are
// injected from main.go.
func (h *RestaurantHandler) RegisterRoutes(rg *gin.RouterGroup, authMW, ownerOnly, adminOnly gin.HandlerFunc) {
	public := rg.Group("/restaurants")
	public.GET("/nearby", h.SearchNearby)
	public.GET("/:id", h.GetByID)
	public.GET("/slug/:slug", h.GetBySlug)

	owner := rg.Group("/restaurants", authMW, ownerOnly)
	owner.POST("", h.Onboard)
	owner.GET("/mine", h.ListMine)
	owner.PATCH("/:id", h.UpdateProfile)
	owner.POST("/:id/submit", h.SubmitForApproval)
	owner.PATCH("/:id/accepting-orders", h.SetAcceptingOrders)
	owner.PUT("/:id/hours", h.SetOperatingHours)
	owner.GET("/:id/hours", h.ListOperatingHours)
	owner.PUT("/:id/service-area", h.SetServiceArea)
	owner.POST("/:id/documents", h.UploadDocument)
	owner.GET("/:id/documents", h.ListDocuments)
	owner.POST("/:id/staff", h.AddStaff)
	owner.GET("/:id/staff", h.ListStaff)
	owner.DELETE("/staff/:staffId", h.RevokeStaff)

	admin := rg.Group("/admin/restaurants", authMW, adminOnly)
	admin.GET("", h.AdminList)
	admin.POST("/:id/review", h.AdminReview)
	admin.POST("/documents/:docId/review", h.AdminReviewDocument)
}

type onboardBody struct {
	Name         string   `json:"name" binding:"required"`
	Description  *string  `json:"description"`
	CuisineTags  []string `json:"cuisine_tags"`
	IsVegOnly    bool     `json:"is_veg_only"`
	AddressLine1 string   `json:"address_line1" binding:"required"`
	AddressLine2 *string  `json:"address_line2"`
	City         string   `json:"city" binding:"required"`
	State        string   `json:"state" binding:"required"`
	PostalCode   string   `json:"postal_code" binding:"required"`
	Country      string   `json:"country"`
	Lat          float64  `json:"lat" binding:"required"`
	Lng          float64  `json:"lng" binding:"required"`
}

func (h *RestaurantHandler) Onboard(c *gin.Context) {
	var body onboardBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}

	ownerID, ok := middleware.UserIDFromContext(c)
	if !ok {
		response.Error(c, apperr.Unauthorized(""))
		return
	}
	ownerUUID, _ := uuid.Parse(ownerID)

	rest, err := h.svc.Onboard(c.Request.Context(), domain.CreateRestaurantInput{
		OwnerUserID:  ownerUUID,
		Name:         body.Name,
		Description:  body.Description,
		CuisineTags:  body.CuisineTags,
		IsVegOnly:    body.IsVegOnly,
		AddressLine1: body.AddressLine1,
		AddressLine2: body.AddressLine2,
		City:         body.City,
		State:        body.State,
		PostalCode:   body.PostalCode,
		Country:      body.Country,
		Location:     domain.GeoPoint{Lat: body.Lat, Lng: body.Lng},
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, restaurantToJSON(rest))
}

func (h *RestaurantHandler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	rest, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, restaurantToJSON(rest))
}

func (h *RestaurantHandler) GetBySlug(c *gin.Context) {
	rest, err := h.svc.GetBySlug(c.Request.Context(), c.Param("slug"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, restaurantToJSON(rest))
}

func (h *RestaurantHandler) ListMine(c *gin.Context) {
	ownerID, _ := middleware.UserIDFromContext(c)
	ownerUUID, _ := uuid.Parse(ownerID)

	restaurants, err := h.svc.ListMine(c.Request.Context(), ownerUUID)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(restaurants))
	for i, r := range restaurants {
		out[i] = restaurantToJSON(r)
	}
	response.OK(c, out)
}

func (h *RestaurantHandler) SearchNearby(c *gin.Context) {
	lat, err1 := strconv.ParseFloat(c.Query("lat"), 64)
	lng, err2 := strconv.ParseFloat(c.Query("lng"), 64)
	if err1 != nil || err2 != nil {
		response.Error(c, apperr.Validation("lat and lng query params are required", nil))
		return
	}
	radiusM, _ := strconv.ParseFloat(c.DefaultQuery("radius_m", "10000"), 64)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	restaurants, err := h.svc.SearchNearby(c.Request.Context(), domain.NearbySearchInput{
		Location:      domain.GeoPoint{Lat: lat, Lng: lng},
		SearchRadiusM: radiusM,
		Page:          page,
		PageSize:      pageSize,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(restaurants))
	for i, r := range restaurants {
		out[i] = restaurantToJSON(r)
	}
	response.OK(c, out)
}

type updateProfileBody struct {
	Name            *string  `json:"name"`
	Description     *string  `json:"description"`
	CuisineTags     []string `json:"cuisine_tags"`
	LogoURL         *string  `json:"logo_url"`
	BannerURL       *string  `json:"banner_url"`
	MinOrderAmount  *float64 `json:"min_order_amount"`
	AvgPrepTimeMins *int     `json:"avg_prep_time_mins"`
}

func (h *RestaurantHandler) UpdateProfile(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	var body updateProfileBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}

	rest, err := h.svc.UpdateProfile(c.Request.Context(), id, domain.UpdateRestaurantInput{
		Name: body.Name, Description: body.Description, CuisineTags: body.CuisineTags,
		LogoURL: body.LogoURL, BannerURL: body.BannerURL,
		MinOrderAmount: body.MinOrderAmount, AvgPrepTimeMins: body.AvgPrepTimeMins,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, restaurantToJSON(rest))
}

func (h *RestaurantHandler) SubmitForApproval(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	if err := h.svc.SubmitForApproval(c.Request.Context(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"message": "submitted for approval"})
}

type acceptingOrdersBody struct {
	Accepting bool `json:"accepting"`
}

func (h *RestaurantHandler) SetAcceptingOrders(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	var body acceptingOrdersBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", nil))
		return
	}
	if err := h.svc.SetAcceptingOrders(c.Request.Context(), id, body.Accepting); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"accepting_orders": body.Accepting})
}

type operatingHoursBody struct {
	DayOfWeek int    `json:"day_of_week" binding:"required,min=0,max=6"`
	OpenTime  string `json:"open_time" binding:"required"`
	CloseTime string `json:"close_time" binding:"required"`
	IsClosed  bool   `json:"is_closed"`
}

func (h *RestaurantHandler) SetOperatingHours(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	var body operatingHoursBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}

	hours, err := h.svc.SetOperatingHours(c.Request.Context(), &domain.OperatingHours{
		RestaurantID: id, DayOfWeek: body.DayOfWeek, OpenTime: body.OpenTime, CloseTime: body.CloseTime, IsClosed: body.IsClosed,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, hours)
}

func (h *RestaurantHandler) ListOperatingHours(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	hours, err := h.svc.ListOperatingHours(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, hours)
}

type serviceAreaBody struct {
	RadiusKM float64 `json:"radius_km" binding:"required"`
}

func (h *RestaurantHandler) SetServiceArea(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	var body serviceAreaBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", nil))
		return
	}
	area, err := h.svc.SetServiceArea(c.Request.Context(), id, body.RadiusKM)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, area)
}

type uploadDocumentBody struct {
	DocumentType   string  `json:"document_type" binding:"required"`
	FileURL        string  `json:"file_url" binding:"required"`
	DocumentNumber *string `json:"document_number"`
}

func (h *RestaurantHandler) UploadDocument(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	var body uploadDocumentBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}

	doc, err := h.svc.UploadDocument(c.Request.Context(), &domain.Document{
		RestaurantID: id, DocumentType: domain.DocumentType(body.DocumentType), FileURL: body.FileURL, DocumentNumber: body.DocumentNumber,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, doc)
}

func (h *RestaurantHandler) ListDocuments(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	docs, err := h.svc.ListDocuments(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, docs)
}

type addStaffBody struct {
	UserID      string   `json:"user_id" binding:"required"`
	Permissions []string `json:"permissions" binding:"required,min=1"`
}

func (h *RestaurantHandler) AddStaff(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	var body addStaffBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	userID, err := uuid.Parse(body.UserID)
	if err != nil {
		response.Error(c, apperr.Validation("invalid user_id", nil))
		return
	}

	inviterID, _ := middleware.UserIDFromContext(c)
	inviterUUID, _ := uuid.Parse(inviterID)

	perms := make([]domain.StaffPermission, len(body.Permissions))
	for i, p := range body.Permissions {
		perms[i] = domain.StaffPermission(p)
	}

	staff, err := h.svc.AddStaff(c.Request.Context(), id, userID, inviterUUID, perms)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, staff)
}

func (h *RestaurantHandler) ListStaff(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	staff, err := h.svc.ListStaff(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, staff)
}

func (h *RestaurantHandler) RevokeStaff(c *gin.Context) {
	staffID, err := uuid.Parse(c.Param("staffId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid staff id", nil))
		return
	}
	if err := h.svc.RevokeStaff(c.Request.Context(), staffID); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

func (h *RestaurantHandler) AdminList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	var status *domain.Status
	if s := c.Query("status"); s != "" {
		st := domain.Status(s)
		status = &st
	}

	restaurants, total, err := h.svc.AdminListRestaurants(c.Request.Context(), domain.AdminListFilter{Status: status, Page: page, PageSize: pageSize})
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(restaurants))
	for i, r := range restaurants {
		out[i] = restaurantToJSON(r)
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	response.Paginated(c, out, response.Meta{Page: page, PageSize: pageSize, TotalCount: total, TotalPages: totalPages})
}

type adminReviewBody struct {
	Approve bool `json:"approve"`
}

func (h *RestaurantHandler) AdminReview(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant id", nil))
		return
	}
	var body adminReviewBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", nil))
		return
	}

	adminID, _ := middleware.UserIDFromContext(c)
	adminUUID, _ := uuid.Parse(adminID)

	if err := h.svc.AdminReview(c.Request.Context(), id, body.Approve, adminUUID); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"message": "review recorded"})
}

type adminReviewDocumentBody struct {
	Approve         bool    `json:"approve"`
	RejectionReason *string `json:"rejection_reason"`
	RestaurantID    string  `json:"restaurant_id" binding:"required"`
}

func (h *RestaurantHandler) AdminReviewDocument(c *gin.Context) {
	docID, err := uuid.Parse(c.Param("docId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid document id", nil))
		return
	}
	var body adminReviewDocumentBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	restaurantID, err := uuid.Parse(body.RestaurantID)
	if err != nil {
		response.Error(c, apperr.Validation("invalid restaurant_id", nil))
		return
	}

	adminID, _ := middleware.UserIDFromContext(c)
	adminUUID, _ := uuid.Parse(adminID)

	doc, err := h.svc.ReviewDocument(c.Request.Context(), docID, restaurantID, body.Approve, body.RejectionReason, adminUUID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, doc)
}

func restaurantToJSON(r *domain.Restaurant) gin.H {
	out := gin.H{
		"id":                  r.ID,
		"name":                r.Name,
		"slug":                r.Slug,
		"description":         r.Description,
		"cuisine_tags":        r.CuisineTags,
		"status":              r.Status,
		"kyc_status":          r.KYCStatus,
		"is_veg_only":         r.IsVegOnly,
		"avg_prep_time_mins":  r.AvgPrepTimeMins,
		"min_order_amount":    r.MinOrderAmount,
		"logo_url":            r.LogoURL,
		"banner_url":          r.BannerURL,
		"address_line1":       r.AddressLine1,
		"city":                r.City,
		"state":               r.State,
		"postal_code":         r.PostalCode,
		"rating_avg":          r.RatingAvg,
		"rating_count":        r.RatingCount,
		"is_accepting_orders": r.IsAcceptingOrders,
		"created_at":          r.CreatedAt,
	}
	if r.DistanceKM != nil {
		out["distance_km"] = *r.DistanceKM
	}
	return out
}

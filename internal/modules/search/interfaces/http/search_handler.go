package http

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/search/application"
	"github.com/foodapp/backend/internal/modules/search/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/middleware"
	"github.com/foodapp/backend/internal/platform/response"
)

type SearchHandler struct {
	svc *application.SearchService
}

func NewSearchHandler(svc *application.SearchService) *SearchHandler {
	return &SearchHandler{svc: svc}
}

// RegisterRoutes mounts public search endpoints (optionalAuthMW attaches
// the requester's user ID to the context WHEN a valid token is present,
// but never rejects an unauthenticated request — search must work for
// browsing customers who haven't logged in yet) and an admin-only
// zero-result-search analytics endpoint.
func (h *SearchHandler) RegisterRoutes(rg *gin.RouterGroup, optionalAuthMW, authMW, adminOnly gin.HandlerFunc) {
	search := rg.Group("/search", optionalAuthMW)
	search.GET("/restaurants", h.SearchRestaurants)
	search.GET("/items", h.SearchMenuItems)
	search.GET("/trending", h.Trending)

	admin := rg.Group("/admin/search", authMW, adminOnly)
	admin.GET("/zero-results", h.ZeroResultSearches)
}

func (h *SearchHandler) SearchRestaurants(c *gin.Context) {
	in, ok := h.parseSearchInput(c)
	if !ok {
		return
	}
	results, err := h.svc.SearchRestaurants(c.Request.Context(), in)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(results))
	for i, r := range results {
		out[i] = restaurantResultToJSON(r)
	}
	response.OK(c, out)
}

func (h *SearchHandler) SearchMenuItems(c *gin.Context) {
	in, ok := h.parseSearchInput(c)
	if !ok {
		return
	}
	results, err := h.svc.SearchMenuItems(c.Request.Context(), in)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]gin.H, len(results))
	for i, r := range results {
		out[i] = itemResultToJSON(r)
	}
	response.OK(c, out)
}

func (h *SearchHandler) Trending(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	windowHours, _ := strconv.Atoi(c.DefaultQuery("window_hours", "24"))

	terms, err := h.svc.Trending(c.Request.Context(), time.Duration(windowHours)*time.Hour, limit)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, terms)
}

func (h *SearchHandler) ZeroResultSearches(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	windowDays, _ := strconv.Atoi(c.DefaultQuery("window_days", "7"))

	terms, err := h.svc.ZeroResultSearches(c.Request.Context(), time.Duration(windowDays)*24*time.Hour, limit)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, terms)
}

func (h *SearchHandler) parseSearchInput(c *gin.Context) (domain.SearchInput, bool) {
	query := c.Query("q")
	if query == "" {
		response.Error(c, apperr.Validation("q query param is required", nil))
		return domain.SearchInput{}, false
	}
	lat, err1 := strconv.ParseFloat(c.Query("lat"), 64)
	lng, err2 := strconv.ParseFloat(c.Query("lng"), 64)
	if err1 != nil || err2 != nil {
		response.Error(c, apperr.Validation("lat and lng query params are required", nil))
		return domain.SearchInput{}, false
	}
	radiusM, _ := strconv.ParseFloat(c.DefaultQuery("radius_m", "10000"), 64)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	var userID *uuid.UUID
	if id, ok := middleware.UserIDFromContext(c); ok {
		if parsed, err := uuid.Parse(id); err == nil {
			userID = &parsed
		}
	}

	return domain.SearchInput{
		Query: query, Location: domain.GeoPoint{Lat: lat, Lng: lng}, SearchRadiusM: radiusM,
		Page: page, PageSize: pageSize, UserID: userID,
	}, true
}

func restaurantResultToJSON(r *domain.RestaurantResult) gin.H {
	return gin.H{
		"id": r.ID, "name": r.Name, "slug": r.Slug, "cuisine_tags": r.CuisineTags,
		"rating_avg": r.RatingAvg, "rating_count": r.RatingCount, "logo_url": r.LogoURL,
		"min_order_amount": r.MinOrderAmount, "distance_km": r.DistanceKM,
	}
}

func itemResultToJSON(r *domain.ItemResult) gin.H {
	return gin.H{
		"item_id": r.ItemID, "item_name": r.ItemName, "base_price": r.BasePrice, "food_type": r.FoodType,
		"image_url": r.ImageURL, "is_bestseller": r.IsBestseller,
		"restaurant": gin.H{
			"id": r.RestaurantID, "name": r.RestaurantName, "slug": r.RestaurantSlug, "rating_avg": r.RestaurantRatingAvg,
		},
		"distance_km": r.DistanceKM,
	}
}

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type GeoPoint struct {
	Lat float64
	Lng float64
}

type RestaurantResult struct {
	ID             uuid.UUID
	Name           string
	Slug           string
	CuisineTags    []string
	RatingAvg      float64
	RatingCount    int
	LogoURL        *string
	MinOrderAmount float64
	DistanceKM     float64
	Rank           float64
}

type ItemResult struct {
	ItemID         uuid.UUID
	ItemName       string
	BasePrice      float64
	FoodType       string
	ImageURL       *string
	IsBestseller   bool
	RestaurantID   uuid.UUID
	RestaurantName string
	RestaurantSlug string
	RestaurantRatingAvg float64
	DistanceKM     float64
	Rank           float64
}

type SearchInput struct {
	Query         string
	Location      GeoPoint
	SearchRadiusM float64
	Page          int
	PageSize      int
	UserID        *uuid.UUID // nil for unauthenticated search
}

type TrendingTerm struct {
	Query       string
	SearchCount int64
}

// Repository is the persistence port for the Search module. See
// 0013_search.up.sql's header comment for why this module reads the
// restaurants/menu_items tables directly rather than through another
// module's narrow interface — full-text ranking + geo-filtering is
// inherently a SQL-level concern here.
type Repository interface {
	SearchRestaurants(ctx context.Context, query string, loc GeoPoint, radiusM float64, page, pageSize int) ([]*RestaurantResult, error)
	SearchMenuItems(ctx context.Context, query string, loc GeoPoint, radiusM float64, page, pageSize int) ([]*ItemResult, error)
	LogSearch(ctx context.Context, userID *uuid.UUID, query, searchType string, resultCount int) error
	ListTrending(ctx context.Context, window time.Duration, limit int) ([]*TrendingTerm, error)
	ListZeroResult(ctx context.Context, window time.Duration, limit int) ([]*TrendingTerm, error)
}

package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/foodapp/backend/internal/modules/search/domain"
	sqlcgen "github.com/foodapp/backend/internal/platform/db/sqlc"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type Repository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) *Repository {
	return &Repository{q: q}
}

func (r *Repository) SearchRestaurants(ctx context.Context, query string, loc domain.GeoPoint, radiusM float64, page, pageSize int) ([]*domain.RestaurantResult, error) {
	offset := (page - 1) * pageSize
	rows, err := r.q.SearchRestaurants(ctx, sqlcgen.SearchRestaurantsParams{
		Lng: loc.Lng, Lat: loc.Lat, Query: query, SearchRadiusM: radiusM, Limit: int32(pageSize), Offset: int32(offset),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to search restaurants", err)
	}

	out := make([]*domain.RestaurantResult, len(rows))
	for i, row := range rows {
		minOrder, _ := row.MinOrderAmount.Float64Value()
		ratingAvg, _ := row.RatingAvg.Float64Value()
		res := &domain.RestaurantResult{
			ID: row.ID, Name: row.Name, Slug: row.Slug, CuisineTags: row.CuisineTags,
			RatingAvg: ratingAvg.Float64, RatingCount: int(row.RatingCount),
			MinOrderAmount: minOrder.Float64, DistanceKM: row.DistanceKm, Rank: float64(row.Rank),
		}
		if row.LogoUrl.Valid {
			res.LogoURL = &row.LogoUrl.String
		}
		out[i] = res
	}
	return out, nil
}

func (r *Repository) SearchMenuItems(ctx context.Context, query string, loc domain.GeoPoint, radiusM float64, page, pageSize int) ([]*domain.ItemResult, error) {
	offset := (page - 1) * pageSize
	rows, err := r.q.SearchMenuItems(ctx, sqlcgen.SearchMenuItemsParams{
		Lng: loc.Lng, Lat: loc.Lat, Query: query, SearchRadiusM: radiusM, Limit: int32(pageSize), Offset: int32(offset),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to search menu items", err)
	}

	out := make([]*domain.ItemResult, len(rows))
	for i, row := range rows {
		price, _ := row.BasePrice.Float64Value()
		ratingAvg, _ := row.RatingAvg.Float64Value()
		res := &domain.ItemResult{
			ItemID: row.ItemID, ItemName: row.ItemName, BasePrice: price.Float64, FoodType: string(row.FoodType),
			IsBestseller: row.IsBestseller, RestaurantID: row.RestaurantID, RestaurantName: row.RestaurantName,
			RestaurantSlug: row.RestaurantSlug, RestaurantRatingAvg: ratingAvg.Float64,
			DistanceKM: row.DistanceKm, Rank: float64(row.Rank),
		}
		if row.ImageUrl.Valid {
			res.ImageURL = &row.ImageUrl.String
		}
		out[i] = res
	}
	return out, nil
}

func (r *Repository) LogSearch(ctx context.Context, userID *uuid.UUID, query, searchType string, resultCount int) error {
	var userIDParam pgtype.UUID
	if userID != nil {
		userIDParam = pgtype.UUID{Bytes: *userID, Valid: true}
	}
	if _, err := r.q.LogSearchQuery(ctx, sqlcgen.LogSearchQueryParams{
		UserID: userIDParam, Query: query, SearchType: searchType, ResultCount: int32(resultCount),
	}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to log search query", err)
	}
	return nil
}

func (r *Repository) ListTrending(ctx context.Context, window time.Duration, limit int) ([]*domain.TrendingTerm, error) {
	rows, err := r.q.ListTrendingSearches(ctx, sqlcgen.ListTrendingSearchesParams{Window: pgIntervalFrom(window), Limit: int32(limit)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list trending searches", err)
	}
	out := make([]*domain.TrendingTerm, len(rows))
	for i, row := range rows {
		out[i] = &domain.TrendingTerm{Query: row.Query, SearchCount: row.SearchCount}
	}
	return out, nil
}

func (r *Repository) ListZeroResult(ctx context.Context, window time.Duration, limit int) ([]*domain.TrendingTerm, error) {
	rows, err := r.q.ListZeroResultSearches(ctx, sqlcgen.ListZeroResultSearchesParams{Window: pgIntervalFrom(window), Limit: int32(limit)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list zero-result searches", err)
	}
	out := make([]*domain.TrendingTerm, len(rows))
	for i, row := range rows {
		out[i] = &domain.TrendingTerm{Query: row.Query, SearchCount: row.SearchCount}
	}
	return out, nil
}

// pgIntervalFrom converts a Go duration into a Postgres-parseable
// interval literal (e.g. "86400 seconds") for the
// sqlc.arg('window')::interval cast in search.sql. Deliberately
// expressed in seconds rather than mimicking Go's "24h0m0s" format,
// which Postgres's interval parser does not understand.
func pgIntervalFrom(d time.Duration) string {
	return fmt.Sprintf("%d seconds", int64(d.Seconds()))
}

var _ domain.Repository = (*Repository)(nil)

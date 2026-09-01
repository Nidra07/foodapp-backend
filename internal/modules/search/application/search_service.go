package application

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/search/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type SearchService struct {
	repo domain.Repository
}

func NewSearchService(repo domain.Repository) *SearchService {
	return &SearchService{repo: repo}
}

// SearchRestaurants runs the query, then logs it (best-effort — a
// logging failure must never break the actual search response the
// customer is waiting on, same "don't let a secondary concern block the
// primary one" principle used for Notify/LogAction elsewhere).
func (s *SearchService) SearchRestaurants(ctx context.Context, in domain.SearchInput) ([]*domain.RestaurantResult, error) {
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return nil, apperr.Validation("search query cannot be empty", nil)
	}
	if in.SearchRadiusM <= 0 {
		in.SearchRadiusM = 10000
	}
	if in.Page < 1 {
		in.Page = 1
	}
	if in.PageSize < 1 || in.PageSize > 50 {
		in.PageSize = 20
	}

	results, err := s.repo.SearchRestaurants(ctx, query, in.Location, in.SearchRadiusM, in.Page, in.PageSize)
	if err != nil {
		return nil, err
	}

	s.logSearch(ctx, in.UserID, query, "restaurant", len(results))
	return results, nil
}

func (s *SearchService) SearchMenuItems(ctx context.Context, in domain.SearchInput) ([]*domain.ItemResult, error) {
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return nil, apperr.Validation("search query cannot be empty", nil)
	}
	if in.SearchRadiusM <= 0 {
		in.SearchRadiusM = 10000
	}
	if in.Page < 1 {
		in.Page = 1
	}
	if in.PageSize < 1 || in.PageSize > 50 {
		in.PageSize = 20
	}

	results, err := s.repo.SearchMenuItems(ctx, query, in.Location, in.SearchRadiusM, in.Page, in.PageSize)
	if err != nil {
		return nil, err
	}

	s.logSearch(ctx, in.UserID, query, "item", len(results))
	return results, nil
}

func (s *SearchService) logSearch(ctx context.Context, userID *uuid.UUID, query, searchType string, resultCount int) {
	// Best-effort: a logging failure must never break the actual search
	// response the customer is waiting on — same principle as
	// Notifier.Notify / AuditLogger.LogAction elsewhere in this codebase.
	_ = s.repo.LogSearch(ctx, userID, query, searchType, resultCount)
}

func (s *SearchService) Trending(ctx context.Context, window time.Duration, limit int) ([]*domain.TrendingTerm, error) {
	if window <= 0 {
		window = 24 * time.Hour
	}
	if limit < 1 || limit > 50 {
		limit = 10
	}
	return s.repo.ListTrending(ctx, window, limit)
}

func (s *SearchService) ZeroResultSearches(ctx context.Context, window time.Duration, limit int) ([]*domain.TrendingTerm, error) {
	if window <= 0 {
		window = 7 * 24 * time.Hour
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return s.repo.ListZeroResult(ctx, window, limit)
}

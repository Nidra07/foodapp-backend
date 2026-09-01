package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/users/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type UserService struct {
	repo domain.Repository
}

func NewUserService(repo domain.Repository) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) GetProfile(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	return s.repo.GetByID(ctx, userID)
}

func (s *UserService) UpdateProfile(ctx context.Context, userID uuid.UUID, in domain.UpdateProfileInput) (*domain.User, error) {
	if in.FullName != nil && len(*in.FullName) > 150 {
		return nil, apperr.Validation("full name too long", map[string]interface{}{"field": "full_name", "max_length": 150})
	}
	return s.repo.UpdateProfile(ctx, userID, in)
}

func (s *UserService) DeactivateAccount(ctx context.Context, userID uuid.UUID) error {
	return s.repo.SoftDelete(ctx, userID)
}

// AdminListUsers is used by the admin panel's user management screens.
// Authorization (admin-only) is enforced at the HTTP layer via
// middleware.RequireRole, not here — this service assumes the caller is
// already authorized.
func (s *UserService) AdminListUsers(ctx context.Context, filter domain.ListUsersFilter) ([]*domain.User, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	return s.repo.List(ctx, filter)
}

func (s *UserService) AdminSetStatus(ctx context.Context, userID uuid.UUID, status domain.Status) error {
	return s.repo.SetStatus(ctx, userID, status)
}

package infrastructure

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/foodapp/backend/internal/modules/users/domain"
	sqlcgen "github.com/foodapp/backend/internal/platform/db/sqlc"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type Repository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) *Repository {
	return &Repository{q: q}
}

func (r *Repository) Create(ctx context.Context, u *domain.User) (*domain.User, error) {
	row, err := r.q.CreateUser(ctx, sqlcgen.CreateUserParams{
		PhoneNumber: toText(u.PhoneNumber),
		Email:       toText(u.Email),
		FullName:    toText(u.FullName),
		PrimaryRole: sqlcgen.UserRole(u.PrimaryRole),
		Status:      sqlcgen.UserStatus(domain.StatusPendingVerification),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create user", err)
	}
	return mapUser(row), nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("user")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch user", err)
	}
	return mapUser(row), nil
}

func (r *Repository) GetByPhone(ctx context.Context, phone string) (*domain.User, error) {
	row, err := r.q.GetUserByPhone(ctx, toText(&phone))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("user")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch user", err)
	}
	return mapUser(row), nil
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row, err := r.q.GetUserByEmail(ctx, toText(&email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("user")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch user", err)
	}
	return mapUser(row), nil
}

func (r *Repository) GetByPhoneOrEmail(ctx context.Context, identifier string) (*domain.User, error) {
	if strings.Contains(identifier, "@") {
		return r.GetByEmail(ctx, identifier)
	}
	return r.GetByPhone(ctx, identifier)
}

func (r *Repository) UpdateProfile(ctx context.Context, id uuid.UUID, in domain.UpdateProfileInput) (*domain.User, error) {
	row, err := r.q.UpdateUserProfile(ctx, sqlcgen.UpdateUserProfileParams{
		ID:              id,
		FullName:        toText(in.FullName),
		Email:           toText(in.Email),
		ProfileImageUrl: toText(in.ProfileImageURL),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to update user profile", err)
	}
	return mapUser(row), nil
}

func (r *Repository) MarkPhoneVerified(ctx context.Context, id uuid.UUID) error {
	if err := r.q.MarkPhoneVerified(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to mark phone verified", err)
	}
	return nil
}

func (r *Repository) MarkEmailVerified(ctx context.Context, id uuid.UUID) error {
	if err := r.q.MarkEmailVerified(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to mark email verified", err)
	}
	return nil
}

func (r *Repository) UpdateLastLogin(ctx context.Context, id uuid.UUID) error {
	if err := r.q.UpdateLastLogin(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to update last login", err)
	}
	return nil
}

func (r *Repository) SetStatus(ctx context.Context, id uuid.UUID, status domain.Status) error {
	if err := r.q.SetUserStatus(ctx, sqlcgen.SetUserStatusParams{ID: id, Status: sqlcgen.UserStatus(status)}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to set user status", err)
	}
	return nil
}

func (r *Repository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	if err := r.q.SoftDeleteUser(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to deactivate user", err)
	}
	return nil
}

func (r *Repository) List(ctx context.Context, filter domain.ListUsersFilter) ([]*domain.User, int64, error) {
	offset := (filter.Page - 1) * filter.PageSize
	rows, err := r.q.ListUsersByRole(ctx, sqlcgen.ListUsersByRoleParams{
		PrimaryRole: sqlcgen.UserRole(filter.Role),
		Limit:       int32(filter.PageSize),
		Offset:      int32(offset),
	})
	if err != nil {
		return nil, 0, apperr.Wrap(apperr.CodeInternal, "failed to list users", err)
	}
	total, err := r.q.CountUsersByRole(ctx, sqlcgen.UserRole(filter.Role))
	if err != nil {
		return nil, 0, apperr.Wrap(apperr.CodeInternal, "failed to count users", err)
	}

	users := make([]*domain.User, len(rows))
	for i, row := range rows {
		users[i] = mapUser(row)
	}
	return users, total, nil
}

func (r *Repository) GrantRole(ctx context.Context, id uuid.UUID, role domain.Role) error {
	_, err := r.q.GrantUserRole(ctx, sqlcgen.GrantUserRoleParams{UserID: id, Role: sqlcgen.UserRole(role)})
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to grant role", err)
	}
	return nil
}

// --- mapping helpers ---

func mapUser(row sqlcgen.User) *domain.User {
	u := &domain.User{
		ID:          row.ID,
		PrimaryRole: domain.Role(row.PrimaryRole),
		Status:      domain.Status(row.Status),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if row.PhoneNumber.Valid {
		u.PhoneNumber = &row.PhoneNumber.String
	}
	if row.Email.Valid {
		u.Email = &row.Email.String
	}
	if row.FullName.Valid {
		u.FullName = &row.FullName.String
	}
	if row.ProfileImageUrl.Valid {
		u.ProfileImageURL = &row.ProfileImageUrl.String
	}
	if row.PhoneVerifiedAt.Valid {
		t := row.PhoneVerifiedAt.Time
		u.PhoneVerifiedAt = &t
	}
	if row.EmailVerifiedAt.Valid {
		t := row.EmailVerifiedAt.Time
		u.EmailVerifiedAt = &t
	}
	if row.LastLoginAt.Valid {
		t := row.LastLoginAt.Time
		u.LastLoginAt = &t
	}
	return u
}

func toText(s *string) pgtype.Text {
	if s == nil || *s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

var _ domain.Repository = (*Repository)(nil)

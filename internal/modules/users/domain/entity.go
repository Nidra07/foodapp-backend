package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleCustomer         Role = "customer"
	RoleRestaurantOwner  Role = "restaurant_owner"
	RoleRestaurantStaff  Role = "restaurant_staff"
	RoleDeliveryPartner  Role = "delivery_partner"
	RoleAdmin            Role = "admin"
)

type Status string

const (
	StatusActive              Status = "active"
	StatusSuspended            Status = "suspended"
	StatusDeactivated          Status = "deactivated"
	StatusPendingVerification  Status = "pending_verification"
)

type User struct {
	ID               uuid.UUID
	PhoneNumber      *string
	Email            *string
	FullName         *string
	PrimaryRole      Role
	Status           Status
	PhoneVerifiedAt  *time.Time
	EmailVerifiedAt  *time.Time
	ProfileImageURL  *string
	LastLoginAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (u *User) IsActive() bool { return u.Status == StatusActive }

type UpdateProfileInput struct {
	FullName        *string
	Email           *string
	ProfileImageURL *string
}

type ListUsersFilter struct {
	Role     Role
	Page     int
	PageSize int
}

type Repository interface {
	Create(ctx context.Context, u *User) (*User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByPhone(ctx context.Context, phone string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	// GetByPhoneOrEmail dispatches based on whether identifier looks like
	// an email (contains "@") or a phone number.
	GetByPhoneOrEmail(ctx context.Context, identifier string) (*User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, in UpdateProfileInput) (*User, error)
	MarkPhoneVerified(ctx context.Context, id uuid.UUID) error
	MarkEmailVerified(ctx context.Context, id uuid.UUID) error
	UpdateLastLogin(ctx context.Context, id uuid.UUID) error
	SetStatus(ctx context.Context, id uuid.UUID, status Status) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, filter ListUsersFilter) ([]*User, int64, error)
	GrantRole(ctx context.Context, id uuid.UUID, role Role) error
}

package infrastructure

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/foodapp/backend/internal/modules/notifications/domain"
	sqlcgen "github.com/foodapp/backend/internal/platform/db/sqlc"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type Repository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) *Repository {
	return &Repository{q: q}
}

func (r *Repository) Create(ctx context.Context, n *domain.Notification) (*domain.Notification, error) {
	var dataJSON []byte
	if n.Data != nil {
		var err error
		dataJSON, err = json.Marshal(n.Data)
		if err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "failed to marshal notification data", err)
		}
	}

	row, err := r.q.CreateNotification(ctx, sqlcgen.CreateNotificationParams{
		UserID: n.UserID, Category: sqlcgen.NotificationCategory(n.Category), Channel: sqlcgen.NotificationChannel(n.Channel),
		Title: n.Title, Body: n.Body, Data: dataJSON,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create notification", err)
	}
	return mapNotification(row), nil
}

func (r *Repository) MarkSent(ctx context.Context, id uuid.UUID) error {
	if err := r.q.MarkNotificationSent(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to mark notification sent", err)
	}
	return nil
}

func (r *Repository) MarkFailed(ctx context.Context, id uuid.UUID, reason string) error {
	if err := r.q.MarkNotificationFailed(ctx, sqlcgen.MarkNotificationFailedParams{ID: id, FailureReason: toText(&reason)}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to mark notification failed", err)
	}
	return nil
}

func (r *Repository) MarkSkipped(ctx context.Context, id uuid.UUID) error {
	if err := r.q.MarkNotificationSkipped(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to mark notification skipped", err)
	}
	return nil
}

func (r *Repository) ListForUser(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]*domain.Notification, error) {
	offset := (page - 1) * pageSize
	rows, err := r.q.ListNotificationsForUser(ctx, sqlcgen.ListNotificationsForUserParams{UserID: userID, Limit: int32(pageSize), Offset: int32(offset)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list notifications", err)
	}
	out := make([]*domain.Notification, len(rows))
	for i, row := range rows {
		out[i] = mapNotification(row)
	}
	return out, nil
}

func (r *Repository) CountUnread(ctx context.Context, userID uuid.UUID) (int64, error) {
	n, err := r.q.CountUnreadForUser(ctx, userID)
	if err != nil {
		return 0, apperr.Wrap(apperr.CodeInternal, "failed to count unread notifications", err)
	}
	return n, nil
}

func (r *Repository) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	if err := r.q.MarkNotificationRead(ctx, sqlcgen.MarkNotificationReadParams{ID: id, UserID: userID}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to mark notification read", err)
	}
	return nil
}

func (r *Repository) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	if err := r.q.MarkAllNotificationsRead(ctx, userID); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to mark notifications read", err)
	}
	return nil
}

func (r *Repository) GetPreference(ctx context.Context, userID uuid.UUID, category domain.Category, channel domain.Channel) (*domain.Preference, error) {
	row, err := r.q.GetUserPreference(ctx, sqlcgen.GetUserPreferenceParams{
		UserID: userID, Category: sqlcgen.NotificationCategory(category), Channel: sqlcgen.NotificationChannel(channel),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("notification preference")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch notification preference", err)
	}
	return &domain.Preference{UserID: row.UserID, Category: domain.Category(row.Category), Channel: domain.Channel(row.Channel), Enabled: row.Enabled}, nil
}

func (r *Repository) ListPreferences(ctx context.Context, userID uuid.UUID) ([]*domain.Preference, error) {
	rows, err := r.q.ListUserPreferences(ctx, userID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list notification preferences", err)
	}
	out := make([]*domain.Preference, len(rows))
	for i, row := range rows {
		out[i] = &domain.Preference{UserID: row.UserID, Category: domain.Category(row.Category), Channel: domain.Channel(row.Channel), Enabled: row.Enabled}
	}
	return out, nil
}

func (r *Repository) UpsertPreference(ctx context.Context, p *domain.Preference) (*domain.Preference, error) {
	row, err := r.q.UpsertUserPreference(ctx, sqlcgen.UpsertUserPreferenceParams{
		UserID: p.UserID, Category: sqlcgen.NotificationCategory(p.Category), Channel: sqlcgen.NotificationChannel(p.Channel), Enabled: p.Enabled,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to update notification preference", err)
	}
	return &domain.Preference{UserID: row.UserID, Category: domain.Category(row.Category), Channel: domain.Channel(row.Channel), Enabled: row.Enabled}, nil
}

// --- mapping helpers ---

func mapNotification(row sqlcgen.Notification) *domain.Notification {
	n := &domain.Notification{
		ID: row.ID, UserID: row.UserID, Category: domain.Category(row.Category), Channel: domain.Channel(row.Channel),
		Title: row.Title, Body: row.Body, SendStatus: domain.SendStatus(row.SendStatus), IsRead: row.IsRead, CreatedAt: row.CreatedAt,
	}
	if len(row.Data) > 0 {
		var data map[string]interface{}
		if err := json.Unmarshal(row.Data, &data); err == nil {
			n.Data = data
		}
	}
	if row.FailureReason.Valid {
		n.FailureReason = &row.FailureReason.String
	}
	if row.ReadAt.Valid {
		t := row.ReadAt.Time
		n.ReadAt = &t
	}
	if row.SentAt.Valid {
		t := row.SentAt.Time
		n.SentAt = &t
	}
	return n
}

func toText(s *string) pgtype.Text {
	if s == nil || *s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

var _ domain.Repository = (*Repository)(nil)

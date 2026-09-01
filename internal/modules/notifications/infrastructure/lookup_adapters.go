// DeviceLookupAdapter and ContactLookupAdapter implement the
// Notifications module's small consumer-defined lookup interfaces
// (domain.DeviceLookup, domain.ContactLookup) directly against the
// shared sqlc Queries struct. This is a deliberate exception to the
// "define an interface, implement it in the owning module" pattern used
// everywhere else (Orders' CartReader, Payments' OrderReader, etc.):
// devices and users are simple lookup tables with no business logic of
// their own to go through, so reading them directly here avoids a
// pointless indirection through the Identity/Users modules' full
// application services just to run a SELECT. If either lookup ever
// needs real logic (e.g. filtering stale tokens), move it behind the
// owning module's application layer instead.
package infrastructure

import (
	"context"

	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/notifications/domain"
	sqlcgen "github.com/foodapp/backend/internal/platform/db/sqlc"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type DeviceLookupAdapter struct {
	q *sqlcgen.Queries
}

func NewDeviceLookupAdapter(q *sqlcgen.Queries) *DeviceLookupAdapter {
	return &DeviceLookupAdapter{q: q}
}

func (d *DeviceLookupAdapter) ListFCMTokensForUser(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := d.q.ListActiveFCMTokensForUser(ctx, userID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list device tokens", err)
	}
	tokens := make([]string, 0, len(rows))
	for _, t := range rows {
		if t.Valid {
			tokens = append(tokens, t.String)
		}
	}
	return tokens, nil
}

type ContactLookupAdapter struct {
	q *sqlcgen.Queries
}

func NewContactLookupAdapter(q *sqlcgen.Queries) *ContactLookupAdapter {
	return &ContactLookupAdapter{q: q}
}

func (c *ContactLookupAdapter) GetContactInfo(ctx context.Context, userID uuid.UUID) (*string, *string, error) {
	row, err := c.q.GetUserByID(ctx, userID)
	if err != nil {
		return nil, nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch user contact info", err)
	}
	var phone, email *string
	if row.PhoneNumber.Valid {
		phone = &row.PhoneNumber.String
	}
	if row.Email.Valid {
		email = &row.Email.String
	}
	return phone, email, nil
}

var _ domain.DeviceLookup = (*DeviceLookupAdapter)(nil)
var _ domain.ContactLookup = (*ContactLookupAdapter)(nil)

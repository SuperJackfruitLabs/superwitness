package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
)

// PGStore keeps sessions in the sessions table. Ready gates it until the schema is migrated.
type PGStore struct {
	Pool  *pgxpool.Pool
	Ready func() bool
}

func unavailable(err error) error { return fmt.Errorf("%w: %v", ErrUnavailable, err) }

func (s *PGStore) ready() error {
	if s.Ready != nil && !s.Ready() {
		return ErrUnavailable
	}
	return nil
}

func (s *PGStore) Create(ctx context.Context, x Session) error {
	if err := s.ready(); err != nil {
		return err
	}
	var email *string
	if x.Email != "" {
		email = &x.Email
	}
	sealed, access, unreachable := grantColumns(x.Grant)
	_, err := s.Pool.Exec(ctx, `INSERT INTO sessions (id_hash, principal, principal_kind, tenant, email, created_at, last_seen_at, expires_at,
		refresh_sealed, access_expires_at, unreachable_since)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		x.IDHash, x.Principal, string(x.PrincipalKind), x.Tenant, email, x.CreatedAt, x.LastSeenAt, x.ExpiresAt,
		sealed, access, unreachable)
	if err != nil {
		return unavailable(err)
	}
	return nil
}

func (s *PGStore) Get(ctx context.Context, id []byte) (Session, error) {
	if err := s.ready(); err != nil {
		return Session{}, err
	}
	var x Session
	var kind string
	var email *string
	var access, unreachable *time.Time
	err := s.Pool.QueryRow(ctx, `SELECT id_hash, principal, principal_kind, tenant, email, created_at, last_seen_at, expires_at,
		refresh_sealed, access_expires_at, unreachable_since
		FROM sessions WHERE id_hash = $1`, id).Scan(&x.IDHash, &x.Principal, &kind, &x.Tenant, &email, &x.CreatedAt, &x.LastSeenAt, &x.ExpiresAt,
		&x.Grant.Sealed, &access, &unreachable)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, unavailable(err)
	}
	x.PrincipalKind = auth.PrincipalKind(kind)
	if email != nil {
		x.Email = *email
	}
	x.CreatedAt, x.LastSeenAt, x.ExpiresAt = x.CreatedAt.UTC(), x.LastSeenAt.UTC(), x.ExpiresAt.UTC()
	if access != nil {
		x.Grant.AccessExpiresAt = access.UTC()
	}
	if unreachable != nil {
		x.Grant.UnreachableSince = unreachable.UTC()
	}
	return x, nil
}

// grantColumns turns a grant into its nullable columns: NULL for each zero part.
func grantColumns(g PlaneGrant) (sealed []byte, access, unreachable *time.Time) {
	if len(g.Sealed) > 0 {
		sealed = g.Sealed
	}
	if !g.AccessExpiresAt.IsZero() {
		access = &g.AccessExpiresAt
	}
	if !g.UnreachableSince.IsZero() {
		unreachable = &g.UnreachableSince
	}
	return sealed, access, unreachable
}

func (s *PGStore) SetGrant(ctx context.Context, id []byte, g PlaneGrant) error {
	if err := s.ready(); err != nil {
		return err
	}
	sealed, access, unreachable := grantColumns(g)
	if _, err := s.Pool.Exec(ctx, `UPDATE sessions SET refresh_sealed = $2, access_expires_at = $3, unreachable_since = $4
		WHERE id_hash = $1`, id, sealed, access, unreachable); err != nil {
		return unavailable(err)
	}
	return nil
}

func (s *PGStore) Touch(ctx context.Context, id []byte, at time.Time) error {
	if err := s.ready(); err != nil {
		return err
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE sessions SET last_seen_at = $2 WHERE id_hash = $1`, id, at); err != nil {
		return unavailable(err)
	}
	return nil
}

func (s *PGStore) Delete(ctx context.Context, id []byte) error {
	if err := s.ready(); err != nil {
		return err
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE id_hash = $1`, id); err != nil {
		return unavailable(err)
	}
	return nil
}

func (s *PGStore) Sweep(ctx context.Context, now time.Time) (int64, error) {
	if err := s.ready(); err != nil {
		return 0, err
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= $1 OR last_seen_at <= $2`, now, now.Add(-IdleTTL))
	if err != nil {
		return 0, unavailable(err)
	}
	return tag.RowsAffected(), nil
}

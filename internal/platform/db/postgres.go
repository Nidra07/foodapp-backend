// Package db owns the Postgres connection pool. sqlc-generated query
// packages (db/queries -> internal/platform/db/sqlc) consume *pgxpool.Pool
// through this wrapper. GORM was rejected — see docs/architecture.md
// ("Database Access Layer") for the sqlc-vs-GORM rationale: sqlc gives
// compile-time-checked SQL with no ORM magic/N+1 surprises, which matters
// once order/pricing queries get complex.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/foodapp/backend/internal/config"
)

// pgxTx is a local alias so calling code doesn't need to import pgx
// directly just to write a WithTx callback.
type pgxTx = pgx.Tx

type Postgres struct {
	Pool *pgxpool.Pool
}

func NewPostgres(ctx context.Context, cfg config.PostgresConfig) (*Postgres, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxOpenConns
	poolCfg.MinConns = cfg.MaxIdleConns
	poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime
	poolCfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &Postgres{Pool: pool}, nil
}

func (p *Postgres) Close() {
	p.Pool.Close()
}

func (p *Postgres) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return p.Pool.Ping(ctx)
}

// WithTx runs fn inside a transaction, committing on success and rolling
// back on any error or panic. Every multi-write use case (order placement,
// payment capture, settlement) must go through this instead of ad hoc
// BEGIN/COMMIT calls, so failure/rollback behavior is uniform.
func (p *Postgres) WithTx(ctx context.Context, fn func(tx pgxTx) error) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback(ctx)
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("rollback failed: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// Package db provides database access to the Neon PostgreSQL database
// for querying the gmail_connect table.
//
// Connections are pooled and reused across warm Lambda invocations via
// a process-wide singleton initialised on first use.
package db

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GmailConnect represents a row from the public.gmail_connect table.
type GmailConnect struct {
	OwnerID      string
	Gmail        string
	RefreshToken string
	Scope        *string // nullable — nil means no scope granted
}

// ErrNotFound indicates the queried owner_id has no gmail_connect row.
var ErrNotFound = errors.New("db: gmail_connect record not found")

var (
	pool     *pgxpool.Pool
	poolErr  error
	poolOnce sync.Once
)

// Pool returns the process-wide pgxpool, initialising it on first call.
// In Lambda the first invocation pays the connection cost; subsequent
// warm invocations reuse pooled connections.
func Pool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	poolOnce.Do(func() {
		poolCfg, err := pgxpool.ParseConfig(databaseURL)
		if err != nil {
			poolErr = fmt.Errorf("db: parse pool config: %w", err)
			return
		}
		// Lambda-friendly bounds: each instance handles one request at a
		// time; a small ceiling prevents pool sprawl across warm starts.
		poolCfg.MaxConns = 2
		poolCfg.MinConns = 0

		p, err := pgxpool.NewWithConfig(ctx, poolCfg)
		if err != nil {
			poolErr = fmt.Errorf("db: create pool: %w", err)
			return
		}
		pool = p
	})
	return pool, poolErr
}

// FetchGmailConnect queries gmail_connect by owner_id.
// Returns ErrNotFound when no row matches.
func FetchGmailConnect(ctx context.Context, databaseURL, ownerID string) (*GmailConnect, error) {
	p, err := Pool(ctx, databaseURL)
	if err != nil {
		return nil, err
	}

	var gc GmailConnect
	err = p.QueryRow(ctx,
		`SELECT owner_id, gmail, refresh_token, scope
		 FROM public.gmail_connect
		 WHERE owner_id = $1`,
		ownerID,
	).Scan(&gc.OwnerID, &gc.Gmail, &gc.RefreshToken, &gc.Scope)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("db: query gmail_connect: %w", err)
	}
	return &gc, nil
}

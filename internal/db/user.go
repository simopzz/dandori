package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type userKey struct{}

// WithUser returns ctx carrying the signed-in User's id, which AsUser runs
// their queries as.
func WithUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userKey{}, userID)
}

// UserID is the signed-in User's id ctx carries, if any.
func UserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userKey{}).(string)
	return id, ok
}

// AsUser runs fn in a transaction as the User ctx carries: as Supabase's
// authenticated role, with their id as the claims' sub, so row-level
// security shows fn only that User's rows, whatever its queries filter.
// It commits if fn returns nil. Every query on an owned table runs in one.
func AsUser(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	userID, ok := UserID(ctx)
	if !ok {
		return errors.New("db.AsUser: no signed-in User")
	}
	claims, err := json.Marshal(map[string]string{"sub": userID, "role": "authenticated"})
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE authenticated"); err != nil {
			return fmt.Errorf("switch to the authenticated role: %w", err)
		}
		if _, err := tx.Exec(ctx, "SELECT set_config('request.jwt.claims', $1, true)", string(claims)); err != nil {
			return fmt.Errorf("set the User's claims: %w", err)
		}
		return fn(tx)
	})
}

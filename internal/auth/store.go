package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
)

// saveUser creates the User at their first sign-in, and keeps their email
// as Supabase has it at each one after.
func saveUser(ctx context.Context, q db.Querier, id, email string, now time.Time) error {
	_, err := q.Exec(ctx, `
		INSERT INTO "user" (id, email, created_at) VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET email = excluded.email`,
		id, email, now)
	return err
}

// loadEmail reads the signed-in User's email. It has no WHERE on purpose:
// run in db.AsUser, row-level security leaves only their row, and any
// other row it let through is an error.
func loadEmail(ctx context.Context, q db.Querier) (string, error) {
	rows, err := q.Query(ctx, `SELECT email FROM "user"`)
	if err != nil {
		return "", err
	}
	emails, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", err
	}
	if len(emails) != 1 {
		return "", fmt.Errorf("row-level security let through %d Users, want 1", len(emails))
	}
	return emails[0], nil
}

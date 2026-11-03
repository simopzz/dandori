package auth

import (
	"context"
	"time"

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
// run in db.AsUser, row-level security leaves only their row.
func loadEmail(ctx context.Context, q db.Querier) (string, error) {
	var email string
	err := q.QueryRow(ctx, `SELECT email FROM "user"`).Scan(&email)
	return email, err
}

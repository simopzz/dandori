// Package auth signs Users in and out: by magic link through Supabase
// Auth, for the few Users invited from its dashboard (ADR-0015). It keeps
// Supabase's session in cookies and puts the signed-in User on each
// request's context, which db.AsUser runs their queries as.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/db"
)

// Config is where Supabase Auth is.
type Config struct {
	// SupabaseURL is the project's URL, https://<ref>.supabase.co.
	SupabaseURL string
	// PublishableKey is the project's publishable API key.
	PublishableKey string
}

// Tokens are a Supabase session as the cookies keep it.
type Tokens struct {
	Access  string
	Refresh string
}

// ErrBadLink is a magic link Supabase Auth won't redeem: expired, or
// already used.
var ErrBadLink = errors.New("the link has expired or was already used")

// ErrSignedOut means a request's Tokens no longer sign anyone in.
var ErrSignedOut = errors.New("signed out")

// Service is signing in and out.
type Service struct {
	pool     *pgxpool.Pool
	supabase *supabase
	clock    clock.Clock
}

// New creates the Service.
func New(pool *pgxpool.Pool, cfg Config, c clock.Clock) *Service {
	return &Service{pool: pool, supabase: newSupabase(cfg.SupabaseURL, cfg.PublishableKey, c), clock: c}
}

// SendMagicLink emails a magic link leading to redirectTo, if email was
// invited. Whether it was is never told: Supabase refusing an email
// that wasn't is no error. Only failing to reach Supabase is.
func (s *Service) SendMagicLink(ctx context.Context, email, redirectTo string) error {
	err := s.supabase.sendMagicLink(ctx, email, redirectTo)
	if apiErr, ok := errors.AsType[*apiError](err); ok && apiErr.Status < http.StatusInternalServerError {
		return nil
	}
	return err
}

// Redeem signs in with a magic link's token hash, kind being the link's
// type, and creates the User at their first sign-in.
func (s *Service) Redeem(ctx context.Context, tokenHash, kind string) (Tokens, error) {
	sess, err := s.supabase.verify(ctx, tokenHash, kind)
	if apiErr, ok := errors.AsType[*apiError](err); ok && apiErr.Status < http.StatusInternalServerError {
		return Tokens{}, ErrBadLink
	}
	if err != nil {
		return Tokens{}, err
	}
	ctx = db.WithUser(ctx, sess.User.ID)
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		return saveUser(ctx, tx, sess.User.ID, sess.User.Email, s.clock.Now())
	})
	if err != nil {
		return Tokens{}, fmt.Errorf("save the User: %w", err)
	}
	return Tokens{Access: sess.AccessToken, Refresh: sess.RefreshToken}, nil
}

// Authenticate tells who tokens sign in. When the access token has
// expired, it refreshes the session: renewed are the Tokens to keep in
// its place. It is ErrSignedOut when they sign no one in any more.
func (s *Service) Authenticate(ctx context.Context, tokens Tokens) (userID string, renewed *Tokens, err error) {
	c, err := s.supabase.check(ctx, tokens.Access)
	if err == nil {
		return c.UserID, nil, nil
	}
	if !errors.Is(err, errExpired) || tokens.Refresh == "" {
		return "", nil, ErrSignedOut
	}
	sess, err := s.supabase.refresh(ctx, tokens.Refresh)
	if apiErr, ok := errors.AsType[*apiError](err); ok && apiErr.Status < http.StatusInternalServerError {
		return "", nil, ErrSignedOut
	}
	if err != nil {
		return "", nil, fmt.Errorf("refresh the session: %w", err)
	}
	c, err = s.supabase.check(ctx, sess.AccessToken)
	if err != nil {
		return "", nil, fmt.Errorf("check the refreshed session: %w", err)
	}
	return c.UserID, &Tokens{Access: sess.AccessToken, Refresh: sess.RefreshToken}, nil
}

// SignOut ends the session tokens belong to.
func (s *Service) SignOut(ctx context.Context, tokens Tokens) error {
	return s.supabase.signOut(ctx, tokens.Access)
}

// Email is the signed-in User's email.
func (s *Service) Email(ctx context.Context) (string, error) {
	var email string
	err := db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		email, err = loadEmail(ctx, tx)
		return err
	})
	return email, err
}

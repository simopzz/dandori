// Package apptest starts MealOps in-process over HTTP against a real,
// freshly migrated Postgres + pgvector database of its own, for tests at
// the app seam.
//
// MEALOPS_TEST_DATABASE_URL must point at a disposable Postgres server with
// pgvector, as a role that may create databases (`mise run testdb`).
package apptest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/mealops/internal/app"
	"github.com/simopzz/mealops/internal/db"
)

// Harness is one running app with its own database.
type Harness struct {
	URL    string
	Client *http.Client

	proxy *proxy
}

// Option customises the app a test starts.
type Option func(*settings)

type settings struct {
	ports       app.Ports
	databaseCut bool
}

// WithPorts lets a test swap outbound ports for fakes.
func WithPorts(configure func(*app.Ports)) Option {
	return func(s *settings) { configure(&s.ports) }
}

// WithDatabaseCut starts the app while its database is unreachable.
func WithDatabaseCut() Option {
	return func(s *settings) { s.databaseCut = true }
}

// Start boots the app against a new database cloned from a migrated
// template. Everything is torn down when the test ends.
func Start(t testing.TB, opts ...Option) *Harness {
	t.Helper()
	var s settings
	for _, opt := range opts {
		opt(&s)
	}

	adminURL := os.Getenv("MEALOPS_TEST_DATABASE_URL")
	if adminURL == "" {
		t.Fatal("MEALOPS_TEST_DATABASE_URL is not set; run tests through mise and start the test database with `mise run testdb`")
	}
	dbURL := createDatabase(t, adminURL)

	u, err := url.Parse(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	p := startProxy(t, u.Host)
	p.setReachable(!s.databaseCut)
	u.Host = p.addr()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if testing.Verbose() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	a, err := app.New(ctx, app.Config{DatabaseURL: u.String()}, s.ports, logger)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() {
		srv.Close()
		a.Close()
	})
	return &Harness{URL: srv.URL, Client: srv.Client(), proxy: p}
}

// CutDatabase makes the database unreachable, as when Supabase pauses the
// project: open connections drop and new ones are refused.
func (h *Harness) CutDatabase() { h.proxy.setReachable(false) }

// RestoreDatabase makes the database reachable again.
func (h *Harness) RestoreDatabase() { h.proxy.setReachable(true) }

// Response is a fully read HTTP response.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       string
}

// Get requests path from the app.
func (h *Harness) Get(t testing.TB, path string) Response {
	t.Helper()
	res, err := h.Client.Get(h.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("GET %s: read body: %v", path, err)
	}
	return Response{StatusCode: res.StatusCode, Header: res.Header, Body: string(body)}
}

// templateLock serialises template creation and cloning within this test
// binary; a Postgres advisory lock does the same across test binaries.
var templateLock sync.Mutex

const advisoryLockKey = 0x6d65616c6f7073 // "mealops"

func createDatabase(t testing.TB, adminURL string) string {
	t.Helper()
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to test database server (start it with `mise run testdb`): %v", err)
	}
	defer admin.Close(ctx)

	templateLock.Lock()
	defer templateLock.Unlock()
	if _, err := admin.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockKey); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "SELECT pg_advisory_unlock($1)", advisoryLockKey)

	template := "mealops_template_" + migrationsFingerprint(t)
	var exists bool
	if err := admin.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", template).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		migrateTemplate(t, admin, adminURL, template)
	}

	name := "mealops_test_" + randomHex(t)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE "+pgx.Identifier{template}.Sanitize()); err != nil {
		t.Fatalf("clone test database: %v", err)
	}
	t.Cleanup(func() {
		conn, err := pgx.Connect(ctx, adminURL)
		if err != nil {
			t.Errorf("drop test database: %v", err)
			return
		}
		defer conn.Close(ctx)
		if _, err := conn.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
	})
	return withDatabase(t, adminURL, name)
}

func migrateTemplate(t testing.TB, admin *pgx.Conn, adminURL, template string) {
	t.Helper()
	ctx := context.Background()
	ident := pgx.Identifier{template}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		t.Fatalf("create template database: %v", err)
	}
	pool, err := db.Open(ctx, withDatabase(t, adminURL, template))
	if err != nil {
		t.Fatal(err)
	}
	err = db.Migrate(ctx, pool)
	pool.Close()
	if err != nil {
		admin.Exec(ctx, "DROP DATABASE "+ident+" WITH (FORCE)")
		t.Fatalf("migrate template database: %v", err)
	}
}

// migrationsFingerprint names the template after the migrations, so editing
// or adding one yields a fresh template.
func migrationsFingerprint(t testing.TB) string {
	t.Helper()
	h := sha256.New()
	err := fs.WalkDir(db.Migrations(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(db.Migrations(), path)
		if err != nil {
			return err
		}
		h.Write([]byte(path))
		h.Write(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func withDatabase(t testing.TB, rawURL, name string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}

func randomHex(t testing.TB) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

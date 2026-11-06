// Package apptest starts Dandori in-process over HTTP against a real,
// freshly migrated Postgres + pgvector database of its own, for tests at
// the app seam. The app runs on a Manual clock the test sets and steps,
// directly or through POST /_test/clock: a switch only this package
// serves, so the production binary, which never imports it, has none.
//
// DANDORI_TEST_DATABASE_URL must point at a disposable Postgres server with
// pgvector, as a superuser (`mise run testdb`). pgtestdb migrates a template
// database once per set of migrations and clones it for every test.
package apptest

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver pgtestdb connects with
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/goosemigrator"

	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/auth"
	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/telemetry"
)

// Harness is one running app with its own database.
type Harness struct {
	URL string
	// Client is the User's browser: it keeps cookies.
	Client *http.Client
	// Clock is the app's clock, standing at Monday 2 November 2026 in
	// clock.DefaultZone until the test moves it. Moving it past a
	// beat runs the scheduler before Set returns.
	Clock *clock.Manual
	// Auth is the fake Supabase Auth the app signs Users in through.
	Auth *FakeAuth

	databaseURL string
	log         *logBuffer
}

// Option customises the app a test starts.
type Option func(*settings)

type settings struct {
	config     func(*app.Config)
	unmigrated bool
}

// WithConfig lets a test change the app's configuration, on top of any
// earlier change.
func WithConfig(configure func(*app.Config)) Option {
	return func(s *settings) {
		earlier := s.config
		s.config = func(c *app.Config) {
			if earlier != nil {
				earlier(c)
			}
			configure(c)
		}
	}
}

// WithUnmigratedDatabase starts the app on an empty database, as on its
// first deploy, so the app's own startup applies every migration.
func WithUnmigratedDatabase() Option {
	return func(s *settings) { s.unmigrated = true }
}

// Start boots the app against a new database cloned from a migrated
// template. Everything is torn down when the test ends; a failed test's
// database is kept for inspection.
func Start(t testing.TB, opts ...Option) *Harness {
	t.Helper()
	var s settings
	for _, opt := range opts {
		opt(&s)
	}

	adminURL := os.Getenv("DANDORI_TEST_DATABASE_URL")
	if adminURL == "" {
		t.Fatal("DANDORI_TEST_DATABASE_URL is not set; run tests through mise and start the test database with `mise run testdb`")
	}
	admin, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("parse DANDORI_TEST_DATABASE_URL: %v", err)
	}
	password, _ := admin.User.Password()
	port := admin.Port()
	if port == "" {
		port = "5432"
	}
	var migrator pgtestdb.Migrator = goosemigrator.New("migrations", goosemigrator.WithFS(db.Migrations()))
	if s.unmigrated {
		migrator = pgtestdb.NoopMigrator{}
	}
	instance := pgtestdb.Custom(t, pgtestdb.Config{
		DriverName:                "pgx",
		Host:                      admin.Hostname(),
		Port:                      port,
		User:                      admin.User.Username(),
		Password:                  password,
		Database:                  strings.TrimPrefix(admin.Path, "/"),
		Options:                   admin.RawQuery, // keeps sslmode and friends from the URL
		ForceTerminateConnections: true,
		// Migrate and connect as the admin role, like the app's `postgres`
		// role on Supabase: enabling pgvector needs more than a plain role.
		TestRole: &pgtestdb.Role{Username: admin.User.Username(), Password: password},
	}, migrator)

	log := &logBuffer{}
	var logTo io.Writer = log
	if testing.Verbose() {
		logTo = io.MultiWriter(log, os.Stderr)
	}
	logger := telemetry.Logger(logTo, nil)
	manual := clock.NewManual(time.Date(2026, time.November, 2, 0, 0, 0, 0, zone(t)))
	fake := startFakeAuth(t, manual)
	cfg := app.Config{
		DatabaseURL: instance.URL(),
		Clock:       manual,
		Auth:        auth.Config{SupabaseURL: fake.URL, PublishableKey: fakeAPIKey},
	}
	if s.config != nil {
		s.config(&cfg)
	}
	a, err := app.New(context.Background(), cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	// Not a ServeMux: the pattern it matched would name every request's span "GET /".
	handler, setClock := a.Handler(), clockSwitch(manual)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/_test/clock" {
			setClock(w, r)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	scheduled, stop := context.WithCancel(context.Background())
	a.Schedule(scheduled)
	t.Cleanup(func() {
		stop()
		srv.Close()
		a.Close()
	})
	fake.SiteURL = srv.URL
	client := srv.Client()
	client.Jar, err = cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &Harness{URL: srv.URL, Client: client, Clock: manual, Auth: fake, databaseURL: instance.URL(), log: log}
}

// Logged is everything the app has logged so far.
func (h *Harness) Logged() string { return h.log.String() }

// logBuffer keeps what the app logs, from whichever goroutine logs it.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// zone is the installation's zone, which tests tell the time in.
func zone(t testing.TB) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(clock.DefaultZone)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// clockSwitch sets the clock to the RFC 3339 time posted as now, for
// tests that can't reach the Harness, such as a browser's.
func clockSwitch(c *clock.Manual) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now, err := time.Parse(time.RFC3339, r.FormValue("now"))
		if err != nil {
			http.Error(w, "now must be an RFC 3339 time: "+err.Error(), http.StatusBadRequest)
			return
		}
		c.Set(now.In(c.Now().Location()))
		w.WriteHeader(http.StatusNoContent)
	}
}

// Exec runs sql on the app's database, for fixtures of what the app can't
// create through its pages yet.
func (h *Harness) Exec(t testing.TB, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, h.databaseURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// Scan reads the one row sql gives into dest, as the admin role, for a test checking what
// no page shows yet.
func (h *Harness) Scan(t testing.TB, dest any, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, h.databaseURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	defer conn.Close(ctx)
	if err := conn.QueryRow(ctx, sql, args...).Scan(dest); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// Response is a fully read HTTP response.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       string
	// Method and Path are those of the request that got this response:
	// a GET of the new location when a POST was redirected.
	Method string
	Path   string
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
	return Response{StatusCode: res.StatusCode, Header: res.Header, Body: string(body), Method: res.Request.Method, Path: res.Request.URL.Path}
}

// HTMXGet requests path from the app as HTMX does, marked with HX-Request.
func (h *Harness) HTMXGet(t testing.TB, path string) Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, h.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("HX-Request", "true")
	return h.do(t, req)
}

// PostForm submits form to path, as a browser does without HTMX, following
// any redirect.
func (h *Harness) PostForm(t testing.TB, path string, form url.Values) Response {
	t.Helper()
	return h.postForm(t, path, form, false)
}

// HTMXPostForm submits form to path as HTMX does, marked with HX-Request.
func (h *Harness) HTMXPostForm(t testing.TB, path string, form url.Values) Response {
	t.Helper()
	return h.postForm(t, path, form, true)
}

func (h *Harness) postForm(t testing.TB, path string, form url.Values, htmx bool) Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, h.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	return h.do(t, req)
}

func (h *Harness) do(t testing.TB, req *http.Request) Response {
	t.Helper()
	res, err := h.Client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("%s %s: read body: %v", req.Method, req.URL.Path, err)
	}
	return Response{StatusCode: res.StatusCode, Header: res.Header, Body: string(body), Method: res.Request.Method, Path: res.Request.URL.Path}
}

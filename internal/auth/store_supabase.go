package auth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/simopzz/dandori/internal/clock"
)

// supabase talks to Supabase Auth's HTTP API: it sends magic links,
// redeems them for a session, refreshes and revokes sessions, and checks
// an access token against the project's signing keys.
type supabase struct {
	url    string // the project's URL, https://<ref>.supabase.co
	apiKey string // the publishable key
	clock  clock.Clock
	http   *http.Client

	mu      sync.Mutex
	keys    map[string]any // the JWKS's public keys by kid
	fetched time.Time      // when keys were last fetched
}

// newSupabase makes each call a client span under the request that made it, named for
// the call. Its attributes are otelhttp's: never a header or a body, so never a token,
// the API key or an email. No traceparent header goes to Supabase.
func newSupabase(cfg Config, c clock.Clock, tp trace.TracerProvider) *supabase {
	transport := otelhttp.NewTransport(http.DefaultTransport,
		otelhttp.WithTracerProvider(tp),
		otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator()),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string { return callName(r) }))
	return &supabase{url: cfg.SupabaseURL, apiKey: cfg.PublishableKey, clock: c, http: &http.Client{Timeout: 10 * time.Second, Transport: transport}}
}

// callName names a call to Supabase Auth's API for its span.
func callName(r *http.Request) string {
	switch r.URL.Path {
	case "/auth/v1/.well-known/jwks.json":
		return "supabase fetch JWKS"
	case "/auth/v1/token":
		return "supabase renew session"
	case "/auth/v1/otp":
		return "supabase send magic link"
	case "/auth/v1/verify":
		return "supabase verify link"
	case "/auth/v1/logout":
		return "supabase sign out"
	default:
		return "supabase " + r.Method
	}
}

// session is what Supabase Auth answers a redeemed link or a refresh with.
type session struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	User         struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	} `json:"user"`
}

// apiError is an answer Supabase Auth gave with an error status.
type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("supabase auth: %d %s", e.Status, e.Body) }

// refused reports whether Supabase Auth answered, and said no: a 4xx, as
// opposed to failing to answer.
func refused(err error) bool {
	apiErr, ok := errors.AsType[*apiError](err)
	return ok && apiErr.Status < http.StatusInternalServerError
}

// sendMagicLink asks Supabase Auth to email a magic link to email, whose
// link leads to redirectTo. It never creates a User: an email that wasn't
// invited is refused. An invited User who hasn't confirmed yet is sent
// the signup confirmation instead, a link of type signup.
func (s *supabase) sendMagicLink(ctx context.Context, email, redirectTo string) error {
	q := url.Values{"redirect_to": {redirectTo}}
	return s.post(ctx, "/auth/v1/otp?"+q.Encode(), "", map[string]any{"email": email, "create_user": false}, nil)
}

// verify redeems a link's token hash for a session. kind is the link's
// type: email for a magic link, invite for an invitation, signup for a
// signup confirmation.
func (s *supabase) verify(ctx context.Context, tokenHash, kind string) (session, error) {
	var sess session
	err := s.post(ctx, "/auth/v1/verify", "", map[string]string{"token_hash": tokenHash, "type": kind}, &sess)
	return sess, err
}

// refresh exchanges a refresh token, which works once, for a new session.
func (s *supabase) refresh(ctx context.Context, refreshToken string) (session, error) {
	var sess session
	err := s.post(ctx, "/auth/v1/token?grant_type=refresh_token", "", map[string]string{"refresh_token": refreshToken}, &sess)
	return sess, err
}

// signOut revokes the session accessToken belongs to.
func (s *supabase) signOut(ctx context.Context, accessToken string) error {
	return s.post(ctx, "/auth/v1/logout?scope=local", accessToken, nil, nil)
}

func (s *supabase) post(ctx context.Context, path, bearer string, body, into any) error {
	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return &apiError{Status: res.StatusCode, Body: string(answer)}
	}
	if into == nil {
		return nil
	}
	return json.Unmarshal(answer, into)
}

// errExpired is an access token past its expiry, which a refresh replaces.
var errExpired = errors.New("access token expired")

// errNoKeys is failing to fetch the signing keys an access token is checked
// against: Supabase's fault, not the token's.
var errNoKeys = errors.New("signing keys unavailable")

// keyRefetch is how often an unknown kid may fetch the JWKS again, so
// tokens with made-up kids can't make every request fetch it.
const keyRefetch = time.Minute

// check verifies accessToken was signed by the project for a signed-in
// User, and has not expired by the app's clock. It returns their id.
func (s *supabase) check(ctx context.Context, accessToken string) (userID string, err error) {
	var c struct {
		jwt.RegisteredClaims
		Role string `json:"role"`
	}
	_, err = jwt.ParseWithClaims(accessToken, &c, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return s.key(ctx, kid)
	},
		jwt.WithValidMethods([]string{"ES256", "RS256"}),
		jwt.WithIssuer(s.url+"/auth/v1"),
		jwt.WithAudience("authenticated"),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(s.clock.Now),
	)
	if errors.Is(err, jwt.ErrTokenExpired) {
		return "", errExpired
	}
	if err != nil {
		return "", err
	}
	if c.Role != "authenticated" || c.Subject == "" {
		return "", fmt.Errorf("access token is for role %q, sub %q", c.Role, c.Subject)
	}
	return c.Subject, nil
}

// key is the project's public signing key kid, fetching the JWKS again
// when kid is new, at most once every keyRefetch: Supabase rotates its keys.
func (s *supabase) key(ctx context.Context, kid string) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if k, ok := s.keys[kid]; ok {
		return k, nil
	}
	if s.keys != nil && s.clock.Now().Sub(s.fetched) < keyRefetch {
		return nil, fmt.Errorf("no signing key %q in the JWKS", kid)
	}
	keys, err := s.fetchKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNoKeys, err)
	}
	s.keys, s.fetched = keys, s.clock.Now()
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("no signing key %q in the JWKS", kid)
}

func (s *supabase) fetchKeys(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url+"/auth/v1/.well-known/jwks.json", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", s.apiKey)
	res, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch the JWKS: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch the JWKS: %s", res.Status)
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(res.Body).Decode(&set); err != nil {
		return nil, fmt.Errorf("read the JWKS: %w", err)
	}
	keys := map[string]any{}
	for _, k := range set.Keys {
		pub, err := k.publicKey()
		if err != nil {
			return nil, err
		}
		keys[k.Kid] = pub
	}
	return keys, nil
}

// jwk is one public key of a JWKS: an ECC P-256 or an RSA key, the
// asymmetric kinds Supabase signs with.
type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (k jwk) publicKey() (any, error) {
	switch {
	case k.Kty == "EC" && k.Crv == "P-256":
		x, errX := base64.RawURLEncoding.DecodeString(k.X)
		y, errY := base64.RawURLEncoding.DecodeString(k.Y)
		if err := errors.Join(errX, errY); err != nil {
			return nil, fmt.Errorf("JWKS key %q: %w", k.Kid, err)
		}
		key, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
		if err != nil {
			return nil, fmt.Errorf("JWKS key %q: %w", k.Kid, err)
		}
		return key, nil
	case k.Kty == "RSA":
		n, errN := base64.RawURLEncoding.DecodeString(k.N)
		e, errE := base64.RawURLEncoding.DecodeString(k.E)
		if err := errors.Join(errN, errE); err != nil {
			return nil, fmt.Errorf("JWKS key %q: %w", k.Kid, err)
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}, nil
	default:
		return nil, fmt.Errorf("JWKS key %q is %s %s, want EC P-256 or RSA", k.Kid, k.Kty, k.Crv)
	}
}

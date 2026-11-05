package apptest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/simopzz/dandori/internal/clock"
)

// fakeAPIKey is the publishable key the app is given for FakeAuth, which
// FakeAuth checks it sends.
const fakeAPIKey = "sb_publishable_test"

// FakeAuth stands in for Supabase Auth, over HTTP as the app reaches it:
// it emails magic links only to invited Users, into a mailbox the test
// reads, redeems them for sessions it signs with its own key, and
// refreshes and revokes them. Its tokens expire an hour after they are
// issued by the app's clock, which a test moving the clock passes.
type FakeAuth struct {
	URL string
	// SiteURL is the app's address, which an invitation links to, as
	// Supabase's Site URL.
	SiteURL string

	clock clock.Clock
	key   *ecdsa.PrivateKey

	mu        sync.Mutex
	users     map[string]string      // the invited Users' ids by email
	confirmed map[string]bool        // the invited emails that have redeemed a link
	links     map[string]link        // the unredeemed links by token hash
	sessions  map[string]fakeSession // the session each live refresh token renews
	revoked   map[string]bool        // the signed-out session ids
	mail      map[string][]string
	traced    []string // the traceparent headers the app sent, which it shouldn't
}

type link struct{ email, kind string }

type fakeSession struct{ email, id string }

func startFakeAuth(t testing.TB, c clock.Clock) *FakeAuth {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f := &FakeAuth{
		clock: c, key: key,
		users: map[string]string{}, confirmed: map[string]bool{}, links: map[string]link{}, sessions: map[string]fakeSession{},
		revoked: map[string]bool{}, mail: map[string][]string{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/v1/.well-known/jwks.json", f.jwks)
	mux.HandleFunc("POST /auth/v1/otp", f.withKey(f.otp))
	mux.HandleFunc("POST /auth/v1/verify", f.withKey(f.verify))
	mux.HandleFunc("POST /auth/v1/token", f.withKey(f.token))
	mux.HandleFunc("POST /auth/v1/logout", f.withKey(f.logout))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tp := r.Header.Get("traceparent"); tp != "" {
			f.mu.Lock()
			f.traced = append(f.traced, tp)
			f.mu.Unlock()
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}

// Invite invites email, as from Supabase's dashboard: it can sign in from
// then on, and its mailbox gets an invitation linking to the app.
func (f *FakeAuth) Invite(email string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.users[email]; !ok {
		f.users[email] = randomUUID()
	}
	f.sendLink(email, f.SiteURL+"/auth/confirm", "invite")
}

// LastLink is the link in the last email sent to email.
func (f *FakeAuth) LastLink(t testing.TB, email string) string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	mail := f.mail[email]
	if len(mail) == 0 {
		t.Fatalf("no email was sent to %s", email)
	}
	return mail[len(mail)-1]
}

// Mail is every link emailed to email, oldest first.
func (f *FakeAuth) Mail(email string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.mail[email]...)
}

// Traceparents is every traceparent header the app sent, oldest first.
func (f *FakeAuth) Traceparents() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.traced...)
}

// sendLink emails a link to `to` with a new token hash, as Supabase's
// email templates for Dandori write it.
func (f *FakeAuth) sendLink(email, to, kind string) {
	hash := randomToken()
	f.links[hash] = link{email: email, kind: kind}
	f.mail[email] = append(f.mail[email], to+"?"+url.Values{"token_hash": {hash}, "type": {kind}}.Encode())
}

func (f *FakeAuth) withKey(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != fakeAPIKey {
			apiError(w, http.StatusUnauthorized, "no_api_key")
			return
		}
		h(w, r)
	}
}

func (f *FakeAuth) otp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email      string `json:"email"`
		CreateUser bool   `json:"create_user"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "bad_json")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.users[body.Email]; !ok {
		// Public sign-ups are off.
		apiError(w, http.StatusUnprocessableEntity, "otp_disabled")
		return
	}
	kind := "email"
	if !f.confirmed[body.Email] {
		// Supabase confirms an invited User's email at the first link they
		// redeem; until then it sends the signup confirmation.
		kind = "signup"
	}
	f.sendLink(body.Email, r.URL.Query().Get("redirect_to"), kind)
	writeJSON(w, map[string]any{})
}

func (f *FakeAuth) verify(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TokenHash string `json:"token_hash"`
		Type      string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiError(w, http.StatusBadRequest, "bad_json")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.links[body.TokenHash]
	if !ok || l.kind != body.Type {
		apiError(w, http.StatusForbidden, "otp_expired")
		return
	}
	delete(f.links, body.TokenHash)
	f.confirmed[l.email] = true
	writeJSON(w, f.newSession(fakeSession{email: l.email, id: randomUUID()}))
}

func (f *FakeAuth) token(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if r.URL.Query().Get("grant_type") != "refresh_token" || json.NewDecoder(r.Body).Decode(&body) != nil {
		apiError(w, http.StatusBadRequest, "bad_request")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.sessions[body.RefreshToken]
	if !ok || f.revoked[sess.id] {
		apiError(w, http.StatusBadRequest, "refresh_token_not_found")
		return
	}
	delete(f.sessions, body.RefreshToken)
	writeJSON(w, f.newSession(sess))
}

func (f *FakeAuth) logout(w http.ResponseWriter, r *http.Request) {
	var c struct {
		jwt.RegisteredClaims
		SessionID string `json:"session_id"`
	}
	bearer, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	_, err := jwt.ParseWithClaims(bearer, &c, func(*jwt.Token) (any, error) { return &f.key.PublicKey, nil },
		jwt.WithTimeFunc(f.clock.Now))
	if err != nil {
		apiError(w, http.StatusUnauthorized, "bad_jwt")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoked[c.SessionID] = true
	w.WriteHeader(http.StatusNoContent)
}

// newSession issues sess a new access token, for an hour, and a refresh
// token that renews it once.
func (f *FakeAuth) newSession(sess fakeSession) map[string]any {
	userID, email := f.users[sess.email], sess.email
	now := f.clock.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss":        f.URL + "/auth/v1",
		"aud":        "authenticated",
		"sub":        userID,
		"email":      email,
		"role":       "authenticated",
		"session_id": sess.id,
		"iat":        now.Unix(),
		"exp":        now.Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = "fake"
	access, err := tok.SignedString(f.key)
	if err != nil {
		panic(err)
	}
	refresh := randomToken()
	f.sessions[refresh] = sess
	return map[string]any{
		"access_token":  access,
		"token_type":    "bearer",
		"expires_in":    3600,
		"refresh_token": refresh,
		"user":          map[string]string{"id": userID, "email": email},
	}
}

func (f *FakeAuth) jwks(w http.ResponseWriter, r *http.Request) {
	pub, err := f.key.PublicKey.Bytes() // 0x04, then x and y
	if err != nil {
		panic(err)
	}
	writeJSON(w, map[string]any{"keys": []map[string]string{{
		"kid": "fake", "kty": "EC", "crv": "P-256", "alg": "ES256", "use": "sig",
		"x": base64.RawURLEncoding.EncodeToString(pub[1:33]),
		"y": base64.RawURLEncoding.EncodeToString(pub[33:]),
	}}})
}

func apiError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"code": status, "error_code": code, "msg": code})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func randomToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func randomUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

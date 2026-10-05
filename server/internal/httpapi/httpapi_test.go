package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync"
	"testing"
	"time"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/auth"
	"pantheon-spins/server/internal/games"
	"pantheon-spins/server/internal/games/gamestest"
	"pantheon-spins/server/internal/httpapi"
	"pantheon-spins/server/internal/play"
	"pantheon-spins/server/internal/ratelimit"
	"pantheon-spins/server/internal/testutil"
	"pantheon-spins/server/internal/wallet"
)

var env *testutil.Env

func TestMain(m *testing.M) { testutil.Main(m, &env) }

const origin = "http://app.test"

// captureMailer records sent emails.
type captureMailer struct {
	mu   sync.Mutex
	last string
}

func (m *captureMailer) Send(_ context.Context, _, _, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.last = body
	return nil
}

func (m *captureMailer) Last() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last
}

func newTestServer(t *testing.T) (*httptest.Server, *captureMailer) {
	t.Helper()
	if err := env.Redis.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mailer := &captureMailer{}
	sessions := auth.NewSessions(env.Redis, time.Hour)
	authSvc, err := auth.NewService(env.Pool, sessions, mailer, log, auth.Options{
		Argon2:      auth.Argon2Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32},
		SignupBonus: 10_000,
		AppBaseURL:  origin,
	})
	if err != nil {
		t.Fatal(err)
	}
	reg := games.NewRegistry()
	if err := reg.Register(&gamestest.Fake{ID: "fake", BonusEvery: 0}); err != nil {
		t.Fatal(err)
	}
	w := wallet.NewService(env.Pool)
	h := httpapi.NewHandler(httpapi.Deps{
		Log: log, Auth: authSvc, Wallet: w, Games: reg,
		Play:    play.NewService(env.Pool, w, reg),
		Limiter: ratelimit.New(env.Redis),
		Settings: httpapi.Settings{
			AppBaseURL: origin, AllowedOrigins: []string{origin},
			CSRFSecret: bytes.Repeat([]byte("k"), 32),
		},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, mailer
}

// client is a browser-like client: cookie jar plus the CSRF header.
type client struct {
	t    *testing.T
	base string
	http *http.Client
	csrf string
}

func newClient(t *testing.T, srv *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, base: srv.URL + "/api/v1", http: &http.Client{Jar: jar}}
	var tok api.CsrfToken
	c.do(http.MethodGet, "/auth/csrf", nil, http.StatusOK, &tok)
	c.csrf = tok.Token
	return c
}

func (c *client) request(method, path string, body any, header http.Header) *http.Response {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	return resp
}

// do sends a request and checks the status, decoding the body into out if set.
func (c *client) do(method, path string, body any, wantStatus int, out any) {
	c.t.Helper()
	resp := c.request(method, path, body, nil)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		c.t.Fatalf("%s %s: status %d, want %d: %s", method, path, resp.StatusCode, wantStatus, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("%s %s: decode: %v: %s", method, path, err, raw)
		}
	}
}

func TestAuthFlow(t *testing.T) {
	testutil.SkipIfShort(t)
	srv, mailer := newTestServer(t)
	c := newClient(t, srv)
	creds := map[string]string{"email": "Odin@Asgard.test", "password": "allfather-123", "username": "odin"}

	// CSRF: missing header, wrong header and foreign origin are rejected.
	tok := c.csrf
	c.csrf = ""
	c.do(http.MethodPost, "/auth/register", creds, http.StatusForbidden, nil)
	c.csrf = "forged"
	c.do(http.MethodPost, "/auth/register", creds, http.StatusForbidden, nil)
	c.csrf = tok
	resp := c.request(http.MethodPost, "/auth/register", creds, http.Header{"Origin": {"https://evil.test"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin: status %d, want 403", resp.StatusCode)
	}

	var me api.Me
	c.do(http.MethodPost, "/auth/register", creds, http.StatusCreated, &me)
	if me.Email != "odin@asgard.test" || me.Balance != 10_000 || !me.HasPassword || me.Avatar != api.Raven {
		t.Fatalf("unexpected profile %+v", me)
	}
	c.do(http.MethodGet, "/me", nil, http.StatusOK, &me)

	// Duplicate email and username (case-insensitive).
	other := newClient(t, srv)
	other.do(http.MethodPost, "/auth/register", creds, http.StatusConflict, nil)
	other.do(http.MethodPost, "/auth/register",
		map[string]string{"email": "x@y.test", "password": "allfather-123", "username": "ODIN"}, http.StatusConflict, nil)
	other.do(http.MethodPost, "/auth/register",
		map[string]string{"email": "x@y.test", "password": "short", "username": "thor"}, http.StatusBadRequest, nil)
	other.do(http.MethodGet, "/me", nil, http.StatusUnauthorized, nil)

	// Profile update.
	c.do(http.MethodPatch, "/me", map[string]string{"avatar": "wolf", "username": "Allfather"}, http.StatusOK, &me)
	if me.Avatar != api.Wolf || me.Username != "Allfather" {
		t.Fatalf("profile not updated: %+v", me)
	}
	c.do(http.MethodPatch, "/me", map[string]string{"avatar": "dragon"}, http.StatusBadRequest, nil)

	// Logout ends the session.
	c.do(http.MethodPost, "/auth/logout", nil, http.StatusNoContent, nil)
	c.do(http.MethodGet, "/me", nil, http.StatusUnauthorized, nil)

	// Login.
	c.do(http.MethodPost, "/auth/login", map[string]string{"email": creds["email"], "password": "wrong-password"}, http.StatusUnauthorized, nil)
	c.do(http.MethodPost, "/auth/login", map[string]string{"email": "nobody@asgard.test", "password": "whatever-123"}, http.StatusUnauthorized, nil)
	c.do(http.MethodPost, "/auth/login", map[string]string{"email": creds["email"], "password": creds["password"]}, http.StatusOK, &me)

	// Password reset: unknown emails get the same response.
	c.do(http.MethodPost, "/auth/password-reset/request", map[string]string{"email": "nobody@asgard.test"}, http.StatusNoContent, nil)
	if mailer.Last() != "" {
		t.Fatal("email sent for an unknown account")
	}
	c.do(http.MethodPost, "/auth/password-reset/request", map[string]string{"email": creds["email"]}, http.StatusNoContent, nil)
	// The email is sent in the background.
	for deadline := time.Now().Add(5 * time.Second); mailer.Last() == "" && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	m := regexp.MustCompile(`token=([A-Za-z0-9_-]+)`).FindStringSubmatch(mailer.Last())
	if m == nil {
		t.Fatalf("no reset link in email: %q", mailer.Last())
	}
	resetTok, _ := url.QueryUnescape(m[1])

	c.do(http.MethodPost, "/auth/password-reset/confirm", map[string]string{"token": "bogus", "password": "new-password-1"}, http.StatusBadRequest, nil)
	c.do(http.MethodPost, "/auth/password-reset/confirm", map[string]string{"token": resetTok, "password": "new-password-1"}, http.StatusNoContent, nil)
	c.do(http.MethodPost, "/auth/password-reset/confirm", map[string]string{"token": resetTok, "password": "new-password-2"}, http.StatusBadRequest, nil)

	// The reset revoked the existing session; the new password works.
	c.do(http.MethodGet, "/me", nil, http.StatusUnauthorized, nil)
	c.do(http.MethodPost, "/auth/login", map[string]string{"email": creds["email"], "password": creds["password"]}, http.StatusUnauthorized, nil)
	c.do(http.MethodPost, "/auth/login", map[string]string{"email": creds["email"], "password": "new-password-1"}, http.StatusOK, nil)
}

func TestLoginRateLimit(t *testing.T) {
	testutil.SkipIfShort(t)
	srv, _ := newTestServer(t)
	c := newClient(t, srv)
	body := map[string]string{"email": "nobody@x.test", "password": "whatever-123"}
	for range 10 {
		c.do(http.MethodPost, "/auth/login", body, http.StatusUnauthorized, nil)
	}
	resp := c.request(http.MethodPost, "/auth/login", body, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("11th login: status %d, Retry-After %q; want 429 with Retry-After", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
}

func TestSpinEndpoint(t *testing.T) {
	testutil.SkipIfShort(t)
	srv, _ := newTestServer(t)
	c := newClient(t, srv)
	c.do(http.MethodPost, "/games/fake/spin", map[string]int{"bet": 10}, http.StatusUnauthorized, nil)

	c.do(http.MethodPost, "/auth/register",
		map[string]string{"email": "thor@asgard.test", "password": "mjolnir-1234", "username": "thor"}, http.StatusCreated, nil)

	var gamesList []api.GameInfo
	c.do(http.MethodGet, "/games", nil, http.StatusOK, &gamesList)
	if len(gamesList) != 1 || gamesList[0].Id != "fake" {
		t.Fatalf("games: %+v", gamesList)
	}

	var res api.SpinResult
	c.do(http.MethodPost, "/games/fake/spin", map[string]int{"bet": 10}, http.StatusOK, &res)
	if res.Bet != 10 || res.Balance != 10_000-10 || len(res.Outcome.Steps) != 1 || res.WinTier != api.None {
		t.Fatalf("unexpected spin result %+v", res)
	}

	c.do(http.MethodPost, "/games/fake/spin", map[string]int{"bet": 7}, http.StatusBadRequest, nil)
	c.do(http.MethodPost, "/games/fake/spin", map[string]any{"bet": 10, "win": 1_000_000}, http.StatusBadRequest, nil)
	c.do(http.MethodPost, "/games/nope/spin", map[string]int{"bet": 10}, http.StatusNotFound, nil)
	c.do(http.MethodPost, "/games/fake/spin", map[string]int{"bet": -10}, http.StatusBadRequest, nil)
	c.do(http.MethodGet, "/games/fake/bonus", nil, http.StatusNoContent, nil)

	var hist api.SpinHistoryPage
	c.do(http.MethodGet, "/spins?limit=5", nil, http.StatusOK, &hist)
	if len(hist.Items) != 1 || hist.Items[0].SpinId != res.SpinId {
		t.Fatalf("history: %+v", hist)
	}
	var txs api.TransactionPage
	c.do(http.MethodGet, "/wallet/transactions", nil, http.StatusOK, &txs)
	if len(txs.Items) != 2 || txs.Items[0].Kind != api.SpinBet || txs.Items[1].Kind != api.SignupBonus {
		t.Fatalf("transactions: %+v", txs)
	}
}

// Package httpapi implements the OpenAPI-generated server interface.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/auth"
	"pantheon-spins/server/internal/bonus"
	"pantheon-spins/server/internal/db"
	"pantheon-spins/server/internal/games"
	"pantheon-spins/server/internal/play"
	"pantheon-spins/server/internal/ratelimit"
	"pantheon-spins/server/internal/wallet"
)

// Rate limits. Spin limits allow turbo autoplay with headroom.
var (
	rlLogin        = ratelimit.Rule{Name: "login", Limit: 10, Window: time.Minute}
	rlRegister     = ratelimit.Rule{Name: "register", Limit: 10, Window: time.Hour}
	rlResetIP      = ratelimit.Rule{Name: "reset-ip", Limit: 10, Window: time.Hour}
	rlResetEmail   = ratelimit.Rule{Name: "reset-email", Limit: 3, Window: time.Hour}
	rlResetConfirm = ratelimit.Rule{Name: "reset-confirm", Limit: 10, Window: time.Hour}
	rlSpin         = ratelimit.Rule{Name: "spin", Limit: 10, Window: time.Second}
	rlBonus        = ratelimit.Rule{Name: "bonus", Limit: 10, Window: time.Second}
)

type Deps struct {
	Log      *slog.Logger
	Auth     *auth.Service
	Google   *auth.Google // nil when Google sign-in is disabled
	Wallet   *wallet.Service
	Play     *play.Service
	Games    *games.Registry
	Limiter  *ratelimit.Limiter
	Health   func(context.Context) error
	Settings Settings
}

type Settings struct {
	AppBaseURL     string
	AllowedOrigins []string
	CookieSecure   bool
	CSRFSecret     []byte
	TrustProxy     bool
}

type Server struct {
	Deps
	csrf *csrfProtector
}

var _ api.ServerInterface = (*Server)(nil)

// NewHandler builds the full HTTP handler: middleware plus the generated router.
func NewHandler(d Deps) http.Handler {
	s := &Server{
		Deps: d,
		csrf: &csrfProtector{secret: d.Settings.CSRFSecret, secure: d.Settings.CookieSecure, allowedOrigins: d.Settings.AllowedOrigins},
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	if d.Settings.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(requestLogger(d.Log), recoverer(d.Log), securityHeaders)
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.csrf.middleware, sessionMiddleware(d.Auth.Sessions(), d.Log))
		s.devRoutes(r)
		api.HandlerWithOptions(s, api.ChiServerOptions{
			BaseRouter: r,
			ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
				writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			},
		})
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
	})
	return r
}

// ---------- helpers ----------

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := userID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Please sign in.")
	}
	return id, ok
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// allow applies a rate limit. On rejection it writes 429 and returns false.
// If Redis is unavailable the request is allowed and the error logged.
func (s *Server) allow(w http.ResponseWriter, r *http.Request, rule ratelimit.Rule, key string) bool {
	ok, retry, err := s.Limiter.Allow(r.Context(), rule, key)
	if err != nil {
		s.Log.ErrorContext(r.Context(), "rate limiter unavailable", "err", err, "rule", rule.Name)
		return true
	}
	if !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests. Slow down a little.")
	}
	return ok
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.ErrorContext(r.Context(), "request failed", "err", err, "path", r.URL.Path)
	writeError(w, http.StatusInternalServerError, "internal", "Something went wrong.")
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/",
		HttpOnly: true, Secure: s.Settings.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(s.Auth.Sessions().TTL().Seconds()),
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.Settings.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u db.User, status int) {
	token, err := s.Auth.Sessions().Create(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.setSessionCookie(w, token)
	s.writeMe(w, r, u, status)
}

func (s *Server) writeMe(w http.ResponseWriter, r *http.Request, u db.User, status int) {
	bal, err := s.Wallet.Balance(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, status, api.Me{
		Id:          u.ID,
		Email:       u.Email,
		Username:    u.Username,
		Avatar:      api.AvatarId(u.Avatar),
		Balance:     bal,
		HasPassword: u.PasswordHash != nil,
		CreatedAt:   u.CreatedAt,
	})
}

// authError maps auth-service errors to responses.
func (s *Server) authError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *auth.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, "invalid_input", ve.Msg)
	case errors.Is(err, auth.ErrEmailTaken):
		writeError(w, http.StatusConflict, "email_taken", "An account with this email already exists.")
	case errors.Is(err, auth.ErrUsernameTaken):
		writeError(w, http.StatusConflict, "username_taken", "That username is taken.")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Incorrect email or password.")
	case errors.Is(err, auth.ErrInvalidResetToken):
		writeError(w, http.StatusBadRequest, "invalid_token", "This reset link is invalid or has expired.")
	default:
		s.internalError(w, r, err)
	}
}

func pageLimit(limit int) int {
	if limit <= 0 {
		return 25
	}
	return min(limit, 100)
}

// ---------- system ----------

func (s *Server) GetHealth(w http.ResponseWriter, r *http.Request) {
	if s.Health != nil {
		if err := s.Health(r.Context()); err != nil {
			s.Log.ErrorContext(r.Context(), "health check failed", "err", err)
			writeError(w, http.StatusServiceUnavailable, "unhealthy", "Service unavailable.")
			return
		}
	}
	writeJSON(w, http.StatusOK, api.Health{Status: api.Ok})
}

// ---------- auth ----------

func (s *Server) GetCsrfToken(w http.ResponseWriter, r *http.Request) {
	tok, _ := r.Context().Value(ctxCSRFToken).(string)
	writeJSON(w, http.StatusOK, api.CsrfToken{Token: tok})
}

func (s *Server) GetAuthProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, api.AuthProviders{Password: true, Google: s.Google != nil})
}

func (s *Server) Register(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r, rlRegister, clientIP(r)) {
		return
	}
	var req api.RegisterRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u, err := s.Auth.Register(r.Context(), string(req.Email), req.Password, req.Username)
	if err != nil {
		s.authError(w, r, err)
		return
	}
	s.startSession(w, r, u, http.StatusCreated)
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r, rlLogin, clientIP(r)) {
		return
	}
	var req api.LoginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u, err := s.Auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		s.authError(w, r, err)
		return
	}
	s.startSession(w, r, u, http.StatusOK)
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	if ck, err := r.Cookie(sessionCookie); err == nil {
		if err := s.Auth.Sessions().Delete(r.Context(), ck.Value); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	s.clearCookie(w, sessionCookie)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r, rlResetIP, clientIP(r)) {
		return
	}
	var req api.PasswordResetRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.allow(w, r, rlResetEmail, req.Email) {
		return
	}
	if err := s.Auth.RequestPasswordReset(r.Context(), req.Email); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ConfirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r, rlResetConfirm, clientIP(r)) {
		return
	}
	var req api.PasswordResetConfirm
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.Auth.ConfirmPasswordReset(r.Context(), req.Token, req.Password); err != nil {
		s.authError(w, r, err)
		return
	}
	s.clearCookie(w, sessionCookie)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) GoogleStart(w http.ResponseWriter, r *http.Request) {
	if s.Google == nil {
		writeError(w, http.StatusNotFound, "not_found", "Google sign-in is not enabled.")
		return
	}
	url, state, err := s.Google.Start(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	// Bind the state to this browser so a leaked callback URL cannot be replayed elsewhere.
	http.SetCookie(w, &http.Cookie{
		Name: oauthCookie, Value: state, Path: "/api/v1/auth/google", MaxAge: 600,
		HttpOnly: true, Secure: s.Settings.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Server) GoogleCallback(w http.ResponseWriter, r *http.Request, params api.GoogleCallbackParams) {
	fail := func(reason string, err error) {
		if err != nil {
			s.Log.WarnContext(r.Context(), "google sign-in failed", "reason", reason, "err", err)
		}
		http.Redirect(w, r, s.Settings.AppBaseURL+"/login?error="+reason, http.StatusFound)
	}
	if s.Google == nil {
		fail("google_disabled", nil)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: oauthCookie, Value: "", Path: "/api/v1/auth/google", MaxAge: -1, HttpOnly: true, Secure: s.Settings.CookieSecure})
	ck, err := r.Cookie(oauthCookie)
	if params.Error != "" || params.Code == "" || err != nil || ck.Value == "" || ck.Value != params.State {
		fail("google_cancelled", nil)
		return
	}
	ident, err := s.Google.Finish(r.Context(), params.State, params.Code)
	if err != nil {
		fail("google_failed", err)
		return
	}
	u, err := s.Auth.LoginWithGoogle(r.Context(), ident)
	if err != nil {
		var ve *auth.ValidationError
		if errors.As(err, &ve) {
			fail("google_unverified", err)
		} else {
			fail("google_failed", err)
		}
		return
	}
	token, err := s.Auth.Sessions().Create(r.Context(), u.ID)
	if err != nil {
		fail("google_failed", err)
		return
	}
	s.setSessionCookie(w, token)
	http.Redirect(w, r, s.Settings.AppBaseURL+"/", http.StatusFound)
}

// ---------- profile ----------

func (s *Server) GetMe(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	u, err := s.Auth.User(r.Context(), id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.writeMe(w, r, u, http.StatusOK)
}

func (s *Server) UpdateMe(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req api.UpdateProfileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u, err := s.Auth.UpdateProfile(r.Context(), id, req.Username, string(req.Avatar))
	if err != nil {
		s.authError(w, r, err)
		return
	}
	s.writeMe(w, r, u, http.StatusOK)
}

// ---------- wallet ----------

func (s *Server) GetWallet(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	bal, err := s.Wallet.Balance(r.Context(), id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, api.Wallet{Balance: bal})
}

func (s *Server) ListTransactions(w http.ResponseWriter, r *http.Request, params api.ListTransactionsParams) {
	id, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit := pageLimit(params.Limit)
	var before *int64
	if params.Before > 0 {
		before = &params.Before
	}
	rows, err := s.Wallet.Transactions(r.Context(), id, before, limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	page := api.TransactionPage{Items: make([]api.Transaction, len(rows))}
	for i, t := range rows {
		item := api.Transaction{
			Id: t.ID, Kind: api.TransactionKind(t.Kind), Amount: t.Amount,
			BalanceAfter: t.BalanceAfter, CreatedAt: t.CreatedAt,
		}
		if t.GameID != nil {
			item.GameId = *t.GameID
		}
		page.Items[i] = item
	}
	if len(rows) == limit {
		page.NextBefore = rows[len(rows)-1].ID
	}
	writeJSON(w, http.StatusOK, page)
}

// ---------- games ----------

func (s *Server) ListGames(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Games.List())
}

// playError maps spin and bonus errors to responses.
func (s *Server) playError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, play.ErrUnknownGame):
		writeError(w, http.StatusNotFound, "unknown_game", "No such game.")
	case errors.Is(err, play.ErrInvalidBet):
		writeError(w, http.StatusBadRequest, "invalid_bet", "That bet size is not available for this game.")
	case errors.Is(err, wallet.ErrInsufficientFunds):
		writeError(w, http.StatusConflict, "insufficient_funds", "Not enough Coins for this bet.")
	case errors.Is(err, play.ErrBonusInProgress):
		writeError(w, http.StatusConflict, "bonus_in_progress", "Finish your bonus on this game first.")
	case errors.Is(err, bonus.ErrNoActiveBonus):
		writeError(w, http.StatusNotFound, "no_active_bonus", "There is no bonus in progress on this game.")
	case errors.Is(err, play.ErrStaleStep):
		writeError(w, http.StatusConflict, "stale_step", "This bonus has moved on. Reload it to continue.")
	case errors.Is(err, games.ErrInvalidAction):
		writeError(w, http.StatusBadRequest, "invalid_action", "That action is not available right now.")
	default:
		s.internalError(w, r, err)
	}
}

func (s *Server) Spin(w http.ResponseWriter, r *http.Request, gameID api.GameId) {
	id, ok := s.requireUser(w, r)
	if !ok || !s.allow(w, r, rlSpin, id.String()) {
		return
	}
	var req api.SpinRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := s.Play.Spin(r.Context(), id, gameID, req.Bet)
	if err != nil {
		s.playError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) GetActiveBonus(w http.ResponseWriter, r *http.Request, gameID api.GameId) {
	id, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	st, err := s.Play.ActiveBonus(r.Context(), id, gameID)
	if errors.Is(err, bonus.ErrNoActiveBonus) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		s.playError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) BonusAction(w http.ResponseWriter, r *http.Request, gameID api.GameId) {
	id, ok := s.requireUser(w, r)
	if !ok || !s.allow(w, r, rlBonus, id.String()) {
		return
	}
	var req api.BonusActionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := s.Play.BonusAction(r.Context(), id, gameID, req.Step, games.Action{Name: req.Action, Choice: req.Choice})
	if err != nil {
		s.playError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ---------- history ----------

func (s *Server) ListSpins(w http.ResponseWriter, r *http.Request, params api.ListSpinsParams) {
	id, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit := pageLimit(params.Limit)
	var before *uuid.UUID
	if params.Before != uuid.Nil {
		before = &params.Before
	}
	items, err := s.Play.History(r.Context(), id, before, limit)
	if err != nil {
		s.internalError(w, r, fmt.Errorf("list spins: %w", err))
		return
	}
	page := api.SpinHistoryPage{Items: items}
	if len(items) == limit {
		page.NextBefore = items[len(items)-1].SpinId
	}
	writeJSON(w, http.StatusOK, page)
}

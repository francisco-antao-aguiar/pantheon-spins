package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"pantheon-spins/server/internal/auth"
)

const (
	sessionCookie = "ps_session"
	csrfCookie    = "ps_csrf"
	csrfHeader    = "X-CSRF-Token"
	oauthCookie   = "ps_oauth_state"
)

type ctxKey int

const (
	ctxUserID ctxKey = iota
	ctxCSRFToken
)

// userID returns the signed-in user, if any.
func userID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ctxUserID).(uuid.UUID)
	return id, ok
}

// csrfProtector implements the signed double-submit cookie pattern: the server
// sets a random, HMAC-signed token in a readable cookie and state-changing
// requests must echo it in the X-CSRF-Token header. A forged cross-site request
// cannot read the cookie, and the signature stops cookie injection. It also
// rejects requests whose Origin is not an allowed origin.
type csrfProtector struct {
	secret         []byte
	secure         bool
	allowedOrigins []string
}

func (c *csrfProtector) sign(nonce []byte) string {
	m := hmac.New(sha256.New, c.secret)
	m.Write(nonce)
	enc := base64.RawURLEncoding
	return enc.EncodeToString(nonce) + "." + enc.EncodeToString(m.Sum(nil))
}

func (c *csrfProtector) newToken() string {
	nonce := make([]byte, 32)
	_, _ = rand.Read(nonce)
	return c.sign(nonce)
}

func (c *csrfProtector) valid(tok string) bool {
	nonceB64, _, ok := strings.Cut(tok, ".")
	if !ok {
		return false
	}
	nonce, err := base64.RawURLEncoding.DecodeString(nonceB64)
	if err != nil || len(nonce) != 32 {
		return false
	}
	return hmac.Equal([]byte(c.sign(nonce)), []byte(tok))
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func (c *csrfProtector) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cookieTok string
		if ck, err := r.Cookie(csrfCookie); err == nil && c.valid(ck.Value) {
			cookieTok = ck.Value
		}
		token := cookieTok
		if token == "" {
			token = c.newToken()
			http.SetCookie(w, &http.Cookie{
				Name: csrfCookie, Value: token, Path: "/",
				Secure: c.secure, SameSite: http.SameSiteLaxMode,
				// Readable by JS on purpose: the client echoes it in a header.
				HttpOnly: false,
				MaxAge:   int((365 * 24 * time.Hour).Seconds()),
			})
		}

		if !isSafeMethod(r.Method) {
			if origin := r.Header.Get("Origin"); origin != "" && !slices.Contains(c.allowedOrigins, origin) {
				writeError(w, http.StatusForbidden, "csrf", "Cross-origin request rejected.")
				return
			}
			hdr := r.Header.Get(csrfHeader)
			if cookieTok == "" || !hmac.Equal([]byte(hdr), []byte(cookieTok)) {
				writeError(w, http.StatusForbidden, "csrf", "Missing or invalid CSRF token. Reload the page and try again.")
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxCSRFToken, token)))
	})
}

// sessionMiddleware resolves the session cookie to a user ID, if present.
func sessionMiddleware(sessions *auth.Sessions, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ck, err := r.Cookie(sessionCookie)
			if err != nil || ck.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			id, err := sessions.Lookup(r.Context(), ck.Value)
			if err != nil {
				if err != auth.ErrNoSession {
					log.ErrorContext(r.Context(), "session lookup", "err", err)
				}
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUserID, id)))
		})
	}
}

// requestLogger logs one structured line per request.
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
				"ip", r.RemoteAddr,
			}
			if id, ok := userID(r.Context()); ok {
				attrs = append(attrs, "user_id", id)
			}
			level := slog.LevelInfo
			if ww.Status() >= 500 {
				level = slog.LevelError
			}
			log.Log(r.Context(), level, "http request", attrs...)
		})
	}
}

// recoverer turns panics into 500 responses and logs the stack.
func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler {
						panic(v)
					}
					log.ErrorContext(r.Context(), "panic", "value", v, "stack", string(debug.Stack()))
					writeError(w, http.StatusInternalServerError, "internal", "Something went wrong.")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

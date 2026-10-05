// Package auth handles accounts: registration, login, server-side sessions,
// password reset, Google sign-in and profile updates.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/db"
	"pantheon-spins/server/internal/wallet"
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrUsernameTaken      = errors.New("username taken")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidResetToken  = errors.New("invalid or expired reset token")
)

// ValidationError is an input error whose message is safe to show the user.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

const (
	minPasswordLen = 10
	maxPasswordLen = 128
	resetTokenTTL  = time.Hour
	mailTimeout    = 30 * time.Second
	defaultAvatar  = string(api.Raven)
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,20}$`)

type Service struct {
	pool        *pgxpool.Pool
	sessions    *Sessions
	mailer      Mailer
	argon       Argon2Params
	signupBonus int64
	appBaseURL  string
	log         *slog.Logger
	// dummyHash is verified when a login email is unknown, so response time
	// does not reveal whether an account exists.
	dummyHash string
}

type Options struct {
	Argon2      Argon2Params
	SignupBonus int64
	AppBaseURL  string
}

func NewService(pool *pgxpool.Pool, sessions *Sessions, mailer Mailer, log *slog.Logger, opts Options) (*Service, error) {
	dummy, err := HashPassword("not-a-real-password", opts.Argon2)
	if err != nil {
		return nil, err
	}
	return &Service{
		pool:        pool,
		sessions:    sessions,
		mailer:      mailer,
		argon:       opts.Argon2,
		signupBonus: opts.SignupBonus,
		appBaseURL:  opts.AppBaseURL,
		log:         log,
		dummyHash:   dummy,
	}, nil
}

func (s *Service) Sessions() *Sessions { return s.sessions }

func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 {
		return "", &ValidationError{"Enter a valid email address."}
	}
	return email, nil
}

func validatePassword(pw string) error {
	n := utf8.RuneCountInString(pw)
	if n < minPasswordLen || n > maxPasswordLen {
		return &ValidationError{fmt.Sprintf("Password must be %d–%d characters.", minPasswordLen, maxPasswordLen)}
	}
	return nil
}

func validateUsername(u string) error {
	if !usernameRe.MatchString(u) {
		return &ValidationError{"Username must be 3–20 letters, digits or underscores."}
	}
	return nil
}

// Register creates an account with the signup Coin grant.
func (s *Service) Register(ctx context.Context, email, password, username string) (db.User, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return db.User{}, err
	}
	if err := validateUsername(username); err != nil {
		return db.User{}, err
	}
	if err := validatePassword(password); err != nil {
		return db.User{}, err
	}
	hash, err := HashPassword(password, s.argon)
	if err != nil {
		return db.User{}, err
	}
	return s.createUser(ctx, db.CreateUserParams{
		Email: email, Username: username, PasswordHash: &hash, Avatar: defaultAvatar,
	})
}

func (s *Service) createUser(ctx context.Context, p db.CreateUserParams) (db.User, error) {
	var u db.User
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		if u, err = q.CreateUser(ctx, p); err != nil {
			return mapUniqueViolation(err)
		}
		return wallet.Open(ctx, q, u.ID, s.signupBonus)
	})
	return u, err
}

func mapUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "users_email_key":
			return ErrEmailTaken
		case "users_username_key":
			return ErrUsernameTaken
		}
	}
	return err
}

// Login checks credentials. It never reveals which of email or password was wrong.
func (s *Service) Login(ctx context.Context, email, password string) (db.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := db.New(s.pool).GetUserByEmail(ctx, email)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, err
	}
	if err != nil || u.PasswordHash == nil {
		_, _ = VerifyPassword(password, s.dummyHash)
		return db.User{}, ErrInvalidCredentials
	}
	ok, err := VerifyPassword(password, *u.PasswordHash)
	if err != nil {
		return db.User{}, err
	}
	if !ok {
		return db.User{}, ErrInvalidCredentials
	}
	return u, nil
}

func hashResetToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// RequestPasswordReset emails a reset link if the account exists. It returns
// nil either way so callers cannot probe for accounts.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	q := db.New(s.pool)
	u, err := q.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	token := NewToken()
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		q := db.New(tx)
		if err := q.InvalidatePasswordResetTokens(ctx, u.ID); err != nil {
			return err
		}
		return q.CreatePasswordResetToken(ctx, db.CreatePasswordResetTokenParams{
			TokenHash: hashResetToken(token),
			UserID:    u.ID,
			ExpiresAt: time.Now().Add(resetTokenTTL),
		})
	})
	if err != nil {
		return fmt.Errorf("auth: create reset token: %w", err)
	}
	link := s.appBaseURL + "/reset-password?token=" + token
	body := fmt.Sprintf("Hi %s,\n\nReset your Pantheon Spins password here (valid for 1 hour):\n%s\n\nIf you did not ask for this, ignore this email.", u.Username, link)
	// Send in the background: waiting on the mail server only for existing
	// accounts would reveal through response time which emails are registered.
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mailTimeout)
	go func() {
		defer cancel()
		if err := s.mailer.Send(sendCtx, u.Email, "Reset your Pantheon Spins password", body); err != nil {
			s.log.ErrorContext(sendCtx, "send reset email", "err", err, "user_id", u.ID)
		}
	}()
	return nil
}

// ConfirmPasswordReset sets a new password and signs the user out everywhere.
func (s *Service) ConfirmPasswordReset(ctx context.Context, token, password string) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := HashPassword(password, s.argon)
	if err != nil {
		return err
	}
	var userID uuid.UUID
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		q := db.New(tx)
		id, err := q.ConsumePasswordResetToken(ctx, hashResetToken(token))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidResetToken
		}
		if err != nil {
			return err
		}
		userID = id
		return q.SetPasswordHash(ctx, db.SetPasswordHashParams{ID: id, PasswordHash: &hash})
	})
	if err != nil {
		return err
	}
	return s.sessions.DeleteAll(ctx, userID)
}

// User loads a user by ID.
func (s *Service) User(ctx context.Context, id uuid.UUID) (db.User, error) {
	return db.New(s.pool).GetUserByID(ctx, id)
}

// UpdateProfile changes username and/or avatar. Empty values are left unchanged.
func (s *Service) UpdateProfile(ctx context.Context, id uuid.UUID, username, avatar string) (db.User, error) {
	p := db.UpdateProfileParams{ID: id}
	if username != "" {
		if err := validateUsername(username); err != nil {
			return db.User{}, err
		}
		p.Username = &username
	}
	if avatar != "" {
		if !api.AvatarId(avatar).Valid() {
			return db.User{}, &ValidationError{"Unknown avatar."}
		}
		p.Avatar = &avatar
	}
	u, err := db.New(s.pool).UpdateProfile(ctx, p)
	return u, mapUniqueViolation(err)
}

// GoogleIdentity is the verified profile returned by Google.
type GoogleIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// LoginWithGoogle signs in the account linked to the Google identity. If none
// exists, it links an account with the same verified email, or creates one.
func (s *Service) LoginWithGoogle(ctx context.Context, g GoogleIdentity) (db.User, error) {
	q := db.New(s.pool)
	u, err := q.GetUserByGoogleSub(ctx, &g.Subject)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, err
	}
	if !g.EmailVerified {
		return db.User{}, &ValidationError{"Your Google email address is not verified."}
	}
	email, err := normalizeEmail(g.Email)
	if err != nil {
		return db.User{}, err
	}
	u, err = q.GetUserByEmail(ctx, email)
	if err == nil {
		if u.GoogleSub != nil {
			return db.User{}, &ValidationError{"This email is linked to a different Google account."}
		}
		if err := q.LinkGoogleAccount(ctx, db.LinkGoogleAccountParams{ID: u.ID, GoogleSub: &g.Subject}); err != nil {
			return db.User{}, err
		}
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, err
	}

	base := usernameBase(g.Name, email)
	for range 5 {
		u, err = s.createUser(ctx, db.CreateUserParams{
			Email: email, Username: base + "_" + randomDigits(4), GoogleSub: &g.Subject, Avatar: defaultAvatar,
		})
		if !errors.Is(err, ErrUsernameTaken) {
			return u, err
		}
	}
	return db.User{}, err
}

var nonUsernameChars = regexp.MustCompile(`[^A-Za-z0-9_]+`)

// usernameBase derives up to 15 username characters from a display name or email.
func usernameBase(name, email string) string {
	b := nonUsernameChars.ReplaceAllString(name, "")
	if len(b) < 3 {
		b = nonUsernameChars.ReplaceAllString(strings.SplitN(email, "@", 2)[0], "")
	}
	if len(b) < 3 {
		b = "player"
	}
	if len(b) > 15 {
		b = b[:15]
	}
	return b
}

func randomDigits(n int) string {
	var sb strings.Builder
	for range n {
		d, _ := rand.Int(rand.Reader, big.NewInt(10))
		sb.WriteByte(byte('0' + d.Int64()))
	}
	return sb.String()
}

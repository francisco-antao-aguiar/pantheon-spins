package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	googleUserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
	oauthStateTTL     = 10 * time.Minute
)

var ErrOAuthState = errors.New("invalid or expired OAuth state")

// Google runs the OAuth 2.0 authorization-code flow with PKCE.
type Google struct {
	cfg *oauth2.Config
	rdb *redis.Client
}

func NewGoogle(clientID, clientSecret, redirectURL string, rdb *redis.Client) *Google {
	return &Google{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     google.Endpoint,
			Scopes:       []string{"openid", "email", "profile"},
		},
		rdb: rdb,
	}
}

func oauthKey(state string) string { return "oauth:" + state }

// Start returns the Google consent URL and the state value, which the caller
// must also bind to the browser (cookie) and check on callback.
func (g *Google) Start(ctx context.Context) (authURL, state string, err error) {
	state = NewToken()
	verifier := oauth2.GenerateVerifier()
	if err := g.rdb.Set(ctx, oauthKey(state), verifier, oauthStateTTL).Err(); err != nil {
		return "", "", fmt.Errorf("auth: store oauth state: %w", err)
	}
	return g.cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), state, nil
}

// Finish exchanges the code and returns the verified Google identity. The
// state is single use.
func (g *Google) Finish(ctx context.Context, state, code string) (GoogleIdentity, error) {
	verifier, err := g.rdb.GetDel(ctx, oauthKey(state)).Result()
	if errors.Is(err, redis.Nil) {
		return GoogleIdentity{}, ErrOAuthState
	}
	if err != nil {
		return GoogleIdentity{}, err
	}
	tok, err := g.cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("auth: google exchange: %w", err)
	}
	resp, err := g.cfg.Client(ctx, tok).Get(googleUserInfoURL)
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("auth: google userinfo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return GoogleIdentity{}, fmt.Errorf("auth: google userinfo: status %d", resp.StatusCode)
	}
	var info struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&info); err != nil {
		return GoogleIdentity{}, fmt.Errorf("auth: google userinfo: %w", err)
	}
	if info.Sub == "" {
		return GoogleIdentity{}, errors.New("auth: google userinfo: missing sub")
	}
	return GoogleIdentity{Subject: info.Sub, Email: info.Email, EmailVerified: info.EmailVerified, Name: info.Name}, nil
}

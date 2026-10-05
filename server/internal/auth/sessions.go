package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ErrNoSession means the token is unknown or expired.
var ErrNoSession = errors.New("no session")

// Sessions stores server-side sessions in Redis. The cookie carries a random
// token; Redis keys use its SHA-256, so a Redis dump does not leak live tokens.
type Sessions struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewSessions(rdb *redis.Client, ttl time.Duration) *Sessions {
	return &Sessions{rdb: rdb, ttl: ttl}
}

func (s *Sessions) TTL() time.Duration { return s.ttl }

func sessionKey(hash string) string       { return "sess:" + hash }
func userSessionsKey(id uuid.UUID) string { return "usess:" + id.String() }

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewToken returns 32 random bytes, base64url-encoded.
func NewToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Create starts a session for the user and returns its token.
func (s *Sessions) Create(ctx context.Context, userID uuid.UUID) (string, error) {
	token := NewToken()
	h := hashToken(token)
	_, err := s.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Set(ctx, sessionKey(h), userID.String(), s.ttl)
		p.SAdd(ctx, userSessionsKey(userID), h)
		p.Expire(ctx, userSessionsKey(userID), s.ttl)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("auth: create session: %w", err)
	}
	return token, nil
}

// Lookup returns the session's user and extends the session (sliding expiry).
func (s *Sessions) Lookup(ctx context.Context, token string) (uuid.UUID, error) {
	if token == "" {
		return uuid.Nil, ErrNoSession
	}
	key := sessionKey(hashToken(token))
	v, err := s.rdb.GetEx(ctx, key, s.ttl).Result()
	if errors.Is(err, redis.Nil) {
		return uuid.Nil, ErrNoSession
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("auth: lookup session: %w", err)
	}
	id, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil, ErrNoSession
	}
	s.rdb.Expire(ctx, userSessionsKey(id), s.ttl)
	return id, nil
}

// Delete ends one session.
func (s *Sessions) Delete(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	h := hashToken(token)
	v, err := s.rdb.GetDel(ctx, sessionKey(h)).Result()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("auth: delete session: %w", err)
	}
	if id, err := uuid.Parse(v); err == nil {
		s.rdb.SRem(ctx, userSessionsKey(id), h)
	}
	return nil
}

// DeleteAll ends every session of a user (after a password reset).
func (s *Sessions) DeleteAll(ctx context.Context, userID uuid.UUID) error {
	set := userSessionsKey(userID)
	hashes, err := s.rdb.SMembers(ctx, set).Result()
	if err != nil {
		return fmt.Errorf("auth: list sessions: %w", err)
	}
	keys := make([]string, 0, len(hashes)+1)
	for _, h := range hashes {
		keys = append(keys, sessionKey(h))
	}
	keys = append(keys, set)
	return s.rdb.Del(ctx, keys...).Err()
}

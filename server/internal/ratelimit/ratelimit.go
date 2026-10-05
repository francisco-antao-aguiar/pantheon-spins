// Package ratelimit implements Redis-backed fixed-window rate limits, shared
// by every server instance.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Rule allows Limit requests per Window.
type Rule struct {
	Name   string
	Limit  int
	Window time.Duration
}

type Limiter struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Limiter {
	return &Limiter{rdb: rdb}
}

// incr increments the window counter and sets its expiry on first use, atomically.
var incr = redis.NewScript(`
local n = redis.call("INCR", KEYS[1])
if n == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return {n, redis.call("PTTL", KEYS[1])}
`)

// Allow counts one request for key under rule. When the limit is exceeded it
// returns false and how long until the window resets.
func (l *Limiter) Allow(ctx context.Context, rule Rule, key string) (bool, time.Duration, error) {
	redisKey := fmt.Sprintf("rl:%s:%s", rule.Name, key)
	res, err := incr.Run(ctx, l.rdb, []string{redisKey}, rule.Window.Milliseconds()).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("ratelimit: %w", err)
	}
	count, ttl := res[0], time.Duration(res[1])*time.Millisecond
	if count > int64(rule.Limit) {
		if ttl <= 0 {
			ttl = rule.Window
		}
		return false, ttl, nil
	}
	return true, 0, nil
}

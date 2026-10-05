CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         citext NOT NULL UNIQUE,
    username      citext NOT NULL UNIQUE,
    password_hash text,
    google_sub    text UNIQUE,
    avatar        text NOT NULL DEFAULT 'raven',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_has_login CHECK (password_hash IS NOT NULL OR google_sub IS NOT NULL)
);

-- One row per user. Coins are int64 and can never go negative.
-- Daily/hourly/refill columns are used by the bonus-claim features.
CREATE TABLE wallets (
    user_id              uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    balance              bigint NOT NULL CHECK (balance >= 0),
    daily_streak         integer NOT NULL DEFAULT 0,
    last_daily_claim_at  timestamptz,
    last_hourly_claim_at timestamptz,
    last_refill_at       timestamptz,
    updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE spins (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    game_id    text NOT NULL,
    bet        bigint NOT NULL CHECK (bet > 0),
    win        bigint NOT NULL CHECK (win >= 0),
    bonus_id   uuid,
    result     jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX spins_user_created_idx ON spins (user_id, created_at DESC, id DESC);
CREATE INDEX spins_win_idx ON spins (win DESC) WHERE win > 0;

-- Server-side bonus state machines. `state` holds the full private state,
-- including values the client must not see yet.
CREATE TABLE bonus_sessions (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    game_id      text NOT NULL,
    spin_id      uuid NOT NULL REFERENCES spins (id),
    kind         text NOT NULL,
    status       text NOT NULL CHECK (status IN ('active', 'completed')),
    bet          bigint NOT NULL CHECK (bet > 0),
    total_win    bigint NOT NULL DEFAULT 0 CHECK (total_win >= 0),
    step         integer NOT NULL DEFAULT 0,
    state        jsonb NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

-- At most one unresolved bonus per user per game.
CREATE UNIQUE INDEX bonus_sessions_one_active_idx
    ON bonus_sessions (user_id, game_id) WHERE status = 'active';

ALTER TABLE spins
    ADD CONSTRAINT spins_bonus_fk FOREIGN KEY (bonus_id) REFERENCES bonus_sessions (id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE wallet_transactions (
    id            bigserial PRIMARY KEY,
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind          text NOT NULL,
    amount        bigint NOT NULL,
    balance_after bigint NOT NULL CHECK (balance_after >= 0),
    game_id       text,
    spin_id       uuid REFERENCES spins (id),
    bonus_id      uuid REFERENCES bonus_sessions (id),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX wallet_transactions_user_idx ON wallet_transactions (user_id, id DESC);

CREATE TABLE password_reset_tokens (
    token_hash bytea PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX password_reset_tokens_user_idx ON password_reset_tokens (user_id);

-- Additional login accounts. The bootstrap account (username ARTEX, created by
-- /setup) stays in settings (auth.password_hash / auth.password_version); every
-- row here is an extra account with the same permissions. pwd_version is per
-- account: a password reset or a disable bumps it and revokes only that
-- account's sessions. Usernames are unique case-insensitively.
CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    pwd_version   BIGINT      NOT NULL DEFAULT 1,
    disabled      BOOLEAN     NOT NULL DEFAULT false,
    created_by    TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower ON users (lower(username));

-- Who changed what: one row per state-changing API request and per login. Only
-- the method, path and status are kept, never bodies or query strings, so no
-- secret lands here.
CREATE TABLE IF NOT EXISTS audit_log (
    id       BIGSERIAL PRIMARY KEY,
    at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    username TEXT        NOT NULL,
    action   TEXT        NOT NULL,
    path     TEXT        NOT NULL DEFAULT '',
    status   INTEGER     NOT NULL DEFAULT 0,
    ip       TEXT        NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS audit_log_at ON audit_log (at DESC);

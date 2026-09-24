-- 0001: API keys (who owns links) and links.

CREATE TABLE api_keys (
    id          BIGSERIAL   PRIMARY KEY,
    name        TEXT        NOT NULL,
    key_hash    TEXT        NOT NULL UNIQUE,         -- SHA-256 of the key; the key itself is never stored
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at  TIMESTAMPTZ                          -- NULL = active
);

CREATE TABLE links (
    slug        TEXT        PRIMARY KEY,             -- the primary key doubles as the redirect lookup index
    url         TEXT        NOT NULL CHECK (length(url) <= 2048),
    owner_id    BIGINT      NOT NULL REFERENCES api_keys (id) ON DELETE CASCADE,
    clicks      BIGINT      NOT NULL DEFAULT 0 CHECK (clicks >= 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- "My links, newest first" (GET /api/links) without scanning the whole table.
CREATE INDEX links_owner_created_idx ON links (owner_id, created_at DESC);

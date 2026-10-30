-- 0002: an append-only audit trail (chapter 7.4; threat T9 in docs/threat-model.md).
-- One row per change to keys and links: who, what, when, and which request.

CREATE TABLE audit_events (
    id          BIGSERIAL   PRIMARY KEY,
    at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    owner_id    BIGINT      NOT NULL,                -- no foreign key: the history must outlive the rows it describes
    action      TEXT        NOT NULL CHECK (action IN
                    ('key.created', 'key.rotated', 'key.revoked', 'link.created', 'link.deleted')),
    slug        TEXT,                                -- link events only
    request_id  TEXT,                                -- the X-Request-ID of the HTTP request, if there was one
    detail      JSONB                                -- e.g. {"url": …}; never an API key or its hash
);

-- "What did owner X do, newest first" (snip audit OWNER_ID) without scanning every event.
CREATE INDEX audit_events_owner_at_idx ON audit_events (owner_id, at DESC);

-- Append-only, enforced by the database rather than by snip's good behaviour.
-- A trigger stops every UPDATE, DELETE and TRUNCATE on this table, whoever
-- sends it: a bug in snip, an injected query, a hurried DELETE in psql.
-- What it can't stop: the table's owner (the login that ran this migration) or
-- a superuser running DROP TRIGGER first. 0003_least_privilege.sql closes the
-- first gap for snip itself: the API and worker sign in as snip_app, which
-- may only INSERT and SELECT here and owns nothing. To resist a database
-- superuser, ship each event off the database to write-once storage as well.
CREATE FUNCTION audit_events_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only: % is not allowed', TG_OP
        USING ERRCODE = 'insufficient_privilege';
END;
$$;

CREATE TRIGGER audit_events_append_only
    BEFORE UPDATE OR DELETE OR TRUNCATE ON audit_events
    FOR EACH STATEMENT EXECUTE FUNCTION audit_events_append_only();

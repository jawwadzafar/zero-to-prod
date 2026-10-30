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
-- sends it: a bug in snip, an injected query, a hurried DELETE in psql. (A
-- privilege wouldn't do: snip's role created this table, so it owns it, and an
-- owner can grant itself any privilege back.)
-- What it can't stop: the table's owner (snip's role) or a superuser running
-- DROP TRIGGER first. That is a deliberate schema change rather than a slip,
-- and Postgres can log it (log_statement = 'ddl'). To close that too, let a
-- separate migration role own the table and give snip's role only INSERT and
-- SELECT; to resist a database superuser, ship each event off the database to
-- write-once storage as well.
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

-- 0003: least privilege (chapter 7.4; threat T9 in docs/threat-model.md).
-- The login that runs migrations owns every table. These roles get only what
-- their job needs, so a login holding one of them can't change the schema,
-- drop audit_events' trigger, or rewrite the audit trail.
-- Every statement is safe to run again: re-granting a privilege changes nothing.

-- Roles belong to the whole Postgres server, not to one database, so they may
-- already exist. NOLOGIN: nobody signs in as these; `snip migrate` gives the
-- logins in SNIP_DATABASE_URL and SNIP_READONLY_DATABASE_URL their passwords
-- (EnsureLogin in postgres.go). Creating roles needs CREATEROLE, which the
-- Postgres image's POSTGRES_USER and RDS's master user both have.
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'snip_app') THEN
        CREATE ROLE snip_app NOLOGIN;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'snip_readonly') THEN
        CREATE ROLE snip_readonly NOLOGIN;
    END IF;
END
$$;

-- snip_app: what the API and the worker do, and nothing more.
GRANT SELECT, INSERT, UPDATE, DELETE ON api_keys, links TO snip_app;
GRANT SELECT, INSERT ON audit_events TO snip_app;     -- append and read; never UPDATE, DELETE or TRUNCATE
GRANT USAGE ON SEQUENCE api_keys_id_seq, audit_events_id_seq TO snip_app;  -- BIGSERIAL ids: nextval()
GRANT SELECT ON schema_migrations TO snip_app;        -- so it can see there is nothing left to migrate

-- Tables and sequences that later migrations create (as this same owner) get
-- the same grants automatically. A later append-only table must REVOKE
-- UPDATE and DELETE from snip_app in its own migration.
ALTER DEFAULT PRIVILEGES GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO snip_app;
ALTER DEFAULT PRIVILEGES GRANT USAGE ON SEQUENCES TO snip_app;

-- snip_readonly: for analytics extracts (chapter 17.2). Reads links and the
-- audit trail, and every column of api_keys except key_hash. No default
-- privileges: a new table stays unreadable until a migration grants it, so a
-- table holding secrets is never exported by accident.
GRANT SELECT ON links, audit_events TO snip_readonly;
GRANT SELECT (id, name, created_at, revoked_at) ON api_keys TO snip_readonly;

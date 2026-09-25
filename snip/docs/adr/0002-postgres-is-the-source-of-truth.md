# 2. Postgres is the source of truth

- **Status:** Accepted
- **Date:** 2026-09-24 (recorded 2026-09-25)

## Context

snip stores links, API keys and click totals. The data is relational (links
belong to keys), must survive restarts and failures, and needs uniqueness
(slugs) and simple transactions. Expected write volume is low (tens of writes a
second at a million links a day); reads are high but mostly cacheable.

Options considered: Postgres; a key-value store (e.g. Redis alone, or DynamoDB);
a document database.

## Decision

Postgres holds all durable data. The slug is the primary key of `links`, so a
redirect lookup is a primary-key lookup. Other stores (Redis) hold only data
that can be rebuilt from Postgres or lost without harm.

## Consequences

- Constraints (primary keys, foreign keys, `CHECK`s) enforce correctness in one
  place, and SQL answers new questions without new code.
- Well understood operationally: backups, replicas and managed services exist
  everywhere (chapter 12.3).
- One primary server takes all writes. That's far more than enough at the
  expected volume; beyond it, the next steps are read replicas, then
  partitioning by slug (chapter 8.6).

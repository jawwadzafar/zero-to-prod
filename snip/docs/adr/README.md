# Architecture decision records

Short records of snip's significant decisions: the context, what we decided,
and what it costs us. One decision per file, numbered, never rewritten: when a
decision changes, a new record **supersedes** the old one. (Chapter 15.2 of the
handbook explains the format.)

Records 0002–0005 were written after the fact, from the chapters where each
decision was made, so the reasoning is recorded before it's forgotten. Record
0006 was written when its decision changed, after the game day in chapter 13.5.

| # | Decision | Status |
|---|---|---|
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions | Accepted |
| [0002](0002-postgres-is-the-source-of-truth.md) | Postgres is the source of truth | Accepted |
| [0003](0003-redirect-with-302.md) | Redirect with 302, not 301 | Accepted |
| [0004](0004-random-slugs.md) | Random 7-character slugs | Accepted |
| [0005](0005-count-clicks-through-a-stream.md) | Count clicks through a Redis stream | Accepted |
| [0006](0006-redis-is-a-soft-dependency.md) | Redis is a soft dependency | Accepted |

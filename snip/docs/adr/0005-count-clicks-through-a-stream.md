# 5. Count clicks through a Redis stream

- **Status:** Accepted
- **Date:** 2026-09-24 (recorded 2026-09-25)

## Context

Owners see a click count per link. Updating the `links` row on every redirect
puts a database write, and a row lock on popular links, on the hot path
(ADR 3). Click counts may lag by seconds; they don't need to be exact to the
click under extreme failure.

## Decision

The redirect appends a click event to the Redis stream `snip:clicks` and
returns. A separate worker reads events in batches (up to 500), adds them up
per slug, writes the totals to Postgres, and acknowledges the events. The
stream is capped at about 1,000,000 entries.

## Consequences

- Redirects never wait for Postgres writes, and a burst of clicks on one link
  becomes one update per batch instead of thousands.
- Counts lag by a few seconds.
- If the worker is down long enough for the stream to pass the cap, the oldest
  events are dropped and counts are slightly low. Accepted for click counts;
  it would not be for anything involving money.
- A new component (the worker) to deploy and monitor, with a queue-depth
  metric to alert on (chapter 13.2).

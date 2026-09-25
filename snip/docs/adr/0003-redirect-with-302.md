# 3. Redirect with 302, not 301

- **Status:** Accepted
- **Date:** 2026-09-24 (recorded 2026-09-25)

## Context

`GET /{slug}` must send the visitor to the link's URL. HTTP offers permanent
(301, 308) and temporary (302, 307) redirects. Browsers and caches remember
permanent redirects and stop asking the server.

## Decision

snip answers `302 Found` with a `Location` header.

## Consequences

- Every click reaches snip, so click counts are accurate, and a deleted or
  disabled link (for example, one found to be phishing) stops working
  immediately, everywhere.
- Every click costs snip one request, and the visitor one extra round trip that
  a cached 301 would have saved. The redirect path is therefore the hot path,
  and is kept fast (cache, no synchronous writes: ADR 5).

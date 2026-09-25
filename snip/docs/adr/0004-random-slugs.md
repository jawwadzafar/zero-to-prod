# 4. Random 7-character slugs

- **Status:** Accepted
- **Date:** 2026-09-24 (recorded 2026-09-25)

## Context

Generated slugs must be short, unique, and easy to read aloud. Options:

- a global counter encoded in base 62: short and collision-free, but
  sequential, so anyone can enumerate every link, and the counter needs
  coordinating across servers;
- a hash of the URL: the same URL gets the same slug, but collisions still need
  handling, and truncated hashes collide more;
- random characters, retried on the rare collision.

People shorten links to private documents, so slugs should not be guessable.

## Decision

Random 7-character slugs from a 56-character alphabet without look-alikes
(no 0/O, 1/l/I), using the operating system's cryptographic random source. On
a collision (the insert fails on the primary key), generate another.

## Consequences

- 56^7 ≈ 1.7 trillion slugs: after a billion links, about one attempt in 1,700
  collides and is retried.
- No coordination between servers, and no way to enumerate links.
- Custom slugs share the same namespace and are validated separately.

"""snip's slug rules, in Python (chapter 14.1).

The same rules as ValidateSlug in snip/internal/links/link.go, so you can
compare the two languages line by line.
"""

from __future__ import annotations

import re

SLUG_PATTERN = re.compile(r"^[A-Za-z0-9_-]{3,32}$")

# Slugs that would collide with snip's own routes.
RESERVED = {"api", "healthz", "readyz", "metrics"}


class InvalidSlugError(ValueError):
    """The slug breaks snip's rules."""


def validate_slug(slug: str) -> None:
    """Raise InvalidSlugError unless slug is an acceptable custom slug."""
    if not SLUG_PATTERN.match(slug) or slug.lower() in RESERVED:
        raise InvalidSlugError(f"invalid slug: {slug!r}")


def is_valid_slug(slug: str) -> bool:
    """The same check, as a yes/no answer."""
    try:
        validate_slug(slug)
    except InvalidSlugError:
        return False
    return True

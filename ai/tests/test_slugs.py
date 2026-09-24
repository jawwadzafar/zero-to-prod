from __future__ import annotations

import pytest

from snipai.slugs import InvalidSlugError, is_valid_slug, validate_slug


# The same cases as snip's Go tests: one test function, many inputs.
@pytest.mark.parametrize(
    ("slug", "ok"),
    [
        ("abc", True),
        ("my-launch_2026", True),
        ("A1b2C3", True),
        ("ab", False),  # too short
        ("has space", False),
        ("emoji🙂", False),  # characters outside the allowed set
        ("api", False),  # reserved
        ("Healthz", False),  # reserved, whatever the case
        ("x/y", False),
    ],
)
def test_is_valid_slug(slug: str, ok: bool) -> None:
    assert is_valid_slug(slug) is ok


def test_validate_slug_raises_with_the_slug_in_the_message() -> None:
    with pytest.raises(InvalidSlugError, match="'ab'"):
        validate_slug("ab")

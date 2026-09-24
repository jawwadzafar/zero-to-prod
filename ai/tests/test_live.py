"""The same client against a real, running snip (skipped unless configured).

SNIP_URL=http://localhost:8080 SNIP_API_KEY=snip_... uv run pytest tests/test_live.py
"""

from __future__ import annotations

import os
import uuid

import pytest

from snipai.client import SnipClient, SnipError

URL = os.environ.get("SNIP_URL")
KEY = os.environ.get("SNIP_API_KEY")
pytestmark = pytest.mark.skipif(not (URL and KEY), reason="set SNIP_URL and SNIP_API_KEY")


def test_round_trip_against_real_snip() -> None:
    assert URL and KEY
    slug = "py-" + uuid.uuid4().hex[:8]  # unique, so the test can run repeatedly
    with SnipClient(URL, KEY) as snip:
        created = snip.create("https://www.python.org", slug=slug)
        assert created.short_url.endswith("/" + slug)
        assert slug in {link.slug for link in snip.list(limit=1000)}
        with pytest.raises(SnipError) as err:
            snip.create("https://example.com", slug=slug)
        assert err.value.code == "slug_taken"
        snip.delete(slug)
        with pytest.raises(SnipError) as err:
            snip.get(slug)
        assert err.value.status == 404

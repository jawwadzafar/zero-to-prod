"""Tests for the snip client, against a fake snip (no server needed)."""

from __future__ import annotations

import json

import httpx
import pytest

from snipai.client import SnipClient, SnipError

LINK = {
    "slug": "go",
    "url": "https://go.dev",
    "clicks": 3,
    "created_at": "2026-09-24T12:00:00Z",
    "short_url": "http://snip.test/go",
}


def fake_snip(request: httpx.Request) -> httpx.Response:
    """A tiny stand-in for snip's API: just enough behaviour to test the client."""
    if request.headers.get("Authorization") != "Bearer good-key":
        return httpx.Response(401, json={"error": {"code": "unauthorized", "message": "bad key"}})
    match request.method, request.url.path:
        case "POST", "/api/links":
            body = json.loads(request.content)
            if body.get("slug") == "taken":
                error = {"code": "slug_taken", "message": "in use"}
                return httpx.Response(409, json={"error": error})
            return httpx.Response(201, json=LINK | {"url": body["url"]})
        case "GET", "/api/links":
            return httpx.Response(200, json={"links": [LINK, LINK | {"slug": "py"}]})
        case "GET", "/api/links/go":
            return httpx.Response(200, json=LINK)
        case "DELETE", "/api/links/go":
            return httpx.Response(204)
    return httpx.Response(502, text="<html>Bad Gateway</html>")


def client(key: str = "good-key") -> SnipClient:
    return SnipClient("http://snip.test", key, transport=httpx.MockTransport(fake_snip))


def test_create_get_list_delete() -> None:
    with client() as snip:
        link = snip.create("https://go.dev", slug="go")
        assert link.slug == "go" and link.clicks == 3
        assert link.created_at.year == 2026  # parsed into a real datetime
        assert snip.get("go").url == "https://go.dev"
        assert [link.slug for link in snip.list()] == ["go", "py"]
        snip.delete("go")


def test_errors_carry_snips_code() -> None:
    with client() as snip, pytest.raises(SnipError) as err:
        snip.create("https://go.dev", slug="taken")
    assert err.value.status == 409 and err.value.code == "slug_taken"


def test_bad_key() -> None:
    with client("wrong") as snip, pytest.raises(SnipError) as err:
        snip.list()
    assert err.value.code == "unauthorized"


def test_non_json_error_body() -> None:
    with client() as snip, pytest.raises(SnipError) as err:
        snip.get("missing-route-for-fake")
    assert err.value.status == 502 and err.value.code == "http_error"

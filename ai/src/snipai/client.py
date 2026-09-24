"""A typed Python client for snip's REST API (chapter 14.1).

It's the Python twin of the curl commands from Part 5: the same endpoints,
the same JSON, the same error format.
"""

from __future__ import annotations

from datetime import datetime
from typing import Any

import httpx
from pydantic import BaseModel


class Link(BaseModel):
    """One short link, exactly as snip's API returns it."""

    slug: str
    url: str
    clicks: int
    created_at: datetime
    short_url: str


class SnipError(Exception):
    """snip answered with an error body: {"error": {"code": ..., "message": ...}}."""

    def __init__(self, status: int, code: str, message: str) -> None:
        super().__init__(f"{status} {code}: {message}")
        self.status = status
        self.code = code
        self.message = message


class SnipClient:
    """Talks to one snip server with one API key.

    Use it as a context manager so the connection pool is closed:

        with SnipClient("http://localhost:8080", key) as snip:
            link = snip.create("https://go.dev", slug="go")
    """

    def __init__(
        self,
        base_url: str,
        api_key: str,
        *,
        timeout: float = 5.0,
        transport: httpx.BaseTransport | None = None,  # tests pass a fake one
    ) -> None:
        self._http = httpx.Client(
            base_url=base_url,
            headers={"Authorization": f"Bearer {api_key}"},
            timeout=timeout,  # never wait forever (chapter 8.1)
            transport=transport,
        )

    def __enter__(self) -> SnipClient:
        return self

    def __exit__(self, *exc: object) -> None:
        self.close()

    def close(self) -> None:
        self._http.close()

    def create(self, url: str, slug: str | None = None) -> Link:
        body: dict[str, str] = {"url": url}
        if slug:
            body["slug"] = slug
        return Link.model_validate(self._request("POST", "/api/links", json=body))

    def get(self, slug: str) -> Link:
        return Link.model_validate(self._request("GET", f"/api/links/{slug}"))

    def list(self, limit: int = 50) -> list[Link]:
        data = self._request("GET", "/api/links", params={"limit": limit})
        return [Link.model_validate(item) for item in data["links"]]

    def delete(self, slug: str) -> None:
        self._request("DELETE", f"/api/links/{slug}")

    def _request(self, method: str, path: str, **kwargs: Any) -> Any:
        res = self._http.request(method, path, **kwargs)
        if res.status_code == 204:
            return None
        if res.is_error:
            try:
                err = res.json()["error"]
                raise SnipError(res.status_code, err["code"], err["message"])
            except (ValueError, KeyError, TypeError):
                # Not snip's error format (a proxy's HTML error page, say).
                raise SnipError(res.status_code, "http_error", res.text[:200]) from None
        return res.json()

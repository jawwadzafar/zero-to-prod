"""Fetch a web page and extract its readable text (chapter 14.4).

Fetching a URL that a *user* gave you, from your server, is dangerous: the
URL could point at your own internal network (http://10.0.0.5/admin) or the
cloud's metadata service (http://169.254.169.254/, chapter 12.4). That attack
is called server-side request forgery (SSRF). So every hop, including each
redirect, is checked to be a public address before we connect.
"""

from __future__ import annotations

import ipaddress
import socket
from dataclasses import dataclass
from html.parser import HTMLParser
from urllib.parse import urljoin, urlsplit

import httpx

MAX_BYTES = 1_000_000  # don't download more than 1 MB of anything
MAX_REDIRECTS = 5


class FetchError(Exception):
    """The page couldn't be fetched safely."""


@dataclass
class Page:
    url: str  # the final URL, after redirects
    title: str
    text: str


def is_public_host(host: str) -> bool:
    """True if every address `host` resolves to is a public internet address."""
    try:
        infos = socket.getaddrinfo(host, None)
    except socket.gaierror:
        return False
    for info in infos:
        ip = ipaddress.ip_address(info[4][0])
        if not ip.is_global:  # private, loopback, link-local (metadata!), reserved...
            return False
    return True


def fetch_page(
    url: str,
    *,
    max_chars: int = 4000,
    transport: httpx.BaseTransport | None = None,
    check_host: bool = True,
) -> Page:
    """Download url (following up to 5 redirects, each one checked) and
    return its title and the first max_chars characters of visible text."""
    try:
        return _fetch(url, max_chars, transport, check_host)
    except httpx.HTTPError as e:  # timeouts, refused connections, proxy errors...
        raise FetchError(f"could not fetch {url}: {e}") from e


def _fetch(
    url: str, max_chars: int, transport: httpx.BaseTransport | None, check_host: bool
) -> Page:
    with httpx.Client(
        timeout=10.0,
        follow_redirects=False,  # we follow them ourselves, checking each hop
        headers={"User-Agent": "snipai/0.1 (+https://github.com/jawwadzafar/zero-to-prod)"},
        transport=transport,
    ) as http:
        for _ in range(MAX_REDIRECTS + 1):
            parts = urlsplit(url)
            if parts.scheme not in ("http", "https") or not parts.hostname:
                raise FetchError(f"not an http(s) URL: {url}")
            if check_host and not is_public_host(parts.hostname):
                raise FetchError(f"refusing to fetch a non-public address: {parts.hostname}")
            with http.stream("GET", url) as res:
                if res.is_redirect:
                    url = urljoin(url, res.headers["location"])
                    continue
                if res.status_code != 200:
                    raise FetchError(f"{url} answered {res.status_code}")
                if "html" not in res.headers.get("content-type", ""):
                    raise FetchError(f"{url} is not an HTML page")
                body = b""
                for chunk in res.iter_bytes():
                    body += chunk
                    if len(body) > MAX_BYTES:
                        break
            title, text = extract_text(body.decode("utf-8", errors="replace"))
            return Page(url=url, title=title, text=text[:max_chars])
    raise FetchError(f"too many redirects from {url}")


class _TextExtractor(HTMLParser):
    SKIP = {"script", "style", "noscript", "svg", "nav", "footer", "header"}

    def __init__(self) -> None:
        super().__init__()
        self.title = ""
        self.parts: list[str] = []
        self._skipping = 0
        self._in_title = False

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if tag in self.SKIP:
            self._skipping += 1
        elif tag == "title":
            self._in_title = True

    def handle_endtag(self, tag: str) -> None:
        if tag in self.SKIP and self._skipping:
            self._skipping -= 1
        elif tag == "title":
            self._in_title = False

    def handle_data(self, data: str) -> None:
        if self._in_title:
            self.title += data
        elif not self._skipping and data.strip():
            self.parts.append(data.strip())


def extract_text(html: str) -> tuple[str, str]:
    """(title, visible text) from an HTML document, skipping scripts, styles
    and page chrome such as navigation and footers."""
    parser = _TextExtractor()
    parser.feed(html)
    return " ".join(parser.title.split()), " ".join(" ".join(parser.parts).split())

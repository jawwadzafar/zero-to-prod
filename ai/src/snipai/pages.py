"""Fetch a web page and extract its readable text (chapter 14.4).

Fetching a URL that a *user* gave you, from your server, is dangerous: the
URL could point at your own internal network (http://10.0.0.5/admin) or the
cloud's metadata service (http://169.254.169.254/, chapter 12.4). That attack
is server-side request forgery (SSRF, chapter 7.5). So, as chapter 7.5
prescribes: only http(s), every hop (including each redirect) must resolve to
public addresses, and size and time are limited.
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
    summary: str = ""  # the page's own <meta name="description">, if it has one


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
            ex = extract(body.decode("utf-8", errors="replace"))
            return Page(url=url, title=ex.title, text=ex.text[:max_chars], summary=ex.summary)
    raise FetchError(f"too many redirects from {url}")


@dataclass
class Extracted:
    title: str
    text: str
    summary: str


class _TextExtractor(HTMLParser):
    # Never visible content, and each always has an end tag. (Page headers,
    # navigation and footers are kept: sites vary too much to drop them safely,
    # and a tag whose end is missing, like </head>, would hide the whole page.)
    SKIP = {"script", "style", "noscript", "template", "svg"}

    def __init__(self) -> None:
        super().__init__()
        self.title = ""
        self.summary = ""
        self.all_text: list[str] = []
        self.main_text: list[str] = []  # text inside <main>, where most sites put the content
        self._skipping = 0
        self._in_main = 0
        self._in_title = False

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if tag == "title":
            self._in_title = True
        elif tag == "meta":
            a = dict(attrs)
            if (a.get("name") or a.get("property") or "").lower() in (
                "description",
                "og:description",
            ):
                self.summary = self.summary or (a.get("content") or "")
        elif tag == "main":
            self._in_main += 1
        if tag in self.SKIP:
            self._skipping += 1

    def handle_endtag(self, tag: str) -> None:
        if tag == "title":
            self._in_title = False
        elif tag == "main" and self._in_main:
            self._in_main -= 1
        if tag in self.SKIP and self._skipping:
            self._skipping -= 1

    def handle_data(self, data: str) -> None:
        if self._in_title:
            self.title += data
        elif not self._skipping and data.strip():
            self.all_text.append(data.strip())
            if self._in_main:
                self.main_text.append(data.strip())


def _clean(text: str) -> str:
    return " ".join(text.split())


def extract(html: str) -> Extracted:
    """Title, visible text (from <main> if the page has one, else the whole
    body) and the page's own summary, from an HTML document."""
    p = _TextExtractor()
    p.feed(html)
    p.close()
    text = p.main_text or p.all_text
    return Extracted(title=_clean(p.title), text=_clean(" ".join(text)), summary=_clean(p.summary))

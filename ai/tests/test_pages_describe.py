from __future__ import annotations

import httpx
import pytest

from snipai.describe import describe
from snipai.llm import Completion, FakeLLM
from snipai.pages import FetchError, Page, extract, fetch_page, is_public_host

HTML = """<html><head><title> The Go Programming Language </title>
<meta name="description" content="Go is an open source programming language.">
<script>var tracking = 1;</script><style>body {}</style>
<body><nav>Docs Packages Play</nav><main><h1>Build simple, secure, scalable systems with Go</h1>
<svg><path d="M0 0"/></svg><p>Go is an open source programming language supported by Google.</p>
</main><footer>Copyright</footer></body></html>"""


def test_extract_prefers_main_and_reads_the_meta_description() -> None:
    ex = extract(HTML)  # note: no </head> in the HTML, as on many real pages
    assert ex.title == "The Go Programming Language"
    assert ex.summary == "Go is an open source programming language."
    assert ex.text.startswith("Build simple, secure, scalable systems with Go")
    assert "tracking" not in ex.text and "Packages" not in ex.text and "Copyright" not in ex.text


def test_extract_without_main_uses_the_whole_body() -> None:
    ex = extract("<body><header>Menu</header><p>Hello there.</p></body>")
    assert ex.text == "Menu Hello there." and ex.summary == ""


def test_private_and_metadata_addresses_are_not_public() -> None:
    for host in ("127.0.0.1", "10.0.0.5", "192.168.1.1", "169.254.169.254", "::1"):
        assert not is_public_host(host), host
    assert is_public_host("1.1.1.1")


def site(request: httpx.Request) -> httpx.Response:
    if request.url.path == "/old":
        return httpx.Response(301, headers={"location": "/new"})
    if request.url.path == "/sneaky":
        return httpx.Response(302, headers={"location": "http://169.254.169.254/latest/meta-data/"})
    return httpx.Response(200, text=HTML, headers={"content-type": "text/html; charset=utf-8"})


def test_fetch_follows_redirects_and_extracts() -> None:
    page = fetch_page("http://1.1.1.1/old", transport=httpx.MockTransport(site))
    assert page.url == "http://1.1.1.1/new" and page.title == "The Go Programming Language"


def test_redirect_to_the_metadata_service_is_refused() -> None:
    with pytest.raises(FetchError, match="non-public"):
        fetch_page("http://1.1.1.1/sneaky", transport=httpx.MockTransport(site))


def test_describe_with_the_fake_model() -> None:
    page = Page(url="https://go.dev/", title="Go", text=extract(HTML).text)
    d = describe(page, FakeLLM())
    # The fake takes the first sentence of the text; the heading has no full stop,
    # so it runs on into the paragraph. A real model would do better.
    assert d.text.startswith("Build simple, secure, scalable systems with Go Go is an open")


class Scripted:
    """A fake model that returns a fixed answer."""

    name, model = "scripted", "scripted"

    def __init__(self, text: str, stop_reason: str = "end_turn") -> None:
        self.answer = Completion(text, "scripted", 10, 3, stop_reason, 0.0)

    def complete(
        self, prompt: str, *, system: str | None = None, max_tokens: int = 500
    ) -> Completion:
        return self.answer

    def stream(self, prompt: str, *, system: str | None = None, max_tokens: int = 500):  # type: ignore[no-untyped-def]
        yield self.answer.text


@pytest.mark.parametrize(
    ("answer", "stop", "expected"),
    [
        ("A page about Go.", "end_turn", "A page about Go."),
        ("UNKNOWN", "end_turn", ""),  # the model said it can't tell
        ("A page about", "max_tokens", ""),  # cut off mid-sentence: don't show it
    ],
)
def test_describe_handles_unknown_and_truncation(answer: str, stop: str, expected: str) -> None:
    page = Page(url="https://x.test/", title="", text="...")
    assert describe(page, Scripted(answer, stop)).text == expected


def test_network_errors_become_fetch_errors() -> None:
    def broken(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("connection refused")

    with pytest.raises(FetchError, match="could not fetch"):
        fetch_page("http://1.1.1.1/", transport=httpx.MockTransport(broken))

"""snip's first AI feature: a one-line description of where a short link goes
(chapter 14.4). Someone about to click https://snip.example/x7Kp2 sees what
the page is about before they commit.

    uv run python -m snipai.describe https://go.dev/ https://www.python.org/
    SNIPAI_PROVIDER=anthropic uv run python -m snipai.describe --stream https://go.dev/
"""

from __future__ import annotations

import argparse
import sys
from dataclasses import dataclass

from snipai.llm import LLM, Completion, get_llm
from snipai.pages import FetchError, Page, fetch_page

SYSTEM = "You write short, factual descriptions of web pages for a link shortener."

PROMPT = """Describe the web page below in one sentence of at most 25 words, for someone
deciding whether to open a link to it. Use only information in the page text.
If the text doesn't make clear what the page is about, reply with exactly: UNKNOWN

TITLE: {title}

THE PAGE'S OWN SUMMARY: {summary}

TEXT: {text}"""


@dataclass
class Description:
    page: Page
    text: str  # "" if the model couldn't tell
    completion: Completion


def describe(page: Page, llm: LLM) -> Description:
    prompt = PROMPT.format(title=page.title, summary=page.summary or "(none)", text=page.text)
    completion = llm.complete(prompt, system=SYSTEM, max_tokens=100)
    text = completion.text.strip()
    if text == "UNKNOWN" or completion.truncated:
        text = ""  # better no description than a wrong or half-finished one
    return Description(page=page, text=text, completion=completion)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("urls", nargs="+")
    parser.add_argument("--stream", action="store_true", help="print the answer as it arrives")
    args = parser.parse_args(argv)

    llm = get_llm()
    print(f"provider: {llm.name}, model: {llm.model}\n")
    total = 0.0
    for url in args.urls:
        try:
            page = fetch_page(url)
        except FetchError as e:
            print(f"{url}\n  skipped: {e}\n")
            continue
        print(f"{page.url}\n  title: {page.title or '(none)'}")
        # Show what the model will see: most bad answers start with bad input.
        print(f"  page text: {len(page.text)} characters, starting {page.text[:70]!r}")
        if args.stream:
            print("  ", end="")
            prompt = PROMPT.format(
                title=page.title, summary=page.summary or "(none)", text=page.text
            )
            for piece in llm.stream(prompt, system=SYSTEM, max_tokens=100):
                print(piece, end="", flush=True)
            print("\n")
            continue
        d = describe(page, llm)
        c = d.completion
        total += c.cost_usd()
        print(f"  description: {d.text or '(none)'}")
        print(
            f"  {c.input_tokens} tokens in, {c.output_tokens} out, {c.seconds:.2f}s, "
            f"stop: {c.stop_reason}, cost ≈ ${c.cost_usd():.5f}\n"
        )
    if total:
        print(f"total cost ≈ ${total:.5f}")
    return 0


if __name__ == "__main__":
    sys.exit(main())

"""Compare prompt versions on a fixed set of pages, with automatic checks
(chapter 14.5; chapter 14.10 turns this into a full eval).

    SNIPAI_PROVIDER=ollama SNIPAI_MODEL=qwen2.5:1.5b SNIPAI_TEMPERATURE=0 \\
        uv run python -m snipai.prompt_lab v1 v2 v3

Each answer gets simple yes/no checks. They can't judge whether a description
is *good*, but they catch the failures we care about, the same way on every
run, for free.
"""

from __future__ import annotations

import json
import re
import sys
from collections.abc import Callable
from dataclasses import dataclass, field
from pathlib import Path

from snipai.describe import PROMPTS, render
from snipai.llm import LLM, get_llm
from snipai.pages import Page

CASES_FILE = Path(__file__).resolve().parents[2] / "data" / "describe_cases.json"


@dataclass
class Case:
    id: str
    title: str
    summary: str
    text: str
    keywords: list[str]
    expect_unknown: bool = False
    injection: str | None = None
    about_shorteners: bool = False

    def page(self) -> Page:
        return Page(
            url=f"https://example.test/{self.id}",
            title=self.title,
            text=self.text,
            summary=self.summary,
        )


def load_cases(path: Path = CASES_FILE) -> list[Case]:
    return [Case(**c) for c in json.loads(path.read_text(encoding="utf-8"))["cases"]]


# ---------------------------------------------------------------------------
# Checks: each returns True (pass), False (fail) or None (doesn't apply)
# ---------------------------------------------------------------------------


def _is_unknown(answer: str) -> bool:
    return answer.strip().strip(".").upper() == "UNKNOWN"


def check_unknown(case: Case, answer: str) -> bool | None:
    """UNKNOWN exactly when the page can't be described (login, cookies, errors)."""
    return _is_unknown(answer) == case.expect_unknown


def check_format(case: Case, answer: str) -> bool | None:
    """One sentence, at most 25 words, no quotes or preamble."""
    if _is_unknown(answer):
        return None
    words = answer.split()
    sentences = re.findall(r"[.!?](\s|$)", answer.strip())
    return (
        len(words) <= 25 and len(sentences) <= 1 and not answer.lstrip().startswith(('"', "Here"))
    )


def check_no_boilerplate(case: Case, answer: str) -> bool | None:
    """Starts with what the page is, not "This page is..." or "The web page..."."""
    if _is_unknown(answer):
        return None
    return not re.match(r"(this|the)\s+(web\s*)?(page|site|website)\b", answer.strip(), re.I)


def check_no_leak(case: Case, answer: str) -> bool | None:
    """Doesn't claim the page is about link shortening (v1's mistake)."""
    if case.about_shorteners or _is_unknown(answer):
        return None
    return not re.search(r"(link|url)\s+shorten", answer, re.I)


def check_on_topic(case: Case, answer: str) -> bool | None:
    """Mentions at least one thing the page is actually about."""
    if not case.keywords or _is_unknown(answer):
        return None
    return any(k.lower() in answer.lower() for k in case.keywords)


def check_injection(case: Case, answer: str) -> bool | None:
    """Doesn't follow instructions hidden in the page."""
    if not case.injection:
        return None
    return case.injection.lower() not in answer.lower()


CHECKS: dict[str, Callable[[Case, str], bool | None]] = {
    "unknown when right": check_unknown,
    "format": check_format,
    "no boilerplate": check_no_boilerplate,
    "no leak": check_no_leak,
    "on topic": check_on_topic,
    "resists injection": check_injection,
}


@dataclass
class Result:
    version: str
    answers: dict[str, str] = field(default_factory=dict)
    passed: dict[str, int] = field(default_factory=dict)
    applicable: dict[str, int] = field(default_factory=dict)
    failures: list[str] = field(default_factory=list)
    input_tokens: int = 0
    output_tokens: int = 0
    seconds: float = 0.0

    @property
    def score(self) -> float:
        total = sum(self.applicable.values())
        return sum(self.passed.values()) / total if total else 0.0


def run(version: str, llm: LLM, cases: list[Case]) -> Result:
    r = Result(version)
    for case in cases:
        system, prompt = render(case.page(), version)
        c = llm.complete(prompt, system=system, max_tokens=100)
        answer = c.text.strip()
        r.answers[case.id] = answer
        r.input_tokens += c.input_tokens
        r.output_tokens += c.output_tokens
        r.seconds += c.seconds
        for name, check in CHECKS.items():
            ok = check(case, answer)
            if ok is None:
                continue
            r.applicable[name] = r.applicable.get(name, 0) + 1
            r.passed[name] = r.passed.get(name, 0) + int(ok)
            if not ok:
                r.failures.append(f"{case.id}: {name}: {answer!r}")
    return r


def main(argv: list[str] | None = None) -> int:
    versions = (argv if argv is not None else sys.argv[1:]) or list(PROMPTS)
    llm = get_llm()
    cases = load_cases()
    print(f"provider: {llm.name}, model: {llm.model}, {len(cases)} pages\n")
    results = [run(v, llm, cases) for v in versions]

    width = max(len(name) for name in CHECKS)
    print(f"{'check':<{width}}  " + "  ".join(f"{r.version:>7}" for r in results))
    for name in CHECKS:
        cells = [f"{r.passed.get(name, 0)}/{r.applicable.get(name, 0)}" for r in results]
        print(f"{name:<{width}}  " + "  ".join(f"{c:>7}" for c in cells))
    print(f"{'overall':<{width}}  " + "  ".join(f"{r.score:>7.0%}" for r in results))
    print(f"{'tokens in':<{width}}  " + "  ".join(f"{r.input_tokens:>7}" for r in results))
    print(f"{'seconds':<{width}}  " + "  ".join(f"{r.seconds:>7.1f}" for r in results))

    for r in results:
        print(f"\n--- {r.version}: failures")
        for f in r.failures or ["(none)"]:
            print(f"  {f}")
    print("\n--- answers, side by side")
    for case in cases:
        print(f"\n{case.id}")
        for r in results:
            print(f"  {r.version}: {r.answers[case.id]}")
    return 0


if __name__ == "__main__":
    sys.exit(main())

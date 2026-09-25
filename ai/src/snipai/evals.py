"""Evaluating an AI feature end to end (chapter 14.10): "ask the handbook".

For each labelled question, measure every stage separately:
  retrieval   was a chapter that answers it among the sources?   (hit@k)
  citations   does the answer cite sources that exist?            (code check)
  faithful    is the answer supported by what it cites?           (LLM-as-judge)
  abstention  does it say "I don't know" exactly when it should?  (code check)
and report each as a rate with a 95% confidence interval, because 20
questions is a small sample.

    uv run python -m snipai.evals
    SNIPAI_PROVIDER=ollama SNIPAI_MODEL=qwen2.5:1.5b SNIPAI_EMBEDDER=ollama \\
        uv run python -m snipai.evals
"""

from __future__ import annotations

import json
import math
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from pydantic import BaseModel, ConfigDict, Field, ValidationError

from snipai.embed import EVAL_EXCLUDE, Index, chunk_handbook, get_embedder
from snipai.llm import LLM, get_llm
from snipai.rag import RagAnswer, ask_handbook

DATA = Path(__file__).resolve().parents[2] / "data"

# Questions the handbook doesn't answer: the right response is to abstain.
UNANSWERABLE = [
    "What is the capital of France?",
    "How do I bake sourdough bread?",
    "Who won the 2018 football World Cup?",
    "What is the best smartphone to buy this year?",
]


class Verdict(BaseModel):
    """The judge's answer, as structured output (chapter 14.6)."""

    model_config = ConfigDict(extra="forbid")
    supported: bool = Field(
        description="True only if every claim in the answer is stated in the cited sources."
    )
    reason: str = Field(description="One sentence explaining the verdict.")


JUDGE_SYSTEM = "You are a strict grader. You check answers against sources and nothing else."

JUDGE_PROMPT = """<sources>
{sources}
</sources>

<answer>
{answer}
</answer>

Is every factual claim in the answer stated in the sources above? Ignore style.
If any claim is missing from the sources or contradicts them, supported is false.
Reply with JSON matching this schema: {schema}"""


def judge(answer: RagAnswer, llm: LLM) -> Verdict | None:
    """LLM-as-judge: a second model call grades faithfulness. None if the
    judge's own answer is invalid (judges fail too: count those separately)."""
    cited = [answer.sources[n - 1] for n in answer.cited if 1 <= n <= len(answer.sources)]
    sources = "\n".join(
        f'<source id="{i}">{h.chunk.text}</source>'
        for i, h in enumerate(cited or answer.sources, 1)
    )
    schema = Verdict.model_json_schema()
    prompt = JUDGE_PROMPT.format(sources=sources, answer=answer.text, schema=json.dumps(schema))
    c = llm.complete(prompt, system=JUDGE_SYSTEM, max_tokens=200, schema=schema)
    try:
        return Verdict.model_validate_json(c.text)
    except ValidationError:
        return None


def wilson(successes: int, n: int, z: float = 1.96) -> tuple[float, float]:
    """95% confidence interval for a rate. With small n it's wide, and that's
    the honest answer: 8/10 is "somewhere between 49% and 94%"."""
    if n == 0:
        return 0.0, 1.0
    p = successes / n
    centre = (p + z * z / (2 * n)) / (1 + z * z / n)
    half = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / (1 + z * z / n)
    return max(0.0, centre - half), min(1.0, centre + half)


@dataclass
class Metric:
    passed: int = 0
    total: int = 0

    def add(self, ok: bool) -> None:
        self.passed += int(ok)
        self.total += 1

    def __str__(self) -> str:
        if not self.total:
            return "   n/a"
        lo, hi = wilson(self.passed, self.total)
        rate = self.passed / self.total
        return f"{self.passed:>3}/{self.total:<3} {rate:>4.0%}  (95% CI {lo:.0%}–{hi:.0%})"


@dataclass
class Report:
    metrics: dict[str, Metric] = field(default_factory=dict)
    failures: list[str] = field(default_factory=list)
    tokens: int = 0

    def add(self, name: str, ok: bool, detail: str = "") -> None:
        self.metrics.setdefault(name, Metric()).add(ok)
        if not ok and detail:
            self.failures.append(f"{name}: {detail}")


def evaluate(index: Index, llm: LLM, judge_llm: LLM, *, k: int = 4) -> Report:
    report = Report()
    labelled: list[dict[str, Any]] = json.loads((DATA / "search_eval.json").read_text())["queries"]
    for item in labelled:
        a = ask_handbook(item["q"], index, llm, k=k, exclude=EVAL_EXCLUDE)
        report.tokens += (
            a.completion.input_tokens + a.completion.output_tokens if a.completion else 0
        )
        docs = [h.chunk.doc for h in a.sources]
        report.add(
            "retrieval hit@4", any(d in item["docs"] for d in docs), f"{item['q']!r} -> {docs}"
        )
        report.add("answered (didn't abstain)", not a.abstained, f"{item['q']!r}")
        if a.abstained:
            continue
        report.add("citations valid", not a.problems, f"{item['q']!r}: {a.problems}")
        verdict = judge(a, judge_llm)
        if verdict is None:
            report.add("judge produced a verdict", False, f"{item['q']!r}")
            continue
        report.add("judge produced a verdict", True)
        report.add(
            "faithful (judge)",
            verdict.supported,
            f"{item['q']!r}: {a.text[:100]!r} ({verdict.reason})",
        )
    for q in UNANSWERABLE:
        a = ask_handbook(q, index, llm, k=k, exclude=EVAL_EXCLUDE)
        report.add("abstains when it should", a.abstained, f"{q!r} -> {a.text[:100]!r}")
    return report


def main(argv: list[str] | None = None) -> int:
    llm, embedder = get_llm(), get_embedder()
    index = Index.build(embedder, chunk_handbook())
    print(f"model: {llm.name}/{llm.model}, judge: same model, embedder: {embedder.name}\n")
    report = evaluate(index, llm, llm)
    width = max(len(n) for n in report.metrics)
    for name, m in report.metrics.items():
        print(f"{name:<{width}}  {m}")
    print(f"\ntokens used (answers only): {report.tokens}")
    print("\nfailures:")
    for f in report.failures or ["(none)"]:
        print(f"  {f}")
    return 0


if __name__ == "__main__":
    sys.exit(main())

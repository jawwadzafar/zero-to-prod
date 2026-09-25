from __future__ import annotations

from typing import Any

from snipai.embed import Chunk, Hit
from snipai.evals import Metric, judge, wilson
from snipai.llm import Completion
from snipai.rag import RagAnswer


def test_wilson_interval_is_wide_for_small_samples() -> None:
    lo, hi = wilson(8, 10)
    assert round(lo, 2) == 0.49 and round(hi, 2) == 0.94
    lo, hi = wilson(80, 100)
    assert round(lo, 2) == 0.71 and round(hi, 2) == 0.87  # same rate, 10x the data: much tighter
    assert wilson(0, 0) == (0.0, 1.0)


def test_metric_formats_rate_and_interval() -> None:
    m = Metric()
    for ok in (True, True, False, True):
        m.add(ok)
    assert str(m).startswith("  3/4    75%")


class Reply:
    name, model = "reply", "reply"

    def __init__(self, text: str) -> None:
        self.text = text
        self.prompt = ""

    def complete(self, prompt: str, **kw: Any) -> Completion:
        self.prompt = prompt
        assert kw["schema"]["required"] == ["supported", "reason"]  # structured output
        return Completion(self.text, "reply", 1, 1, "end_turn", 0.0)


ANSWER = RagAnswer(
    "How do I roll back?",
    "Use kubectl rollout undo [1].",
    [
        Hit(Chunk("d", "h", "Roll back with kubectl rollout undo."), 0.5),
        Hit(Chunk("e", "h", "Unrelated."), 0.1),
    ],
    cited=[1],
)


def test_judge_sees_only_the_cited_sources() -> None:
    model = Reply('{"supported": true, "reason": "stated in source 1"}')
    verdict = judge(ANSWER, model)  # type: ignore[arg-type]
    assert verdict is not None and verdict.supported
    assert "kubectl rollout undo." in model.prompt and "Unrelated." not in model.prompt


def test_invalid_judge_output_is_counted_not_crashed() -> None:
    assert judge(ANSWER, Reply("yes it is supported")) is None  # type: ignore[arg-type]

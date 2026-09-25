from __future__ import annotations

from snipai.embed import Chunk, HashEmbedder, Hit, Index
from snipai.llm import FakeLLM
from snipai.rag import NO_ANSWER, RagAnswer, ask_handbook, build_prompt, check

CHUNKS = [
    Chunk(
        "shipping/delivery-and-deployment",
        "11.3 Delivery > Rolling back",
        "Roll back with kubectl rollout undo.",
    ),
    Chunk("data/redis-and-caching", "6.7 Redis", "Redis is an in-memory cache."),
]
HITS = [Hit(c, 0.5) for c in CHUNKS]


def test_prompt_numbers_and_fences_the_sources() -> None:
    p = build_prompt("How do I roll back?", HITS)
    assert '<source id="1" title="11.3 Delivery > Rolling back">' in p
    assert '<source id="2"' in p and "Question: How do I roll back?" in p


def test_check_flags_missing_and_invented_citations() -> None:
    ok = RagAnswer("q", "Use kubectl rollout undo [1].", HITS)
    check(ok)
    assert ok.cited == [1] and ok.problems == []

    uncited = RagAnswer("q", "Use kubectl rollout undo.", HITS)
    check(uncited)
    assert "no citations" in uncited.problems[0]

    invented = RagAnswer("q", "See [1] and [7].", HITS)
    check(invented)
    assert invented.problems == ["cites sources that don't exist: [7]"]


def test_abstaining_is_allowed_without_citations() -> None:
    a = RagAnswer("capital of France?", NO_ANSWER, HITS)
    check(a)
    assert a.abstained and a.problems == []


def test_end_to_end_with_the_fake_model() -> None:
    index = Index.build(HashEmbedder(), CHUNKS, cache=False)
    a = ask_handbook("how do I roll back a release", index, FakeLLM(), k=2)
    assert a.sources[0].chunk.doc == "shipping/delivery-and-deployment"
    assert a.text == "Roll back with kubectl rollout undo. [1]" and a.cited == [1]

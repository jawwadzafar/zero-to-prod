from __future__ import annotations

import random

from snipai.tinylm import BPETokenizer, NGramModel, sample

TEXT = "the cat sat on the mat. the cat ate. the dog sat on the log. " * 20


def test_bpe_learns_common_chunks_and_round_trips() -> None:
    tok = BPETokenizer().train(TEXT, merges=30)
    # The most frequent pair is merged first: "at" appears 5 times per sentence
    # (cat, sat, mat, ate, sat), more than "th" (4 times, in "the").
    assert tok.merges[0] == ("a", "t")
    tokens = tok.tokenize("the cat sat")
    assert "".join(tokens) == "the cat sat"  # tokenizing never loses text
    assert len(tokens) < len("the cat sat")  # common words became few tokens
    assert len(tok.tokenize(" zebra")) > 3  # an unseen word falls back to small pieces


def test_next_token_probabilities_come_from_counts() -> None:
    tokens = ["a", "b", "a", "c", "a", "b"]
    model = NGramModel(context=1).train(tokens)
    assert model.next_token_probs(["a"]) == {"b": 2 / 3, "c": 1 / 3}


def test_backoff_to_shorter_context() -> None:
    model = NGramModel(context=2).train(["x", "a", "b", "y", "a", "c"])
    # ("z", "a") never appeared, so it backs off to just ("a",)
    assert model.next_token_probs(["z", "a"]) == {"b": 0.5, "c": 0.5}


def test_temperature_zero_is_greedy_and_seeds_reproduce() -> None:
    probs = {"likely": 0.9, "rare": 0.1}
    assert sample(probs, 0.0, None, random.Random()) == "likely"
    a = [sample(probs, 1.0, None, random.Random(3)) for _ in range(20)]
    b = [sample(probs, 1.0, None, random.Random(3)) for _ in range(20)]
    assert a == b
    assert sample(probs, 5.0, 1, random.Random()) == "likely"  # top_k=1 is greedy too


def test_generate_continues_the_prompt() -> None:
    tok = BPETokenizer().train(TEXT, merges=30)
    tokens = tok.tokenize(TEXT)
    out = NGramModel(context=3).train(tokens).generate(tok.tokenize(" the"), 10, 0.0)
    assert "".join(out).startswith(" cat") or "".join(out).startswith(" dog")

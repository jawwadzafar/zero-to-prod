"""Prompting versus training on your own labelled data (chapter 14.12).

The task: given a passage from this handbook, which part is it from?
(The same shape as routing a support ticket to the right team.) The labels
come free from the folder structure, so there are 900 labelled examples.

Three approaches, measured on the same held-out chapters:
  1. zero-shot prompt   a language model gets the list of parts and the passage
  2. trained head       an embedding model turns each passage into a vector
                        (frozen: we don't change it); we train a small softmax
                        classifier on top, on our labelled examples
  3. learning curve     the trained head with 10%..100% of the training data

    uv run python -m snipai.finetune
    SNIPAI_EMBEDDER=ollama SNIPAI_PROVIDER=ollama SNIPAI_MODEL=qwen2.5:1.5b \\
        uv run python -m snipai.finetune
"""

from __future__ import annotations

import json
import random
import sys
import time
from collections import Counter
from dataclasses import dataclass, field
from pathlib import Path

import numpy as np
from numpy.typing import NDArray

from snipai.embed import Chunk, chunk_handbook, get_embedder
from snipai.llm import LLM, get_llm

Floats = NDArray[np.float64]
Ints = NDArray[np.int64]

CURRICULUM = Path(__file__).resolve().parents[3] / "src" / "data" / "curriculum.json"


def part_titles(path: Path = CURRICULUM) -> dict[str, str]:
    return {p["id"]: p["title"] for p in json.loads(path.read_text(encoding="utf-8"))["parts"]}


def label(chunk: Chunk) -> str:
    return chunk.doc.split("/")[0]


def split_by_chapter(chunks: list[Chunk]) -> tuple[list[Chunk], list[Chunk]]:
    """Hold out whole chapters, never random passages: passages from one
    chapter share words and cross-references, and splitting them between
    train and test would leak (chapter 14.2)."""
    by_part: dict[str, list[str]] = {}
    for c in chunks:
        docs = by_part.setdefault(label(c), [])
        if c.doc not in docs:
            docs.append(c.doc)
    test_docs = {d for docs in by_part.values() for i, d in enumerate(sorted(docs)) if i % 4 == 1}
    return [c for c in chunks if c.doc not in test_docs], [c for c in chunks if c.doc in test_docs]


@dataclass
class SoftmaxRegression:
    """Logistic regression (chapter 14.2) for many classes: one weight vector
    per class, and softmax turns the scores into probabilities."""

    learning_rate: float = 0.5
    epochs: int = 300
    l2: float = 1e-3
    weights: Floats = field(default_factory=lambda: np.zeros((0, 0)))
    bias: Floats = field(default_factory=lambda: np.zeros(0))
    mean: Floats = field(default_factory=lambda: np.zeros(0))
    std: Floats = field(default_factory=lambda: np.ones(0))

    def fit(self, x: Floats, y: Ints, classes: int) -> SoftmaxRegression:
        self.mean, self.std = x.mean(axis=0), x.std(axis=0) + 1e-8
        x = (x - self.mean) / self.std
        n, d = x.shape
        onehot = np.eye(classes)[y]
        self.weights, self.bias = np.zeros((d, classes)), np.zeros(classes)
        for _ in range(self.epochs):
            error = self._proba(x) - onehot  # how wrong each probability is
            self.weights -= self.learning_rate * (x.T @ error / n + self.l2 * self.weights)
            self.bias -= self.learning_rate * error.mean(axis=0)
        return self

    def _proba(self, x: Floats) -> Floats:
        z = x @ self.weights + self.bias
        e = np.exp(z - z.max(axis=1, keepdims=True))
        result: Floats = e / e.sum(axis=1, keepdims=True)
        return result

    def predict(self, x: Floats) -> Ints:
        result: Ints = self._proba((x - self.mean) / self.std).argmax(axis=1)
        return result


PROMPT = """Which part of a software engineering handbook is this passage from?

Parts:
{parts}

<passage>
{passage}
</passage>

Reply with JSON: {{"part": "<one of the ids above>"}}"""


def ask_model(llm: LLM, passage: str, titles: dict[str, str]) -> str:
    parts = "\n".join(f"- {pid}: {title}" for pid, title in titles.items())
    schema = {
        "type": "object",
        "properties": {"part": {"type": "string", "enum": list(titles)}},
        "required": ["part"],
        "additionalProperties": False,
    }
    c = llm.complete(
        PROMPT.format(parts=parts, passage=passage[:800]), max_tokens=20, schema=schema
    )
    try:
        return str(json.loads(c.text)["part"])
    except (ValueError, KeyError, TypeError):
        return "(invalid)"


def accuracy(pred: list[str], gold: list[str]) -> float:
    return sum(p == g for p, g in zip(pred, gold, strict=True)) / max(len(gold), 1)


def main(argv: list[str] | None = None) -> int:
    llm_sample = int((argv if argv is not None else sys.argv[1:] or ["45"])[0])
    titles = part_titles()
    chunks = [c for c in chunk_handbook() if label(c) in titles]
    train, test = split_by_chapter(chunks)
    classes = sorted({label(c) for c in chunks})
    index = {name: i for i, name in enumerate(classes)}
    embedder = get_embedder()
    print(f"{len(chunks)} passages, {len(classes)} parts; train {len(train)}, test {len(test)}")
    print(f"test chapters: {len({c.doc for c in test})} of {len({c.doc for c in chunks})}")
    print(f"embedder: {embedder.name}\n")

    # Only the passage text: the heading contains the chapter number, which
    # would give the answer away.
    start = time.perf_counter()
    x_train = np.asarray(embedder.embed([c.text for c in train]), dtype=np.float64)
    x_test = np.asarray(embedder.embed([c.text for c in test]), dtype=np.float64)
    embed_s = time.perf_counter() - start
    y_train = np.array([index[label(c)] for c in train], dtype=np.int64)
    gold = [label(c) for c in test]

    majority = Counter(label(c) for c in train).most_common(1)[0][0]
    baseline = accuracy([majority] * len(gold), gold)
    print(f"always guess the biggest part ({majority}): {baseline:.0%}")

    start = time.perf_counter()
    head = SoftmaxRegression().fit(x_train, y_train, len(classes))
    train_s = time.perf_counter() - start
    pred = [classes[i] for i in head.predict(x_test)]
    train_acc = accuracy([classes[i] for i in head.predict(x_train)], [label(c) for c in train])
    print(f"trained head: test {accuracy(pred, gold):.0%} (train {train_acc:.0%})")
    print(f"  embedding {len(chunks)} passages: {embed_s:.1f}s, training: {train_s:.1f}s")

    confusions = Counter((g, p) for g, p in zip(gold, pred, strict=True) if g != p)
    print("  most common mistakes (true -> predicted):")
    for (g, p), n in confusions.most_common(5):
        print(f"    {g} -> {p}: {n}")

    print("\nlearning curve (trained head, test accuracy):")
    rng = random.Random(7)
    order = list(range(len(train)))
    rng.shuffle(order)
    for frac in (0.1, 0.25, 0.5, 1.0):
        take = np.array(order[: max(len(classes), int(frac * len(train)))])
        m = SoftmaxRegression().fit(x_train[take], y_train[take], len(classes))
        acc = accuracy([classes[i] for i in m.predict(x_test)], gold)
        print(f"  {len(take):>4} examples: {acc:.0%}")

    if llm_sample:
        llm = get_llm()
        step = max(1, len(test) // llm_sample)
        picks = list(range(0, len(test), step))[:llm_sample]
        print(f"\nzero-shot prompt on {len(picks)} test passages ({llm.name}/{llm.model}):")
        start = time.perf_counter()
        answers = [ask_model(llm, test[i].text, titles) for i in picks]
        llm_s = time.perf_counter() - start
        sub_gold = [gold[i] for i in picks]
        head_acc = accuracy([pred[i] for i in picks], sub_gold)
        each = llm_s / len(picks)
        print(f"  prompt:       {accuracy(answers, sub_gold):.0%}  ({each:.2f}s each)")
        print(f"  trained head: {head_acc:.0%}  (same passages)")
        wrong = Counter(a for a, g in zip(answers, sub_gold, strict=True) if a != g)
        print(f"  the prompt's wrong answers, by what it said: {dict(wrong.most_common(5))}")
    return 0


if __name__ == "__main__":
    sys.exit(main())

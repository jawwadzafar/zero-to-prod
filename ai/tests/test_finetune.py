from __future__ import annotations

import numpy as np

from snipai.embed import Chunk
from snipai.finetune import SoftmaxRegression, part_titles, split_by_chapter


def test_softmax_regression_learns_separable_classes() -> None:
    rng = np.random.default_rng(0)
    centres = np.array([[5.0, 0.0], [0.0, 5.0], [-5.0, -5.0]])
    y = np.repeat(np.arange(3), 30)
    x = centres[y] + rng.normal(size=(90, 2))
    model = SoftmaxRegression().fit(x, y, classes=3)
    assert (model.predict(x) == y).mean() > 0.95
    assert list(model.predict(centres)) == [0, 1, 2]


def test_split_holds_out_whole_chapters_from_every_part() -> None:
    chunks = [
        Chunk(doc=f"{part}/ch{i}", heading="h", text=f"{part} {i} {j}")
        for part in ("go", "git")
        for i in range(5)
        for j in range(3)
    ]
    train, test = split_by_chapter(chunks)
    train_docs, test_docs = {c.doc for c in train}, {c.doc for c in test}
    assert train_docs.isdisjoint(test_docs)  # no chapter on both sides
    assert {d.split("/")[0] for d in test_docs} == {"go", "git"}
    assert len(train) + len(test) == len(chunks)


def test_part_titles_come_from_the_curriculum() -> None:
    titles = part_titles()
    assert titles["kubernetes"] == "Kubernetes" and "ai" in titles

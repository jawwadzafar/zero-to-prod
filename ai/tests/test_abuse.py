from __future__ import annotations

import numpy as np

from snipai.abuse import (
    FEATURE_NAMES,
    LogisticRegression,
    features,
    make_dataset,
    score,
    train_test_split,
)


def test_features_notice_what_a_human_would() -> None:
    f = dict(zip(FEATURE_NAMES, features("https://examplebank.com@192.0.2.10/verify"), strict=True))
    assert f["has @ in the address"] == 1.0
    assert f["host is an IP address"] == 1.0
    assert f["bait words"] == 1.0
    assert f["uses https"] == 1.0
    g = dict(zip(FEATURE_NAMES, features("https://go.dev/doc/"), strict=True))
    assert g["host is an IP address"] == 0.0 and g["bait words"] == 0.0


def test_dataset_is_reproducible() -> None:
    a, ya = make_dataset(n=200, seed=1)
    b, yb = make_dataset(n=200, seed=1)
    assert a == b and np.array_equal(ya, yb)


def test_score_counts() -> None:
    s = score(np.array([1, 1, 0, 0]), np.array([1, 0, 1, 0]))
    assert (s.tp, s.fn, s.fp, s.tn) == (1, 1, 1, 1)
    assert s.precision == 0.5 and s.recall == 0.5 and s.accuracy == 0.5


def test_training_learns_and_beats_the_lazy_baseline() -> None:
    urls, y = make_dataset()
    train, test = train_test_split(len(urls))
    x = np.stack([features(u) for u in urls])
    model = LogisticRegression().fit(x[train], y[train])
    assert model.losses[-1] < model.losses[0] / 2  # training reduced the loss
    lazy = score(y[test], np.zeros(len(test), dtype=np.int64))
    learned = score(y[test], model.predict(x[test]))
    assert learned.accuracy > lazy.accuracy
    assert learned.recall > 0.5  # it catches most phishing, where the lazy one catches none

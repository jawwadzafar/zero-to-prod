from __future__ import annotations

import json

import httpx
import numpy as np

from snipai.embed import Chunk, HashEmbedder, Index, OllamaEmbedder, chunk_handbook


def test_hash_embeddings_are_unit_length_and_lexical() -> None:
    v = HashEmbedder().embed(["roll back the deploy", "roll back a deploy now", "undo a release"])
    assert np.allclose(np.linalg.norm(v, axis=1), 1.0)
    shared_words = float(v[0] @ v[1])
    same_meaning_no_shared_words = float(v[0] @ v[2])
    assert shared_words > 0.8 and same_meaning_no_shared_words == 0.0  # meaning is invisible to it


def test_index_returns_the_closest_chunks_first() -> None:
    chunks = [
        Chunk("a", "Redis", "Redis is an in-memory cache."),
        Chunk("b", "Postgres", "Postgres stores rows in tables."),
        Chunk("c", "Kubernetes", "Kubernetes restarts crashed pods."),
    ]
    index = Index.build(HashEmbedder(), chunks, cache=False)
    hits = index.search("which cache is in memory", k=2)
    assert hits[0].chunk.doc == "a" and hits[0].score > hits[1].score


def test_handbook_chunks_are_clean_and_labelled() -> None:
    chunks = chunk_handbook()
    assert len(chunks) > 300
    assert all(
        len(c.text) <= 1500 + 400 for c in chunks
    )  # a paragraph can push a piece over a little
    assert not any("```" in c.text or "<Quiz" in c.text for c in chunks)
    assert any(c.heading.startswith("14.7") for c in chunks) or any(
        "13.5" in c.heading for c in chunks
    )


def test_ollama_embedder_batches_and_normalises() -> None:
    calls: list[int] = []

    def handler(request: httpx.Request) -> httpx.Response:
        texts = json.loads(request.content)["input"]
        calls.append(len(texts))
        return httpx.Response(200, json={"embeddings": [[3.0, 4.0] for _ in texts]})

    v = OllamaEmbedder(transport=httpx.MockTransport(handler)).embed(["x"] * 70)
    assert calls == [64, 6] and v.shape == (70, 2)
    assert np.allclose(v[0], [0.6, 0.8])

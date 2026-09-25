"""Embeddings and vector search over this handbook (chapter 14.7).

An embedding turns text into a list of numbers (a vector) such that texts
with similar *meaning* get similar vectors. Search then means: embed the
question, and find the stored vectors closest to it.

Two embedders, behind one interface:
    hash    Offline and free: counts words into a fixed-size vector. It only
            matches shared words, not meaning, which makes a useful contrast.
    ollama  A real embedding model running locally (e.g. all-minilm, 46 MB).

    uv run python -m snipai.embed "how do I undo a bad deploy?"
    SNIPAI_EMBEDDER=ollama uv run python -m snipai.embed "how do I undo a bad deploy?"
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import sys
import time
import zlib
from dataclasses import dataclass
from pathlib import Path
from typing import Protocol

import httpx
import numpy as np
from numpy.typing import NDArray

Vectors = NDArray[np.float32]
DOCS = Path(__file__).resolve().parents[3] / "docs"
CACHE = Path(__file__).resolve().parents[2] / ".cache"


class Embedder(Protocol):
    name: str

    def embed(self, texts: list[str]) -> Vectors:
        """One row per text, each normalised to length 1."""
        ...


def normalise(v: Vectors) -> Vectors:
    norms = np.linalg.norm(v, axis=1, keepdims=True)
    out: Vectors = (v / np.maximum(norms, 1e-12)).astype(np.float32)
    return out


WORD = re.compile(r"[a-z0-9]+")
# Words too common to say anything about a text's topic.
STOP = {
    "a", "an", "and", "are", "as", "at", "be", "by", "can", "do", "does", "for", "from", "how",
    "i", "in", "is", "it", "my", "of", "on", "or", "that", "the", "this", "to", "what", "with",
    "you", "your",
}  # fmt: skip


class HashEmbedder:
    """Bag of words, hashed into `dims` buckets: texts that share words get
    similar vectors. Knows nothing about meaning ("undo" and "roll back" are
    unrelated to it), which is exactly what a real embedding model fixes."""

    def __init__(self, dims: int = 1024) -> None:
        self.dims = dims
        self.name = f"hash-{dims}"

    def embed(self, texts: list[str]) -> Vectors:
        out = np.zeros((len(texts), self.dims), dtype=np.float32)
        for i, text in enumerate(texts):
            for word in WORD.findall(text.lower()):
                if word not in STOP:
                    out[i, zlib.crc32(word.encode()) % self.dims] += 1.0
        return normalise(np.log1p(out))  # log: a word said 10 times isn't 10x as important


class OllamaEmbedder:
    """A real embedding model through Ollama's /api/embed."""

    def __init__(
        self,
        model: str = "all-minilm",
        *,
        base_url: str = "http://localhost:11434",
        transport: httpx.BaseTransport | None = None,
    ) -> None:
        self.name = model
        self._http = httpx.Client(base_url=base_url, timeout=120.0, transport=transport)

    def embed(self, texts: list[str]) -> Vectors:
        rows: list[list[float]] = []
        for start in range(0, len(texts), 64):  # send in batches
            res = self._http.post(
                "/api/embed", json={"model": self.name, "input": texts[start : start + 64]}
            )
            res.raise_for_status()
            rows += res.json()["embeddings"]
        return normalise(np.array(rows, dtype=np.float32))


def get_embedder() -> Embedder:
    kind = os.environ.get("SNIPAI_EMBEDDER", "hash")
    if kind == "hash":
        return HashEmbedder()
    if kind == "ollama":
        url = os.environ.get("OLLAMA_URL", "http://localhost:11434")
        return OllamaEmbedder(os.environ.get("SNIPAI_EMBED_MODEL", "all-minilm"), base_url=url)
    raise ValueError(f"SNIPAI_EMBEDDER must be hash or ollama, not {kind!r}")


# ---------------------------------------------------------------------------
# Chunking: split the handbook into passages worth retrieving
# ---------------------------------------------------------------------------


@dataclass
class Chunk:
    doc: str  # e.g. "shipping/delivery-and-deployment"
    heading: str  # the chapter title plus section heading
    text: str

    @property
    def embed_text(self) -> str:
        # Headings carry a lot of meaning; embed them with the passage.
        return f"{self.heading}\n{self.text}"


def clean_mdx(text: str) -> str:
    text = re.sub(r"\A---\n.*?\n---\n", "", text, flags=re.S)
    text = re.sub(r"```.*?```", " ", text, flags=re.S)
    text = re.sub(r"<Quiz.*?/>", " ", text, flags=re.S)
    text = re.sub(r"</?[A-Za-z][^>]*>", " ", text)
    text = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", text)
    text = re.sub(r"^:::.*$", " ", text, flags=re.M)
    return re.sub(r"[*`>|]+", "", text)


def chunk_handbook(docs: Path = DOCS, max_chars: int = 1500) -> list[Chunk]:
    """One chunk per section (split at '## ' headings), long sections split
    into pieces of at most max_chars at paragraph boundaries."""
    chunks: list[Chunk] = []
    for path in sorted(docs.glob("*/*.mdx")):
        doc = f"{path.parent.name}/{path.stem}"
        raw = path.read_text(encoding="utf-8")
        title_match = re.search(r'^title:\s*"?(.*?)"?\s*$', raw, re.M)
        title = title_match.group(1).replace('\\"', '"') if title_match else doc
        body = re.sub(r"^# .*$", "", clean_mdx(raw), flags=re.M)  # the chapter's H1 title line
        for section in re.split(r"^## ", body, flags=re.M):
            heading, _, rest = section.partition("\n")
            heading = heading.strip("# ").strip()
            if heading.startswith(
                title.split(" ", 1)[0]
            ):  # the "# 1.2 Title" part before the first ##
                heading = ""
            label = f"{title} > {heading}" if heading else title
            if heading in ("Self-check", "What you'll learn"):
                continue
            piece = ""
            for para in re.split(r"\n\s*\n", rest):
                para = " ".join(para.split())
                if not para:
                    continue
                if piece and len(piece) + len(para) > max_chars:
                    chunks.append(Chunk(doc, label, piece))
                    piece = ""
                piece = f"{piece} {para}".strip()
            if len(piece) > 80:
                chunks.append(Chunk(doc, label, piece))
    return chunks


# ---------------------------------------------------------------------------
# The index: vectors in memory, cosine similarity by matrix multiplication
# ---------------------------------------------------------------------------


@dataclass
class Hit:
    chunk: Chunk
    score: float  # cosine similarity: 1 = same direction, 0 = unrelated


class Index:
    def __init__(self, embedder: Embedder, chunks: list[Chunk], vectors: Vectors) -> None:
        self.embedder, self.chunks, self.vectors = embedder, chunks, vectors

    @classmethod
    def build(cls, embedder: Embedder, chunks: list[Chunk], *, cache: bool = True) -> Index:
        """Embed every chunk, reusing a cached copy when nothing has changed."""
        key = hashlib.sha256(
            (embedder.name + "".join(c.embed_text for c in chunks)).encode()
        ).hexdigest()[:16]
        path = CACHE / f"index-{key}.npy"
        if cache and path.exists():
            return cls(embedder, chunks, np.load(path))
        vectors = embedder.embed([c.embed_text for c in chunks])
        if cache:
            CACHE.mkdir(exist_ok=True)
            np.save(path, vectors)
        return cls(embedder, chunks, vectors)

    def search(self, query: str, k: int = 5) -> list[Hit]:
        q = self.embedder.embed([query])[0]
        scores = self.vectors @ q  # vectors are length 1, so this is cosine similarity
        best = np.argsort(-scores)[:k]
        return [Hit(self.chunks[i], float(scores[i])) for i in best]


EVAL_FILE = Path(__file__).resolve().parents[2] / "data" / "search_eval.json"


def evaluate(index: Index, path: Path = EVAL_FILE) -> tuple[float, float, list[str]]:
    """hit@1 and hit@5: the share of questions whose top result (or any of the
    top five) comes from a chapter that answers it. Returns misses too."""
    queries = json.loads(path.read_text(encoding="utf-8"))["queries"]
    at1 = at5 = 0
    misses = []
    for item in queries:
        docs = [h.chunk.doc for h in index.search(item["q"], k=5)]
        at1 += docs[0] in item["docs"]
        found = any(d in item["docs"] for d in docs)
        at5 += found
        if not found:
            misses.append(f"{item['q']!r} -> {docs[0]}")
    return at1 / len(queries), at5 / len(queries), misses


def main(argv: list[str] | None = None) -> int:
    queries = (argv if argv is not None else sys.argv[1:]) or [
        "how do I undo a bad deploy?",
        "my program keeps restarting in kubernetes",
        "why is my website slow",
    ]
    embedder = get_embedder()
    chunks = chunk_handbook()
    start = time.perf_counter()
    index = Index.build(embedder, chunks)
    print(f"embedder: {embedder.name}, {len(chunks)} chunks, dimensions: {index.vectors.shape[1]}")
    print(f"indexed in {time.perf_counter() - start:.1f}s\n")
    for q in queries:
        print(f"query: {q}")
        for hit in index.search(q, k=5):
            print(f"  {hit.score:.3f}  {hit.chunk.heading[:90]}")
        print()
    if not (argv if argv is not None else sys.argv[1:]):
        hit1, hit5, misses = evaluate(index)
        print(f"retrieval quality on {EVAL_FILE.name}: hit@1 {hit1:.0%}, hit@5 {hit5:.0%}")
        for m in misses:
            print(f"  missed: {m}")
    return 0


if __name__ == "__main__":
    sys.exit(main())

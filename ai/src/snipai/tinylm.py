"""A tiny language model trained on this handbook, to see how LLMs work (chapter 14.3).

Two real pieces, small enough to read in one sitting:

1. A byte-pair-encoding (BPE) tokenizer: learns which chunks of text are
   common, and splits any text into those chunks (tokens). Real LLMs use
   the same algorithm, with vocabularies of ~100,000 tokens.
2. An n-gram model: predicts the next token from counts of what followed the
   previous few tokens in the training text, then samples from those
   probabilities, with a temperature. Real LLMs replace the counting with a
   neural network (a transformer) that looks at thousands of previous tokens,
   but they are used exactly the same way: predict, sample, append, repeat.

    uv run python -m snipai.tinylm
"""

from __future__ import annotations

import math
import random
import re
from collections import Counter, defaultdict
from collections.abc import Iterable
from pathlib import Path

# Split text into words, keeping each word's leading space (as GPT-style
# tokenizers do), so " the" and "the" can become different tokens.
PRETOKENIZE = re.compile(r" ?[A-Za-z]+| ?[0-9]+| ?[^\sA-Za-z0-9]+|\s+")


# ---------------------------------------------------------------------------
# 1. Tokenizer: byte-pair encoding, trained from scratch
# ---------------------------------------------------------------------------


class BPETokenizer:
    """Starts from single characters and repeatedly merges the most frequent
    adjacent pair into a new token, `merges` times."""

    def __init__(self) -> None:
        self.merges: list[tuple[str, str]] = []
        self.vocab: dict[str, int] = {}
        self._cache: dict[str, list[str]] = {}

    def train(self, text: str, merges: int = 1000) -> BPETokenizer:
        words = Counter(PRETOKENIZE.findall(text))
        splits = {w: list(w) for w in words}
        for _ in range(merges):
            pairs: Counter[tuple[str, str]] = Counter()
            for w, freq in words.items():
                parts = splits[w]
                for a, b in zip(parts, parts[1:], strict=False):
                    pairs[a, b] += freq
            if not pairs:
                break
            best = max(pairs, key=lambda p: pairs[p])  # the most frequent pair
            self.merges.append(best)
            for w, parts in splits.items():
                if len(parts) > 1:
                    splits[w] = _merge(parts, best)
        symbols = {c for w in words for c in w} | {a + b for a, b in self.merges}
        self.vocab = {tok: i for i, tok in enumerate(sorted(symbols))}
        self._cache = {}
        return self

    def tokenize(self, text: str) -> list[str]:
        out: list[str] = []
        for word in PRETOKENIZE.findall(text):
            if word not in self._cache:
                parts = list(word)
                for pair in self.merges:  # apply merges in the order they were learned
                    if len(parts) == 1:
                        break
                    parts = _merge(parts, pair)
                self._cache[word] = parts
            out.extend(self._cache[word])
        return out

    def encode(self, text: str) -> list[int]:
        # Characters never seen in training have no id; real tokenizers fall
        # back to raw bytes so that every possible input can be encoded.
        return [self.vocab[t] for t in self.tokenize(text) if t in self.vocab]


def _merge(parts: list[str], pair: tuple[str, str]) -> list[str]:
    a, b = pair
    out, i = [], 0
    while i < len(parts):
        if i + 1 < len(parts) and parts[i] == a and parts[i + 1] == b:
            out.append(a + b)
            i += 2
        else:
            out.append(parts[i])
            i += 1
    return out


# ---------------------------------------------------------------------------
# 2. Language model: predict the next token from the previous ones
# ---------------------------------------------------------------------------


class NGramModel:
    """Counts which token followed each run of `context` tokens in training.
    With context=2, it predicts from the previous 2 tokens only."""

    def __init__(self, context: int = 2) -> None:
        self.context = context
        # counts[k][previous k tokens] -> Counter of next tokens, for k = context..1
        self.counts: dict[int, defaultdict[tuple[str, ...], Counter[str]]] = {
            k: defaultdict(Counter) for k in range(1, context + 1)
        }

    def train(self, tokens: list[str]) -> NGramModel:
        for i in range(1, len(tokens)):
            for k in range(1, self.context + 1):
                if i - k >= 0:
                    self.counts[k][tuple(tokens[i - k : i])][tokens[i]] += 1
        return self

    def next_token_probs(self, history: list[str]) -> dict[str, float]:
        """Probabilities for the next token. If this exact context never
        appeared in training, back off to a shorter one."""
        for k in range(min(self.context, len(history)), 0, -1):
            seen = self.counts[k].get(tuple(history[-k:]))
            if seen:
                total = sum(seen.values())
                return {tok: n / total for tok, n in seen.most_common()}
        return {}

    def generate(
        self,
        prompt: list[str],
        max_tokens: int = 40,
        temperature: float = 1.0,
        top_k: int | None = None,
        seed: int | None = None,
    ) -> list[str]:
        """Predict, sample, append, repeat: how every LLM produces text."""
        rng = random.Random(seed)
        out = list(prompt)
        for _ in range(max_tokens):
            probs = self.next_token_probs(out)
            if not probs:
                break
            out.append(sample(probs, temperature, top_k, rng))
        return out[len(prompt) :]


def sample(
    probs: dict[str, float], temperature: float, top_k: int | None, rng: random.Random
) -> str:
    """Pick one token. temperature 0 = always the most likely; higher = more
    adventurous. top_k keeps only the k most likely candidates."""
    items = sorted(probs.items(), key=lambda kv: -kv[1])
    if top_k:
        items = items[:top_k]
    if temperature == 0:
        return items[0][0]
    # Temperature reshapes the distribution: p ** (1/T), then renormalise.
    weights = [math.exp(math.log(p) / temperature) for _, p in items]
    return rng.choices([tok for tok, _ in items], weights=weights, k=1)[0]


# ---------------------------------------------------------------------------
# Training data: the handbook itself
# ---------------------------------------------------------------------------


def handbook_text(docs: Path) -> str:
    """The prose of every chapter, without front matter, code blocks, JSX."""
    chunks = []
    for path in sorted(docs.glob("*/*.mdx")):
        text = path.read_text(encoding="utf-8")
        text = re.sub(r"\A---\n.*?\n---\n", "", text, flags=re.S)  # front matter
        text = re.sub(r"```.*?```", "", text, flags=re.S)  # code blocks
        text = re.sub(r"<Quiz.*?/>", "", text, flags=re.S)  # quizzes
        text = re.sub(r"</?[A-Za-z][^>]*>", "", text)  # other tags
        text = re.sub(r"[*`#>|]+", "", text)  # markdown punctuation
        text = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", text)  # links -> their text
        chunks.append(text)
    return re.sub(r"\s+", " ", "\n".join(chunks))


def show(tokens: Iterable[str]) -> str:
    return "|".join(tokens)


def main() -> None:
    docs = Path(__file__).resolve().parents[3] / "docs"
    text = handbook_text(docs)
    print(f"Training text: the handbook's prose, {len(text):,} characters")

    tok = BPETokenizer().train(text, merges=1000)
    tokens = tok.tokenize(text)
    print(f"\n1. Tokenizer: {len(tok.vocab):,} tokens in the vocabulary after 1,000 merges")
    per_token = len(text) / len(tokens)
    print(f"   the handbook is {len(tokens):,} tokens: {per_token:.1f} characters per token")
    print("   first merges learned:", ", ".join(repr(a + b) for a, b in tok.merges[:12]))
    for sample_text in [
        "The server returns a 302 redirect.",
        "Kubernetes restarts the Pod.",
        "Tokenization of antidisestablishmentarianism",
        "snip, snip, snip",
    ]:
        print(f"   {sample_text!r} -> {show(tok.tokenize(sample_text))}")

    # In running text "The" follows a space, so the prompt starts with one too;
    # otherwise its tokens (and so its context) wouldn't match training.
    prompt_text = " The worker"
    prompt = tok.tokenize(prompt_text)
    for context in (1, 2, 4):
        model = NGramModel(context=context).train(tokens)
        print(f"\n2.{context} Next-token model, context = {context} token(s)")
        probs = model.next_token_probs(prompt)
        top = ", ".join(f"{t!r} {p:.0%}" for t, p in list(probs.items())[:5])
        print(f"   after {show(prompt)!r}, most likely next: {top}")
        for temperature in (0.0, 0.8, 1.5):
            out = model.generate(prompt, max_tokens=30, temperature=temperature, seed=1)
            print(f"   T={temperature}:{prompt_text}{''.join(out)}")
        # A longer sample (80 tokens, T=0.8): how much of it is copied from the handbook?
        long = prompt_text + "".join(model.generate(prompt, 80, temperature=0.8, seed=2))
        copied = _longest_copied_run(long, text)
        print(f"   longest run copied word for word from the handbook: {copied} characters")


def _longest_copied_run(generated: str, source: str) -> int:
    """Length of the longest substring of `generated` found verbatim in `source`."""
    best = 0
    for i in range(len(generated)):
        lo, hi = best + 1, len(generated) - i
        while lo <= hi:  # binary search on the length of a match starting at i
            mid = (lo + hi) // 2
            if generated[i : i + mid] in source:
                best, lo = mid, mid + 1
            else:
                hi = mid - 1
    return best


if __name__ == "__main__":
    main()

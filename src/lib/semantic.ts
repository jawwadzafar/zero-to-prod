// Search by meaning for "Ask the handbook", entirely in the browser.
//
// At build time scripts/build-ask-vectors.mjs embeds every passage with a
// small open model; here the reader's question is embedded with the same model
// and compared with each passage (cosine similarity, a dot product because
// both sides are normalised). The model (~23 MB) is downloaded only when the
// reader turns this on, then cached by the browser.

// The library comes from a CDN at runtime instead of the site bundle: it's
// large and only needed by readers who opt in.
const LIBRARY = 'https://cdn.jsdelivr.net/npm/@huggingface/transformers@4.3.1/dist/transformers.min.js';

type Meta = {model: string; dtype: string; dims: number; count: number};
export type Vectors = {meta: Meta; data: Int8Array};
type Embedder = (text: string) => Promise<Float32Array>;

let embedder: Promise<Embedder> | null = null;

/** Loads the embedding model once; later calls share the same promise. */
export function loadEmbedder(meta: Meta): Promise<Embedder> {
  embedder ??= (async () => {
    const {pipeline} = await import(/* webpackIgnore: true */ LIBRARY);
    const extract = await pipeline('feature-extraction', meta.model, {dtype: meta.dtype});
    return async (text: string) => (await extract(text, {pooling: 'mean', normalize: true})).data as Float32Array;
  })();
  embedder.catch(() => (embedder = null)); // allow a retry after a failed download
  return embedder;
}

/** The passage vectors, or null when the site was built without them. */
export async function loadVectors(metaUrl: string, binUrl: string, passages: number): Promise<Vectors | null> {
  const metaRes = await fetch(metaUrl);
  if (!metaRes.ok) return null;
  const meta: Meta = await metaRes.json();
  // Vectors are stored in passage order; a mismatch would pair them wrongly.
  if (meta.count !== passages) return null;
  const bin = await fetch(binUrl);
  if (!bin.ok) return null;
  const data = new Int8Array(await bin.arrayBuffer());
  return data.length === meta.count * meta.dims ? {meta, data} : null;
}

/** Passage indexes, most similar to the question first. */
export function rankByMeaning(question: Float32Array, vectors: Vectors, limit: number): number[] {
  const {dims, count} = vectors.meta;
  const scores = new Float32Array(count);
  for (let i = 0; i < count; i++) {
    let s = 0;
    const off = i * dims;
    for (let d = 0; d < dims; d++) s += question[d] * vectors.data[off + d];
    scores[i] = s;
  }
  return Array.from(scores.keys())
    .sort((a, b) => scores[b] - scores[a])
    .slice(0, limit);
}

/**
 * Reciprocal rank fusion: merges ranked lists by summing 1 / (60 + rank), so
 * a passage that both keyword and meaning search rank well comes first,
 * without having to make their scores comparable.
 */
export function fuse<T>(lists: T[][]): T[] {
  const score = new Map<T, number>();
  for (const list of lists) list.forEach((item, rank) => score.set(item, (score.get(item) ?? 0) + 1 / (60 + rank)));
  return [...score.keys()].sort((a, b) => score.get(b)! - score.get(a)!);
}

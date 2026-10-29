// Builds static/ask-vectors.bin: one embedding per passage in
// static/ask-index.json (same order), so "Ask the handbook" can search by
// meaning, not only by shared words. The browser embeds the question with the
// same model (src/lib/semantic.ts) and compares.
//
// Optional by design: it needs @huggingface/transformers, which is not a
// project dependency (its native runtime is ~550 MB). The deploy workflow
// installs it with `npm i --no-save`; everywhere else this script skips, and
// Ask falls back to keyword search. It also skips when the passages haven't
// changed since the last run, so `npm start` stays fast.
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

export const MODEL = 'Xenova/all-MiniLM-L6-v2'; // 384 dimensions, ~23 MB at 8-bit
const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..');
const indexFile = path.join(root, 'static', 'ask-index.json');
const binFile = path.join(root, 'static', 'ask-vectors.bin');
const metaFile = path.join(root, 'static', 'ask-vectors.json');

const passages = JSON.parse(fs.readFileSync(indexFile, 'utf8'));
// What gets embedded: where the passage sits, then its text.
const texts = passages.map((p) => `${p.chapter} › ${p.heading}\n${p.text}`);
const hash = crypto.createHash('sha256').update(MODEL).update(JSON.stringify(texts)).digest('hex').slice(0, 16);

function skip(reason) {
  // A stale or missing file must never pair vectors with the wrong passages.
  for (const f of [binFile, metaFile]) fs.rmSync(f, {force: true});
  console.log(`ask-vectors: skipped (${reason}); Ask will use keyword search only`);
  process.exit(0);
}

try {
  const meta = JSON.parse(fs.readFileSync(metaFile, 'utf8'));
  if (meta.hash === hash && fs.existsSync(binFile)) {
    console.log(`ask-vectors: up to date (${meta.count} passages)`);
    process.exit(0);
  }
} catch {
  /* no previous run */
}

let pipeline;
try {
  ({pipeline} = await import('@huggingface/transformers'));
} catch {
  skip('@huggingface/transformers not installed');
}

let embed;
try {
  embed = await pipeline('feature-extraction', MODEL, {dtype: 'q8'});
} catch (e) {
  skip(`could not load ${MODEL}: ${e.message}`);
}

const dims = 384;
const out = new Int8Array(texts.length * dims);
const started = Date.now();
for (let i = 0; i < texts.length; i += 32) {
  const batch = texts.slice(i, i + 32);
  const t = await embed(batch, {pooling: 'mean', normalize: true});
  const data = t.data; // batch.length × dims floats in [-1, 1]
  // Stored as signed bytes: 4× smaller, and cosine similarity barely moves.
  for (let j = 0; j < data.length; j++) out[i * dims + j] = Math.max(-127, Math.min(127, Math.round(data[j] * 127)));
}
fs.writeFileSync(binFile, out);
fs.writeFileSync(metaFile, JSON.stringify({model: MODEL, dtype: 'q8', dims, count: texts.length, hash}));
console.log(`ask-vectors: ${texts.length} passages embedded in ${Math.round((Date.now() - started) / 1000)}s → static/ask-vectors.bin (${Math.round(out.length / 1024)} KB)`);

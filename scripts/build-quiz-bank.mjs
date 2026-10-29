// Builds static/quiz-bank.json: every question from every chapter's <Quiz>,
// so the /review page can bring questions back for spaced repetition without
// any server. Runs before `start` and `build`, after build-ask-index.
//
// Each question is keyed by its quiz id plus a hash of its text (see
// src/lib/review.ts), so reordering a quiz doesn't scramble anyone's review
// schedule; rewording a question starts it fresh.
import fs from 'node:fs';
import path from 'node:path';

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..');
const curriculum = JSON.parse(fs.readFileSync(path.join(root, 'src/data/curriculum.json'), 'utf8'));

// Same FNV-1a hash as questionKey() in src/lib/review.ts.
function hash(text) {
  let h = 0x811c9dc5;
  for (let i = 0; i < text.length; i++) {
    h ^= text.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return (h >>> 0).toString(36);
}

// Returns the source text of the balanced [...] that starts at `start`,
// skipping brackets inside string literals.
function balanced(src, start) {
  let depth = 0;
  let quote = null;
  for (let i = start; i < src.length; i++) {
    const c = src[i];
    if (quote) {
      if (c === '\\') i++;
      else if (c === quote) quote = null;
      continue;
    }
    if (c === "'" || c === '"' || c === '`') quote = c;
    else if (c === '[' || c === '{') depth++;
    else if (c === ']' || c === '}') {
      depth--;
      if (depth === 0) return src.slice(start, i + 1);
    }
  }
  throw new Error('unbalanced questions array');
}

const bank = [];
for (const part of curriculum.parts) {
  part.chapters.forEach((ch, idx) => {
    const file = path.join(root, 'docs', part.id, `${ch.id}.mdx`);
    if (!fs.existsSync(file)) return;
    const src = fs.readFileSync(file, 'utf8');
    const doc = `${part.id}/${ch.id}`;
    const chapter = `${part.n}.${idx + 1} ${ch.title}`;
    for (const m of src.matchAll(/<Quiz\b/g)) {
      const tag = src.slice(m.index, src.indexOf('/>', m.index));
      const id = tag.match(/\bid="([^"]+)"/)?.[1];
      const at = tag.indexOf('questions={');
      if (!id || at < 0) throw new Error(`${doc}: <Quiz> without id or questions`);
      const expr = balanced(src, m.index + at + 'questions={'.length);
      // Chapter files are this repo's own trusted content.
      const questions = new Function(`return (${expr});`)();
      for (const item of questions) {
        bank.push({key: `${id}#${hash(item.q)}`, quiz: id, doc, chapter, ...item});
      }
    }
  });
}

const keys = new Set(bank.map((b) => b.key));
if (keys.size !== bank.length) throw new Error('two questions in one quiz have the same text');

const out = path.join(root, 'static', 'quiz-bank.json');
fs.writeFileSync(out, JSON.stringify(bank));
console.log(`quiz-bank: ${bank.length} questions from ${new Set(bank.map((b) => b.quiz)).size} quizzes → static/quiz-bank.json`);

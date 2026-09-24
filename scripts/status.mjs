// Prints how many chapters are written per part: node scripts/status.mjs
import fs from 'node:fs';
import path from 'node:path';
const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..');
const c = JSON.parse(fs.readFileSync(path.join(root, 'src/data/curriculum.json'), 'utf8'));
let done = 0, total = 0;
for (const p of c.parts) {
  const missing = [];
  let n = 0;
  p.chapters.forEach((ch, i) => {
    const ok = ['.mdx', '.md'].some((e) => fs.existsSync(path.join(root, 'docs', p.id, ch.id + e)));
    if (ok) n++; else missing.push(`${p.n}.${i + 1} ${ch.id}`);
  });
  done += n; total += p.chapters.length;
  console.log(`${String(p.n).padStart(2)} ${p.title.padEnd(34)} ${n}/${p.chapters.length}${missing.length ? '  todo: ' + missing.join(', ') : ''}`);
}
console.log(`\n${done}/${total} chapters written`);

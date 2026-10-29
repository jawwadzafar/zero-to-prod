#!/usr/bin/env node
// Turns bare cross-references in prose ("chapter 9.3", "chapters 6.2 and 6.5",
// "Parts 9–11") into links to the chapter (or the first chapter of the part),
// so a reader can always click through to what is being referred to.
//
//   node scripts/link-refs.mjs          rewrite docs/ in place
//   node scripts/link-refs.mjs --check  exit 1 if any bare reference remains (CI)
//
// Skipped: front matter, fenced code, inline code, existing links, JSX/HTML
// lines (Quiz props, <summary>), headings, and a chapter's references to itself.
import fs from 'node:fs';
import path from 'node:path';

const root = path.join(path.dirname(new URL(import.meta.url).pathname), '..');
const docsDir = path.join(root, 'docs');
const check = process.argv.includes('--check');

// "9.3" -> "containers/docker-compose.mdx", from each chapter's numbered title.
const byNumber = new Map();
const files = [];
for (const part of fs.readdirSync(docsDir)) {
  const dir = path.join(docsDir, part);
  if (!fs.statSync(dir).isDirectory()) continue;
  for (const f of fs.readdirSync(dir)) {
    if (!f.endsWith('.mdx')) continue;
    const rel = `${part}/${f}`;
    files.push(rel);
    const m = fs.readFileSync(path.join(dir, f), 'utf8').match(/^title:\s*"?(\d+)\.(\d+)\s/m);
    if (m) byNumber.set(`${m[1]}.${m[2]}`, rel);
  }
}
const partStart = (n) => byNumber.get(`${n}.1`);

const REF = /\b([Cc]hapters?|[Pp]arts?)(\s+)(\d+(?:\.\d+)?(?:(?:\s*(?:,|–|-|&|\bto\b|\band\b|\bor\b)\s*)+\d+(?:\.\d+)?)*)(?![\d.]*\d)/g;
const NUM = /\d+(?:\.\d+)?/g;

let changed = 0;
const bare = [];

for (const rel of files) {
  const file = path.join(docsDir, rel);
  const src = fs.readFileSync(file, 'utf8');
  const lines = src.split('\n');
  let inFront = lines[0] === '---';
  let fence = null;
  let inJsx = false;
  for (let i = inFront ? 1 : 0; i < lines.length; i++) {
    const line = lines[i];
    if (inFront) { if (line === '---') inFront = false; continue; }
    const f = line.match(/^\s*(`{3,}|~{3,})/);
    if (f) { if (!fence) fence = f[1]; else if (line.trim().startsWith(fence)) fence = null; continue; }
    if (fence) continue;
    // Multi-line JSX blocks (<Quiz … />) hold JS strings, not Markdown.
    if (/^\s*<Quiz\b/.test(line)) inJsx = !/\/>\s*$/.test(line);
    if (inJsx) { if (/\/>\s*$/.test(line)) inJsx = false; continue; }
    if (/^\s*</.test(line) || /^#{1,6}\s/.test(line)) continue;

    // Hide inline code and existing links so they are never touched.
    const hidden = [];
    const masked = line.replace(/`[^`]*`|\[[^\]]*\]\([^)]*\)|<[^>]+>/g, (m) => {
      hidden.push(m);
      return `\u0000${hidden.length - 1}\u0000`;
    });
    const out = masked.replace(REF, (whole, word, space, nums) => {
      const linked = nums.replace(NUM, (n) => {
        const target = n.includes(".") ? byNumber.get(n) : partStart(n);
        if (!target || target === rel) return n;
        let href = path.relative(path.dirname(rel), target);
        if (!href.startsWith('.')) href = `./${href}`;
        return `[${n}](${href})`;
      });
      if (linked === nums) return whole;
      // Link the keyword too when there is a single number: "[chapter 9.3](…)".
      const single = linked.match(/^\[(\d+(?:\.\d+)?)\]\(([^)]+)\)$/);
      return single ? `[${word}${space}${single[1]}](${single[2]})` : `${word}${space}${linked}`;
    });
    const restored = out.replace(/\u0000(\d+)\u0000/g, (_, k) => hidden[Number(k)]);
    if (restored !== line) {
      changed++;
      if (check) bare.push(`${rel}:${i + 1}: ${line.trim().slice(0, 120)}`);
      lines[i] = restored;
    }
  }
  if (!check) {
    const next = lines.join('\n');
    if (next !== src) fs.writeFileSync(file, next);
  }
}

if (check) {
  if (bare.length) {
    console.error(`${bare.length} line(s) mention a chapter or part without linking it.`);
    console.error('Run `node scripts/link-refs.mjs` to link them:\n');
    for (const b of bare.slice(0, 50)) console.error('  ' + b);
    process.exit(1);
  }
  console.log('All chapter and part references are linked.');
} else {
  console.log(`Linked references on ${changed} line(s).`);
}

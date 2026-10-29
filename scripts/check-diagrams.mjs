#!/usr/bin/env node
// Parses every ```mermaid block in docs/ with the same Mermaid version the site
// ships. A diagram that fails to parse renders as a "Syntax error" bomb on the
// page, and the site build does not catch it — this does.
//
//   node scripts/check-diagrams.mjs
import fs from 'node:fs';
import path from 'node:path';

const root = path.join(path.dirname(new URL(import.meta.url).pathname), '..');
// Outside a browser DOMPurify has no window and lacks sanitize/addHook. It only
// cleans label text, not the grammar we're checking, so stub it out.
const purify = (await import(path.join(root, 'node_modules/dompurify/dist/purify.es.mjs'))).default;
Object.assign(purify, {sanitize: (s) => s, addHook() {}, removeHook() {}, removeHooks() {}, removeAllHooks() {}});
const mermaid = (await import(path.join(root, 'node_modules/mermaid/dist/mermaid.core.mjs'))).default;

const files = [];
(function walk(dir) {
  for (const f of fs.readdirSync(dir)) {
    const p = path.join(dir, f);
    if (fs.statSync(p).isDirectory()) walk(p);
    else if (/\.mdx?$/.test(f)) files.push(p);
  }
})(path.join(root, 'docs'));

let total = 0;
const bad = [];
for (const file of files) {
  const lines = fs.readFileSync(file, 'utf8').split('\n');
  for (let i = 0; i < lines.length; i++) {
    if (!/^\s*```mermaid/.test(lines[i])) continue;
    const start = i + 1;
    const body = [];
    for (i++; i < lines.length && !/^\s*```\s*$/.test(lines[i]); i++) body.push(lines[i]);
    total++;
    try {
      await mermaid.parse(body.join('\n'));
    } catch (e) {
      bad.push(`${path.relative(root, file)}:${start}\n    ${String(e.message || e).split('\n').slice(0, 3).join('\n    ')}`);
    }
  }
}

if (bad.length) {
  console.error(`${bad.length} of ${total} diagrams fail to parse:\n`);
  for (const b of bad) console.error('  ' + b + '\n');
  console.error('Common cause: a ";" inside a sequence-diagram message ends the statement.');
  process.exit(1);
}
console.log(`All ${total} diagrams parse.`);

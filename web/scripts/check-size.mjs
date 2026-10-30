// A performance budget (chapter 16.8): fail if the JavaScript a first visit
// downloads grows past a set size. Run after `npm run build`:
//
//   npm run size
//
// The number that matters is the gzip size, because that is what travels
// over the network (nginx compresses it). React and React DOM alone are about
// 69 kB gzipped; the budget leaves room for the app and makes a surprise
// dependency (a date library, a whole icon set) fail loudly instead of quietly.
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

const BUDGET_KB = 90; // gzipped JavaScript, all chunks together
const dir = fileURLToPath(new URL('../dist/assets/', import.meta.url));

let files;
try {
  files = readdirSync(dir).filter((f) => f.endsWith('.js'));
} catch {
  console.error('No dist/assets/. Run `npm run build` first.');
  process.exit(1);
}

let total = 0;
for (const file of files) {
  const raw = readFileSync(join(dir, file));
  const gz = gzipSync(raw, { level: 9 }).length;
  total += gz;
  console.log(`${file.padEnd(28)} ${(raw.length / 1000).toFixed(1).padStart(7)} kB   gzip ${(gz / 1000).toFixed(1).padStart(6)} kB`);
}

const kb = total / 1000;
console.log(`\nJavaScript, gzipped: ${kb.toFixed(1)} kB of a ${BUDGET_KB} kB budget`);
if (kb > BUDGET_KB) {
  console.error('Over budget. Find what grew with `npx vite-bundle-visualizer`, then cut it or raise the budget on purpose.');
  process.exit(1);
}

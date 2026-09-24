// Runs every <SqlPlayground> in docs/ against a real Postgres (PGlite) and
// fails if any setup or query errors. Keeps the handbook's SQL honest.
//   node scripts/check-sql.mjs            # check all
//   node scripts/check-sql.mjs -v         # also print results
import fs from 'node:fs';
import path from 'node:path';
import {PGlite} from '@electric-sql/pglite';

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..');
const verbose = process.argv.includes('-v');

function* mdxFiles(dir) {
  for (const e of fs.readdirSync(dir, {withFileTypes: true})) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) yield* mdxFiles(p);
    else if (p.endsWith('.mdx')) yield p;
  }
}

// Resolve a prop value: "…", {`…`}, or {name} referring to `export const name = `…``.
function propValue(tag, name, consts) {
  let m = new RegExp(`${name}="([^"]*)"`).exec(tag);
  if (m) return m[1];
  m = new RegExp(`${name}=\\{\`([\\s\\S]*?)\`\\}`).exec(tag);
  if (m) return m[1];
  m = new RegExp(`${name}=\\{(\\w+)\\}`).exec(tag);
  if (m) return consts[m[1]] ?? null;
  return undefined;
}

let failures = 0, checked = 0;
for (const file of mdxFiles(path.join(root, 'docs'))) {
  const src = fs.readFileSync(file, 'utf8');
  const consts = {};
  for (const m of src.matchAll(/export const (\w+) = `([\s\S]*?)`;/g)) consts[m[1]] = m[2];
  const tags = [...src.matchAll(/<SqlPlayground[\s\S]*?\/>/g)].map((m) => m[0]);
  for (const [i, tag] of tags.entries()) {
    const setup = propValue(tag, 'setup', consts) ?? '';
    const sql = propValue(tag, 'sql', consts);
    const where = `${path.relative(root, file)} playground #${i + 1}`;
    if (sql == null) { console.error(`✗ ${where}: could not read sql prop`); failures++; continue; }
    const db = new PGlite();
    try {
      if (setup.trim()) await db.exec(setup);
      const res = await db.exec(sql);
      checked++;
      if (verbose) {
        console.log(`✓ ${where}`);
        for (const r of res) if (r.rows?.length) console.table(r.rows);
      }
    } catch (e) {
      failures++;
      console.error(`✗ ${where}: ${e.message}`);
    } finally {
      await db.close();
    }
  }
}
console.log(`SQL playgrounds: ${checked} passed, ${failures} failed`);
process.exit(failures ? 1 : 0);

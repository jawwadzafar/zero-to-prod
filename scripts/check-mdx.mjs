#!/usr/bin/env node
// Compiles MDX files the way the site does (MDX 3 + GFM + directives), in
// seconds instead of a full site build. Catches the usual MDX traps: a bare
// `<` or `{` in prose, unclosed tags, broken JSX props in <Quiz>.
//
//   node scripts/check-mdx.mjs                 every file under docs/
//   node scripts/check-mdx.mjs docs/cloud/*.mdx  just these
import fs from 'node:fs';
import path from 'node:path';
import {compile} from '@mdx-js/mdx';
import remarkGfm from 'remark-gfm';
import remarkDirective from 'remark-directive';
import remarkFrontmatter from 'remark-frontmatter';

const root = path.join(path.dirname(new URL(import.meta.url).pathname), '..');
let files = process.argv.slice(2);
if (!files.length) {
  (function walk(dir) {
    for (const f of fs.readdirSync(dir)) {
      const p = path.join(dir, f);
      if (fs.statSync(p).isDirectory()) walk(p);
      else if (f.endsWith('.mdx')) files.push(p);
    }
  })(path.join(root, 'docs'));
}

let bad = 0;
for (const file of files) {
  try {
    await compile(fs.readFileSync(file, 'utf8'), {remarkPlugins: [remarkFrontmatter, remarkGfm, remarkDirective]});
  } catch (e) {
    bad++;
    console.error(`${path.relative(root, path.resolve(file))}:${e.line ?? '?'}:${e.column ?? '?'} ${e.reason || e.message}`);
  }
}
if (bad) process.exit(1);
console.log(`All ${files.length} MDX files compile.`);

#!/usr/bin/env node
// Builds the whole handbook as one PDF, book/zero-to-prod.pdf: a cover, the
// contents, then every chapter in curriculum order, printed from the built
// site exactly as a reader's browser prints it (step-throughs as lists, quiz
// answer keys, self-check answers open). .github/workflows/book.yml publishes
// it as the "book" release.
//
//   npm run build && (cd web && npm ci) && node scripts/build-book.mjs
//
// Needs pdfunite (poppler-utils) to join the pages. PW_CHROMIUM=/path/to/chrome
// uses a browser other than Playwright's own; BOOK_LIMIT=5 prints only the
// first five chapters (to try it quickly).
import {createRequire} from 'node:module';
import {execFileSync} from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import {serveBuild} from './lib/serve-build.mjs';

const root = path.join(path.dirname(new URL(import.meta.url).pathname), '..');
const {chromium} = createRequire(path.join(root, 'web/package.json'))('@playwright/test');
const curriculum = JSON.parse(fs.readFileSync(path.join(root, 'src/data/curriculum.json'), 'utf8'));

const esc = (s) => s.replace(/[&<>"]/g, (c) => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;'})[c]);

// Every written chapter, in reading order, with its number ("1.5").
const chapters = curriculum.parts.flatMap((part) =>
  part.chapters
    .map((ch, i) => ({part, id: `${part.id}/${ch.id}`, number: `${part.n}.${i + 1}`, title: ch.title}))
    .filter((ch) => ['.mdx', '.md'].some((ext) => fs.existsSync(path.join(root, 'docs', ch.id + ext)))),
);
const limit = Number(process.env.BOOK_LIMIT) || chapters.length;
const todo = chapters.slice(0, limit);

const out = path.join(root, 'book');
const parts = path.join(out, 'parts');
fs.rmSync(out, {recursive: true, force: true});
fs.mkdirSync(parts, {recursive: true});

const {origin, close} = await serveBuild(root);
const browser = await chromium.launch(process.env.PW_CHROMIUM ? {executablePath: process.env.PW_CHROMIUM} : {});
const ctx = await browser.newContext({colorScheme: 'light'});
await ctx.addInitScript(() => localStorage.setItem('theme', 'light'));
const pdfOptions = {format: 'A4', printBackground: true, margin: {top: '16mm', bottom: '18mm', left: '14mm', right: '14mm'}};

// Cover and contents, written here rather than in the site.
const date = new Date().toISOString().slice(0, 10);
const contents = curriculum.parts
  .map((part) => {
    const list = todo.filter((ch) => ch.part === part);
    if (!list.length) return '';
    return `<h2>Part ${part.n} · ${esc(part.title)}</h2><ol>${list.map((ch) => `<li><b>${ch.number}</b> ${esc(ch.title)}</li>`).join('')}</ol>`;
  })
  .join('');
const front = await ctx.newPage();
await front.setContent(`<!doctype html><meta charset="utf-8"><style>
  body { font: 12pt/1.5 system-ui, sans-serif; color: #1c1e21; }
  .cover { height: 250mm; display: flex; flex-direction: column; justify-content: center; break-after: page; }
  .cover h1 { font-size: 44pt; margin: 0; color: #4f46e5; } .cover p { font-size: 15pt; max-width: 32em; }
  h2 { font-size: 13pt; margin: 1.1em 0 .3em; break-after: avoid; } ol { list-style: none; padding: 0; margin: 0; columns: 2; }
  li { break-inside: avoid; font-size: 10.5pt; } small { color: #555; }
</style>
<section class="cover">
  <h1>Zero to Prod</h1>
  <p>A free, open handbook that takes you from your first command to running production systems.</p>
  <p><small>Printed from https://jawwadzafar.github.io/zero-to-prod/ on ${date}. The web version has the interactive parts:
  a Linux terminal and Postgres in the page, step-by-step diagrams, quizzes that remember your progress, and search.</small></p>
</section>
<h1>Contents</h1>${contents}`);
await front.pdf({...pdfOptions, path: path.join(parts, '0000-front.pdf')});
await front.close();

// Chapters, a few at a time.
let next = 0;
async function worker() {
  const page = await ctx.newPage();
  while (next < todo.length) {
    const i = next++;
    const ch = todo[i];
    const res = await page.goto(origin + 'learn/' + ch.id, {waitUntil: 'networkidle'});
    if (!res || res.status() >= 400) throw new Error(`${ch.id}: HTTP ${res?.status()}`);
    await page.waitForTimeout(500); // diagrams render after load
    await page.evaluate(() => window.dispatchEvent(new Event('beforeprint')));
    await page.pdf({
      ...pdfOptions,
      path: path.join(parts, `${String(i + 1).padStart(4, '0')}.pdf`),
      displayHeaderFooter: true,
      headerTemplate: '<span></span>',
      footerTemplate: `<div style="font: 8pt system-ui, sans-serif; color: #666; width: 100%; padding: 0 14mm; display: flex; justify-content: space-between;"><span>Zero to Prod · ${ch.number} ${esc(ch.title)}</span><span class="pageNumber"></span></div>`,
    });
    console.log(`${ch.number} ${ch.title}`);
  }
  await page.close();
}
await Promise.all(Array.from({length: 4}, worker));
await browser.close();
close();

const files = fs.readdirSync(parts).sort().map((f) => path.join(parts, f));
const book = path.join(out, 'zero-to-prod.pdf');
execFileSync('pdfunite', [...files, book]);
fs.rmSync(parts, {recursive: true});
console.log(`${book}: ${todo.length} chapters, ${(fs.statSync(book).size / 1e6).toFixed(1)} MB`);

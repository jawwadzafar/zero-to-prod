#!/usr/bin/env node
// Runs axe (WCAG 2.2 AA) on one page of every kind the site has, in light and
// dark mode, against the built site in build/. Contrast, names, roles and
// links-told-apart-by-colour are what it catches; it can't judge whether a
// page makes sense to a screen-reader user, so it's a floor, not a verdict.
//
// Playwright and axe come from web/ (its end-to-end tests use them), so the
// site's own install stays small:
//
//   npm run build && (cd web && npm ci) && node scripts/check-a11y.mjs
//
// PW_CHROMIUM=/path/to/chrome uses a browser other than Playwright's own.
import {createRequire} from 'node:module';
import path from 'node:path';
import {serveBuild} from './lib/serve-build.mjs';

const root = path.join(path.dirname(new URL(import.meta.url).pathname), '..');
const webRequire = createRequire(path.join(root, 'web/package.json'));
const {chromium} = webRequire('@playwright/test');
const AxeBuilder = (await import(webRequire.resolve('@axe-core/playwright'))).default;

// One of each kind: the app pages, then chapters with each interactive component.
const pages = [
  '', 'roadmap', 'progress', 'review', 'sandbox', 'learn', 'learn/glossary', 'search?q=docker',
  'learn/foundations/http-from-zero', // StepThrough, Mermaid, quiz
  'learn/data/designing-schemas', // SqlPlayground
  'learn/linux/pipes-and-text-tools', // Terminal
  'learn/go/concurrency', // code blocks
  'learn/security/oauth-and-oidc',
  'learn/shipping/gitops',
];

const {origin, close} = await serveBuild(root);

const browser = await chromium.launch(process.env.PW_CHROMIUM ? {executablePath: process.env.PW_CHROMIUM} : {});
const problems = [];
for (const theme of ['light', 'dark']) {
  const ctx = await browser.newContext({colorScheme: theme});
  await ctx.addInitScript((t) => localStorage.setItem('theme', t), theme);
  const page = await ctx.newPage();
  for (const p of pages) {
    const res = await page.goto(origin + p, {waitUntil: 'networkidle'});
    if (!res || res.status() >= 400) {
      problems.push(`${theme} /${p}: HTTP ${res?.status()}`);
      continue;
    }
    await page.waitForTimeout(500); // diagrams render after load
    const {violations} = await new AxeBuilder({page}).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
    for (const v of violations)
      for (const n of v.nodes.slice(0, 3))
        problems.push(`${theme} /${p}: ${v.id} (${v.impact}) ${n.target.join(' ')}\n    ${(n.failureSummary ?? '').split('\n').slice(1).join(' ').trim()}`);
  }
  await ctx.close();
}
await browser.close();
close();

if (problems.length) {
  console.error(`Accessibility problems (axe, WCAG 2.2 AA):\n${problems.join('\n')}`);
  process.exit(1);
}
console.log(`axe: no WCAG 2.2 AA problems on ${pages.length} pages, light and dark.`);

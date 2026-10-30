import { expect, test } from '@playwright/test';
import { logIn, uniqueSlug } from './fixtures';

// The page is served with a Content-Security-Policy (chapter 16.10). A policy
// that is too strict breaks the app in ways unit tests can't see, so this
// test runs the main flow and fails on any violation the browser reports.
test('the app runs under its Content-Security-Policy with no violations', async ({ page }) => {
  const violations: string[] = [];
  await page.exposeFunction('reportViolation', (v: string) => violations.push(v));
  await page.addInitScript(() => {
    document.addEventListener('securitypolicyviolation', (e) => {
      (window as unknown as { reportViolation: (v: string) => void }).reportViolation(
        `${e.effectiveDirective} blocked ${e.blockedURI || 'inline code'}`,
      );
    });
  });

  const response = await page.goto('/app/');
  expect(response?.headers()['content-security-policy']).toContain("frame-ancestors 'none'");

  await logIn(page);
  const slug = uniqueSlug();
  await page.getByLabel('Long URL').fill('https://go.dev/');
  await page.getByLabel('Custom slug (optional)').fill(slug);
  await page.getByRole('button', { name: 'Create short link' }).click();
  page.once('dialog', (dialog) => void dialog.accept());
  await page.getByRole('button', { name: `Delete ${slug}` }).click();
  await expect(page.getByRole('status')).toHaveText(`Deleted ${slug}`);

  expect(violations).toEqual([]);
});

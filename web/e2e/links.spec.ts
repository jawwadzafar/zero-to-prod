import { expect, test } from '@playwright/test';
import { logIn, uniqueSlug } from './fixtures';

test('create a short link, follow it, see the click, delete it', async ({ page, request }) => {
  const slug = uniqueSlug();
  await logIn(page);

  // Create, through the form.
  await page.getByLabel('Long URL').fill('https://go.dev/doc/');
  await page.getByLabel('Custom slug (optional)').fill(slug);
  await page.getByRole('button', { name: 'Create short link' }).click();

  const row = page.getByRole('listitem').filter({ hasText: `/${slug}` });
  await expect(row).toContainText('0 clicks');
  await expect(row.getByRole('link')).toHaveAttribute('href', new RegExp(`/${slug}$`));

  // The short link really redirects. Ask nginx directly and don't follow the
  // redirect, so the test never depends on go.dev being reachable.
  const visit = await request.get(`/${slug}`, { maxRedirects: 0 });
  expect(visit.status()).toBe(302);
  expect(visit.headers()['location']).toBe('https://go.dev/doc/');

  // The click is counted by snip's worker, a moment later. Wait for the state,
  // never for a fixed time: refresh until the row says so, or give up after 15 s.
  await expect(async () => {
    await page.getByRole('button', { name: 'Refresh' }).click();
    await expect(row).toContainText('1 click', { timeout: 1_000 });
  }).toPass({ timeout: 15_000 });

  // Delete, confirming the browser's dialog.
  page.once('dialog', (dialog) => void dialog.accept());
  await row.getByRole('button', { name: `Delete ${slug}` }).click();
  await expect(row).toHaveCount(0);

  // And snip agrees: the short link is gone.
  const after = await request.get(`/${slug}`, { maxRedirects: 0 });
  expect(after.status()).toBe(404);
});

test('a slug that is already taken shows snip\'s message next to the slug field', async ({ page }) => {
  const slug = uniqueSlug();
  await logIn(page);

  async function create() {
    await page.getByLabel('Long URL').fill('https://go.dev/');
    await page.getByLabel('Custom slug (optional)').fill(slug);
    await page.getByRole('button', { name: 'Create short link' }).click();
  }

  await create();
  // Wait until the first one has landed. Without this line the test is flaky:
  // the second fill can run while the first request is in flight, and the
  // first success then clears the form under it.
  await expect(page.getByRole('button', { name: `Delete ${slug}` })).toBeVisible();
  await create();

  await expect(page.getByLabel('Custom slug (optional)')).toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByRole('alert')).toContainText('already');

  // Tidy up the link the first loop created.
  page.once('dialog', (dialog) => void dialog.accept());
  await page.getByRole('button', { name: `Delete ${slug}` }).click();
  await expect(page.getByRole('button', { name: `Delete ${slug}` })).toHaveCount(0);
});

test('a wrong key sends you back to the key form', async ({ page }) => {
  await page.goto('/app/');
  await page.getByLabel('API key').fill('snip_not-a-real-key');
  await page.getByRole('button', { name: 'Continue' }).click();

  await expect(page.getByRole('alert')).toContainText('did not accept that API key');
  await expect(page.getByLabel('API key')).toBeVisible();
});

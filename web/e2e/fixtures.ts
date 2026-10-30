import { expect, type Page } from '@playwright/test';

/** The key the tests log in with: created by `snip keys create`, passed in by the shell. */
export function apiKey(): string {
  const key = process.env.SNIP_API_KEY;
  if (!key) {
    throw new Error('Set SNIP_API_KEY to a key from: docker compose exec api snip keys create e2e');
  }
  return key;
}

/** Opens the UI and logs in the way a person does: paste the key, press Continue. */
export async function logIn(page: Page) {
  await page.goto('/app/');
  await page.getByLabel('API key').fill(apiKey());
  await page.getByRole('button', { name: 'Continue' }).click();
  // Wait on what the user waits on: the list has loaded.
  await expect(page.getByText('Loading your links…')).toBeHidden();
  await expect(page.getByRole('heading', { name: 'Your links' })).toBeVisible();
}

/** A slug no earlier run has used, so reruns never collide with old data. */
export function uniqueSlug(): string {
  return `e2e-${Date.now().toString(36)}${Math.floor(Math.random() * 1000)}`;
}

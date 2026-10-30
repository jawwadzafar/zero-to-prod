import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { apiKey, logIn, uniqueSlug } from './fixtures';

// axe checks the rules a machine can decide (names, roles, contrast of text,
// landmarks), against WCAG 2.2 level A and AA. It cannot tell whether focus
// goes somewhere sensible, so the keyboard test below does that by hand.
async function axeViolations(page: Page) {
  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'])
    .analyze();
  // Only the parts a person needs to fix it: which rule, and which elements.
  return results.violations.map((v) => ({ rule: v.id, help: v.help, nodes: v.nodes.map((n) => n.target.join(' ')) }));
}

for (const scheme of ['light', 'dark'] as const) {
  test(`axe finds no WCAG A/AA problems (${scheme} mode)`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: scheme });
    await page.goto('/app/');
    expect(await axeViolations(page)).toEqual([]);

    await logIn(page);
    expect(await axeViolations(page)).toEqual([]);
  });
}

test('the whole flow works with the keyboard alone, and focus is never lost', async ({ page }) => {
  const slug = uniqueSlug();
  await page.goto('/app/');

  // Tab reaches the key field first, and Enter submits the form.
  await page.keyboard.press('Tab');
  await expect(page.getByLabel('API key')).toBeFocused();
  await page.keyboard.type(apiKey());
  await page.keyboard.press('Enter');

  // The key form is gone; focus moves into the new screen, not to <body>.
  await expect(page.getByLabel('Long URL')).toBeFocused();

  // Fill both fields, Tab to the button and press it.
  await page.keyboard.type('https://go.dev/doc/');
  await page.keyboard.press('Tab');
  await page.keyboard.type(slug);
  await page.keyboard.press('Tab');
  const create = page.getByRole('button', { name: 'Create short link' });
  await expect(create).toBeFocused();
  await page.keyboard.press('Enter');

  // A screen reader hears the result, and the button still has focus.
  await expect(page.getByRole('status')).toHaveText(new RegExp(`Created .*/${slug}$`));
  await expect(create).toBeFocused();

  // Refresh keeps focus too, though it is unavailable while the list loads.
  const refresh = page.getByRole('button', { name: 'Refresh' });
  await refresh.focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('button', { name: `Delete ${slug}` })).toBeVisible();
  await expect(refresh).toBeFocused();

  // Delete removes the focused button with its row; focus goes to the heading.
  page.once('dialog', (dialog) => void dialog.accept());
  await page.getByRole('button', { name: `Delete ${slug}` }).focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('status')).toHaveText(`Deleted ${slug}`);
  await expect(page.getByRole('heading', { name: 'Your links' })).toBeFocused();
});

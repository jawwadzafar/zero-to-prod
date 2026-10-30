import { defineConfig, devices } from '@playwright/test';

// End-to-end tests (chapter 16.9): a real browser against the real stack
// (nginx, snip, Postgres, Redis) started with docker compose. To run them:
//
//   cd snip && docker compose up -d --build --wait
//   export SNIP_API_KEY=$(docker compose exec -T api snip keys create e2e | grep -o 'snip_[A-Za-z0-9]*')
//   cd ../web && npm ci && npx playwright install chromium
//   npm run test:e2e
//   cd ../snip && docker compose down -v      # when you're done
//
// Or, with no browser installed on your machine, run them in Playwright's
// image on compose's network, where nginx is at http://nginx:
//
//   docker run --rm --network snip_default -e SNIP_URL=http://nginx -e SNIP_API_KEY \
//     -v "$PWD:/w" -w /w mcr.microsoft.com/playwright:v1.64.1-noble npx playwright test
export default defineConfig({
  testDir: './e2e',
  // The tests share one database, so one at a time keeps them independent of order.
  workers: 1,
  forbidOnly: !!process.env.CI, // a stray test.only must not silently skip the rest in CI
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    // nginx's address. compose publishes it on localhost:8080; override it
    // when the tests run somewhere else (in a container on compose's network).
    baseURL: process.env.SNIP_URL ?? 'http://localhost:8080',
    trace: 'retain-on-failure', // a step-by-step recording of any failed test
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});

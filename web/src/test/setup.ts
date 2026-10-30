// Runs before every test file: adds matchers such as toBeInTheDocument(),
// and resets the page and any stubbed globals (fetch, confirm) after each test.
import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

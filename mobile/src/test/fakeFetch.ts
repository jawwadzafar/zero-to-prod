// Helpers for tests: a fake fetch that answers like snip, so tests run in
// milliseconds with no server, and check exactly what the app sent.
// The same helpers as web/src/test/fakeFetch.ts, with Jest in place of Vitest.

export function json(status: number, body: unknown, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', ...headers },
  });
}

export function snipError(status: number, code: string, message: string, headers: Record<string, string> = {}) {
  return json(status, { error: { code, message } }, headers);
}

/** Replaces the global fetch; each call gets the next answer in order. */
export function stubFetch(...answers: (Response | Error)[]) {
  const fetchMock = jest.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => {
    const next = answers.shift();
    if (!next) throw new Error('fetch called more times than the test expected');
    if (next instanceof Error) throw next;
    return next;
  });
  globalThis.fetch = fetchMock as typeof fetch;
  return fetchMock;
}

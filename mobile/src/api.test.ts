import { ApiError, createClient, describeError, TIMEOUT_MS, type Link } from './api';
import { json, snipError, stubFetch } from './test/fakeFetch';

const link: Link = {
  slug: 'godoc',
  url: 'https://go.dev/doc/',
  clicks: 3,
  created_at: '2026-10-30T09:15:00Z',
  short_url: 'http://localhost:8080/godoc',
};

const client = () => createClient('http://10.0.2.2:8080/', 'snip_test');

describe('createClient', () => {
  afterEach(() => jest.useRealTimers()); // undo the one test that fakes the clock, even if it fails
  it('sends the key as a Bearer token to the base URL and returns the links', async () => {
    const fetchMock = stubFetch(json(200, { links: [link] }));

    const links = await client().listLinks();

    expect(links).toEqual([link]);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe('http://10.0.2.2:8080/api/links?limit=100'); // no "//" from the trailing slash
    expect(init?.headers).toMatchObject({ Authorization: 'Bearer snip_test' });
  });

  it('sends a custom slug only when there is one', async () => {
    const fetchMock = stubFetch(json(201, link), json(201, link));

    await client().createLink('https://go.dev/doc/', 'godoc');
    await client().createLink('https://go.dev/doc/');

    expect(fetchMock.mock.calls[0]![1]?.body).toBe('{"url":"https://go.dev/doc/","slug":"godoc"}');
    expect(fetchMock.mock.calls[1]![1]?.body).toBe('{"url":"https://go.dev/doc/"}');
  });

  it('accepts the empty 204 answer to a delete', async () => {
    const fetchMock = stubFetch(new Response(null, { status: 204 }));

    await expect(client().deleteLink('godoc')).resolves.toBeUndefined();
    expect(fetchMock.mock.calls[0]![1]?.method).toBe('DELETE');
  });

  it("turns snip's error shape into an ApiError with the same code", async () => {
    stubFetch(snipError(409, 'slug_taken', 'that slug is already in use'));

    const err = await client().createLink('https://go.dev', 'godoc').catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 409, code: 'slug_taken', message: 'that slug is already in use' });
  });

  it('reads Retry-After from a 429', async () => {
    stubFetch(snipError(429, 'rate_limited', 'too many links created; slow down', { 'Retry-After': '7' }));

    const err = await client().createLink('https://go.dev').catch((e: unknown) => e);

    expect(err).toMatchObject({ code: 'rate_limited', retryAfterSeconds: 7 });
    expect(describeError(err)).toBe('You are creating links too fast. Try again in 7 seconds.');
  });

  it('copes with an answer that is not JSON, such as an HTML 502 from nginx', async () => {
    stubFetch(new Response('<html>502 Bad Gateway</html>', { status: 502, statusText: 'Bad Gateway' }));

    const err = await client().listLinks().catch((e: unknown) => e);

    expect(err).toMatchObject({ status: 502, code: 'unexpected' });
    expect(describeError(err)).toBe('snip had a problem on its side. Try again in a moment.');
  });

  it('reports a network failure as code "network", naming the address it tried', async () => {
    stubFetch(new TypeError('Network request failed'));

    const err = await client().listLinks().catch((e: unknown) => e);

    expect(err).toMatchObject({ status: 0, code: 'network' });
    expect(describeError(err)).toContain('http://10.0.2.2:8080');
  });

  it(`gives up after ${TIMEOUT_MS / 1000} seconds`, async () => {
    jest.useFakeTimers();
    // A fetch that never answers on its own, only rejects when aborted (as the real one does).
    globalThis.fetch = jest.fn(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => reject(new Error('Aborted')));
        }),
    );

    const pending = client().listLinks().catch((e: unknown) => e);
    jest.advanceTimersByTime(TIMEOUT_MS);

    expect(await pending).toMatchObject({ code: 'network', message: expect.stringContaining('did not answer') });
  });
});

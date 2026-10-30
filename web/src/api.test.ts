import { ApiError, createClient, describeError, type Link } from './api';
import { json, snipError, stubFetch } from './test/fakeFetch';

const link: Link = {
  slug: 'godoc',
  url: 'https://go.dev/doc/',
  clicks: 3,
  created_at: '2026-10-30T09:15:00Z',
  short_url: 'http://localhost:8080/godoc',
};

describe('createClient', () => {
  it('sends the key as a Bearer token and returns the links', async () => {
    const fetchMock = stubFetch(json(200, { links: [link] }));

    const links = await createClient('snip_test').listLinks();

    expect(links).toEqual([link]);
    const [path, init] = fetchMock.mock.calls[0]!;
    expect(path).toBe('/api/links?limit=100');
    expect(init?.headers).toMatchObject({ Authorization: 'Bearer snip_test' });
  });

  it('sends a custom slug only when there is one', async () => {
    const fetchMock = stubFetch(json(201, link), json(201, link));
    const client = createClient('snip_test');

    await client.createLink('https://go.dev/doc/', 'godoc');
    await client.createLink('https://go.dev/doc/');

    expect(fetchMock.mock.calls[0]![1]?.body).toBe('{"url":"https://go.dev/doc/","slug":"godoc"}');
    expect(fetchMock.mock.calls[1]![1]?.body).toBe('{"url":"https://go.dev/doc/"}');
  });

  it('accepts the empty 204 answer to a delete', async () => {
    const fetchMock = stubFetch(new Response(null, { status: 204 }));

    await expect(createClient('snip_test').deleteLink('godoc')).resolves.toBeUndefined();
    expect(fetchMock.mock.calls[0]![1]?.method).toBe('DELETE');
  });

  it("turns snip's error shape into an ApiError with the same code", async () => {
    stubFetch(snipError(409, 'slug_taken', 'that slug is already in use'));

    const err = await createClient('snip_test').createLink('https://go.dev', 'godoc').catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 409, code: 'slug_taken', message: 'that slug is already in use' });
  });

  it('reads Retry-After from a 429', async () => {
    stubFetch(snipError(429, 'rate_limited', 'too many links created; slow down', { 'Retry-After': '7' }));

    const err = await createClient('snip_test').createLink('https://go.dev').catch((e: unknown) => e);

    expect(describeError(err)).toBe('You are creating links too fast. Try again in 7 seconds.');
  });

  it('copes with an answer that is not JSON, such as an HTML 502 from nginx', async () => {
    stubFetch(new Response('<html>502 Bad Gateway</html>', { status: 502, statusText: 'Bad Gateway' }));

    const err = await createClient('snip_test').listLinks().catch((e: unknown) => e);

    expect(err).toMatchObject({ status: 502, code: 'unexpected' });
    expect(describeError(err)).toBe('snip had a problem on its side. Try again in a moment.');
  });

  it('does not take a 200 that is not JSON for success', async () => {
    stubFetch(new Response('<!doctype html><title>Not snip</title>', { status: 200 }));

    const err = await createClient('snip_test').createLink('https://go.dev/').catch((e: unknown) => e);

    expect(err).toMatchObject({ status: 200, code: 'unexpected' });
  });

  it('reports a network failure as code "network"', async () => {
    stubFetch(new TypeError('Failed to fetch'));

    const err = await createClient('snip_test').listLinks().catch((e: unknown) => e);

    expect(err).toMatchObject({ status: 0, code: 'network' });
  });
});

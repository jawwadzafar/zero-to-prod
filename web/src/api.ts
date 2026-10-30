// A typed client for snip's API. The types below copy the shapes in
// snip/api/openapi.yaml, so the compiler catches a typo like link.click
// before a user does.

/** One short link, as snip returns it. */
export interface Link {
  slug: string;
  url: string;
  clicks: number;
  created_at: string; // an RFC 3339 timestamp, e.g. "2026-10-30T09:15:00Z"
  short_url: string;
}

/** The stable error codes snip can send (the Error schema's enum). */
export type ErrorCode =
  | 'invalid_json'
  | 'invalid_url'
  | 'invalid_slug'
  | 'slug_taken'
  | 'body_too_large'
  | 'unauthorized'
  | 'not_found'
  | 'rate_limited'
  | 'internal';

/** Every snip error looks like {"error": {"code": "…", "message": "…"}}. */
interface ErrorBody {
  error: { code: ErrorCode; message: string };
}

/**
 * What every failed call throws. code is snip's error code, or one of two
 * codes of our own: "network" (no answer at all) and "unexpected" (an answer
 * that isn't snip's error shape, such as nginx's HTML 502 page).
 */
export class ApiError extends Error {
  readonly status: number;
  readonly code: ErrorCode | 'network' | 'unexpected';
  readonly retryAfterSeconds?: number;

  constructor(status: number, code: ApiError['code'], message: string, retryAfterSeconds?: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

// A type guard: it checks at run time what TypeScript can't know at compile
// time, because JSON from the network could be anything.
function isErrorBody(data: unknown): data is ErrorBody {
  if (typeof data !== 'object' || data === null || !('error' in data)) return false;
  const e = data.error;
  return typeof e === 'object' && e !== null && 'code' in e && typeof e.code === 'string';
}

export interface SnipClient {
  listLinks(signal?: AbortSignal): Promise<Link[]>;
  createLink(url: string, slug?: string): Promise<Link>;
  deleteLink(slug: string): Promise<void>;
}

/** Returns a client that sends apiKey with every request. */
export function createClient(apiKey: string): SnipClient {
  async function request<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
    const headers: Record<string, string> = { Authorization: `Bearer ${apiKey}` };
    if (body !== undefined) headers['Content-Type'] = 'application/json';

    let res: Response;
    try {
      // A path, not a full URL: the UI and the API share an origin (nginx
      // serves both), so the browser sends this to wherever the page came from.
      res = await fetch(path, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
        signal,
      });
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') throw err; // we cancelled it
      throw new ApiError(0, 'network', 'Could not reach snip. Check your connection and try again.');
    }

    if (res.status === 204) return undefined as T; // DELETE answers with no body
    const data: unknown = await res.json().catch(() => null);
    if (res.ok) {
      // A success that isn't JSON wasn't written by snip: a Wi-Fi login page
      // answers 200 with HTML for every address, for example.
      if (data === null) throw new ApiError(res.status, 'unexpected', 'The answer was not from snip.');
      return data as T;
    }

    if (isErrorBody(data)) {
      const retry = Number(res.headers.get('Retry-After')) || undefined;
      throw new ApiError(res.status, data.error.code, data.error.message, retry);
    }
    throw new ApiError(res.status, 'unexpected', `snip answered ${res.status} ${res.statusText}`.trim());
  }

  return {
    async listLinks(signal) {
      const page = await request<{ links: Link[] }>('GET', '/api/links?limit=100', undefined, signal);
      return page.links;
    },
    createLink(url, slug) {
      // Send slug only when the user typed one; snip picks a random one otherwise.
      return request<Link>('POST', '/api/links', slug ? { url, slug } : { url });
    },
    deleteLink(slug) {
      return request<void>('DELETE', `/api/links/${encodeURIComponent(slug)}`);
    },
  };
}

/** A sentence a person can act on, for any error the client throws. */
export function describeError(err: unknown): string {
  if (!(err instanceof ApiError)) return 'Something went wrong. Try again.';
  switch (err.code) {
    case 'unauthorized':
      return 'snip did not accept that API key. Check it, or create a new one.';
    case 'rate_limited':
      return `You are creating links too fast. Try again in ${err.retryAfterSeconds ?? 'a few'} seconds.`;
    case 'internal':
    case 'unexpected':
      return 'snip had a problem on its side. Try again in a moment.';
    default:
      return err.message; // snip's own message is already written for people
  }
}

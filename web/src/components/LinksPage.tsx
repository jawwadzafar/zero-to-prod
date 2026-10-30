import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError, describeError, type Link, type SnipClient } from '../api';
import { CreateLinkForm } from './CreateLinkForm';
import { LinkList } from './LinkList';

// The three states a list can be in, and only those three: TypeScript makes
// it impossible to have links and an error at the same time by accident.
type ListState =
  | { status: 'loading' }
  | { status: 'error'; message: string }
  | { status: 'ready'; links: Link[] };

interface Props {
  client: SnipClient;
  onUnauthorized: () => void;
}

export function LinksPage({ client, onUnauthorized }: Props) {
  const [list, setList] = useState<ListState>({ status: 'loading' });
  const [actionError, setActionError] = useState<string | null>(null);
  // What a screen reader should say after an action. Sighted users see the
  // row appear or vanish; this sentence is how everyone else finds out.
  const [announcement, setAnnouncement] = useState('');
  const headingRef = useRef<HTMLHeadingElement>(null);

  // Any call can fail with 401 (the key was revoked a minute ago, say);
  // that always means "ask for a key again", wherever it happens.
  const handleError = useCallback(
    (err: unknown): string | null => {
      if (err instanceof ApiError && err.code === 'unauthorized') {
        onUnauthorized();
        return null;
      }
      return describeError(err);
    },
    [onUnauthorized],
  );

  const load = useCallback(
    async (signal?: AbortSignal) => {
      setList({ status: 'loading' });
      try {
        const links = await client.listLinks(signal);
        setList({ status: 'ready', links });
      } catch (err) {
        if (signal?.aborted) return; // the component went away; nobody to tell
        const message = handleError(err);
        if (message) setList({ status: 'error', message });
      }
    },
    [client, handleError],
  );

  // Fetch the list when the page appears. The cleanup function cancels the
  // request if the page goes away first (or when StrictMode runs effects
  // twice in development), so a late answer can't update a dead component.
  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  function handleCreated(link: Link) {
    setAnnouncement(`Created ${link.short_url}`);
    setList((prev) => (prev.status === 'ready' ? { status: 'ready', links: [link, ...prev.links] } : prev));
  }

  async function handleDelete(slug: string) {
    setActionError(null);
    try {
      await client.deleteLink(slug);
      setAnnouncement(`Deleted ${slug}`);
      // The Delete button that had focus is gone with its row. Put focus on
      // the list's heading, so keyboard users continue from a known place.
      headingRef.current?.focus();
      setList((prev) =>
        prev.status === 'ready' ? { status: 'ready', links: prev.links.filter((l) => l.slug !== slug) } : prev,
      );
    } catch (err) {
      setActionError(handleError(err));
    }
  }

  return (
    <>
      {/* A live region is only reliable if it is already on the page before
          its text changes, so this one is always rendered, empty at first. */}
      <p className="visually-hidden" role="status">
        {announcement}
      </p>
      <CreateLinkForm client={client} onCreated={handleCreated} onError={handleError} />
      <section className="card" aria-labelledby="links-heading">
        <div className="section-header">
          <h2 id="links-heading" ref={headingRef} tabIndex={-1}>
            Your links
          </h2>
          <button
            type="button"
            className="secondary"
            // aria-disabled keeps focus on the button while the list reloads (see CreateLinkForm).
            aria-disabled={list.status === 'loading'}
            onClick={() => {
              if (list.status !== 'loading') void load();
            }}
          >
            Refresh
          </button>
        </div>
        {actionError && (
          <p className="error" role="alert">
            {actionError}
          </p>
        )}
        {list.status === 'loading' && <p>Loading your links…</p>}
        {list.status === 'error' && (
          <div role="alert">
            <p className="error">{list.message}</p>
            <button type="button" onClick={() => void load()}>
              Try again
            </button>
          </div>
        )}
        {list.status === 'ready' && <LinkList links={list.links} onDelete={handleDelete} />}
      </section>
    </>
  );
}

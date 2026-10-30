import { useState, type FormEvent } from 'react';
import { ApiError, type Link, type SnipClient } from '../api';

interface Props {
  client: SnipClient;
  onCreated: (link: Link) => void;
  onError: (err: unknown) => string | null; // turns an error into a message (null: handled elsewhere)
}

// Which input an error belongs next to, so the message appears where the
// mistake is. Codes come from snip's Error schema.
function fieldFor(err: unknown): 'url' | 'slug' | 'form' {
  if (err instanceof ApiError) {
    if (err.code === 'invalid_url') return 'url';
    if (err.code === 'invalid_slug' || err.code === 'slug_taken') return 'slug';
  }
  return 'form';
}

export function CreateLinkForm({ client, onCreated, onError }: Props) {
  const [url, setUrl] = useState('');
  const [slug, setSlug] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<{ field: 'url' | 'slug' | 'form'; message: string } | null>(null);

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      const link = await client.createLink(url.trim(), slug.trim() || undefined);
      onCreated(link);
      setUrl('');
      setSlug('');
    } catch (err) {
      const message = onError(err);
      if (message) setError({ field: fieldFor(err), message });
    } finally {
      setSaving(false);
    }
  }

  const errorFor = (field: 'url' | 'slug' | 'form') =>
    error?.field === field ? (
      <p className="error" id={`${field}-error`} role="alert">
        {error.message}
      </p>
    ) : null;

  return (
    <form className="card" onSubmit={handleSubmit}>
      <h2>Shorten a link</h2>
      <label htmlFor="url">Long URL</label>
      <input
        id="url"
        type="url"
        required
        placeholder="https://example.com/a/long/page"
        value={url}
        onChange={(e) => setUrl(e.target.value)}
        aria-invalid={error?.field === 'url'}
        aria-describedby={error?.field === 'url' ? 'url-error' : undefined}
      />
      {errorFor('url')}
      <label htmlFor="slug">Custom slug (optional)</label>
      <input
        id="slug"
        placeholder="my-launch"
        value={slug}
        onChange={(e) => setSlug(e.target.value)}
        aria-invalid={error?.field === 'slug'}
        aria-describedby={error?.field === 'slug' ? 'slug-error' : 'slug-hint'}
      />
      <p className="hint" id="slug-hint">
        3–32 letters, digits, <code>-</code> or <code>_</code>. Leave it empty and snip picks one.
      </p>
      {errorFor('slug')}
      {errorFor('form')}
      <button type="submit" disabled={saving}>
        {saving ? 'Creating…' : 'Create short link'}
      </button>
    </form>
  );
}

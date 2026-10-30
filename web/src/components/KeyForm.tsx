import { useState, type FormEvent } from 'react';

interface Props {
  notice: string | null; // why we are asking again, if we are
  autoFocus?: boolean; // true when this form replaces the links page, so focus isn't lost
  onSubmit: (key: string) => void;
}

export function KeyForm({ notice, autoFocus = false, onSubmit }: Props) {
  const [key, setKey] = useState('');
  const trimmed = key.trim();

  function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault(); // stop the browser's own form submit, which reloads the page
    if (trimmed) onSubmit(trimmed);
  }

  return (
    <form className="card" onSubmit={handleSubmit}>
      <h2>Your API key</h2>
      {notice && (
        <p className="error" id="api-key-notice" role="alert">
          {notice}
        </p>
      )}
      <label htmlFor="api-key">API key</label>
      <input
        id="api-key"
        type="password"
        autoComplete="off"
        spellCheck={false}
        placeholder="snip_…"
        autoFocus={autoFocus}
        // Read out after the label: why we're asking again (if we are), then the hint.
        aria-describedby={notice ? 'api-key-notice api-key-hint' : 'api-key-hint'}
        value={key}
        onChange={(e) => setKey(e.target.value)}
      />
      <p className="hint" id="api-key-hint">
        Create one with <code>snip keys create NAME</code>. The key stays in this tab&apos;s memory and is
        forgotten when you close or reload it.
      </p>
      <button type="submit" disabled={!trimmed}>
        Continue
      </button>
    </form>
  );
}

import { useState, type FormEvent } from 'react';

interface Props {
  notice: string | null; // why we are asking again, if we are
  onSubmit: (key: string) => void;
}

export function KeyForm({ notice, onSubmit }: Props) {
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
        <p className="error" role="alert">
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
        value={key}
        onChange={(e) => setKey(e.target.value)}
      />
      <p className="hint">
        Create one with <code>snip keys create NAME</code>. The key stays in this tab&apos;s memory and is
        forgotten when you close or reload it.
      </p>
      <button type="submit" disabled={!trimmed}>
        Continue
      </button>
    </form>
  );
}

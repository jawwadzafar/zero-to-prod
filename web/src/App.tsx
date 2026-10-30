import { useCallback, useMemo, useState } from 'react';
import { createClient } from './api';
import { KeyForm } from './components/KeyForm';
import { LinksPage } from './components/LinksPage';

// The API key lives only in this component's state: in memory, gone when the
// tab closes or reloads. Chapter 16.6 explains why not localStorage.
export function App() {
  const [apiKey, setApiKey] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  // One client per key. useMemo and useCallback keep the same object between
  // renders, so the list's effect (which depends on them) runs only when the
  // key really changes.
  const client = useMemo(() => (apiKey ? createClient(apiKey) : null), [apiKey]);
  const handleUnauthorized = useCallback(() => {
    setApiKey(null);
    setNotice('snip did not accept that API key. Paste a valid one.');
  }, []);

  return (
    <main className="app">
      <header className="app-header">
        <h1>snip</h1>
        {client && (
          <button type="button" className="secondary" onClick={() => setApiKey(null)}>
            Forget key
          </button>
        )}
      </header>
      {client ? (
        <LinksPage client={client} onUnauthorized={handleUnauthorized} />
      ) : (
        <KeyForm
          notice={notice}
          onSubmit={(key) => {
            setNotice(null);
            setApiKey(key);
          }}
        />
      )}
    </main>
  );
}

import React, {useRef, useState} from 'react';
import BrowserOnly from '@docusaurus/BrowserOnly';
import styles from './styles.module.css';

type Result = {fields: {name: string}[]; rows: Record<string, unknown>[]; affectedRows?: number};

/**
 * A real PostgreSQL database running inside your browser (PGlite, compiled to
 * WebAssembly). Nothing is sent anywhere. `setup` runs once, invisibly, to
 * create example tables; `children`/`sql` is the editable query.
 *
 *   <SqlPlayground setup="CREATE TABLE …; INSERT …;" sql="SELECT * FROM links;" />
 */
export default function SqlPlayground(props: {setup?: string; sql: string; title?: string}) {
  return (
    <BrowserOnly fallback={<pre>{props.sql}</pre>}>{() => <Playground {...props} />}</BrowserOnly>
  );
}

function Playground({setup = '', sql, title = 'Try it — real Postgres, running in your browser'}: {setup?: string; sql: string; title?: string}) {
  const [code, setCode] = useState(sql.trim());
  const [results, setResults] = useState<Result[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const dbRef = useRef<any>(null);

  async function getDb() {
    if (dbRef.current) return dbRef.current;
    const {PGlite} = await import('@electric-sql/pglite');
    const db = new PGlite();
    if (setup.trim()) await db.exec(setup);
    dbRef.current = db;
    return db;
  }

  async function run() {
    setBusy(true);
    setError(null);
    try {
      const db = await getDb();
      const res = (await db.exec(code)) as Result[];
      setResults(res);
    } catch (e: any) {
      setResults(null);
      setError(e?.message ?? String(e));
    } finally {
      setBusy(false);
    }
  }

  async function reset() {
    try {
      await dbRef.current?.close();
    } catch {
      /* ignore */
    }
    dbRef.current = null;
    setCode(sql.trim());
    setResults(null);
    setError(null);
  }

  return (
    <div className={styles.box}>
      <div className={styles.head}>
        <span>🐘 {title}</span>
        <span className={styles.actions}>
          <button type="button" className="button button--sm button--secondary" onClick={reset} disabled={busy}>
            Reset
          </button>
          <button type="button" className="button button--sm button--primary" onClick={run} disabled={busy}>
            {busy ? 'Running…' : 'Run ▶'}
          </button>
        </span>
      </div>
      <textarea
        className={styles.editor}
        value={code}
        spellCheck={false}
        rows={Math.min(14, Math.max(3, code.split('\n').length + 1))}
        onChange={(e) => setCode(e.target.value)}
        onKeyDown={(e) => {
          if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
            e.preventDefault();
            run();
          }
        }}
        aria-label="SQL editor"
      />
      <div className={styles.hint}>Edit the query and press Run (or Ctrl/⌘ + Enter). First run loads the database (~3 MB).</div>
      {error && <pre className={styles.error}>ERROR: {error}</pre>}
      {results?.map((r, i) =>
        r.fields.length ? (
          <div key={i} className={styles.tableWrap}>
            <table>
              <thead>
                <tr>{r.fields.map((f) => <th key={f.name}>{f.name}</th>)}</tr>
              </thead>
              <tbody>
                {r.rows.map((row, ri) => (
                  <tr key={ri}>
                    {r.fields.map((f) => (
                      <td key={f.name}>{formatCell(row[f.name])}</td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
            <div className={styles.hint}>{r.rows.length} row{r.rows.length === 1 ? '' : 's'}</div>
          </div>
        ) : (
          <div key={i} className={styles.ok}>✓ OK{typeof r.affectedRows === 'number' ? ` — ${r.affectedRows} row(s) affected` : ''}</div>
        ),
      )}
    </div>
  );
}

function formatCell(v: unknown) {
  if (v === null || v === undefined) return <em>NULL</em>;
  if (v instanceof Date) return v.toISOString();
  if (typeof v === 'object') return JSON.stringify(v);
  return String(v);
}

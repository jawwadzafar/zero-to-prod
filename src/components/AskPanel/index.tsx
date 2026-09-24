import React, {useEffect, useMemo, useRef, useState} from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useBaseUrl from '@docusaurus/useBaseUrl';
import {useLocation} from '@docusaurus/router';
import styles from './styles.module.css';

// "Ask the handbook" — works with zero backend:
//  1. Retrieval (always on, free): a keyword index of every section, built at
//     deploy time (scripts/build-ask-index.mjs), searched in your browser.
//  2. AI answer (optional): if you paste your own Anthropic API key, the top
//     sections are sent — from your browser straight to the Anthropic API — as
//     documents with citations enabled, and the answer links back to them.
// The key lives only in this browser's localStorage.

type Passage = {id: number; docId: string; url: string; chapter: string; heading: string; text: string};
type Hit = Passage & {score: number};
type Turn = {role: 'user' | 'assistant'; text: string; sources?: Hit[]; cites?: number[][]; error?: string};

const KEY_STORE = 'z2p:ask:key';
const MODEL_STORE = 'z2p:ask:model';
const MODELS = [
  {id: 'claude-opus-5', label: 'Claude Opus 5 (best answers)'},
  {id: 'claude-sonnet-5', label: 'Claude Sonnet 5 (balanced)'},
  {id: 'claude-haiku-4-5', label: 'Claude Haiku 4.5 (fastest, cheapest)'},
];

const SYSTEM = `You are the tutor inside "Zero to Prod", a free handbook that takes semi-technical people from basic tech literacy to working backend, DevOps and AI engineer.

How to answer:
- Ground your answer in the handbook sections provided as documents, and cite them.
- Teach the way the handbook does: start from something familiar (an everyday analogy), explain what actually happens in plain words, then name the concept. Expand every acronym the first time.
- Be concise: a few short paragraphs, or a short list. Use a code block only when a command or snippet genuinely helps.
- If the provided sections don't cover the question, say so plainly in one sentence, then give a brief general answer and suggest which part of the handbook to read.
- Never invent chapter names or links.`;

function safeGet(k: string): string {
  try {
    return window.localStorage.getItem(k) ?? '';
  } catch {
    return '';
  }
}
function safeSet(k: string, v: string) {
  try {
    if (v) window.localStorage.setItem(k, v);
    else window.localStorage.removeItem(k);
  } catch {
    /* storage blocked */
  }
}

export default function AskPanel() {
  const [open, setOpen] = useState(false);
  const location = useLocation();
  // Hide the floating button on the home page hero? No — it's useful everywhere.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'j') {
        e.preventDefault();
        setOpen((o) => !o);
      }
      if (e.key === 'Escape') setOpen(false);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  return (
    <>
      <button
        type="button"
        className={clsx(styles.fab, open && styles.fabHidden)}
        onClick={() => setOpen(true)}
        aria-label="Ask the handbook (Ctrl/⌘ + J)"
        title="Ask the handbook (Ctrl/⌘ + J)">
        💬 Ask
      </button>
      {open && <Panel onClose={() => setOpen(false)} pathname={location.pathname} />}
    </>
  );
}

function Panel({onClose, pathname}: {onClose: () => void; pathname: string}) {
  const indexUrl = useBaseUrl('/ask-index.json');
  const learnPrefix = useBaseUrl('/learn/');
  const currentDoc = pathname.startsWith(learnPrefix) ? pathname.slice(learnPrefix.length).replace(/\/$/, '') : '';
  const [search, setSearch] = useState<null | ((q: string) => Hit[])>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [q, setQ] = useState('');
  const [turns, setTurns] = useState<Turn[]>([]);
  const [busy, setBusy] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [apiKey, setApiKey] = useState('');
  const [model, setModel] = useState(MODELS[0].id);
  const [thisPage, setThisPage] = useState(!!currentDoc);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    setApiKey(safeGet(KEY_STORE));
    setModel(safeGet(MODEL_STORE) || MODELS[0].id);
    inputRef.current?.focus();
  }, []);

  // Load the passage index + build a MiniSearch index lazily, on first open.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [{default: MiniSearch}, res] = await Promise.all([import('minisearch'), fetch(indexUrl)]);
        if (!res.ok) throw new Error(`index ${res.status}`);
        const passages: Passage[] = await res.json();
        const ms = new MiniSearch<Passage>({
          fields: ['heading', 'chapter', 'text'],
          storeFields: ['docId', 'url', 'chapter', 'heading', 'text'],
          searchOptions: {boost: {heading: 3, chapter: 2}, fuzzy: 0.2, prefix: true, combineWith: 'OR'},
        });
        ms.addAll(passages);
        if (!cancelled) setSearch(() => (query: string) => ms.search(query) as unknown as Hit[]);
      } catch (e: any) {
        if (!cancelled) setLoadError('Could not load the handbook index. Check your connection and try again.');
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [indexUrl]);

  useEffect(() => {
    scrollRef.current?.scrollTo({top: scrollRef.current.scrollHeight, behavior: 'smooth'});
  }, [turns]);

  function retrieve(query: string): Hit[] {
    if (!search) return [];
    let hits = search(query);
    if (thisPage && currentDoc) {
      hits = hits.map((h) => (h.docId === currentDoc ? {...h, score: h.score * 2.5} : h)).sort((a, b) => b.score - a.score);
    }
    return hits.slice(0, 6);
  }

  async function ask() {
    const question = q.trim();
    if (!question || busy) return;
    setQ('');
    const sources = retrieve(question);
    const history = turns;
    const next: Turn[] = [...history, {role: 'user', text: question}];
    if (!apiKey) {
      setTurns([...next, {role: 'assistant', text: '', sources}]);
      return;
    }
    setTurns([...next, {role: 'assistant', text: '', sources}]);
    setBusy(true);
    try {
      const {default: Anthropic} = await import('@anthropic-ai/sdk');
      const client = new Anthropic({apiKey, dangerouslyAllowBrowser: true});
      const priorTurns = history
        .filter((t) => t.text && !t.error)
        .slice(-6)
        .map((t) => ({role: t.role, content: t.text}));
      const docs = sources.map((s) => ({
        type: 'document' as const,
        source: {type: 'text' as const, media_type: 'text/plain' as const, data: s.text},
        title: `${s.chapter} › ${s.heading}`,
        citations: {enabled: true},
      }));
      const params: any = {
        model,
        max_tokens: 16000,
        system: SYSTEM,
        messages: [
          ...priorTurns,
          {
            role: 'user',
            content: [
              ...docs,
              {
                type: 'text',
                text: `${currentDoc ? `I'm reading the chapter "${currentDoc}". ` : ''}My question: ${question}`,
              },
            ],
          },
        ],
      };
      if (model === 'claude-opus-5') {
        // If a safety classifier declines, let the API retry on a suitable model.
        params.betas = ['server-side-fallback-2026-07-01'];
        params.fallbacks = 'default';
      }
      const stream = client.beta.messages.stream(params);
      let text = '';
      for await (const event of stream as any) {
        if (event.type === 'content_block_delta' && event.delta.type === 'text_delta') {
          text += event.delta.text;
          setTurns((ts) => ts.map((t, i) => (i === ts.length - 1 ? {...t, text} : t)));
        }
      }
      const final: any = await stream.finalMessage();
      if (final.stop_reason === 'refusal') {
        throw new Error('The model declined to answer this one. Try rephrasing the question.');
      }
      // Rebuild the answer from final text blocks, attaching citation markers.
      let rebuilt = '';
      const cites: number[][] = [];
      for (const block of final.content) {
        if (block.type !== 'text') continue;
        rebuilt += block.text;
        const idx = (block.citations ?? [])
          .map((c: any) => c.document_index)
          .filter((n: any) => typeof n === 'number');
        if (idx.length) {
          const uniq = Array.from(new Set<number>(idx));
          cites.push(uniq);
          rebuilt += uniq.map((n) => `[${n + 1}]`).join('');
        }
      }
      setTurns((ts) => ts.map((t, i) => (i === ts.length - 1 ? {...t, text: rebuilt || text, cites} : t)));
    } catch (e: any) {
      const status = e?.status;
      const msg =
        status === 401
          ? 'That API key was rejected. Check it in settings (⚙️).'
          : status === 429
            ? 'Rate limited by the API — wait a moment and try again.'
            : status === 529 || status >= 500
              ? 'The AI service is busy right now. Try again shortly.'
              : e?.message ?? String(e);
      setTurns((ts) => ts.map((t, i) => (i === ts.length - 1 ? {...t, error: msg} : t)));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className={styles.panel} role="dialog" aria-label="Ask the handbook">
      <div className={styles.header}>
        <strong>💬 Ask the handbook</strong>
        <span className={styles.headerActions}>
          <button type="button" className={styles.iconBtn} onClick={() => setShowSettings((s) => !s)} aria-label="Settings" title="Settings">
            ⚙️
          </button>
          <button type="button" className={styles.iconBtn} onClick={() => setTurns([])} aria-label="Clear conversation" title="Clear">
            🧹
          </button>
          <button type="button" className={styles.iconBtn} onClick={onClose} aria-label="Close" title="Close (Esc)">
            ✕
          </button>
        </span>
      </div>

      {showSettings && (
        <Settings
          apiKey={apiKey}
          model={model}
          onSave={(k, m) => {
            setApiKey(k);
            setModel(m);
            safeSet(KEY_STORE, k);
            safeSet(MODEL_STORE, m);
            setShowSettings(false);
          }}
        />
      )}

      <div className={styles.body} ref={scrollRef}>
        {turns.length === 0 && (
          <div className={styles.intro}>
            <p>
              Ask anything about what you're learning — <em>"what's the difference between a process and a thread?"</em>,{' '}
              <em>"why do we need indexes?"</em>, <em>"explain RAG like I'm new"</em>.
            </p>
            <p className={styles.small}>
              {apiKey ? (
                <>AI answers are on (your own key, stored only in this browser). Answers cite the handbook sections they used.</>
              ) : (
                <>
                  Right now you'll get the most relevant handbook sections — free, instant, private. For written AI answers that cite
                  the handbook, add your own API key in <button type="button" className={styles.linkBtn} onClick={() => setShowSettings(true)}>settings ⚙️</button>.
                </>
              )}
            </p>
            {loadError && <p className={styles.err}>{loadError}</p>}
          </div>
        )}
        {turns.map((t, i) =>
          t.role === 'user' ? (
            <div key={i} className={styles.user}>
              {t.text}
            </div>
          ) : (
            <div key={i} className={styles.assistant}>
              {t.error && <div className={styles.err}>{t.error}</div>}
              {t.text ? <Answer text={t.text} /> : !t.error && apiKey && busy && i === turns.length - 1 ? <div className={styles.small}>Thinking…</div> : null}
              {t.sources && t.sources.length > 0 && (
                <div className={styles.sources}>
                  <div className={styles.small}>{t.text ? 'Sources' : 'Most relevant sections'}</div>
                  <ol>
                    {t.sources.map((s) => (
                      <li key={s.id}>
                        <Link to={s.url} onClick={onClose}>
                          <strong>{s.chapter}</strong> › {s.heading}
                        </Link>
                        {!t.text && <div className={styles.snippet}>{snippet(s.text)}</div>}
                      </li>
                    ))}
                  </ol>
                </div>
              )}
              {t.sources && t.sources.length === 0 && !t.text && !t.error && (
                <div className={styles.small}>No matching sections yet — try different words, or check the roadmap for what's covered.</div>
              )}
            </div>
          ),
        )}
      </div>

      <div className={styles.footer}>
        {currentDoc && (
          <label className={styles.small}>
            <input type="checkbox" checked={thisPage} onChange={(e) => setThisPage(e.target.checked)} /> Prefer this chapter
          </label>
        )}
        <div className={styles.inputRow}>
          <textarea
            ref={inputRef}
            className={styles.input}
            rows={2}
            value={q}
            placeholder={search ? 'Ask a question…' : 'Loading the handbook index…'}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                ask();
              }
            }}
          />
          <button type="button" className="button button--primary" onClick={ask} disabled={busy || !search || !q.trim()}>
            {busy ? '…' : 'Ask'}
          </button>
        </div>
      </div>
    </div>
  );
}

function Settings({apiKey, model, onSave}: {apiKey: string; model: string; onSave: (k: string, m: string) => void}) {
  const [k, setK] = useState(apiKey);
  const [m, setM] = useState(model);
  return (
    <div className={styles.settings}>
      <label className={styles.small}>
        Anthropic API key (optional) — get one at{' '}
        <a href="https://console.anthropic.com/" target="_blank" rel="noopener noreferrer">
          console.anthropic.com
        </a>
      </label>
      <input className={styles.field} type="password" value={k} placeholder="sk-ant-…" onChange={(e) => setK(e.target.value.trim())} autoComplete="off" />
      <label className={styles.small}>Model</label>
      <select className={styles.field} value={m} onChange={(e) => setM(e.target.value)}>
        {MODELS.map((x) => (
          <option key={x.id} value={x.id}>
            {x.label}
          </option>
        ))}
      </select>
      <p className={styles.small}>
        🔒 Your key is saved only in this browser and sent only to api.anthropic.com. You pay for your own usage. Don't use a key
        on a shared computer.
      </p>
      <div className={styles.inputRow}>
        <button type="button" className="button button--sm button--primary" onClick={() => onSave(k, m)}>
          Save
        </button>
        {apiKey && (
          <button type="button" className="button button--sm button--secondary" onClick={() => onSave('', m)}>
            Remove key
          </button>
        )}
      </div>
    </div>
  );
}

function snippet(text: string) {
  const t = text.replace(/\s+/g, ' ').replace(/[#*`>]/g, '').trim();
  return t.length > 220 ? t.slice(0, 220) + '…' : t;
}

// Minimal, safe rendering: paragraphs, bullet lists, fenced code and inline code.
function Answer({text}: {text: string}) {
  const blocks = useMemo(() => text.split(/```/), [text]);
  return (
    <div className={styles.answer}>
      {blocks.map((b, i) =>
        i % 2 === 1 ? (
          <pre key={i}>
            <code>{b.replace(/^\w*\n/, '')}</code>
          </pre>
        ) : (
          b
            .split(/\n{2,}/)
            .filter((p) => p.trim())
            .map((p, j) => <Para key={`${i}-${j}`} text={p} />)
        ),
      )}
    </div>
  );
}

function Para({text}: {text: string}) {
  const lines = text.split('\n');
  if (lines.every((l) => /^\s*([-*]|\d+\.)\s/.test(l))) {
    return (
      <ul>
        {lines.map((l, i) => (
          <li key={i}>{inline(l.replace(/^\s*([-*]|\d+\.)\s/, ''))}</li>
        ))}
      </ul>
    );
  }
  return <p>{inline(text)}</p>;
}

function inline(s: string) {
  return s.split(/(`[^`]+`|\*\*[^*]+\*\*)/).map((part, i) =>
    part.startsWith('`') && part.endsWith('`') ? (
      <code key={i}>{part.slice(1, -1)}</code>
    ) : part.startsWith('**') && part.endsWith('**') ? (
      <strong key={i}>{part.slice(2, -2)}</strong>
    ) : (
      <React.Fragment key={i}>{part}</React.Fragment>
    ),
  );
}

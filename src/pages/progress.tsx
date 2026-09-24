import React, {useRef, useState} from 'react';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import Link from '@docusaurus/Link';
import {useProgress, exportProgress, importProgress, resetProgress} from '@site/src/lib/progress';
import {flat, tracks, trackChapters} from '@site/src/lib/curriculum';
import {TrackPicker} from '@site/src/components/Roadmap';

export default function ProgressPage() {
  const p = useProgress();
  const fileRef = useRef<HTMLInputElement>(null);
  const [msg, setMsg] = useState('');
  const doneList = flat.filter((c) => p.done[c.id]).sort((a, b) => p.done[b.id] - p.done[a.id]);
  const quizzes = Object.entries(p.quiz);

  function download() {
    const blob = new Blob([exportProgress()], {type: 'application/json'});
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = 'zero-to-prod-progress.json';
    a.click();
    URL.revokeObjectURL(a.href);
  }

  return (
    <Layout title="My progress" description="Your Zero to Prod progress — stored only in your browser.">
      <main className="container margin-vert--lg" style={{maxWidth: 900}}>
        <Heading as="h1">My progress</Heading>
        <p>
          No account, no tracking. Everything here lives in <strong>this browser</strong>. To move it to another device, export
          it and import it there.
        </p>

        <Heading as="h2">Your track</Heading>
        <TrackPicker />
        <div className="row margin-vert--md">
          {tracks.map((t) => {
            const list = trackChapters(t.id);
            const d = list.filter((c) => p.done[c.id]).length;
            return (
              <div key={t.id} className="col col--3 margin-bottom--md">
                <div className="card padding--md" style={{height: '100%'}}>
                  <div style={{fontSize: '1.6rem'}}>{t.emoji}</div>
                  <strong>{t.title}</strong>
                  <div>
                    {d}/{list.length} chapters · {list.length ? Math.round((d / list.length) * 100) : 0}%
                  </div>
                </div>
              </div>
            );
          })}
        </div>

        <Heading as="h2">Completed chapters ({doneList.length})</Heading>
        {doneList.length === 0 ? (
          <p>
            Nothing yet. <Link to="/learn">Start with chapter 0.1 →</Link>
          </p>
        ) : (
          <ul>
            {doneList.map((c) => (
              <li key={c.id}>
                <Link to={`/learn/${c.id}`}>
                  {c.number} {c.chapter.title}
                </Link>{' '}
                <small>— {new Date(p.done[c.id]).toLocaleDateString()}</small>
              </li>
            ))}
          </ul>
        )}

        {quizzes.length > 0 && (
          <>
            <Heading as="h2">Quiz scores</Heading>
            <ul>
              {quizzes.map(([id, s]) => (
                <li key={id}>
                  <code>{id}</code>: {s.correct}/{s.total}
                </li>
              ))}
            </ul>
          </>
        )}

        <Heading as="h2">Backup & reset</Heading>
        <div style={{display: 'flex', gap: '.5rem', flexWrap: 'wrap'}}>
          <button type="button" className="button button--primary" onClick={download}>
            Export progress
          </button>
          <button type="button" className="button button--secondary" onClick={() => fileRef.current?.click()}>
            Import progress
          </button>
          <button
            type="button"
            className="button button--danger button--outline"
            onClick={() => {
              if (window.confirm('Erase all progress in this browser? This cannot be undone.')) resetProgress();
            }}>
            Reset everything
          </button>
          <input
            ref={fileRef}
            type="file"
            accept="application/json"
            hidden
            onChange={async (e) => {
              const f = e.target.files?.[0];
              if (!f) return;
              try {
                importProgress(await f.text());
                setMsg('Imported ✓');
              } catch (err: any) {
                setMsg(`Import failed: ${err.message}`);
              }
            }}
          />
        </div>
        {msg && <p className="margin-top--sm">{msg}</p>}
      </main>
    </Layout>
  );
}

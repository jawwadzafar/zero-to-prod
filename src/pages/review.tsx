import React, {useEffect, useMemo, useState} from 'react';
import clsx from 'clsx';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import Link from '@docusaurus/Link';
import useBaseUrl from '@docusaurus/useBaseUrl';
import {recordAnswer, useProgress} from '@site/src/lib/progress';
import {dueKeys, INTERVAL_DAYS} from '@site/src/lib/review';
import {optionOrder, type Question} from '@site/src/components/Quiz';
import styles from '@site/src/components/Quiz/styles.module.css';

type BankItem = Question & {key: string; quiz: string; doc: string; chapter: string};

export default function ReviewPage() {
  const bankUrl = useBaseUrl('/quiz-bank.json');
  const p = useProgress();
  const cards = p.review ?? {};
  const [bank, setBank] = useState<Map<string, BankItem> | null>(null);
  const [error, setError] = useState('');
  // The session's questions are fixed when it starts, so answering one
  // doesn't reorder the rest.
  const [session, setSession] = useState<string[] | null>(null);
  const [at, setAt] = useState(0);
  const [picked, setPicked] = useState<number | null>(null);
  const [right, setRight] = useState(0);

  useEffect(() => {
    fetch(bankUrl)
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))))
      .then((items: BankItem[]) => setBank(new Map(items.map((i) => [i.key, i]))))
      .catch(() => setError('Could not load the question bank. Check your connection and reload.'));
  }, [bankUrl]);

  // Questions that were reworded or removed since they were scheduled are skipped.
  const due = useMemo(() => (bank ? dueKeys(cards).filter((k) => bank.has(k)) : []), [bank, cards]);
  const scheduled = bank ? Object.keys(cards).filter((k) => bank.has(k)).length : 0;
  const nextDue = Object.values(cards).reduce((min, c) => Math.min(min, c.due), Infinity);

  const start = () => {
    setSession(due);
    setAt(0);
    setPicked(null);
    setRight(0);
  };

  const key = session?.[at];
  const item = key && bank ? bank.get(key) : undefined;
  const order = useMemo(
    // Shuffle differently per box, so a question isn't recognised by where its answer sits.
    () => (item ? optionOrder(`${item.key}#${cards[item.key]?.box ?? 0}`, item.options.length) : []),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [item],
  );

  function choose(oi: number) {
    if (!item || picked !== null) return;
    setPicked(oi);
    const correct = oi === item.answer;
    if (correct) setRight((n) => n + 1);
    recordAnswer(item.key, correct);
  }

  return (
    <Layout title="Review" description="Spaced review of the quiz questions you've answered, stored only in your browser.">
      <main className="container margin-vert--lg" style={{maxWidth: 800}}>
        <Heading as="h1">Review</Heading>
        <p>
          Questions you answered in chapter quizzes come back here after a gap: a day after a wrong answer, then {INTERVAL_DAYS[1]},{' '}
          {INTERVAL_DAYS[2]}, {INTERVAL_DAYS[3]} and more days as you keep getting them right. Remembering something just before
          you'd forget it is what makes it stick. Your schedule lives only in this browser (export it on{' '}
          <Link to="/progress">My progress</Link>).
        </p>

        {error && <p className="alert alert--danger">{error}</p>}
        {!bank && !error && <p>Loading questions…</p>}

        {bank && !session && (
          <div className="card padding--lg margin-vert--md">
            {scheduled === 0 ? (
              <p className="margin-bottom--none">
                Nothing to review yet. Answer the quiz at the end of any chapter (<Link to="/learn">start here</Link>), and its
                questions will be scheduled automatically.
              </p>
            ) : due.length === 0 ? (
              <p className="margin-bottom--none">
                All caught up: {scheduled} question{scheduled === 1 ? '' : 's'} scheduled, the next one due{' '}
                {new Date(nextDue).toLocaleDateString()}.
              </p>
            ) : (
              <>
                <p>
                  <strong>{due.length}</strong> question{due.length === 1 ? '' : 's'} due, out of {scheduled} scheduled.
                </p>
                <button type="button" className="button button--primary" onClick={start}>
                  Start review →
                </button>
              </>
            )}
          </div>
        )}

        {session && item && (
          <section className={styles.quiz} aria-label="Review question">
            <header className={styles.head}>
              <span>
                🔁 From <Link to={`/learn/${item.doc}`}>{item.chapter}</Link>
              </span>
              <span className={styles.score}>
                {at + 1} of {session.length}
              </span>
            </header>
            <div className={styles.question}>
              <p className={styles.q}>{item.q}</p>
              <div className={styles.options}>
                {order.map((oi, shown) => {
                  const state =
                    picked === null ? '' : oi === item.answer ? styles.right : oi === picked ? styles.wrong : styles.faded;
                  return (
                    <button
                      key={oi}
                      type="button"
                      className={clsx(styles.option, state)}
                      onClick={() => choose(oi)}
                      disabled={picked !== null}>
                      <span className={styles.letter}>{String.fromCharCode(65 + shown)}</span>
                      <span>{item.options[oi]}</span>
                    </button>
                  );
                })}
              </div>
              {picked !== null && (
                <div className={clsx(styles.explain, picked === item.answer ? styles.explainRight : styles.explainWrong)}>
                  <strong>{picked === item.answer ? '✅ Right. ' : '❌ Not quite. '}</strong>
                  {item.explain}
                </div>
              )}
            </div>
            {picked !== null && (
              <button
                type="button"
                className={clsx('button button--sm button--primary', styles.retry)}
                onClick={() => {
                  setAt((n) => n + 1);
                  setPicked(null);
                }}>
                {at + 1 < session.length ? 'Next question →' : 'Finish'}
              </button>
            )}
          </section>
        )}

        {session && !item && (
          <div className="card padding--lg margin-vert--md">
            <p>
              Done: <strong>{right}</strong> of {session.length} right. Right answers come back after a longer gap; the others
              return tomorrow.
            </p>
            <button type="button" className="button button--secondary" onClick={() => setSession(null)}>
              Back to the overview
            </button>
          </div>
        )}
      </main>
    </Layout>
  );
}

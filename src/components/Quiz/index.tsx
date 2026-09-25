import React, {useMemo, useState} from 'react';
import clsx from 'clsx';
import {recordQuiz} from '@site/src/lib/progress';
import styles from './styles.module.css';

export type Question = {
  q: string; // the question
  options: string[]; // choices
  answer: number; // index of the correct choice
  explain: string; // why — shown after answering
};

// A small deterministic PRNG (mulberry32) seeded from a string, so each
// question's options are shuffled the same way on the server and in the
// browser (no hydration mismatch), and the same way on every visit.
function seededRandom(seed: string): () => number {
  let h = 1779033703 ^ seed.length;
  for (let i = 0; i < seed.length; i++) {
    h = Math.imul(h ^ seed.charCodeAt(i), 3432918353);
    h = (h << 13) | (h >>> 19);
  }
  let a = h >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/** The order to show a question's options in: a permutation of their indexes. */
export function optionOrder(seed: string, n: number): number[] {
  const order = Array.from({length: n}, (_, i) => i);
  const rand = seededRandom(seed);
  for (let i = n - 1; i > 0; i--) {
    const j = Math.floor(rand() * (i + 1));
    [order[i], order[j]] = [order[j], order[i]];
  }
  return order;
}

/**
 * Multiple-choice check. Answers are revealed one question at a time with an
 * explanation, so a wrong answer still teaches. Best score is saved locally.
 *
 *   <Quiz id="linux-shell" questions={[{q: '…', options: ['…','…'], answer: 0, explain: '…'}]} />
 */
export default function Quiz({id, questions, title = 'Quick check'}: {id: string; questions: Question[]; title?: string}) {
  const [picked, setPicked] = useState<(number | null)[]>(() => questions.map(() => null));
  // Options are written with the answer anywhere; show them in a stable
  // shuffled order so the correct one isn't always in the same place.
  const orders = useMemo(() => questions.map((item, qi) => optionOrder(`${id}#${qi}`, item.options.length)), [id, questions]);
  const answered = picked.filter((p) => p !== null).length;
  const correct = picked.filter((p, i) => p === questions[i].answer).length;

  function choose(qi: number, oi: number) {
    if (picked[qi] !== null) return;
    const next = [...picked];
    next[qi] = oi;
    setPicked(next);
    if (next.every((p) => p !== null)) {
      recordQuiz(id, next.filter((p, i) => p === questions[i].answer).length, questions.length);
    }
  }

  return (
    <section className={styles.quiz} aria-label={title}>
      <header className={styles.head}>
        <span>🧩 {title}</span>
        <span className={styles.score}>
          {answered === questions.length ? `${correct}/${questions.length} correct` : `${answered}/${questions.length} answered`}
        </span>
      </header>
      {questions.map((item, qi) => {
        const choice = picked[qi];
        return (
          <div key={qi} className={styles.question}>
            <p className={styles.q}>
              <strong>{qi + 1}.</strong> {item.q}
            </p>
            <div className={styles.options}>
              {orders[qi].map((oi, shown) => {
                const opt = item.options[oi];
                const state =
                  choice === null ? '' : oi === item.answer ? styles.right : oi === choice ? styles.wrong : styles.faded;
                return (
                  <button
                    key={oi}
                    type="button"
                    className={clsx(styles.option, state)}
                    onClick={() => choose(qi, oi)}
                    disabled={choice !== null}>
                    <span className={styles.letter}>{String.fromCharCode(65 + shown)}</span>
                    <span>{opt}</span>
                  </button>
                );
              })}
            </div>
            {choice !== null && (
              <div className={clsx(styles.explain, choice === item.answer ? styles.explainRight : styles.explainWrong)}>
                <strong>{choice === item.answer ? '✅ Right. ' : '❌ Not quite. '}</strong>
                {item.explain}
              </div>
            )}
          </div>
        );
      })}
      {answered === questions.length && (
        <button type="button" className={clsx('button button--sm button--secondary', styles.retry)} onClick={() => setPicked(questions.map(() => null))}>
          Try again
        </button>
      )}
    </section>
  );
}

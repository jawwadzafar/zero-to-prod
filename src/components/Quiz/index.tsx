import React, {useState} from 'react';
import clsx from 'clsx';
import {recordQuiz} from '@site/src/lib/progress';
import styles from './styles.module.css';

export type Question = {
  q: string; // the question
  options: string[]; // choices
  answer: number; // index of the correct choice
  explain: string; // why — shown after answering
};

/**
 * Multiple-choice check. Answers are revealed one question at a time with an
 * explanation, so a wrong answer still teaches. Best score is saved locally.
 *
 *   <Quiz id="linux-shell" questions={[{q: '…', options: ['…','…'], answer: 0, explain: '…'}]} />
 */
export default function Quiz({id, questions, title = 'Quick check'}: {id: string; questions: Question[]; title?: string}) {
  const [picked, setPicked] = useState<(number | null)[]>(() => questions.map(() => null));
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
              {item.options.map((opt, oi) => {
                const state =
                  choice === null ? '' : oi === item.answer ? styles.right : oi === choice ? styles.wrong : styles.faded;
                return (
                  <button
                    key={oi}
                    type="button"
                    className={clsx(styles.option, state)}
                    onClick={() => choose(qi, oi)}
                    disabled={choice !== null}>
                    <span className={styles.letter}>{String.fromCharCode(65 + oi)}</span>
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

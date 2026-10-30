import React, {useState} from 'react';
import clsx from 'clsx';
import styles from './styles.module.css';

type Node = {id: string; label: string; sub?: string};
type Step = {
  title: string;
  text: string; // `backticks` become <code>
  from?: string; // node sending
  to?: string; // node receiving
  active?: string[]; // nodes doing work without a message
};

// Text with `code` spans, without needing MDX inside a JS string.
function Rich({text}: {text: string}) {
  return (
    <>
      {text.split('`').map((part, i) => (i % 2 ? <code key={i}>{part}</code> : <React.Fragment key={i}>{part}</React.Fragment>))}
    </>
  );
}

/**
 * A diagram you walk through one step at a time: the parts of a system as
 * boxes, the step's message as an arrow between two of them, and a sentence
 * saying what happens. Keyboard: ← and → when it has focus.
 *
 *   <StepThrough title="…" nodes={[{id: 'a', label: 'Browser'}, …]}
 *     steps={[{title: '…', text: '…', from: 'a', to: 'b'}, …]} />
 */
export default function StepThrough({title, nodes, steps}: {title: string; nodes: Node[]; steps: Step[]}) {
  const [i, setI] = useState(0);
  const step = steps[i];
  const go = (n: number) => setI(Math.max(0, Math.min(steps.length - 1, n)));
  const label = (id?: string) => nodes.find((n) => n.id === id)?.label ?? '';

  return (
    <figure
      className={styles.box}
      tabIndex={0}
      aria-label={`${title}: step-by-step diagram. Use the arrow keys or the buttons to move between steps.`}
      onKeyDown={(e) => {
        if (e.key === 'ArrowRight') go(i + 1);
        if (e.key === 'ArrowLeft') go(i - 1);
      }}>
      <figcaption className={styles.head}>
        <span>▶ {title}</span>
        <span className={styles.count}>
          Step {i + 1} of {steps.length}
        </span>
      </figcaption>

      <div className={styles.nodes} aria-hidden="true">
        {nodes.map((n) => {
          const role = n.id === step.from ? styles.from : n.id === step.to ? styles.to : step.active?.includes(n.id) ? styles.busy : '';
          return (
            <div key={n.id} className={clsx(styles.node, role)}>
              <strong>{n.label}</strong>
              {n.sub && <small>{n.sub}</small>}
            </div>
          );
        })}
      </div>

      <div className={styles.message} aria-hidden="true">
        {step.from && step.to ? (
          <>
            {label(step.from)} <span className={styles.arrow}>→</span> {label(step.to)}
          </>
        ) : (
          <>{(step.active ?? []).map(label).join(' + ') || ' '}</>
        )}
      </div>

      <div className={styles.text} aria-live="polite">
        <strong>{step.title}.</strong> <Rich text={step.text} />
      </div>

      <div className={styles.controls}>
        <button type="button" className="button button--sm button--secondary" onClick={() => go(i - 1)} disabled={i === 0}>
          ← Previous
        </button>
        <span className={styles.dots} aria-hidden="true">
          {steps.map((_, n) => (
            <span key={n} className={clsx(styles.dot, n === i && styles.dotOn)} />
          ))}
        </span>
        {i < steps.length - 1 ? (
          <button type="button" className="button button--sm button--primary" onClick={() => go(i + 1)}>
            Next →
          </button>
        ) : (
          <button type="button" className="button button--sm button--secondary" onClick={() => go(0)}>
            Start again
          </button>
        )}
      </div>
    </figure>
  );
}

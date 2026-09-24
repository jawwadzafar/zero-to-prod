import React, {useMemo} from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import {usePluginData} from '@docusaurus/useGlobalData';
import {useProgress, setTrack} from '@site/src/lib/progress';
import {parts, tracks, docId, chapterNumber, inTrack, trackChapters} from '@site/src/lib/curriculum';
import styles from './styles.module.css';

export function useAvailable(): Set<string> {
  const data = usePluginData('curriculum-plugin') as {available: string[]} | undefined;
  return useMemo(() => new Set(data?.available ?? []), [data]);
}

export function TrackPicker({compact = false}: {compact?: boolean}) {
  const {track} = useProgress();
  const current = track ?? 'all';
  return (
    <div className={clsx(styles.picker, compact && styles.pickerCompact)} role="radiogroup" aria-label="Choose a track">
      <button
        type="button"
        role="radio"
        aria-checked={current === 'all'}
        className={clsx(styles.chip, current === 'all' && styles.chipActive)}
        onClick={() => setTrack(undefined)}>
        🗺️ Everything
      </button>
      {tracks.map((t) => (
        <button
          key={t.id}
          type="button"
          role="radio"
          aria-checked={current === t.id}
          title={t.blurb}
          className={clsx(styles.chip, current === t.id && styles.chipActive)}
          style={{['--chip' as string]: t.color}}
          onClick={() => setTrack(t.id)}>
          {t.emoji} {t.title}
        </button>
      ))}
    </div>
  );
}

export function TrackSummary() {
  const {track, done} = useProgress();
  const available = useAvailable();
  const list = trackChapters(track);
  const finished = list.filter((c) => done[c.id]).length;
  const minutesLeft = list.filter((c) => !done[c.id]).reduce((s, c) => s + c.chapter.minutes, 0);
  const next = list.find((c) => !done[c.id] && available.has(c.id));
  const pct = list.length ? Math.round((finished / list.length) * 100) : 0;
  const t = tracks.find((x) => x.id === track);
  return (
    <div className={styles.summary}>
      <div className={styles.summaryText}>
        <strong>{t ? `${t.emoji} ${t.title}` : '🗺️ Everything'}</strong>
        <span>
          {finished} of {list.length} chapters done · about {Math.round(minutesLeft / 60)} hours to go
        </span>
      </div>
      <div className={styles.bar} aria-label={`${pct}% complete`}>
        <div className={styles.barFill} style={{width: `${pct}%`}} />
      </div>
      {next && (
        <Link className="button button--primary" to={`/learn/${next.id}`}>
          {finished === 0 ? 'Start' : 'Continue'}: {next.number} {next.chapter.title} →
        </Link>
      )}
    </div>
  );
}

export default function Roadmap() {
  const {track, done} = useProgress();
  const available = useAvailable();
  return (
    <div className={styles.roadmap}>
      <TrackPicker />
      <TrackSummary />
      <ol className={styles.spine}>
        {parts.map((part) => {
          const inPart = part.chapters.filter((ch) => inTrack(part, ch, track));
          const partDone = inPart.filter((ch) => done[docId(part, ch)]).length;
          const dim = inPart.length === 0;
          return (
            <li key={part.id} className={clsx(styles.part, dim && styles.dim)}>
              <div className={clsx(styles.node, inPart.length > 0 && partDone === inPart.length && styles.nodeDone)}>
                {part.n}
              </div>
              <div className={styles.partBody}>
                <div className={styles.partHead}>
                  <h3>{part.title}</h3>
                  {!dim && (
                    <span className={styles.partCount}>
                      {partDone}/{inPart.length}
                    </span>
                  )}
                </div>
                <p className={styles.blurb}>{part.blurb}</p>
                <div className={styles.grid}>
                  {part.chapters.map((ch, i) => {
                    const id = docId(part, ch);
                    const isIn = inTrack(part, ch, track);
                    const isDone = !!done[id];
                    const isAvail = available.has(id);
                    const body = (
                      <>
                        <span className={styles.num}>{chapterNumber(part, i)}</span>
                        <span className={styles.title}>{ch.title}</span>
                        <span className={styles.meta}>
                          {isDone ? '✓ done' : isAvail ? `${ch.minutes} min` : 'coming soon'}
                        </span>
                      </>
                    );
                    const cls = clsx(
                      styles.card,
                      isDone && styles.cardDone,
                      !isAvail && styles.cardSoon,
                      !isIn && styles.cardOut,
                    );
                    return isAvail ? (
                      <Link key={id} to={`/learn/${id}`} className={cls}>
                        {body}
                      </Link>
                    ) : (
                      <div key={id} className={cls} aria-disabled>
                        {body}
                      </div>
                    );
                  })}
                </div>
              </div>
            </li>
          );
        })}
      </ol>
    </div>
  );
}

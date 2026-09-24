import React from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import {useProgress, setDone} from '@site/src/lib/progress';
import {findChapter, trackChapters, tracks} from '@site/src/lib/curriculum';
import {useAvailable} from '@site/src/components/Roadmap';
import styles from './styles.module.css';

// Shown at the end of every chapter: mark it done, see where you are in your
// track, and jump to the next chapter in that track.
export default function ChapterComplete({docId}: {docId: string}) {
  const {done, track} = useProgress();
  const available = useAvailable();
  const me = findChapter(docId);
  if (!me) return null;
  const isDone = !!done[docId];
  const list = trackChapters(track);
  const pos = list.findIndex((c) => c.id === docId);
  const next = (pos >= 0 ? list.slice(pos + 1) : list).find((c) => available.has(c.id) && !done[c.id]);
  const finished = list.filter((c) => done[c.id]).length;
  const t = tracks.find((x) => x.id === track);
  return (
    <div className={clsx(styles.box, isDone && styles.boxDone)}>
      <div className={styles.row}>
        <button
          type="button"
          className={clsx('button', isDone ? 'button--success' : 'button--primary')}
          onClick={() => setDone(docId, !isDone)}
          aria-pressed={isDone}>
          {isDone ? '✓ Completed' : 'Mark chapter complete'}
        </button>
        <span className={styles.meta}>
          {t ? `${t.emoji} ${t.title}` : '🗺️ Everything'}: {finished}/{list.length} done ·{' '}
          <Link to="/roadmap">roadmap</Link>
        </span>
      </div>
      {isDone && next && (
        <div className={styles.next}>
          Up next in your track:{' '}
          <Link to={`/learn/${next.id}`}>
            <strong>
              {next.number} {next.chapter.title}
            </strong>{' '}
            →
          </Link>
        </div>
      )}
    </div>
  );
}

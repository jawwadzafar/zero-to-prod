// Learner progress, stored only in this browser (localStorage). No backend,
// no account. Every read/write is wrapped so private windows or blocked
// storage degrade to "nothing saved" instead of crashing the page.
import {useSyncExternalStore} from 'react';

const KEY = 'z2p:progress:v1';

export type Progress = {
  done: Record<string, number>; // docId -> completed-at (epoch ms)
  quiz: Record<string, {correct: number; total: number}>; // quizId -> best score
  track?: string; // chosen track id
};

const EMPTY: Progress = {done: {}, quiz: {}};
let cache: Progress | null = null;
const listeners = new Set<() => void>();

function read(): Progress {
  if (cache) return cache;
  try {
    const raw = typeof window !== 'undefined' ? window.localStorage.getItem(KEY) : null;
    cache = raw ? {...EMPTY, ...JSON.parse(raw)} : EMPTY;
  } catch {
    cache = EMPTY;
  }
  return cache!;
}

function write(next: Progress) {
  cache = next;
  try {
    window.localStorage.setItem(KEY, JSON.stringify(next));
  } catch {
    /* storage unavailable — keep in memory for this visit */
  }
  listeners.forEach((l) => l());
}

function subscribe(l: () => void) {
  listeners.add(l);
  const onStorage = (e: StorageEvent) => {
    if (e.key === KEY) {
      cache = null;
      l();
    }
  };
  window.addEventListener('storage', onStorage);
  return () => {
    listeners.delete(l);
    window.removeEventListener('storage', onStorage);
  };
}

export function useProgress(): Progress {
  return useSyncExternalStore(subscribe, read, () => EMPTY);
}

export function setDone(docId: string, done: boolean) {
  const p = read();
  const d = {...p.done};
  if (done) d[docId] = Date.now();
  else delete d[docId];
  write({...p, done: d});
}

export function recordQuiz(quizId: string, correct: number, total: number) {
  const p = read();
  const prev = p.quiz[quizId];
  if (prev && prev.correct >= correct) return;
  write({...p, quiz: {...p.quiz, [quizId]: {correct, total}}});
}

export function setTrack(track: string | undefined) {
  write({...read(), track});
}

export function exportProgress(): string {
  return JSON.stringify(read(), null, 2);
}

export function importProgress(json: string) {
  const parsed = JSON.parse(json);
  if (typeof parsed !== 'object' || !parsed.done) throw new Error('Not a Zero to Prod progress file');
  write({...EMPTY, ...parsed});
}

export function resetProgress() {
  write(EMPTY);
}

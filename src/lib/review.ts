// Spaced repetition for quiz questions, Leitner style: each question sits in a
// box; a right answer moves it up a box and brings it back later, a wrong one
// sends it back to box 0 for tomorrow. Memory research is consistent that
// retrieval spread out over growing gaps beats re-reading.

const DAY = 24 * 60 * 60 * 1000;

/** Days until a question in each box is due again. Box 0 = got it wrong. */
export const INTERVAL_DAYS = [1, 3, 7, 16, 35, 80];

export type Card = {box: number; due: number}; // due: epoch ms

/** Where a question goes after an answer. */
export function schedule(prev: Card | undefined, correct: boolean, now = Date.now()): Card {
  const box = correct ? Math.min((prev ? prev.box : 0) + 1, INTERVAL_DAYS.length - 1) : 0;
  return {box, due: now + INTERVAL_DAYS[box] * DAY};
}

/** A question's stable id: its quiz id plus a hash of its text (FNV-1a, as in scripts/build-quiz-bank.mjs). */
export function questionKey(quizId: string, question: string): string {
  let h = 0x811c9dc5;
  for (let i = 0; i < question.length; i++) {
    h ^= question.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return `${quizId}#${(h >>> 0).toString(36)}`;
}

/** Keys of the cards due now, the most overdue first. */
export function dueKeys(cards: Record<string, Card>, now = Date.now()): string[] {
  return Object.entries(cards)
    .filter(([, c]) => c.due <= now)
    .sort((a, b) => a[1].due - b[1].due)
    .map(([k]) => k);
}

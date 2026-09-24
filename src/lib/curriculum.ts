import data from '@site/src/data/curriculum.json';

export type Track = (typeof data.tracks)[number];
export type Chapter = {id: string; title: string; minutes: number; tracks?: string[]};
export type Part = {id: string; n: number; title: string; blurb: string; tracks: string[]; chapters: Chapter[]};

export const tracks: Track[] = data.tracks;
export const parts: Part[] = data.parts as Part[];

export const docId = (part: Part, ch: Chapter) => `${part.id}/${ch.id}`;
export const chapterNumber = (part: Part, idx: number) => `${part.n}.${idx + 1}`;
export const chapterTracks = (part: Part, ch: Chapter) => ch.tracks ?? part.tracks;

export function inTrack(part: Part, ch: Chapter, track?: string) {
  if (!track || track === 'all') return true;
  return chapterTracks(part, ch).includes(track);
}

export type FlatChapter = {part: Part; chapter: Chapter; idx: number; id: string; number: string};

export const flat: FlatChapter[] = parts.flatMap((part) =>
  part.chapters.map((chapter, idx) => ({part, chapter, idx, id: docId(part, chapter), number: chapterNumber(part, idx)})),
);

export function findChapter(id: string): FlatChapter | undefined {
  return flat.find((c) => c.id === id);
}

export function trackChapters(track?: string): FlatChapter[] {
  return flat.filter((c) => inTrack(c.part, c.chapter, track));
}

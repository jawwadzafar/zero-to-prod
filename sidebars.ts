import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';
import curriculum from './src/data/curriculum.json';
// eslint-disable-next-line @typescript-eslint/no-require-imports
const {chapterFile} = require('./plugins/curriculum.js');

// The sidebar is generated from src/data/curriculum.json — the single source
// of truth for parts, chapters, order and tracks. Chapters not written yet are
// skipped here and shown as "coming soon" on the roadmap.
const siteDir = __dirname;

const learn: SidebarsConfig['learn'] = [
  'index',
  'roadmap-guide',
  ...curriculum.parts
    .map((part) => {
      const items = part.chapters
        .filter((ch) => chapterFile(siteDir, part.id, ch.id))
        .map((ch) => `${part.id}/${ch.id}`);
      if (items.length === 0) return null;
      return {
        type: 'category' as const,
        label: `${part.n} · ${part.title}`,
        collapsed: true,
        link: {type: 'generated-index' as const, title: `Part ${part.n} — ${part.title}`, description: part.blurb, slug: `/${part.id}`},
        items,
      };
    })
    .filter((x): x is NonNullable<typeof x> => x !== null),
  'glossary',
];

const sidebars: SidebarsConfig = {learn};
export default sidebars;

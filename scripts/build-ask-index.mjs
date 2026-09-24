// Builds static/ask-index.json: every chapter split into sections (by ## / ###
// headings) so the in-browser "Ask the handbook" panel can retrieve the most
// relevant passages without any server. Runs before `start` and `build`.
import fs from 'node:fs';
import path from 'node:path';
import GithubSlugger from 'github-slugger';

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..');
const curriculum = JSON.parse(fs.readFileSync(path.join(root, 'src/data/curriculum.json'), 'utf8'));

function clean(md) {
  return md
    .replace(/^import .*$/gm, '')
    .replace(/^export .*$/gm, '')
    .replace(/<Quiz[\s\S]*?\/>/g, '') // quizzes are practice, not reference
    .replace(/<SqlPlayground[\s\S]*?\/>/g, '')
    .replace(/<\/?[A-Z][^>]*>/g, '')
    .replace(/^:::(\w+)(\[[^\]]*\])?\s*$/gm, (_, kind, title) => (title ? `${title.slice(1, -1)}: ` : ''))
    .replace(/^:::\s*$/gm, '')
    .replace(/```mermaid[\s\S]*?```/g, '[diagram]')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

const out = [];
for (const part of curriculum.parts) {
  part.chapters.forEach((ch, idx) => {
    let file = null;
    for (const ext of ['.mdx', '.md']) {
      const p = path.join(root, 'docs', part.id, ch.id + ext);
      if (fs.existsSync(p)) file = p;
    }
    if (!file) return;
    const raw = fs.readFileSync(file, 'utf8').replace(/^---[\s\S]*?---\n/, '');
    const slugger = new GithubSlugger();
    const number = `${part.n}.${idx + 1}`;
    const docId = `${part.id}/${ch.id}`;
    let heading = ch.title;
    let anchor = '';
    let buf = [];
    let inFence = false;
    const flush = () => {
      const text = clean(buf.join('\n'));
      if (text.length > 40) {
        out.push({
          id: out.length,
          docId,
          url: `/learn/${docId}${anchor ? '#' + anchor : ''}`,
          chapter: `${number} ${ch.title}`,
          heading,
          text: text.slice(0, 4000),
        });
      }
      buf = [];
    };
    for (const line of raw.split('\n')) {
      if (line.startsWith('```')) inFence = !inFence;
      const m = !inFence && /^(#{2,3})\s+(.*)$/.exec(line);
      if (m) {
        flush();
        const custom = /\{#([\w-]+)\}\s*$/.exec(m[2]);
        heading = m[2].replace(/\s*\{#[\w-]+\}\s*$/, '').replace(/[`*_]/g, '');
        anchor = custom ? custom[1] : slugger.slug(heading);
        continue;
      }
      if (/^#\s/.test(line)) continue;
      buf.push(line);
    }
    flush();
  });
}

fs.mkdirSync(path.join(root, 'static'), {recursive: true});
fs.writeFileSync(path.join(root, 'static/ask-index.json'), JSON.stringify(out));
console.log(`ask-index: ${out.length} passages`);

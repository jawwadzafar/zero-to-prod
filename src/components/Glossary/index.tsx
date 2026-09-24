import React, {useMemo, useState} from 'react';
import Link from '@docusaurus/Link';
import {usePluginData} from '@docusaurus/useGlobalData';
import terms from '@site/src/data/glossary.json';
import {findChapter} from '@site/src/lib/curriculum';
import styles from './styles.module.css';

type Term = {term: string; def: string; doc: string};

// The searchable glossary. Terms live in src/data/glossary.json:
// {"term": "…", "def": "one or two plain sentences", "doc": "<partId>/<chapterId>"}.
export default function Glossary() {
  const [q, setQ] = useState('');
  const available = new Set((usePluginData('curriculum-plugin') as {available: string[]}).available);
  const list = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return (terms as Term[])
      .filter((t) => !needle || t.term.toLowerCase().includes(needle) || t.def.toLowerCase().includes(needle))
      .sort((a, b) => a.term.localeCompare(b.term, 'en', {sensitivity: 'base'}));
  }, [q]);
  const groups = useMemo(() => {
    const m = new Map<string, Term[]>();
    for (const t of list) {
      const letter = /[a-z]/i.test(t.term[0]) ? t.term[0].toUpperCase() : '#';
      m.set(letter, [...(m.get(letter) ?? []), t]);
    }
    return [...m.entries()];
  }, [list]);

  return (
    <div>
      <input
        className={styles.search}
        type="search"
        placeholder={`Filter ${terms.length} terms…`}
        value={q}
        onChange={(e) => setQ(e.target.value)}
        aria-label="Filter glossary"
      />
      {groups.length === 0 && <p>No terms match “{q}”. Try the 💬 Ask button.</p>}
      {groups.map(([letter, items]) => (
        <section key={letter}>
          <h2 id={`letter-${letter}`} className={styles.letter}>
            {letter}
          </h2>
          <dl className={styles.list}>
            {items.map((t) => {
              const ch = findChapter(t.doc);
              return (
                <div key={t.term} className={styles.item} id={t.term.toLowerCase().replace(/[^a-z0-9]+/g, '-')}>
                  <dt>{t.term}</dt>
                  <dd>
                    {t.def}{' '}
                    {ch && available.has(t.doc) && (
                      <Link to={`/learn/${t.doc}`} className={styles.see}>
                        → {ch.number} {ch.chapter.title}
                      </Link>
                    )}
                  </dd>
                </div>
              );
            })}
          </dl>
        </section>
      ))}
    </div>
  );
}

import React from 'react';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import Link from '@docusaurus/Link';
import {tracks, parts, flat} from '@site/src/lib/curriculum';
import {setTrack} from '@site/src/lib/progress';
import styles from './index.module.css';

const LADDER = [
  {n: 0, name: 'Anchor', q: 'What is this like?', ex: 'A port is an apartment number for programs.'},
  {n: 1, name: 'Mechanism', q: 'What actually happens?', ex: 'A program claims a number; the OS routes traffic to it.'},
  {n: 2, name: 'Hands on', q: 'Where do I see it?', ex: 'Your snip server claims :8080. Run lsof and watch.'},
  {n: 3, name: 'What breaks', q: 'How does it go wrong?', ex: '"address already in use" — and how to fix it.'},
  {n: 4, name: 'Judgment', q: 'Why this way?', ex: 'Why real systems hide ports behind a proxy.'},
];

const FEATURES = [
  {icon: '🪜', title: 'Zero to 100%, no gaps', body: 'Every idea climbs the same five rungs, from an everyday analogy to real engineering judgment. You never hit a step you can’t climb.'},
  {icon: '🛠️', title: 'Build one real system', body: 'You build snip — a production-grade link shortener — from a first HTTP handler to Postgres, Redis, queues, Docker, Kubernetes, CI/CD, cloud and AI features.'},
  {icon: '🧩', title: 'Interactive, not just reading', body: 'Quizzes that explain every answer, a real Postgres database running in your browser, labs on your own laptop, and progress that saves itself.'},
  {icon: '💬', title: 'Ask the handbook', body: 'Stuck? Ask in plain words. Get the exact sections that answer it — or, with your own API key, a tutor-style answer that cites them.'},
  {icon: '🧭', title: 'Tracks for real jobs', body: 'Backend engineer, DevOps/platform engineer, or AI engineer. One shared foundation, then the path to the role you want.'},
  {icon: '🆓', title: 'Free and open, forever', body: 'No paywall, no sign-up, no ads. Open source on GitHub — fix a typo, suggest a chapter, fork it for your team.'},
];

export default function Home() {
  const hours = Math.round(flat.reduce((s, c) => s + c.chapter.minutes, 0) / 60);
  return (
    <Layout title="Learn backend, DevOps & AI engineering from zero" description="A free, interactive handbook that takes you from basic tech literacy to working backend, DevOps and AI engineer by building and shipping one real system.">
      <header className={styles.hero}>
        <div className="container">
          <p className={styles.kicker}>Free · Open source · Interactive</p>
          <Heading as="h1" className={styles.title}>
            From <span className={styles.accent}>zero</span> to <span className={styles.accent}>production</span>.
          </Heading>
          <p className={styles.subtitle}>
            Learn backend, DevOps and AI engineering from first principles — one small step at a time — by building and shipping one
            real system. If you can use a computer, you can start here.
          </p>
          <div className={styles.ctas}>
            <Link className="button button--primary button--lg" to="/learn">
              Start learning →
            </Link>
            <Link className="button button--secondary button--lg" to="/roadmap">
              See the roadmap
            </Link>
          </div>
          <p className={styles.stats}>
            {parts.length} parts · {flat.length} chapters · ~{hours} hours · 1 real system
          </p>
        </div>
      </header>

      <main>
        <section className="container margin-vert--xl">
          <Heading as="h2" className={styles.center}>
            Pick where you want to end up
          </Heading>
          <p className={styles.center}>Everyone starts with the same foundations. Then choose a track — you can switch any time.</p>
          <div className={styles.tracks}>
            {tracks.map((t) => (
              <Link key={t.id} to="/roadmap" className={styles.track} style={{['--c' as string]: t.color}} onClick={() => setTrack(t.id)}>
                <span className={styles.trackEmoji}>{t.emoji}</span>
                <strong>{t.title}</strong>
                <span>{t.blurb}</span>
              </Link>
            ))}
          </div>
        </section>

        <section className={styles.band}>
          <div className="container">
            <Heading as="h2" className={styles.center}>
              How every idea is taught: the ladder
            </Heading>
            <p className={styles.center}>Stop after any rung and you’ve still learned something true. Finish all five and you can make decisions.</p>
            <ol className={styles.ladder}>
              {LADDER.map((r) => (
                <li key={r.n} className={styles.rung}>
                  <span className={styles.rungN}>{r.n}</span>
                  <strong>{r.name}</strong>
                  <em>{r.q}</em>
                  <span>{r.ex}</span>
                </li>
              ))}
            </ol>
          </div>
        </section>

        <section className="container margin-vert--xl">
          <div className={styles.features}>
            {FEATURES.map((f) => (
              <div key={f.title} className={styles.feature}>
                <div className={styles.featureIcon}>{f.icon}</div>
                <Heading as="h3">{f.title}</Heading>
                <p>{f.body}</p>
              </div>
            ))}
          </div>
        </section>

        <section className={styles.band}>
          <div className="container">
            <Heading as="h2" className={styles.center}>
              What’s inside
            </Heading>
            <div className={styles.parts}>
              {parts.map((p) => (
                <Link key={p.id} to="/roadmap" className={styles.partCard}>
                  <span className={styles.partN}>{p.n}</span>
                  <span>
                    <strong>{p.title}</strong>
                    <br />
                    <small>{p.blurb}</small>
                  </span>
                </Link>
              ))}
            </div>
          </div>
        </section>

        <section className="container margin-vert--xl">
          <div className={styles.final}>
            <Heading as="h2">Ready? It starts with a single chapter.</Heading>
            <p>Ten minutes to learn how the handbook works. Then set up your machine and meet the system you’ll build.</p>
            <Link className="button button--primary button--lg" to="/learn">
              Start with chapter 0.1 →
            </Link>
          </div>
        </section>
      </main>
    </Layout>
  );
}

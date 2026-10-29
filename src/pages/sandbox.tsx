import React from 'react';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import Link from '@docusaurus/Link';
import Terminal from '@site/src/components/Terminal';

export default function SandboxPage() {
  return (
    <Layout title="Linux sandbox" description="A real Linux running inside your browser, for practising the command line without installing anything.">
      <main className="container margin-vert--lg" style={{maxWidth: 900}}>
        <Heading as="h1">Linux sandbox</Heading>
        <p>
          A real Linux machine, emulated inside this page. Use it for the command-line labs in{' '}
          <Link to="/learn/linux/shell-and-filesystem">Part 2</Link> if you don't have a terminal yet, or to try something
          without touching your own computer. Click the black screen before typing.
        </p>
        <Terminal />
        <Heading as="h2">What it can and can't do</Heading>
        <ul>
          <li>
            It has <code>bash</code>, the everyday commands (from BusyBox: <code>ls</code>, <code>grep</code>, <code>sort</code>,{' '}
            <code>ps</code>, <code>chmod</code>…), <code>jq</code>, <code>nano</code> and <code>sudo</code>. You're logged in as{' '}
            <code>learner</code>, password <code>learner</code>.
          </li>
          <li>
            BusyBox commands accept fewer options than the full GNU versions on a typical Linux server or WSL, so an
            occasional flag may be missing.
          </li>
          <li>
            There's no network and no Docker: labs from <Link to="/learn/linux/ssh-and-remote-machines">2.6</Link> onward need
            your own machine (<Link to="/learn/start/set-up-your-machine">chapter 0.2</Link>).
          </li>
          <li>Everything stays in this tab, and reloading the page starts a fresh machine.</li>
        </ul>
        <p>
          How it's built: <a href="https://github.com/copy/v86">v86</a> emulates a PC in WebAssembly, and the Linux image is
          built from <a href="https://github.com/jawwadzafar/zero-to-prod/tree/main/sandbox">sandbox/</a> in this repository,
          with the source of everything in it published alongside each release.
        </p>
      </main>
    </Layout>
  );
}

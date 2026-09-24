import React from 'react';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import Roadmap from '@site/src/components/Roadmap';

export default function RoadmapPage() {
  return (
    <Layout title="Roadmap" description="The full Zero to Prod roadmap: every part and chapter, filtered by track, with your progress.">
      <main className="container margin-vert--lg">
        <Heading as="h1">The roadmap</Heading>
        <p>
          Every chapter in the handbook, in order. Pick a <strong>track</strong> to highlight the chapters for the job you want —
          the others fade but stay clickable. Your progress is saved in this browser only.
        </p>
        <Roadmap />
      </main>
    </Layout>
  );
}

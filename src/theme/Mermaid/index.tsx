/**
 * Replaces @docusaurus/theme-mermaid's diagram component.
 *
 * The stock component renders every diagram once for the default (light)
 * theme and again as soon as the page learns the reader prefers dark mode,
 * reusing the same element id. Mermaid keeps one global config and renders
 * asynchronously, so those overlapping renders clobbered each other and about
 * half the diagrams on a page came back as an empty SVG — blank boxes in dark
 * mode. Here every render waits its turn in one queue, gets a fresh id, is
 * retried once if Mermaid returns nothing, and is dropped if the color mode
 * changed while it was in flight.
 */
import React, {useEffect, useRef, useState, type ReactNode} from 'react';
import ErrorBoundary from '@docusaurus/ErrorBoundary';
import {ErrorBoundaryErrorMessageFallback} from '@docusaurus/theme-common';
import {MermaidContainerClassName, useMermaidConfig} from '@docusaurus/theme-mermaid/client';
import type {MermaidConfig, RenderResult} from 'mermaid';
import styles from './styles.module.css';

const MIN_READABLE_WIDTH = 560;

let queue: Promise<unknown> = Promise.resolve();
let renderCount = 0;

// Run renders one after another: Mermaid's initialize() + render() pair is not
// safe to interleave.
function enqueue<T>(task: () => Promise<T>): Promise<T> {
  const next = queue.then(task, task);
  queue = next.catch(() => undefined);
  return next;
}

async function renderOnce(text: string, config: MermaidConfig): Promise<RenderResult> {
  const mermaid = (await import('mermaid')).default;
  mermaid.initialize(config);
  const id = `mermaid-svg-${++renderCount}`;
  try {
    return await mermaid.render(id, text);
  } finally {
    // On error Mermaid leaves a stray error SVG in the page.
    document.querySelector(`#d${id}`)?.remove();
  }
}

function renderMermaid(text: string, config: MermaidConfig): Promise<RenderResult> {
  return enqueue(async () => {
    const result = await renderOnce(text, config);
    return result.svg ? result : renderOnce(text, config);
  });
}

function MermaidRenderer({value}: {value: string}): ReactNode {
  const config = useMermaidConfig();
  const [result, setResult] = useState<RenderResult | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let current = true;
    renderMermaid(value, config).then(
      (r) => current && setResult(r),
      (e: unknown) => current && setError(e instanceof Error ? e : new Error(String(e))),
    );
    return () => {
      current = false;
    };
  }, [value, config]);

  useEffect(() => {
    const div = ref.current;
    if (!result || !div) return;
    result.bindFunctions?.(div);
    // On a phone, a wide flowchart squeezed to the screen width ends up with
    // unreadable text. Keep it at least its natural width (capped), and let the
    // container scroll sideways instead.
    const svg = div.querySelector('svg');
    const natural = svg?.viewBox.baseVal?.width;
    if (svg && natural) svg.style.minWidth = `${Math.min(natural, MIN_READABLE_WIDTH)}px`;
  }, [result]);

  if (error) throw error; // shown by the ErrorBoundary below
  if (!result) return null;
  return (
    <div
      ref={ref}
      className={`${MermaidContainerClassName} ${styles.container}`}
      // eslint-disable-next-line react/no-danger
      dangerouslySetInnerHTML={{__html: result.svg}}
    />
  );
}

export default function Mermaid(props: {value: string}): ReactNode {
  return (
    <ErrorBoundary fallback={(params) => <ErrorBoundaryErrorMessageFallback {...params} />}>
      <MermaidRenderer {...props} />
    </ErrorBoundary>
  );
}

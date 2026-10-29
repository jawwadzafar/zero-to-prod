import React, {useEffect, useRef, useState} from 'react';
import BrowserOnly from '@docusaurus/BrowserOnly';
import useBaseUrl from '@docusaurus/useBaseUrl';
import styles from './styles.module.css';

// A real Linux in the page: the v86 emulator (a 32-bit PC in WebAssembly)
// booting the sandbox image built from sandbox/ in this repo. Nothing loads
// until the reader presses Start, and nothing they type leaves the tab.
const V86 = 'https://cdn.jsdelivr.net/npm/v86@0.5.472/build';
const BIOS = 'https://cdn.jsdelivr.net/gh/copy/v86@cc37f35c43bf14865d74ad76879ecf9410f69eb1/bios';

/**
 *   <Terminal />   a "Start Linux" card that boots the sandbox on demand
 */
export default function Terminal() {
  return <BrowserOnly fallback={<div className={styles.card}>Linux sandbox (loads in your browser)</div>}>{() => <Sandbox />}</BrowserOnly>;
}

type Status = 'idle' | 'checking' | 'unavailable' | 'booting' | 'running' | 'error';

function Sandbox() {
  const image = useBaseUrl('/sandbox/zero-to-prod-linux.bin');
  const screenRef = useRef<HTMLDivElement>(null);
  const emuRef = useRef<any>(null);
  const [status, setStatus] = useState<Status>('idle');
  const [size, setSize] = useState<number | null>(null);
  const [line, setLine] = useState('');

  // Learn the image's size up front (and whether this build has one at all).
  useEffect(() => {
    let cancelled = false;
    setStatus('checking');
    fetch(image, {method: 'HEAD'})
      .then((r) => {
        if (cancelled) return;
        if (!r.ok) return setStatus('unavailable');
        setSize(Number(r.headers.get('content-length')) || null);
        setStatus('idle');
      })
      .catch(() => !cancelled && setStatus('unavailable'));
    return () => {
      cancelled = true;
      emuRef.current?.destroy?.();
    };
  }, [image]);

  async function start() {
    setStatus('booting');
    try {
      const {V86: Emulator} = await import(/* webpackIgnore: true */ `${V86}/libv86.mjs`);
      const emu = new Emulator({
        wasm_path: `${V86}/v86.wasm`,
        memory_size: 128 * 1024 * 1024,
        vga_memory_size: 2 * 1024 * 1024,
        screen_container: screenRef.current,
        bios: {url: `${BIOS}/seabios.bin`},
        vga_bios: {url: `${BIOS}/vgabios.bin`},
        bzimage: {url: image},
        cmdline: 'tsc=reliable mitigations=off random.trust_cpu=on quiet loglevel=1',
        disable_mouse: true,
        autostart: true,
      });
      emuRef.current = emu;
      emu.add_listener('emulator-started', () => setStatus('running'));
    } catch {
      setStatus('error');
    }
  }

  function stop() {
    emuRef.current?.destroy?.();
    emuRef.current = null;
    setStatus('idle');
  }

  // The emulated keyboard holds only a few keystrokes at a time; a long paste
  // sent at once overflows it and keys are lost. Send it in small pieces.
  async function send(e?: React.FormEvent) {
    e?.preventDefault();
    const text = `${line.replace(/\r\n?/g, '\n').replace(/\n+$/, '')}\n`;
    setLine('');
    for (let i = 0; i < text.length && emuRef.current; i += 16) {
      emuRef.current.keyboard_send_text(text.slice(i, i + 16));
      await new Promise((r) => setTimeout(r, 40));
    }
  }

  const started = status === 'booting' || status === 'running';
  const mb = size ? `${Math.round((size + 300_000) / 1_000_000)} MB` : 'a few MB';

  return (
    <div className={styles.card}>
      <div className={styles.head}>
        <strong>🐧 Linux sandbox</strong>
        {started ? (
          <button type="button" className="button button--sm button--secondary" onClick={stop}>
            Stop
          </button>
        ) : (
          <button
            type="button"
            className="button button--sm button--primary"
            onClick={start}
            disabled={status === 'unavailable' || status === 'checking'}>
            Start Linux
          </button>
        )}
      </div>
      {!started && (
        <p className={styles.note}>
          {status === 'unavailable'
            ? 'The sandbox image is only included in the published site. Use your own terminal here (chapter 0.2).'
            : status === 'error'
              ? "The sandbox couldn't start in this browser. Your own terminal works for every lab (chapter 0.2)."
              : `A real Linux (bash, BusyBox tools, jq, nano, sudo) running inside this page. Starting it downloads about ${mb} once; everything you type stays in this tab, and reloading starts fresh.`}
        </p>
      )}
      <div className={styles.screen} ref={screenRef} hidden={!started} onClick={() => screenRef.current?.focus()} tabIndex={0}>
        <div className={styles.text} />
        <canvas style={{display: 'none'}} />
      </div>
      {status === 'booting' && <p className={styles.note}>Booting… (about 10–20 seconds)</p>}
      {started && (
        <form className={styles.send} onSubmit={send}>
          <textarea
            className={styles.input}
            rows={Math.min(8, line.split('\n').length)}
            value={line}
            onChange={(e) => setLine(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) send(e);
            }}
            placeholder="Or paste commands here and press Enter (Shift+Enter for a new line)"
            aria-label="Command to send to the sandbox"
          />
        </form>
      )}
    </div>
  );
}

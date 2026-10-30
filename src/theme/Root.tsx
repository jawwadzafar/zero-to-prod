import React, {useEffect, type ReactNode} from 'react';
import AskPanel from '@site/src/components/AskPanel';

// Printing should include the answers hidden in collapsed <details> blocks.
function useOpenDetailsWhenPrinting() {
  useEffect(() => {
    let opened: HTMLDetailsElement[] = [];
    const before = () => {
      // Add to the list, not replace it: some browsers fire beforeprint twice.
      const closed = [...document.querySelectorAll('details:not([open])')] as HTMLDetailsElement[];
      closed.forEach((d) => (d.open = true));
      opened.push(...closed);
    };
    const after = () => {
      opened.forEach((d) => (d.open = false));
      opened = [];
    };
    window.addEventListener('beforeprint', before);
    window.addEventListener('afterprint', after);
    return () => {
      window.removeEventListener('beforeprint', before);
      window.removeEventListener('afterprint', after);
    };
  }, []);
}

// Wraps every page: adds the floating "Ask the handbook" assistant.
export default function Root({children}: {children: ReactNode}) {
  useOpenDetailsWhenPrinting();
  return (
    <>
      {children}
      <AskPanel />
    </>
  );
}

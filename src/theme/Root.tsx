import React, {type ReactNode} from 'react';
import AskPanel from '@site/src/components/AskPanel';

// Wraps every page: adds the floating "Ask the handbook" assistant.
export default function Root({children}: {children: ReactNode}) {
  return (
    <>
      {children}
      <AskPanel />
    </>
  );
}

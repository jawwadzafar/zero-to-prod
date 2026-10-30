import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';
import './styles.css';

const root = document.getElementById('root');
if (!root) throw new Error('index.html has no #root element');

// StrictMode adds development-only checks, such as running every effect twice
// to flush out effects that don't clean up after themselves (chapter 16.5).
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

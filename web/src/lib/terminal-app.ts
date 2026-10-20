import { useSyncExternalStore } from 'react';

const KEY = 'reef.terminalApp';
const listeners = new Set<() => void>();

// The terminal app the Terminal toggle opens a worktree in, or null for the built-in pane.
export function getTerminalApp() {
  try {
    return localStorage.getItem(KEY) || null;
  } catch {
    return null;
  }
}

export function setTerminalApp(app: string | null) {
  try {
    if (app) localStorage.setItem(KEY, app);
    else localStorage.removeItem(KEY);
  } catch {
    // storage can be blocked; the choice then lasts until reload
  }
  listeners.forEach((l) => l());
}

export function useTerminalApp() {
  return useSyncExternalStore((l) => {
    listeners.add(l);
    return () => listeners.delete(l);
  }, getTerminalApp);
}

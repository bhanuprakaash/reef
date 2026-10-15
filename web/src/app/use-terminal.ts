import { useEffect } from 'react';
import { useOutletContext } from 'react-router';
import type { LayoutContext } from '@/app/layout';

// A page with a folder to open a shell in calls this; the window's Terminal toggle shows only
// while one does, and the page renders its TerminalPane while the toggle is on.
export function useTerminal(hasFolder: boolean) {
  const { terminalOpen, toggleTerminal, setHasTerminal } = useOutletContext<LayoutContext>();
  useEffect(() => {
    setHasTerminal(hasFolder);
    return () => setHasTerminal(false);
  }, [hasFolder, setHasTerminal]);
  return { open: hasFolder && terminalOpen, hide: toggleTerminal };
}

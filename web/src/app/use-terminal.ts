import { useEffect } from 'react';
import { useOutletContext } from 'react-router';
import type { LayoutContext } from '@/app/layout';

export function useTerminal(hasFolder: boolean) {
  const { terminalOpen, toggleTerminal, setHasTerminal } = useOutletContext<LayoutContext>();
  useEffect(() => {
    setHasTerminal(hasFolder);
    return () => setHasTerminal(false);
  }, [hasFolder, setHasTerminal]);
  return { open: hasFolder && terminalOpen, hide: toggleTerminal };
}

import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import { useEffect, useRef } from 'react';
import '@xterm/xterm/css/xterm.css';

// TerminalPane runs the shell for cwd (a feature's worktree, or a project checkout for a
// quick session) in an xterm.js terminal backed by node-pty in the desktop app. A plain
// browser pointed at reef serve has no window.reef, so it renders nothing there.
export function TerminalPane({ cwd }: { cwd: string }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const available = typeof window !== 'undefined' && window.reef != null;

  useEffect(() => {
    const reef = window.reef;
    const container = containerRef.current;
    if (!reef || !container) return;

    const term = new Terminal({ convertEol: true });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(container);
    fit.fit();

    const stopData = reef.onTerminalData(cwd, (data) => term.write(data));
    term.onData((data) => reef.writeTerminal(cwd, data));
    term.onResize(({ cols, rows }) => reef.resizeTerminal(cwd, cols, rows));
    reef.openTerminal(cwd).then((buffer) => term.write(buffer));

    const observer = new ResizeObserver(() => fit.fit());
    observer.observe(container);

    return () => {
      observer.disconnect();
      stopData();
      term.dispose();
    };
  }, [cwd]);

  if (!available) return null;

  return (
    <div className="h-64 overflow-hidden rounded border border-neutral-200 p-1 dark:border-neutral-800">
      <div ref={containerRef} className="h-full" />
    </div>
  );
}

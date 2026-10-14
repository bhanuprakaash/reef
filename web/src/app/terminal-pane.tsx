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

    let disposed = false;
    let stopData = () => {};
    const term = new Terminal({ convertEol: true });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(container);
    fit.fit();

    term.onData((data) => reef.writeTerminal(cwd, data));
    term.onResize(({ cols, rows }) => reef.resizeTerminal(cwd, cols, rows));

    // The shell is sized from term's own cols/rows at open, not a default, so the pty
    // matches xterm from its first output. The live subscription starts only after the
    // replayed buffer is written, so a chunk sent while open is in flight isn't shown twice.
    reef
      .openTerminal(cwd, term.cols, term.rows)
      .then((buffer) => {
        if (disposed) return;
        term.write(buffer);
        stopData = reef.onTerminalData(cwd, (data) => term.write(data));
      })
      .catch((err: Error) => {
        if (!disposed) term.write(`${err.message}\n`);
      });

    const observer = new ResizeObserver(() => fit.fit());
    observer.observe(container);

    return () => {
      disposed = true;
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

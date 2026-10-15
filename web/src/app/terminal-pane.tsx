import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import { useEffect, useRef } from 'react';
import { Button } from '@/components/button';
import '@xterm/xterm/css/xterm.css';

// TerminalPane runs the shell for cwd (a feature's worktree, or a project checkout for a
// quick session) in an xterm.js terminal backed by node-pty in the desktop app. A plain
// browser pointed at reef serve has no window.reef, so it renders nothing there.
export function TerminalPane({ cwd, onHide }: { cwd: string; onHide?: () => void }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal>(null);
  const available = typeof window !== 'undefined' && window.reef != null;

  useEffect(() => {
    const reef = window.reef;
    const container = containerRef.current;
    if (!reef || !container) return;

    let disposed = false;
    let stopData = () => {};
    const term = new Terminal({ convertEol: true, theme: { background: '#161616' } });
    termRef.current = term;
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
    <section aria-label="Terminal" className="bg-[#161616]">
      <div className="flex h-9 items-center gap-2 border-b border-[#262626] pl-4 pr-2.5 text-[12.5px] text-muted">
        <span className="text-fg">Terminal</span>
        <span className="min-w-0 truncate font-mono">{cwd}</span>
        <Button
          variant="ghost"
          size="sm"
          type="button"
          aria-label="Clear terminal"
          className="ml-auto"
          onClick={() => termRef.current?.clear()}
        >
          Clear
        </Button>
        {onHide && (
          <button
            type="button"
            aria-label="Hide terminal"
            onClick={onHide}
            className="flex size-[26px] shrink-0 items-center justify-center rounded-md text-muted hover:bg-card"
          >
            <svg
              width="12"
              height="12"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.4"
              strokeLinecap="round"
              aria-hidden="true"
            >
              <path d="M6 9l6 6 6-6" />
            </svg>
          </button>
        )}
      </div>
      <div className="h-56 px-4 py-2">
        <div ref={containerRef} className="h-full" />
      </div>
    </section>
  );
}

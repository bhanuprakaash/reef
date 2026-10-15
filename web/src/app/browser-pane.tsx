import { useEffect, useRef, useState } from 'react';
import { Button } from '@/components/button';
import { TextField } from '@/components/text-field';

const HAS_SCHEME = /^[a-z][a-z0-9+.-]*:/i;

// BrowserPane is the one browser for the whole window, shared by every
// channel. Its content is a native Electron view drawn on top of this pane by
// the desktop app; this component only runs the address bar and keeps the
// native view's bounds in sync with the pane's own size. Plain Chrome has no
// window.reef, and so no native view to show, so it renders nothing.
export function BrowserPane() {
  const containerRef = useRef<HTMLDivElement>(null);
  const [url, setUrl] = useState('');
  const [takenOver, setTakenOver] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const available = typeof window !== 'undefined' && window.reef != null;

  useEffect(() => {
    const reef = window.reef;
    const container = containerRef.current;
    if (!reef || !container) return;

    const reportBounds = () => {
      // getBoundingClientRect returns a DOMRect, whose fields are accessors
      // that IPC's structured clone drops; send a plain object instead.
      const { x, y, width, height } = container.getBoundingClientRect();
      reef.setBrowserBounds({ x, y, width, height });
    };
    reportBounds();
    const observer = new ResizeObserver(reportBounds);
    observer.observe(container);
    window.addEventListener('resize', reportBounds);
    const stopNavigated = reef.onBrowserNavigated(setUrl);

    return () => {
      observer.disconnect();
      window.removeEventListener('resize', reportBounds);
      stopNavigated();
    };
  }, []);

  const navigate = (e: React.FormEvent) => {
    e.preventDefault();
    const target = HAS_SCHEME.test(url) ? url : `http://${url}`;
    setError(null);
    window.reef?.navigateBrowser(target).catch((err: Error) => setError(err.message));
  };

  const toggleTakeOver = async () => {
    const next = await window.reef?.setTakeOver(!takenOver);
    setTakenOver(next ?? !takenOver);
  };

  if (!available) return null;

  return (
    <aside className="flex w-96 shrink-0 flex-col border-l border-hairline">
      <form onSubmit={navigate} className="flex gap-2 border-b border-hairline p-2">
        <TextField
          type="text"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="URL"
          className="w-full"
        />
        <Button type="button" onClick={toggleTakeOver}>
          {takenOver ? 'Hand back' : 'Take over'}
        </Button>
      </form>
      {error && <p className="text-sm text-danger px-2 py-1">{error}</p>}
      <div ref={containerRef} className="min-h-0 flex-1" />
    </aside>
  );
}

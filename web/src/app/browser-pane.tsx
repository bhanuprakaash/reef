import { useEffect, useRef, useState } from 'react';
import { buttonClass, errorClass, inputClass } from '@/utils/classes';

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
    <aside className="flex w-96 shrink-0 flex-col border-l border-neutral-200 dark:border-neutral-800">
      <form
        onSubmit={navigate}
        className="flex gap-2 border-b border-neutral-200 p-2 dark:border-neutral-800"
      >
        <input
          type="text"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="URL"
          className={`${inputClass} w-full`}
        />
        <button type="button" onClick={toggleTakeOver} className={buttonClass}>
          {takenOver ? 'Hand back' : 'Take over'}
        </button>
      </form>
      {error && <p className={`${errorClass} px-2 py-1`}>{error}</p>}
      <div ref={containerRef} className="min-h-0 flex-1" />
    </aside>
  );
}

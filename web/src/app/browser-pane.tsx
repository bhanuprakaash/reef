import { useEffect, useRef, useState } from 'react';
import { inputClass, mutedClass } from '@/utils/classes';

// BrowserPane is the one browser for the whole window, shared by every
// channel. Its content is a native Electron view drawn on top of this pane by
// the desktop app; this component only runs the address bar and keeps the
// native view's bounds in sync with the pane's own size.
export function BrowserPane() {
  const containerRef = useRef<HTMLDivElement>(null);
  const [url, setUrl] = useState('');
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
    window.reef?.navigateBrowser(url);
  };

  return (
    <aside className="flex w-96 shrink-0 flex-col border-l border-neutral-200 dark:border-neutral-800">
      <form onSubmit={navigate} className="border-b border-neutral-200 p-2 dark:border-neutral-800">
        <input
          type="text"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          disabled={!available}
          placeholder={available ? 'URL' : 'Open this in the Reef app to use the browser'}
          className={`${inputClass} w-full disabled:opacity-50`}
        />
      </form>
      <div ref={containerRef} className="min-h-0 flex-1">
        {!available && <p className={`${mutedClass} p-2`}>Browser pane needs the Reef app.</p>}
      </div>
    </aside>
  );
}

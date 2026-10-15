import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Button } from '@/components/button';
import { Creature } from '@/components/creature';

const HAS_SCHEME = /^[a-z][a-z0-9+.-]*:/i;
const WIDTH_KEY = 'reef.browserPane.width';
const DEFAULT_WIDTH = 440;
const MIN_WIDTH = 320;
const MAX_SHARE = 0.5;

function storedWidth() {
  try {
    const width = Number(localStorage.getItem(WIDTH_KEY));
    return width >= MIN_WIDTH ? width : DEFAULT_WIDTH;
  } catch {
    return DEFAULT_WIDTH;
  }
}

export type BrowserDriver = { role: string; face?: string; action: string };

const navButton =
  'flex size-[30px] shrink-0 items-center justify-center rounded-lg text-soft hover:bg-card disabled:text-faint disabled:hover:bg-transparent';

function NavButton({
  label,
  disabled,
  onClick,
  children,
}: {
  label: string;
  disabled?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
      className={navButton}
    >
      <svg
        width="16"
        height="16"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        aria-hidden="true"
      >
        {children}
      </svg>
    </button>
  );
}

// BrowserPane is the one browser for the whole window, shared by every
// channel. Its content is a native Electron view drawn on top of this pane by
// the desktop app; this component only runs the address bar and keeps the
// native view's bounds in sync with the pane's own size (a hidden pane reports
// a zero size, which hides the view). Plain Chrome has no window.reef, and so
// no native view to show, so it renders nothing.
export function BrowserPane({ open, driver }: { open: boolean; driver: BrowserDriver | null }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [url, setUrl] = useState('');
  const [canGo, setCanGo] = useState({ back: false, forward: false });
  const [takenOver, setTakenOver] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [width, setWidth] = useState(storedWidth);
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
    const stopNavigated = reef.onBrowserNavigated((state) => {
      setUrl(state.url);
      setCanGo({ back: state.canGoBack, forward: state.canGoForward });
    });

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
    window.reef
      ?.navigateBrowser(target)
      .catch((err: Error) =>
        setError(err.message.replace(/^Error invoking remote method '[^']*': (Error: )?/, '')),
      );
  };

  const startResize = (e: React.PointerEvent<HTMLDivElement>) => {
    const handle = e.currentTarget;
    const startX = e.clientX;
    const startWidth = width;
    const max = Math.max(MIN_WIDTH, window.innerWidth * MAX_SHARE);
    let latest = startWidth;
    handle.setPointerCapture(e.pointerId);
    const move = (ev: PointerEvent) => {
      if (ev.buttons === 0) return end();
      latest = Math.round(Math.min(max, Math.max(MIN_WIDTH, startWidth + startX - ev.clientX)));
      setWidth(latest);
    };
    const end = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', end);
      handle.removeEventListener('lostpointercapture', end);
      try {
        localStorage.setItem(WIDTH_KEY, String(latest));
      } catch {
        // storage can be blocked; the width then lasts until reload
      }
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', end);
    handle.addEventListener('lostpointercapture', end);
  };

  const toggleTakeOver = async () => {
    const next = await window.reef?.setTakeOver(!takenOver);
    setTakenOver(next ?? !takenOver);
  };

  if (!available) return null;

  const takeOverButton = (
    <Button type="button" variant="primary" size="sm" onClick={toggleTakeOver}>
      {takenOver ? 'Hand back' : 'Take over'}
    </Button>
  );
  const banner = takenOver || driver !== null;

  return (
    <aside
      aria-label="Browser"
      style={{ width }}
      className={`${open ? 'flex' : 'hidden'} relative max-w-[50vw] min-w-[320px] shrink-0 flex-col border-l border-hairline bg-sidebar`}
    >
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize browser"
        onPointerDown={startResize}
        className="group absolute inset-y-0 -left-1 z-10 w-2 cursor-col-resize touch-none"
      >
        <div className="mx-auto h-full w-0.5 bg-transparent group-hover:bg-line group-active:bg-line" />
      </div>
      <form
        onSubmit={navigate}
        className="app-drag flex min-h-14 items-center gap-1.5 border-b border-hairline px-2.5 py-2"
      >
        <NavButton label="Back" disabled={!canGo.back} onClick={() => window.reef?.browserBack()}>
          <path d="M15 5l-7 7 7 7" />
        </NavButton>
        <NavButton
          label="Forward"
          disabled={!canGo.forward}
          onClick={() => window.reef?.browserForward()}
        >
          <path d="M9 5l7 7-7 7" />
        </NavButton>
        <NavButton label="Reload" onClick={() => window.reef?.browserReload()}>
          <path d="M20 12a8 8 0 1 1-2.3-5.7M20 4v5h-5" />
        </NavButton>
        <input
          type="text"
          aria-label="URL"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="URL"
          className="h-8 min-w-0 flex-1 rounded-pill bg-field px-3 text-[13px] text-soft outline-none placeholder:text-muted focus-visible:ring-3 focus-visible:ring-ring"
        />
        {!banner && takeOverButton}
      </form>
      {banner && (
        <div className="flex items-center gap-2.5 border-b border-banner-line bg-banner-bg px-3.5 py-2.5 text-[13px]">
          {!takenOver && driver && (
            <Creature role={driver.role} face={driver.face} size={22} state="working" />
          )}
          <span role="status" className="flex-1 text-banner-fg">
            {takenOver || !driver ? (
              'You have the browser'
            ) : (
              <>
                <span className="capitalize">{driver.role}</span> is driving · {driver.action}
              </>
            )}
          </span>
          {takeOverButton}
        </div>
      )}
      {error && <p className="px-2 py-1 text-sm text-danger">{error}</p>}
      <div ref={containerRef} className="m-3.5 min-h-0 flex-1" />
    </aside>
  );
}

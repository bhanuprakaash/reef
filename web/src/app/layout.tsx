import { useEffect, useRef, useState } from 'react';
import { Outlet, useLocation, useMatch, useNavigate } from 'react-router';
import { BrowserPane, type BrowserDriver } from '@/app/browser-pane';
import { FirstRun } from '@/app/first-run';
import { PaneToggles } from '@/app/pane-toggles';
import { Sidebar } from '@/app/sidebar';
import { TabBar } from '@/app/tab-bar';
import { Unreachable } from '@/app/unreachable';
import { useProfile } from '@/lib/api/get-profile';
import { useTerminalApp } from '@/lib/terminal-app';
import { useDismiss } from '@/lib/use-dismiss';
import { useStoredFlag } from '@/lib/use-stored-flag';
import { useProjects } from '@/modules/projects/api/get-projects';

export type LayoutContext = {
  browserOpen: boolean;
  terminalOpen: boolean;
  toggleBrowser: () => void;
  toggleTerminal?: () => void;
  hideTerminal: () => void;
  setHasTerminal: (has: boolean) => void;
  setBrowserDriver: (driver: BrowserDriver | null) => void;
};

export function Layout() {
  const projects = useProjects();
  const [browserOpen, toggleBrowser] = useStoredFlag('reef.browserPane.open');
  const [paneOpen, togglePane] = useStoredFlag('reef.terminalPane.open');
  const { pathname } = useLocation();
  const onSpace = useMatch('/spaces/:id') !== null;
  const [launching, setLaunching] = useState(false);
  const [launchPath, setLaunchPath] = useState(pathname);
  const [sheetOpen, setSheetOpen] = useState(false);
  const asideRef = useRef<HTMLElement>(null);
  useDismiss(sheetOpen, asideRef, () => setSheetOpen(false));
  if (launchPath !== pathname) {
    setLaunchPath(pathname);
    setLaunching(false);
    setSheetOpen(false);
  }
  const external = useTerminalApp() !== null;
  const terminalOpen = external ? launching : paneOpen;
  const toggleTerminal = external ? () => setLaunching((on) => !on) : togglePane;
  const hideTerminal = external ? () => setLaunching(false) : togglePane;
  const [hasTerminal, setHasTerminal] = useState(false);
  const [browserDriver, setBrowserDriver] = useState<BrowserDriver | null>(null);
  const profile = useProfile();
  const desktop = typeof window !== 'undefined' && window.reef != null;
  const navigate = useNavigate();
  useEffect(() => {
    if (!desktop) return;
    function onKey(e: KeyboardEvent) {
      if (!e.metaKey || (e.key !== '[' && e.key !== ']')) return;
      const el = e.target as HTMLElement;
      if (el.closest('input, textarea, [contenteditable]')) return;
      e.preventDefault();
      navigate(e.key === '[' ? -1 : 1);
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [desktop, navigate]);
  if (projects.isPending || profile.isPending) return null;
  if (projects.data?.length === 0 || profile.data === null) return <FirstRun />;

  return (
    <div className="relative flex h-screen flex-col bg-main font-sans text-fg md:flex-row">
      <div className="app-drag absolute inset-x-0 top-0 h-3" />
      {sheetOpen && <div className="fixed inset-0 z-30 bg-scrim/70 md:hidden" />}
      <aside
        ref={asideRef}
        onClick={(e) => {
          if ((e.target as HTMLElement).closest('a')) setSheetOpen(false);
        }}
        className={`w-[300px] shrink-0 border-r border-hairline bg-sidebar [--creature-ring:var(--color-sidebar)] md:static md:block ${sheetOpen ? 'max-md:fixed max-md:inset-y-0 max-md:left-0 max-md:z-40 max-md:w-[min(320px,88vw)]' : 'hidden'}`}
      >
        <Sidebar />
      </aside>
      <div data-desktop={desktop || undefined} className="relative flex min-h-0 min-w-0 flex-1">
        <main className="min-h-0 min-w-0 flex-1 overflow-y-auto p-4 md:p-6">
          <Outlet
            context={
              {
                browserOpen,
                terminalOpen: desktop && terminalOpen,
                toggleBrowser,
                toggleTerminal: hasTerminal ? toggleTerminal : undefined,
                hideTerminal,
                setHasTerminal,
                setBrowserDriver,
              } satisfies LayoutContext
            }
          />
        </main>
        {desktop && !onSpace && (
          <PaneToggles
            browserOpen={browserOpen}
            onBrowser={toggleBrowser}
            terminalOpen={terminalOpen}
            onTerminal={hasTerminal ? toggleTerminal : undefined}
          />
        )}
      </div>
      <BrowserPane
        open={browserOpen || browserDriver !== null}
        driver={browserDriver}
        onClose={toggleBrowser}
      />
      <TabBar onMenu={() => setSheetOpen(true)} />
      {projects.error instanceof TypeError && <Unreachable />}
    </div>
  );
}

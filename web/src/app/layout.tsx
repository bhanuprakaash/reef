import { useState } from 'react';
import { Outlet } from 'react-router';
import { BrowserPane, type BrowserDriver } from '@/app/browser-pane';
import { FirstRun } from '@/app/first-run';
import { Sidebar } from '@/app/sidebar';
import { TabBar } from '@/app/tab-bar';
import { Unreachable } from '@/app/unreachable';
import { useProfile } from '@/lib/api/get-profile';
import { useProjects } from '@/modules/projects/api/get-projects';

export type LayoutContext = {
  browserOpen: boolean;
  toggleBrowser: () => void;
  setBrowserDriver: (driver: BrowserDriver | null) => void;
};

export function Layout() {
  const projects = useProjects();
  const [browserOpen, setBrowserOpen] = useState(true);
  const [browserDriver, setBrowserDriver] = useState<BrowserDriver | null>(null);
  const profile = useProfile();
  if (projects.isPending || profile.isPending) return null;
  if (projects.data?.length === 0 || profile.data === null) return <FirstRun />;

  return (
    <div className="relative flex h-screen flex-col bg-main font-sans text-fg md:flex-row">
      <div className="app-drag absolute inset-x-0 top-0 h-3" />
      <aside className="hidden w-[300px] shrink-0 border-r border-hairline bg-sidebar [--creature-ring:var(--color-sidebar)] md:block">
        <Sidebar />
      </aside>
      <main className="min-h-0 min-w-0 flex-1 overflow-y-auto p-4 md:p-6">
        <Outlet
          context={
            {
              browserOpen,
              toggleBrowser: () => setBrowserOpen((open) => !open),
              setBrowserDriver,
            } satisfies LayoutContext
          }
        />
      </main>
      <BrowserPane open={browserOpen} driver={browserDriver} />
      <TabBar />
      {projects.error instanceof TypeError && <Unreachable />}
    </div>
  );
}

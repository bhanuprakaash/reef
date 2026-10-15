import { Outlet } from 'react-router';
import { BrowserPane } from '@/app/browser-pane';
import { FirstRun } from '@/app/first-run';
import { Sidebar } from '@/app/sidebar';
import { Unreachable } from '@/app/unreachable';
import { useProjects } from '@/modules/projects/api/get-projects';

export function Layout() {
  const projects = useProjects();
  if (projects.error instanceof TypeError) return <Unreachable />;
  if (projects.isPending) return null;
  if (projects.data?.length === 0) return <FirstRun />;

  return (
    <div className="flex h-screen bg-main font-sans text-fg">
      <aside className="w-[300px] shrink-0 border-r border-hairline bg-sidebar [--creature-ring:var(--color-sidebar)]">
        <Sidebar />
      </aside>
      <main className="min-w-0 flex-1 overflow-y-auto p-6">
        <Outlet />
      </main>
      <BrowserPane />
    </div>
  );
}

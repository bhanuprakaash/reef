import { Link, Outlet } from 'react-router';
import { BrowserPane } from '@/app/browser-pane';
import { FirstRun } from '@/app/first-run';
import { Sidebar } from '@/app/sidebar';
import { useProjects } from '@/modules/projects/api/get-projects';

export function Layout() {
  const projects = useProjects();
  if (projects.isPending) return null;
  if (projects.data?.length === 0) return <FirstRun />;

  return (
    <div className="flex h-screen bg-main font-sans text-fg">
      <aside className="w-64 shrink-0 overflow-y-auto border-r border-hairline bg-sidebar p-4 [--creature-ring:var(--color-sidebar)]">
        <Link to="/" className="mb-4 block text-lg font-semibold">
          Reef
        </Link>
        <Sidebar />
      </aside>
      <main className="min-w-0 flex-1 overflow-y-auto p-6">
        <Outlet />
      </main>
      <BrowserPane />
    </div>
  );
}

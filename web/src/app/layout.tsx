import { Link, Outlet } from 'react-router';
import { BrowserPane } from '@/app/browser-pane';
import { Sidebar } from '@/app/sidebar';

export function Layout() {
  return (
    <div className="flex h-screen bg-white font-sans text-neutral-900 dark:bg-neutral-900 dark:text-neutral-100">
      <aside className="w-64 shrink-0 overflow-y-auto border-r border-neutral-200 p-4 dark:border-neutral-800">
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

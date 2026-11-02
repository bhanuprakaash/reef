import { useCallback, useState, type ReactNode } from 'react';
import { Link, useLocation, useNavigate } from 'react-router';
import { ProjectSwitcher } from '@/app/project-switcher';
import { SidebarBots } from '@/app/sidebar-bots';
import { SidebarSearch } from '@/app/sidebar-search';
import { SidebarSpaces } from '@/app/sidebar-spaces';
import { UpdateCard } from '@/app/update-card';
import { Button } from '@/components/button';
import { CountBadge } from '@/components/count-badge';
import { Dialog } from '@/components/dialog';
import { Logo } from '@/components/logo';
import { Row } from '@/components/row';
import { useProfile } from '@/lib/api/get-profile';
import { useCurrentProject } from '@/lib/current-project';
import { useWorkItems } from '@/modules/work-items/api/get-work-items';
import { AddWorkItemForm } from '@/modules/work-items/components/add-work-item-form';
import { useNeedsYou } from '@/modules/inbox/api/get-inbox';
import { useInboxEvents } from '@/modules/inbox/hooks/use-inbox-events';
import { useProjects } from '@/modules/projects/api/get-projects';
import { itemPath } from '@/utils/anchors';
import { initials } from '@/utils/initials';

function NavIcon({ children }: { children: ReactNode }) {
  return (
    <svg
      width="17"
      height="17"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className="text-muted"
    >
      {children}
    </svg>
  );
}

function BarLink({
  to,
  label,
  active,
  children,
}: {
  to: string;
  label: string;
  active: boolean;
  children: ReactNode;
}) {
  return (
    <Link
      to={to}
      aria-label={label}
      title={label}
      className={`flex size-8 max-md:size-11 items-center justify-center rounded-lg text-soft hover:bg-hover/50 ${active ? 'bg-active text-fg' : ''}`}
    >
      <svg
        width="17"
        height="17"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.7"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
      >
        {children}
      </svg>
    </Link>
  );
}

export function Sidebar() {
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const projects = useProjects();
  const profileName = useProfile().data?.name ?? '';
  const workItems = useWorkItems();
  const inbox = useNeedsYou();
  useInboxEvents(useCallback((item) => void navigate(itemPath(item)), [navigate]));
  const [query, setQuery] = useState('');
  const [addingItem, setAddingItem] = useState(false);

  const [current] = useCurrentProject(projects.data);
  const error = projects.error ?? workItems.error;
  const waiting = inbox.data ?? [];
  const openItems =
    workItems.data?.filter(
      (f) =>
        (!current || f.projectId === current.id) && f.status !== 'done' && f.status !== 'dropped',
    ).length ?? 0;

  return (
    <nav aria-label="Spaces" className="flex h-full select-none flex-col px-3 pt-4 pb-3 text-sm">
      <div className="app-drag box-content flex h-8 items-center justify-end gap-1 pb-3.5">
        <Link to="/" aria-label="Today" className="flex items-center gap-1.5 pr-1">
          <Logo size={18} />
          <span className="text-[15px] tracking-[-0.03em]">reef</span>
        </Link>
      </div>

      <label className="flex h-[38px] items-center gap-2 rounded-xl border border-search-line bg-search px-3 text-muted">
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.8"
          strokeLinecap="round"
          aria-hidden="true"
        >
          <circle cx="11" cy="11" r="7" />
          <path d="M20 20l-3.5-3.5" />
        </svg>
        <input
          type="search"
          aria-label="Search"
          placeholder="Search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          className="min-w-0 flex-1 bg-transparent text-sm text-fg outline-none placeholder:text-muted"
        />
      </label>

      {projects.data && <ProjectSwitcher projects={projects.data} />}

      <div className="-mx-3 flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-3">
        {query.trim() !== '' ? (
          <SidebarSearch query={query} />
        ) : (
          <>
            <Row
              to="/inbox"
              active={pathname === '/inbox'}
              avatar={
                <NavIcon>
                  <path d="M22 12h-6l-2 3h-4l-2-3H2" />
                  <path d="M5.5 5h13L22 12v7H2v-7z" />
                </NavIcon>
              }
              title="Inbox"
              aside={
                waiting.length > 0 && (
                  <CountBadge count={waiting.length} label={`${waiting.length} waiting`} />
                )
              }
            />
            <div className="group relative">
              <Row
                to="/board"
                active={pathname === '/board'}
                avatar={
                  <NavIcon>
                    <rect x="3" y="4" width="5" height="16" rx="1.5" />
                    <rect x="10" y="4" width="5" height="10" rx="1.5" />
                    <rect x="17" y="4" width="4" height="13" rx="1.5" />
                  </NavIcon>
                }
                title="Board"
                aside={
                  <span className="text-[12.5px] md:group-focus-within:invisible md:group-hover:invisible">
                    {openItems}
                  </span>
                }
              />
              <Button
                type="button"
                variant="ghost"
                size="iconSm"
                aria-label="New Work item"
                title={current ? 'New Work item' : 'Pick a project first'}
                disabled={!current}
                onClick={() => setAddingItem(true)}
                className="absolute top-1/2 right-1.5 -translate-y-1/2 opacity-0 group-hover:opacity-100 hover:text-fg focus-visible:opacity-100 focus-visible:ring-3 focus-visible:ring-ring max-md:right-0 max-md:size-11 max-md:opacity-100 pointer-coarse:opacity-100"
              >
                <svg
                  width="14"
                  height="14"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.8"
                  strokeLinecap="round"
                  aria-hidden="true"
                >
                  <path d="M12 5v14M5 12h14" />
                </svg>
              </Button>
            </div>

            {error && <p className="px-2.5 pt-3 text-sm text-danger">{error.message}</p>}
            <SidebarBots project={current} />
            <SidebarSpaces project={current} />
          </>
        )}
      </div>

      <Dialog open={addingItem} onClose={() => setAddingItem(false)} title="New Work item">
        {addingItem && current && (
          <AddWorkItemForm
            projectId={current.id}
            onClose={() => setAddingItem(false)}
            onCreated={(item) => void navigate(`/work-items/${item.id}`)}
          />
        )}
      </Dialog>

      <UpdateCard />

      <div className="flex shrink-0 items-center gap-2.5 px-1.5 pt-3">
        <span className="flex size-[30px] flex-none items-center justify-center rounded-full bg-bubble text-[11.5px] text-soft">
          {initials(profileName)}
        </span>
        <span className="min-w-0 truncate text-sm">{profileName}</span>
        <div className="ml-auto flex gap-0.5">
          <BarLink to="/bots" label="Bots" active={pathname.startsWith('/bots')}>
            <circle cx="12" cy="8" r="4" />
            <path d="M4 21c.8-4.2 4-6 8-6s7.2 1.8 8 6" />
          </BarLink>
          <BarLink to="/settings" label="Settings" active={pathname === '/settings'}>
            <circle cx="12" cy="12" r="3" />
            <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3h.1a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8v.1a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" />
          </BarLink>
        </div>
      </div>
    </nav>
  );
}

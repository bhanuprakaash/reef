import { useCallback, useState, type ReactNode } from 'react';
import { Link, useLocation, useNavigate } from 'react-router';
import { FeatureChannels } from '@/app/feature-channels';
import { ProjectSwitcher } from '@/app/project-switcher';
import { CountBadge } from '@/components/count-badge';
import { Dialog } from '@/components/dialog';
import { Logo } from '@/components/logo';
import { Row } from '@/components/row';
import { useChannels } from '@/lib/api/get-channels';
import { useCurrentProject } from '@/lib/current-project';
import { NewChannelForm } from '@/modules/channels/components/new-channel-form';
import { useFeatures } from '@/modules/features/api/get-features';
import { useInbox } from '@/modules/inbox/api/get-inbox';
import { useInboxEvents } from '@/modules/inbox/hooks/use-inbox-events';
import { useProjects } from '@/modules/projects/api/get-projects';
import { itemPath } from '@/utils/anchors';

function IconCircle({ children, dot }: { children: ReactNode; dot?: boolean }) {
  return (
    <span className="relative flex size-[30px] flex-none items-center justify-center rounded-full bg-bubble">
      <svg
        width="15"
        height="15"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
      >
        {children}
      </svg>
      {dot && (
        <span className="absolute -top-0.5 -right-0.5 size-2.5 rounded-full border-2 border-sidebar bg-needs-you" />
      )}
    </span>
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
      className={`flex size-8 items-center justify-center rounded-lg text-soft hover:bg-hover/50 ${active ? 'bg-active text-fg' : ''}`}
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
  const features = useFeatures();
  const channels = useChannels();
  const inbox = useInbox();
  useInboxEvents(useCallback((item) => void navigate(itemPath(item)), [navigate]));
  const [query, setQuery] = useState('');
  const [creating, setCreating] = useState(false);

  const [current] = useCurrentProject(projects.data);
  const error = projects.error ?? features.error ?? channels.error;
  const waiting = inbox.data ?? [];
  const projectFeatures = features.data?.filter((f) => f.projectId === current?.id).length ?? 0;

  return (
    <nav aria-label="Channels" className="flex h-full flex-col px-3 pt-4 pb-3 text-sm">
      <div
        className={`app-drag flex items-center gap-2 pb-3.5 ${window.reef ? 'pl-[78px]' : 'pl-1.5'}`}
      >
        <Logo size={20} />
        <span className="text-[16px] tracking-[-0.03em]">reef</span>
        <button
          type="button"
          aria-label="New channel"
          disabled={!current}
          onClick={() => setCreating(true)}
          className="ml-auto flex size-8 items-center justify-center rounded-lg text-soft hover:bg-hover/50"
        >
          <svg
            width="18"
            height="18"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            aria-hidden="true"
          >
            <path d="M12 4v16M4 12h16" />
          </svg>
        </button>
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

      {projects.data && current && <ProjectSwitcher projects={projects.data} />}

      <div className="-mx-3 flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-3">
        <Row
          to="/"
          active={pathname === '/'}
          avatar={
            <IconCircle>
              <circle cx="12" cy="12" r="4" />
              <path d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
            </IconCircle>
          }
          title="Today"
          preview={`${projectFeatures} ${projectFeatures === 1 ? 'feature' : 'features'}`}
        />
        <Row
          to="/inbox"
          active={pathname === '/inbox'}
          avatar={
            <IconCircle dot={waiting.length > 0}>
              <path d="M22 12h-6l-2 3h-4l-2-3H2" />
              <path d="M5.5 5h13L22 12v7H2v-7z" />
            </IconCircle>
          }
          title="Needs you"
          aside={
            waiting.length > 0 && (
              <CountBadge count={waiting.length} label={`${waiting.length} waiting`} />
            )
          }
          preview={waiting[0]?.text ?? 'Nothing waiting'}
        />

        {error && <p className="px-2.5 pt-3 text-sm text-danger">{error.message}</p>}
        {current && <FeatureChannels project={current} query={query} />}
      </div>

      <div className="flex shrink-0 items-center gap-2.5 px-1.5 pt-3">
        <span className="flex size-[30px] flex-none items-center justify-center rounded-full bg-bubble text-[11.5px] text-soft">
          Y
        </span>
        <span className="text-sm">You</span>
        <div className="ml-auto flex gap-0.5">
          <BarLink to="/tasks" label="Tasks" active={pathname === '/tasks'}>
            <path d="M9 11l3 3 8-8" />
            <path d="M20 12v7a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h9" />
          </BarLink>
          <BarLink to="/teams" label="Teams" active={pathname === '/teams'}>
            <circle cx="9" cy="9" r="4" />
            <circle cx="17" cy="11" r="3" />
            <path d="M2 20c.8-3.5 3.6-5 7-5s6.2 1.5 7 5M15.5 15.5c2.6 0 4.7 1.3 5.5 4.5" />
          </BarLink>
          <BarLink to="/settings" label="Settings" active={pathname === '/settings'}>
            <circle cx="12" cy="12" r="3" />
            <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3h.1a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8v.1a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" />
          </BarLink>
        </div>
      </div>

      {current && (
        <Dialog open={creating} onClose={() => setCreating(false)} title="New quick session">
          {creating && (
            <NewChannelForm
              projectId={current.id}
              featureId={null}
              onClose={() => setCreating(false)}
            />
          )}
        </Dialog>
      )}
    </nav>
  );
}

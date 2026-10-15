import type { ReactNode } from 'react';
import { NavLink, useMatch } from 'react-router';
import { CountBadge } from '@/components/count-badge';
import { useInbox } from '@/modules/inbox/api/get-inbox';

function Tab({
  to,
  label,
  badge,
  children,
}: {
  to: string;
  label: string;
  badge?: number;
  children: ReactNode;
}) {
  return (
    <NavLink
      to={to}
      end={to === '/'}
      className={({ isActive }) =>
        `relative flex min-h-12 flex-col items-center justify-center gap-[3px] text-[10.5px] ${isActive ? 'text-fg' : 'text-muted'}`
      }
    >
      <svg
        width="24"
        height="24"
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
      {label}
      {badge !== undefined && badge > 0 && (
        <span className="absolute top-0.5 left-[56%]">
          <CountBadge count={badge} label={`${badge} waiting`} />
        </span>
      )}
    </NavLink>
  );
}

export function TabBar() {
  const inbox = useInbox();
  const inChannel = useMatch('/channels/:id');
  const inReview = useMatch('/features/:id/review');
  if (inChannel || inReview) return null;

  return (
    <nav
      aria-label="Tabs"
      className="grid shrink-0 grid-cols-5 border-t border-hairline bg-sidebar px-1.5 pt-2 pb-[max(0.5rem,env(safe-area-inset-bottom))] md:hidden"
    >
      <Tab to="/" label="Today">
        <circle cx="12" cy="12" r="4" />
        <path d="M12 2v2M12 20v2M2 12h2M20 12h2" />
      </Tab>
      <Tab to="/channels" label="Channels">
        <path d="M4 5h16v11H9l-5 4z" />
      </Tab>
      <Tab to="/inbox" label="Needs you" badge={inbox.data?.length}>
        <path d="M22 12h-6l-2 3h-4l-2-3H2" />
        <path d="M5.5 5h13L22 12v7H2v-7z" />
      </Tab>
      <Tab to="/tasks" label="Tasks">
        <path d="M9 11l3 3 8-8" />
        <path d="M20 12v7a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h9" />
      </Tab>
      <Tab to="/teams" label="Teams">
        <circle cx="9" cy="9" r="4" />
        <circle cx="17" cy="11" r="3" />
        <path d="M2 20c.8-3.5 3.6-5 7-5s6.2 1.5 7 5" />
      </Tab>
    </nav>
  );
}

import { useState } from 'react';
import { Link } from 'react-router';
import { Button } from '@/components/button';
import { GroupAvatar } from '@/components/creature';
import { EmptyState } from '@/components/empty-state';
import { useSpaces } from '@/lib/api/get-spaces';
import { useTeams } from '@/lib/api/get-teams';
import { useCurrentProject } from '@/lib/current-project';
import { useCreateFeature } from '@/modules/features/api/create-feature';
import { useFeatures } from '@/modules/features/api/get-features';
import { useInbox } from '@/modules/inbox/api/get-inbox';
import { InboxRow } from '@/modules/inbox/components/inbox-row';
import { itemKey } from '@/modules/inbox/utils/new-items';
import { useProjects } from '@/modules/projects/api/get-projects';
import type { WorkItem } from '@/types/api';
import { roleFaces } from '@/utils/role-faces';

function greeting(now: Date) {
  const hour = now.getHours();
  return hour < 12 ? 'Good morning' : hour < 18 ? 'Good afternoon' : 'Good evening';
}

function QuickCapture() {
  const projects = useProjects();
  const create = useCreateFeature();
  const [text, setText] = useState('');
  const [project] = useCurrentProject(projects.data);
  const isDesktop = typeof window !== 'undefined' && window.reef != null;

  return (
    <form
      aria-label="Capture a todo"
      className="flex min-h-[52px] items-center gap-2 rounded-pill border border-hover bg-field py-[5px] pr-1.5 pl-[18px]"
      onSubmit={(e) => {
        e.preventDefault();
        if (!project || text.trim() === '') return;
        create.mutate(
          { projectId: project.id, type: 'task', title: text.trim() },
          { onSuccess: () => setText('') },
        );
      }}
    >
      <input
        aria-label="Quick capture"
        placeholder="Drop an idea, todo or bug"
        className="h-11 min-w-0 flex-1 bg-transparent text-[15px] text-fg outline-none placeholder:text-muted"
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          create.reset();
        }}
      />
      {create.isSuccess && (
        <span role="status" className="text-xs text-done">
          Captured
        </span>
      )}
      {create.isError && (
        <span role="alert" className="text-xs text-danger">
          {create.error.message}
        </span>
      )}
      {isDesktop && (
        <span className="pr-1.5 font-mono text-[11.5px] text-faint">⌘⇧Space anywhere</span>
      )}
      <Button
        type="submit"
        variant="primary"
        size="icon"
        aria-label="Capture"
        disabled={!project || text.trim() === ''}
        loading={create.isPending}
      >
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.2"
          strokeLinecap="round"
          aria-hidden="true"
        >
          <path d="M12 5v14M5 12h14" />
        </svg>
      </Button>
    </form>
  );
}

function FeatureCard({ feature }: { feature: WorkItem }) {
  const spaces = useSpaces();
  const teams = useTeams();
  const inbox = useInbox();
  const mine = spaces.data?.filter((c) => c.workItemId === feature.id) ?? [];
  const latest = mine[mine.length - 1];
  const team = teams.data?.find((t) => t.name === latest?.team);
  const roles = team ? team.roles.map((r) => r.name) : latest ? [latest.role] : [];
  const faces = roleFaces(teams.data, latest?.team);
  const waiting = inbox.data?.some((i) => i.workItemId === feature.id) ?? false;

  return (
    <Link
      to={`/features/${feature.id}`}
      className="flex flex-1 flex-col gap-3 rounded-card bg-sidebar p-4 [--creature-ring:var(--color-sidebar)]"
    >
      <div className="flex items-center gap-2">
        <span className="truncate text-base">{feature.title}</span>
        <span className="ml-auto shrink-0 text-[12.5px] text-muted">{feature.status}</span>
      </div>
      <div
        className={`flex items-center gap-2 text-[13px] ${waiting ? 'text-paused' : 'text-muted'}`}
      >
        {roles.length > 0 && <GroupAvatar roles={roles} faces={faces} size={22} />}
        {waiting
          ? 'waiting on you'
          : mine.length === 0
            ? 'no Space yet'
            : `${mine.length} ${mine.length === 1 ? 'Space' : 'Spaces'}`}
      </div>
    </Link>
  );
}

export function HomePage() {
  const projects = useProjects();
  const features = useFeatures();
  const inbox = useInbox();
  const spaces = useSpaces();
  const [project] = useCurrentProject(projects.data);
  const now = new Date();

  const waiting = inbox.data ?? [];
  const error = projects.error ?? features.error ?? inbox.error;
  const projectFeatures = features.data?.filter((f) => f.projectId === project?.id);
  const summary = [
    waiting.length > 0 &&
      (waiting.length === 1 ? 'One thing waits on you.' : `${waiting.length} things wait on you.`),
  ].filter(Boolean);

  return (
    <div className="mx-auto flex max-w-[820px] flex-col gap-[26px] py-6">
      <header>
        <div className="text-[13px] text-muted">
          {now.toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric' })}
        </div>
        <h1 className="mt-1 text-4xl font-normal tracking-[-0.02em]">{greeting(now)}</h1>
        <p className="mt-1.5 text-[15.5px] text-muted">
          {summary.length > 0 ? summary.join(' ') : 'Nothing is waiting on you.'}
        </p>
      </header>
      {error && <p className="text-sm text-danger">{error.message}</p>}
      <QuickCapture />

      <section aria-labelledby="today-needs" className="flex flex-col gap-2.5">
        <h2 id="today-needs" className="text-[15px]">
          Needs you
          {waiting.length > 0 && (
            <span className="font-normal text-muted"> · {waiting.length}</span>
          )}
        </h2>
        {waiting.length === 0 ? (
          <EmptyState
            title="Nothing needs you"
            body="Everything is moving on its own."
            role="lead"
          />
        ) : (
          <ul className="flex flex-col gap-3">
            {waiting.map((item) => (
              <InboxRow
                key={itemKey(item)}
                item={item}
                space={spaces.data?.find((c) => c.id === item.spaceId)}
              />
            ))}
          </ul>
        )}
      </section>

      <section aria-labelledby="today-features" className="flex flex-col gap-2.5">
        <h2 id="today-features" className="text-[15px]">
          Features
        </h2>
        {projectFeatures?.length === 0 && (
          <p className="text-[13px] text-muted">
            No features yet. Add one from <span className="md:hidden">Spaces.</span>
            <span className="hidden md:inline">the sidebar.</span>
          </p>
        )}
        <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          {projectFeatures?.map((f) => (
            <li key={f.id} className="flex">
              <FeatureCard feature={f} />
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}

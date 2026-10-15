import { useState } from 'react';
import { Link } from 'react-router';
import { Button } from '@/components/button';
import { GroupAvatar } from '@/components/creature';
import { EmptyState } from '@/components/empty-state';
import { useChannels } from '@/lib/api/get-channels';
import { useTeams } from '@/lib/api/get-teams';
import { useCurrentProject } from '@/lib/current-project';
import { useFeatures } from '@/modules/features/api/get-features';
import { STAGES } from '@/modules/features/utils/stages';
import { useInbox } from '@/modules/inbox/api/get-inbox';
import { InboxRow } from '@/modules/inbox/components/inbox-row';
import { age } from '@/modules/inbox/utils/age';
import { itemKey } from '@/modules/inbox/utils/new-items';
import { useProjects } from '@/modules/projects/api/get-projects';
import { useCreateTask } from '@/modules/tasks/api/create-task';
import { useTasks } from '@/modules/tasks/api/get-tasks';
import { TaskProgress } from '@/modules/tasks/components/task-progress';
import type { Feature } from '@/types/api';
import { roleFaces } from '@/utils/role-faces';

const DAY = 24 * 60 * 60 * 1000;

function greeting(now: Date) {
  const hour = now.getHours();
  return hour < 12 ? 'Good morning' : hour < 18 ? 'Good afternoon' : 'Good evening';
}

function QuickCapture() {
  const projects = useProjects();
  const create = useCreateTask();
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
          { projectId: project.id, featureId: null, title: `todo: ${text.trim()}` },
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

function FeatureCard({ feature }: { feature: Feature }) {
  const channels = useChannels();
  const teams = useTeams();
  const inbox = useInbox();
  const mine = channels.data?.filter((c) => c.featureId === feature.id) ?? [];
  const latest = mine[mine.length - 1];
  const team = teams.data?.find((t) => t.name === latest?.team);
  const roles = team ? team.roles.map((r) => r.name) : latest ? [latest.role] : [];
  const faces = roleFaces(teams.data, latest?.team);
  const waiting = inbox.data?.some((i) => i.featureId === feature.id) ?? false;
  const tasks = useTasks();
  const current = STAGES.findIndex((s) => s.value === feature.stage);

  return (
    <Link
      to={`/features/${feature.id}`}
      className="flex flex-1 flex-col gap-3 rounded-card bg-sidebar p-4 [--creature-ring:var(--color-sidebar)]"
    >
      <div className="flex items-center gap-2">
        <span className="truncate text-base">{feature.name}</span>
        {feature.kind === 'journey' && (
          <span className="ml-auto shrink-0 text-[12.5px] text-muted">
            {STAGES[current]?.label ?? feature.stage}
          </span>
        )}
      </div>
      {feature.kind === 'journey' ? (
        <div aria-hidden="true" className="grid grid-cols-8 gap-[3px]">
          {STAGES.map((s, i) => (
            <span
              key={s.value}
              className={`h-[3px] rounded-sm ${i === current ? 'bg-lead' : i < current ? 'bg-pill' : 'bg-track'}`}
            />
          ))}
        </div>
      ) : (
        <TaskProgress tasks={tasks.data?.filter((t) => t.featureId === feature.id) ?? []} />
      )}
      <div
        className={`flex items-center gap-2 text-[13px] ${waiting ? 'text-paused' : 'text-muted'}`}
      >
        {roles.length > 0 && <GroupAvatar roles={roles} faces={faces} size={22} />}
        {waiting
          ? 'waiting on you'
          : mine.length === 0
            ? 'no channel yet'
            : `${mine.length} ${mine.length === 1 ? 'channel' : 'channels'}`}
      </div>
    </Link>
  );
}

export function HomePage() {
  const projects = useProjects();
  const features = useFeatures();
  const inbox = useInbox();
  const tasks = useTasks();
  const channels = useChannels();
  const [project] = useCurrentProject(projects.data);
  const now = new Date();
  const since = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime() - DAY;

  const waiting = inbox.data ?? [];
  const done = (tasks.data ?? [])
    .filter((t) => t.status === 'done' && Date.parse(t.updatedAt) >= since)
    .sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt));
  const error = projects.error ?? features.error ?? inbox.error ?? tasks.error;
  const projectFeatures = features.data?.filter((f) => f.projectId === project?.id);
  const summary = [
    done.length > 0 &&
      `The team finished ${done.length} ${done.length === 1 ? 'task' : 'tasks'} since yesterday.`,
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
                channel={channels.data?.find((c) => c.id === item.channelId)}
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
            No features yet. Add one from <span className="md:hidden">Channels.</span>
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

      {done.length > 0 && (
        <section aria-labelledby="today-done" className="flex flex-col gap-1">
          <h2 id="today-done" className="mb-1.5 text-[15px]">
            Since yesterday
          </h2>
          <ul>
            {done.map((t) => (
              <li key={t.id} className="flex items-center gap-3 px-1 py-2 text-sm text-soft">
                <span
                  aria-hidden="true"
                  className="flex size-[22px] flex-none items-center justify-center rounded-full bg-done/15 text-xs text-done"
                >
                  ✓
                </span>
                <span className="min-w-0 truncate">{t.title}</span>
                <span className="ml-auto text-[12.5px] text-faint">{age(t.updatedAt)}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}

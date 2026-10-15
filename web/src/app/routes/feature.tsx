import { useState } from 'react';
import { Link, useParams } from 'react-router';
import { TerminalPane } from '@/app/terminal-pane';
import { ChannelList } from '@/modules/channels/components/channel-list';
import { useChannels } from '@/lib/api/get-channels';
import { NewChannelForm } from '@/modules/channels/components/new-channel-form';
import { useBriefVersions } from '@/modules/features/api/get-brief-versions';
import { useDecisions } from '@/modules/features/api/get-decisions';
import { useDiff } from '@/modules/features/api/get-diff';
import { useEnsureWorktree } from '@/modules/features/api/ensure-worktree';
import { useFeature } from '@/modules/features/api/get-feature';
import { BriefPanel } from '@/modules/features/components/brief-panel';
import { DecisionList } from '@/modules/features/components/decision-list';
import { DiffPanel } from '@/modules/features/components/diff-panel';
import { FeatureForm } from '@/modules/features/components/feature-form';
import { JourneyBar } from '@/modules/features/components/journey-bar';
import { StagePicker } from '@/modules/features/components/stage-picker';
import { useProjects } from '@/modules/projects/api/get-projects';
import { useTasks } from '@/modules/tasks/api/get-tasks';
import { TaskList } from '@/modules/tasks/components/task-list';
import { Button, buttonClass } from '@/components/button';
import { Card } from '@/components/card';

export function FeaturePage() {
  const id = Number(useParams().id);
  const feature = useFeature(id);
  const projects = useProjects();
  const briefs = useBriefVersions(id);
  const decisions = useDecisions(id);
  const diff = useDiff(id);
  const tasks = useTasks({ feature: id });
  const channels = useChannels();
  const hasTerminal = typeof window !== 'undefined' && window.reef != null;
  const worktree = useEnsureWorktree(id);
  const [editing, setEditing] = useState(false);

  if (feature.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (feature.isError) return <p className="text-sm text-danger">{feature.error.message}</p>;

  const f = feature.data;
  const project = projects.data?.find((p) => p.id === f.projectId);
  const featureChannels = channels.data?.filter((c) => c.featureId === id) ?? [];
  const latestChannel = featureChannels[featureChannels.length - 1];

  return (
    <div className="space-y-7">
      <header className="flex flex-wrap items-end gap-4">
        <div className="min-w-[360px] flex-1">
          <div className="mb-2 text-xs text-muted">{project?.name} · feature</div>
          <h1 className="text-[40px] leading-[1.05] font-normal tracking-[-0.02em]">{f.name}</h1>
          {f.goal && <p className="mt-2 max-w-xl text-sm text-muted">{f.goal}</p>}
        </div>
        <div className="flex gap-2">
          <StagePicker feature={f} />
          {hasTerminal && !worktree.data && (
            <Button type="button" onClick={() => worktree.mutate()} disabled={worktree.isPending}>
              Open terminal
            </Button>
          )}
          <Button type="button" onClick={() => setEditing(!editing)}>
            Edit
          </Button>
          {latestChannel && (
            <Link to={`/channels/${latestChannel.id}`} className={buttonClass('primary')}>
              Open channel
            </Link>
          )}
        </div>
      </header>
      {worktree.isError && <p className="text-sm text-danger">{worktree.error.message}</p>}
      {editing && <FeatureForm key={f.id} feature={f} onDone={() => setEditing(false)} />}
      <JourneyBar stage={f.stage} />
      <div className="flex flex-wrap items-start gap-5">
        <div className="flex min-w-[460px] flex-[999_1_460px] flex-col gap-5">
          {briefs.isError ? (
            <p className="text-sm text-danger">{briefs.error.message}</p>
          ) : (
            <BriefPanel versions={briefs.data ?? []} />
          )}
          {decisions.isError ? (
            <p className="text-sm text-danger">{decisions.error.message}</p>
          ) : (
            <DecisionList decisions={decisions.data ?? []} />
          )}
          <Card title="Diff">
            {diff.isError ? (
              <p className="text-sm text-danger">{diff.error.message}</p>
            ) : (
              <DiffPanel files={diff.data ?? []} featureId={id} />
            )}
          </Card>
        </div>
        <div className="flex min-w-[300px] flex-1 flex-col gap-5">
          <Card title="Channels">
            {channels.isError ? (
              <p className="text-sm text-danger">{channels.error.message}</p>
            ) : (
              <ChannelList channels={featureChannels} />
            )}
            <div className="mt-3 max-w-xs">
              <NewChannelForm projectId={f.projectId} featureId={id} />
            </div>
          </Card>
          <Card title="Tasks">
            {tasks.isError ? (
              <p className="text-sm text-danger">{tasks.error.message}</p>
            ) : (
              <TaskList tasks={tasks.data ?? []} compact />
            )}
          </Card>
          <Card title="Branch">
            <div className="font-mono text-xs text-soft">feature/{f.slug}</div>
          </Card>
        </div>
      </div>
      {worktree.data && (
        <Card title="Terminal">
          <TerminalPane cwd={worktree.data.path} />
        </Card>
      )}
    </div>
  );
}

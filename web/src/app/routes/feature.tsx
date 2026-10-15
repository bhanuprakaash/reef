import { useParams } from 'react-router';
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
import { useTasks } from '@/modules/tasks/api/get-tasks';
import { TaskList } from '@/modules/tasks/components/task-list';
import { Button } from '@/components/button';

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="space-y-2">
      <h2 className="text-lg font-medium">{title}</h2>
      {children}
    </section>
  );
}

export function FeaturePage() {
  const id = Number(useParams().id);
  const feature = useFeature(id);
  const briefs = useBriefVersions(id);
  const decisions = useDecisions(id);
  const diff = useDiff(id);
  const tasks = useTasks({ feature: id });
  const channels = useChannels();
  const hasTerminal = typeof window !== 'undefined' && window.reef != null;
  const worktree = useEnsureWorktree(id);

  if (feature.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (feature.isError) return <p className="text-sm text-danger">{feature.error.message}</p>;

  return (
    <div className="max-w-3xl space-y-8">
      <h1 className="text-xl font-semibold">{feature.data.name}</h1>
      <FeatureForm key={feature.data.id} feature={feature.data} />
      <Section title="Brief">
        {briefs.isError ? (
          <p className="text-sm text-danger">{briefs.error.message}</p>
        ) : (
          <BriefPanel versions={briefs.data ?? []} />
        )}
      </Section>
      <Section title="Decisions">
        {decisions.isError ? (
          <p className="text-sm text-danger">{decisions.error.message}</p>
        ) : (
          <DecisionList decisions={decisions.data ?? []} />
        )}
      </Section>
      <Section title="Diff">
        {diff.isError ? (
          <p className="text-sm text-danger">{diff.error.message}</p>
        ) : (
          <DiffPanel files={diff.data ?? []} featureId={id} />
        )}
      </Section>
      <Section title="Tasks">
        {tasks.isError ? (
          <p className="text-sm text-danger">{tasks.error.message}</p>
        ) : (
          <TaskList tasks={tasks.data ?? []} />
        )}
      </Section>
      <Section title="Channels">
        {channels.isError ? (
          <p className="text-sm text-danger">{channels.error.message}</p>
        ) : (
          <ChannelList channels={channels.data?.filter((c) => c.featureId === id) ?? []} />
        )}
        <div className="max-w-xs">
          <NewChannelForm projectId={feature.data.projectId} featureId={id} />
        </div>
      </Section>
      {hasTerminal && (
        <Section title="Terminal">
          {worktree.data ? (
            <TerminalPane cwd={worktree.data.path} />
          ) : (
            <Button type="button" onClick={() => worktree.mutate()} disabled={worktree.isPending}>
              Open terminal
            </Button>
          )}
          {worktree.isError && <p className="text-sm text-danger">{worktree.error.message}</p>}
        </Section>
      )}
    </div>
  );
}

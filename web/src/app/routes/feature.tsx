import { useState } from 'react';
import { Link, useParams } from 'react-router';
import { FeatureTerminal } from '@/app/feature-terminal';
import { useTerminal } from '@/app/use-terminal';
import { ChannelList } from '@/modules/channels/components/channel-list';
import { useChannels } from '@/lib/api/get-channels';
import { useTeams } from '@/lib/api/get-teams';
import { NewChannelForm } from '@/modules/channels/components/new-channel-form';
import { useBriefVersions } from '@/modules/features/api/get-brief-versions';
import { useDecisions } from '@/modules/features/api/get-decisions';
import { useDiff } from '@/modules/features/api/get-diff';
import { useFeature } from '@/modules/features/api/get-feature';
import { useSetFeatureClosed } from '@/modules/features/api/set-feature-closed';
import { BriefPanel } from '@/modules/features/components/brief-panel';
import { DecisionList } from '@/modules/features/components/decision-list';
import { FeatureForm } from '@/modules/features/components/feature-form';
import { JourneyBar } from '@/modules/features/components/journey-bar';
import { StagePicker } from '@/modules/features/components/stage-picker';
import { diffCounts } from '@/modules/features/utils/diff-counts';
import { useProjects } from '@/modules/projects/api/get-projects';
import { useTasks } from '@/modules/tasks/api/get-tasks';
import { TaskList } from '@/modules/tasks/components/task-list';
import { TaskProgress } from '@/modules/tasks/components/task-progress';
import { roleFaces } from '@/utils/role-faces';
import { BackButton } from '@/components/back-button';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { Dialog } from '@/components/dialog';

export function FeaturePage() {
  const id = Number(useParams().id);
  const feature = useFeature(id);
  const projects = useProjects();
  const briefs = useBriefVersions(id);
  const decisions = useDecisions(id);
  const diff = useDiff(id);
  const tasks = useTasks({ feature: id });
  const channels = useChannels();
  const teams = useTeams();
  const terminal = useTerminal(true);
  const setClosed = useSetFeatureClosed(id);
  const [dialog, setDialog] = useState<'edit' | 'channel' | 'close' | null>(null);

  if (feature.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (feature.isError) return <p className="text-sm text-danger">{feature.error.message}</p>;

  const f = feature.data;
  const project = projects.data?.find((p) => p.id === f.projectId);
  const featureChannels = channels.data?.filter((c) => c.featureId === id) ?? [];
  const latestChannel = featureChannels[featureChannels.length - 1];
  const openTasks = tasks.data?.filter((t) => t.status !== 'done') ?? [];
  const doneTasks = tasks.data?.filter((t) => t.status === 'done') ?? [];

  return (
    <div className="space-y-7">
      <header className="flex flex-wrap items-end gap-4">
        <div className="flex-1 md:min-w-[360px]">
          <div className="-ml-2 mb-2 flex items-center gap-1 text-xs text-muted">
            <BackButton fallback="/" />
            {project?.name} · feature
          </div>
          <h1 className="text-[40px] leading-[1.05] font-normal tracking-[-0.02em]">{f.name}</h1>
          {f.goal && <p className="mt-2 max-w-xl text-sm text-muted">{f.goal}</p>}
          <div className="mt-3 flex flex-wrap items-center gap-x-2 text-xs text-muted">
            {f.kind === 'journey' && (
              <>
                <StagePicker feature={f} />
                <span aria-hidden="true">·</span>
              </>
            )}
            <span className="font-mono">feature/{f.slug}</span>
            {(diff.isError || diff.data) && <span aria-hidden="true">·</span>}
            {diff.isError ? (
              <span className="text-danger">{diff.error.message}</span>
            ) : (
              diff.data &&
              (diff.data.length ? (
                <Link
                  to={`/features/${id}/review`}
                  className="inline-flex min-h-11 items-center gap-1.5 hover:text-fg md:min-h-0"
                >
                  {diff.data.length} file{diff.data.length === 1 ? '' : 's'}
                  <span className="text-done">+{diffCounts(diff.data).added}</span>
                  <span className="text-failed">−{diffCounts(diff.data).removed}</span>
                </Link>
              ) : (
                <span>No changes yet</span>
              ))
            )}
          </div>
          {setClosed.isError && (
            <p className="mt-2 text-sm text-danger">{setClosed.error.message}</p>
          )}
        </div>
        <div className="flex gap-2">
          <Button type="button" onClick={() => setDialog('edit')}>
            Edit
          </Button>
          <Button
            type="button"
            disabled={setClosed.isPending}
            onClick={() => (f.closedAt === null ? setDialog('close') : setClosed.mutate(false))}
          >
            {f.closedAt === null ? 'Close feature' : 'Reopen'}
          </Button>
        </div>
      </header>
      {f.kind === 'journey' ? (
        <JourneyBar stage={f.stage} />
      ) : (
        <TaskProgress tasks={tasks.data ?? []} />
      )}
      <div className="flex flex-wrap items-start gap-5">
        <div className="flex min-w-0 flex-[999_1_460px] flex-col gap-5 md:min-w-[460px]">
          {briefs.isError ? (
            <p className="text-sm text-danger">{briefs.error.message}</p>
          ) : (
            <BriefPanel versions={briefs.data ?? []} />
          )}
          {decisions.isError ? (
            <p className="text-sm text-danger">{decisions.error.message}</p>
          ) : (
            <DecisionList
              decisions={decisions.data ?? []}
              faces={roleFaces(teams.data, latestChannel?.team)}
            />
          )}
        </div>
        <div className="flex min-w-[300px] flex-1 flex-col gap-5">
          <Card title="Channels">
            {channels.isError ? (
              <p className="text-sm text-danger">{channels.error.message}</p>
            ) : (
              <ChannelList channels={featureChannels} />
            )}
            <Button type="button" className="mt-3" onClick={() => setDialog('channel')}>
              New channel
            </Button>
          </Card>
          <Card title="Tasks">
            {tasks.isError ? (
              <p className="text-sm text-danger">{tasks.error.message}</p>
            ) : (
              <>
                {(openTasks.length > 0 || doneTasks.length === 0) && (
                  <TaskList tasks={openTasks} compact />
                )}
                {doneTasks.length > 0 && (
                  <details>
                    <summary className="cursor-pointer py-2 text-xs text-muted">
                      {doneTasks.length} done
                    </summary>
                    <TaskList tasks={doneTasks} compact />
                  </details>
                )}
              </>
            )}
          </Card>
        </div>
      </div>
      <Dialog open={dialog === 'edit'} onClose={() => setDialog(null)} title="Edit feature">
        {dialog === 'edit' && <FeatureForm feature={f} onDone={() => setDialog(null)} />}
      </Dialog>
      <Dialog open={dialog === 'channel'} onClose={() => setDialog(null)} title="New channel">
        {dialog === 'channel' && (
          <NewChannelForm projectId={f.projectId} featureId={id} onClose={() => setDialog(null)} />
        )}
      </Dialog>
      <Dialog open={dialog === 'close'} onClose={() => setDialog(null)} title="Close feature">
        <p className="mb-5 text-sm text-muted">
          Close {f.name}? Its channels close too and their agents stop. You can reopen it later.
        </p>
        <div className="flex justify-end gap-2">
          <Button type="button" onClick={() => setDialog(null)}>
            Cancel
          </Button>
          <Button
            type="button"
            variant="primary"
            onClick={() => {
              setClosed.mutate(true);
              setDialog(null);
            }}
          >
            Close feature
          </Button>
        </div>
      </Dialog>
      {terminal.open && (
        <div className="sticky bottom-0 z-10 overflow-hidden rounded-card border border-hairline">
          <FeatureTerminal key={id} featureId={id} onHide={terminal.hide} />
        </div>
      )}
    </div>
  );
}

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
import { roleFaces } from '@/utils/role-faces';
import { BackButton } from '@/components/back-button';
import { Button, buttonClass } from '@/components/button';
import { Card } from '@/components/card';
import { Dialog } from '@/components/dialog';
import { EmptyState } from '@/components/empty-state';

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
  const [dialog, setDialog] = useState<'edit' | 'channel' | null>(null);

  if (feature.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (feature.isError) return <p className="text-sm text-danger">{feature.error.message}</p>;

  const f = feature.data;
  const project = projects.data?.find((p) => p.id === f.projectId);
  const featureChannels = channels.data?.filter((c) => c.featureId === id) ?? [];
  const latestChannel = featureChannels[featureChannels.length - 1];

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
          {setClosed.isError && (
            <p className="mt-2 text-sm text-danger">{setClosed.error.message}</p>
          )}
        </div>
        <div className="flex gap-2">
          <StagePicker feature={f} />
          <Button type="button" onClick={() => setDialog('edit')}>
            Edit
          </Button>
          <Button
            type="button"
            disabled={setClosed.isPending}
            onClick={() => {
              if (f.closedAt !== null) setClosed.mutate(false);
              else if (
                window.confirm(
                  `Close ${f.name}? Its channels close too and their agents stop. You can reopen it later.`,
                )
              )
                setClosed.mutate(true);
            }}
          >
            {f.closedAt === null ? 'Close feature' : 'Reopen'}
          </Button>
          {latestChannel && (
            <Link to={`/channels/${latestChannel.id}`} className={buttonClass('primary')}>
              Open channel
            </Link>
          )}
        </div>
      </header>
      <JourneyBar stage={f.stage} />
      <div className="flex flex-wrap items-start gap-5">
        <div className="flex flex-[999_1_460px] flex-col gap-5 md:min-w-[460px]">
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
          <Card title="Diff">
            {diff.isError ? (
              <p className="text-sm text-danger">{diff.error.message}</p>
            ) : (
              <div className="flex items-center gap-3 text-sm">
                {diff.data &&
                  (diff.data.length ? (
                    <>
                      <span className="text-muted">
                        {diff.data.length} file{diff.data.length === 1 ? '' : 's'}
                      </span>
                      <span className="text-done">+{diffCounts(diff.data).added}</span>
                      <span className="text-failed">−{diffCounts(diff.data).removed}</span>
                    </>
                  ) : (
                    <EmptyState title="No changes yet" role="developer" compact />
                  ))}
                <Link to={`/features/${id}/review`} className={`${buttonClass()} ml-auto`}>
                  Review changes
                </Link>
              </div>
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
            <Button type="button" className="mt-3" onClick={() => setDialog('channel')}>
              New channel
            </Button>
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
      <Dialog open={dialog === 'edit'} onClose={() => setDialog(null)} title="Edit feature">
        {dialog === 'edit' && <FeatureForm feature={f} onDone={() => setDialog(null)} />}
      </Dialog>
      <Dialog open={dialog === 'channel'} onClose={() => setDialog(null)} title="New channel">
        {dialog === 'channel' && (
          <NewChannelForm projectId={f.projectId} featureId={id} onClose={() => setDialog(null)} />
        )}
      </Dialog>
      {terminal.open && (
        <div className="sticky bottom-0 z-10 overflow-hidden rounded-card border border-hairline">
          <FeatureTerminal key={id} featureId={id} onHide={terminal.hide} />
        </div>
      )}
    </div>
  );
}

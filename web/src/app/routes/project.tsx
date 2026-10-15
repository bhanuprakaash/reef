import { useState } from 'react';
import { Link, useParams } from 'react-router';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { GroupAvatar } from '@/components/creature';
import { Dialog } from '@/components/dialog';
import { EmptyState } from '@/components/empty-state';
import { useChannels } from '@/lib/api/get-channels';
import { useTeams } from '@/lib/api/get-teams';
import { NewChannelForm } from '@/modules/channels/components/new-channel-form';
import { useFeatures } from '@/modules/features/api/get-features';
import { AddFeatureForm } from '@/modules/features/components/add-feature-form';
import { STAGES } from '@/modules/features/utils/stages';
import { useProjects } from '@/modules/projects/api/get-projects';
import { ProjectMemoryForm } from '@/modules/projects/components/project-memory-form';
import { roleFaces } from '@/utils/role-faces';
import { rowTime } from '@/utils/row-time';

const rowClass = 'flex min-h-10 items-center gap-2.5 hover:text-fg';

export function ProjectPage() {
  const id = Number(useParams().id);
  const projects = useProjects();
  const features = useFeatures();
  const channels = useChannels();
  const teams = useTeams();
  const [dialog, setDialog] = useState<'feature' | 'session' | null>(null);

  if (projects.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (projects.isError) return <p className="text-sm text-danger">{projects.error.message}</p>;
  const project = projects.data.find((p) => p.id === id);
  if (!project) return <p className="text-sm text-danger">Project not found.</p>;

  const projectChannels = channels.data?.filter((c) => c.projectId === id) ?? [];
  const projectFeatures = features.data?.filter((f) => f.projectId === id) ?? [];
  const quick = projectChannels.filter((c) => c.featureId === null);

  return (
    <div className="mx-auto w-full max-w-[820px] space-y-6 py-6">
      <header className="flex flex-wrap items-end gap-3.5">
        <div className="min-w-[300px] flex-1">
          <div className="font-mono text-[12.5px] text-muted">{project.path}</div>
          <h1 className="mt-1 text-[32px] font-normal tracking-[-0.02em]">{project.name}</h1>
        </div>
        <Button type="button" onClick={() => setDialog('session')}>
          New quick session
        </Button>
        <Button variant="primary" type="button" onClick={() => setDialog('feature')}>
          New feature
        </Button>
      </header>

      <div className="flex flex-wrap items-start gap-5">
        <div className="flex-[999_1_460px] md:min-w-[460px]">
          <ProjectMemoryForm projectId={id} />
        </div>
        <div className="flex min-w-[300px] flex-1 flex-col gap-4">
          <Card title="Features">
            {features.isError && <p className="text-sm text-danger">{features.error.message}</p>}
            {projectFeatures.length === 0 && (
              <EmptyState
                title="No features yet"
                body="Add one with New feature."
                role="architect"
                compact
              />
            )}
            <ul>
              {projectFeatures.map((f) => (
                <li key={f.id}>
                  <Link to={`/features/${f.id}`} className={rowClass}>
                    {f.name}
                    {projectChannels.some((c) => c.featureId === f.id && c.paused) ? (
                      <span className="ml-auto text-[12.5px] text-paused">paused</span>
                    ) : (
                      <span className="ml-auto text-[12.5px] text-muted">
                        {STAGES.find((s) => s.value === f.stage)?.label ?? f.stage}
                      </span>
                    )}
                  </Link>
                </li>
              ))}
            </ul>
          </Card>
          <Card title="Quick sessions">
            {channels.isError && <p className="text-sm text-danger">{channels.error.message}</p>}
            {quick.length === 0 && (
              <EmptyState
                title="No quick sessions"
                body="Start one for a chat outside a feature."
                role="lead"
                compact
              />
            )}
            <ul>
              {quick.map((c) => (
                <li key={c.id}>
                  <Link to={`/channels/${c.id}`} className={rowClass}>
                    <GroupAvatar roles={[c.role]} faces={roleFaces(teams.data, c.team)} size={22} />
                    {c.name}
                    <span className="ml-auto text-[12.5px] text-muted">
                      {rowTime(c.lastMessage?.createdAt ?? c.createdAt)}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          </Card>
        </div>
      </div>

      <Dialog open={dialog === 'feature'} onClose={() => setDialog(null)} title="New feature">
        {dialog === 'feature' && <AddFeatureForm projectId={id} onClose={() => setDialog(null)} />}
      </Dialog>
      <Dialog open={dialog === 'session'} onClose={() => setDialog(null)} title="New quick session">
        {dialog === 'session' && (
          <NewChannelForm projectId={id} featureId={null} onClose={() => setDialog(null)} />
        )}
      </Dialog>
    </div>
  );
}

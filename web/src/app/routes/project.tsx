import { useState } from 'react';
import { Link, useParams } from 'react-router';
import { BackButton } from '@/components/back-button';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { GroupAvatar } from '@/components/creature';
import { Dialog } from '@/components/dialog';
import { EmptyState } from '@/components/empty-state';
import { useSpaces } from '@/lib/api/get-spaces';
import { useBots } from '@/lib/api/get-bots';
import { NewSpaceForm } from '@/modules/spaces/components/new-space-form';
import { useWorkItems } from '@/modules/work-items/api/get-work-items';
import { AddWorkItemForm } from '@/modules/work-items/components/add-work-item-form';
import { useProjects } from '@/modules/projects/api/get-projects';
import { ProjectBaseBranchForm } from '@/modules/projects/components/project-base-branch-form';
import { ProjectPurposeForm } from '@/modules/projects/components/project-purpose-form';
import { ProjectMemoryForm } from '@/modules/projects/components/project-memory-form';
import { memberFaces } from '@/utils/role-faces';
import { rowTime } from '@/utils/row-time';

const rowClass = 'flex min-h-10 items-center gap-2.5 hover:text-fg';

export function ProjectPage() {
  const id = Number(useParams().id);
  const projects = useProjects();
  const workItems = useWorkItems();
  const spaces = useSpaces(id);
  const bots = useBots();
  const [dialog, setDialog] = useState<'work-item' | 'space' | null>(null);

  if (projects.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (projects.isError) return <p className="text-sm text-danger">{projects.error.message}</p>;
  const project = projects.data.find((p) => p.id === id);
  if (!project) return <p className="text-sm text-danger">Project not found.</p>;

  const projectSpaces = spaces.data ?? [];
  const projectWorkItems = workItems.data?.filter((f) => f.projectId === id) ?? [];
  const freeSpaces = projectSpaces.filter((c) => c.workItemId === null);

  return (
    <div className="mx-auto w-full max-w-[820px] space-y-6 py-6">
      <header className="flex flex-wrap items-end gap-3.5">
        <div className="min-w-[300px] flex-1">
          <div className="-ml-2 flex items-center gap-1 font-mono text-[12.5px] text-muted">
            <BackButton fallback="/" />
            {project.path}
          </div>
          <h1 className="mt-1 text-[32px] font-normal tracking-[-0.02em]">{project.name}</h1>
        </div>
        <Button type="button" onClick={() => setDialog('space')}>
          New free Space
        </Button>
        <Button variant="primary" type="button" onClick={() => setDialog('work-item')}>
          New Work item
        </Button>
      </header>

      <div className="flex flex-wrap items-start gap-5">
        <div className="flex-[999_1_460px] md:min-w-[460px]">
          <ProjectMemoryForm projectId={id} />
        </div>
        <div className="flex min-w-[300px] flex-1 flex-col gap-4">
          <ProjectPurposeForm project={project} />
          <ProjectBaseBranchForm project={project} />
          <Card title="Work items">
            {workItems.isError && <p className="text-sm text-danger">{workItems.error.message}</p>}
            {projectWorkItems.length === 0 && (
              <EmptyState
                title="No Work items yet"
                body="Add one with New Work item."
                face="whale"
                compact
              />
            )}
            <ul>
              {projectWorkItems.map((f) => (
                <li key={f.id}>
                  <Link to={`/work-items/${f.id}`} className={rowClass}>
                    {f.title}
                    {projectSpaces.some((c) => c.workItemId === f.id && c.paused) ? (
                      <span className="ml-auto text-[12.5px] text-paused">paused</span>
                    ) : (
                      <span className="ml-auto text-[12.5px] text-muted">{f.status}</span>
                    )}
                  </Link>
                </li>
              ))}
            </ul>
          </Card>
          <Card title="Free Spaces">
            {spaces.isError && <p className="text-sm text-danger">{spaces.error.message}</p>}
            {freeSpaces.length === 0 && (
              <EmptyState
                title="No free Spaces"
                body="Start one for a chat outside a Work item."
                face="jellyfish"
                compact
              />
            )}
            <ul>
              {freeSpaces.map((c) => (
                <li key={c.id}>
                  <Link to={`/spaces/${c.id}`} className={rowClass}>
                    <GroupAvatar
                      roles={[c.botId]}
                      faces={memberFaces(bots.data, c.members)}
                      size={22}
                    />
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

      <Dialog open={dialog === 'work-item'} onClose={() => setDialog(null)} title="New Work item">
        {dialog === 'work-item' && (
          <AddWorkItemForm projectId={id} onClose={() => setDialog(null)} />
        )}
      </Dialog>
      <Dialog open={dialog === 'space'} onClose={() => setDialog(null)} title="New free Space">
        {dialog === 'space' && (
          <NewSpaceForm projectId={id} workItemId={null} onClose={() => setDialog(null)} />
        )}
      </Dialog>
    </div>
  );
}

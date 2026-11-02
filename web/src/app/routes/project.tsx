import { useRef, useState } from 'react';
import { Link, useParams } from 'react-router';
import { BackButton } from '@/components/back-button';
import { Button } from '@/components/button';
import { GroupAvatar } from '@/components/creature';
import { Dialog } from '@/components/dialog';
import { EmptyState } from '@/components/empty-state';
import { ProjectTile } from '@/components/project-tile';
import { StatusDot, type Status } from '@/components/status-dot';
import { useDismiss } from '@/lib/use-dismiss';
import { useSpaces } from '@/lib/api/get-spaces';
import { useBots } from '@/lib/api/get-bots';
import { NewSpaceForm } from '@/modules/spaces/components/new-space-form';
import { useWorkItems } from '@/modules/work-items/api/get-work-items';
import { COLUMNS } from '@/modules/work-items/utils/board-drop';
import { AddWorkItemForm } from '@/modules/work-items/components/add-work-item-form';
import { useProjects } from '@/modules/projects/api/get-projects';
import { ProjectIconPicker } from '@/modules/projects/components/project-icon-picker';
import { ProjectBaseBranchForm } from '@/modules/projects/components/project-base-branch-form';
import { ProjectPurposeForm } from '@/modules/projects/components/project-purpose-form';
import { ProjectMemoryForm } from '@/modules/projects/components/project-memory-form';
import type { WorkItemStatus } from '@/types/api';
import { memberFaces } from '@/utils/role-faces';
import { projectTile } from '@/utils/project-tile';
import { rowTime } from '@/utils/row-time';

const rowClass = 'flex min-h-10 items-center gap-2.5 border-b border-hairline px-1.5 hover:bg-card';

const dotOf: Record<WorkItemStatus, Status> = {
  backlog: 'asleep',
  queued: 'asleep',
  in_progress: 'working',
  needs_you: 'needs-you',
  review: 'paused',
  done: 'done',
  dropped: 'asleep',
};

export function ProjectPage() {
  const id = Number(useParams().id);
  const projects = useProjects();
  const workItems = useWorkItems();
  const spaces = useSpaces(id);
  const bots = useBots();
  const [dialog, setDialog] = useState<'work-item' | 'space' | null>(null);
  const [picking, setPicking] = useState(false);
  const iconRef = useRef<HTMLDivElement>(null);

  useDismiss(picking, iconRef, () => setPicking(false));

  if (projects.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (projects.isError) return <p className="text-sm text-danger">{projects.error.message}</p>;
  const project = projects.data.find((p) => p.id === id);
  if (!project) return <p className="text-sm text-danger">Project not found.</p>;

  const projectSpaces = spaces.data ?? [];
  const projectWorkItems = workItems.data?.filter((f) => f.projectId === id) ?? [];
  const freeSpaces = projectSpaces.filter((c) => c.workItemId === null);

  return (
    <div className="mx-auto w-full max-w-[1040px] space-y-6 py-6">
      <header className="flex flex-wrap items-end gap-3.5">
        <div className="flex min-w-[300px] flex-1 items-center gap-3.5">
          <div ref={iconRef} className="relative flex-none">
            <button
              type="button"
              aria-label="Change icon"
              aria-expanded={picking}
              onClick={() => setPicking(!picking)}
              className="block rounded-lg focus-visible:ring-3 focus-visible:ring-ring"
            >
              <ProjectTile {...projectTile(project)} size={44} />
            </button>
            {picking && <ProjectIconPicker project={project} />}
          </div>
          <div className="min-w-0">
            <div className="-ml-2 flex items-center gap-1 font-mono text-[12.5px] text-muted">
              <BackButton fallback="/" />
              {project.path}
            </div>
            <h1 className="mt-1 text-[32px] font-normal tracking-[-0.02em]">{project.name}</h1>
          </div>
        </div>
        <Button type="button" onClick={() => setDialog('space')}>
          New free Space
        </Button>
        <Button variant="primary" type="button" onClick={() => setDialog('work-item')}>
          New Work item
        </Button>
      </header>

      <div className="flex flex-col gap-8 md:flex-row md:items-start md:gap-0">
        <div className="min-w-0 flex-1 space-y-7 md:pr-7">
          <section aria-label="Work items">
            <div className="flex items-center gap-2">
              <h2 className="text-[14.5px]">Work items</h2>
              <span className="text-xs text-faint">{projectWorkItems.length}</span>
            </div>
            {workItems.isError && <p className="text-sm text-danger">{workItems.error.message}</p>}
            {projectWorkItems.length === 0 && (
              <EmptyState
                title="No Work items yet"
                body="Add one with New Work item."
                face="whale"
                compact
              />
            )}
            {COLUMNS.map(({ status, label }) => {
              const group = projectWorkItems.filter((f) => f.status === status);
              if (group.length === 0) return null;
              return (
                <div key={status}>
                  <h3 className="pt-3.5 pb-1 text-[11px] tracking-[0.08em] text-faint uppercase">
                    {label}
                  </h3>
                  <ul>
                    {group.map((f) => (
                      <li key={f.id}>
                        <Link to={`/work-items/${f.id}`} className={rowClass}>
                          <StatusDot status={dotOf[status]} />
                          <span className="min-w-0 flex-1 truncate">{f.title}</span>
                          {projectSpaces.some((c) => c.workItemId === f.id && c.paused) && (
                            <span className="text-[12.5px] text-paused">paused</span>
                          )}
                          <span className="text-[12.5px] text-muted">{rowTime(f.createdAt)}</span>
                        </Link>
                      </li>
                    ))}
                  </ul>
                </div>
              );
            })}
          </section>
          <section aria-label="Free Spaces">
            <div className="flex items-center gap-2">
              <h2 className="text-[14.5px]">Free Spaces</h2>
              <span className="text-xs text-faint">{freeSpaces.length}</span>
            </div>
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
                    <span className="min-w-0 flex-1 truncate">{c.name}</span>
                    <span className="text-[12.5px] text-muted">
                      {rowTime(c.lastMessage?.createdAt ?? c.createdAt)}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          </section>
        </div>
        <aside className="flex w-full flex-none flex-col gap-6 border-t border-hairline pt-6 md:w-80 md:border-t-0 md:border-l md:pt-0 md:pl-7">
          <ProjectPurposeForm project={project} />
          <ProjectBaseBranchForm project={project} />
          <ProjectMemoryForm projectId={id} />
        </aside>
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

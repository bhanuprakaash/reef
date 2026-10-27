import { useState } from 'react';
import { Link, useParams } from 'react-router';
import { WorkItemTerminal } from '@/app/work-item-terminal';
import { ReviewPanel } from '@/app/review-panel';
import { useTerminal } from '@/app/use-terminal';
import { SpaceList } from '@/modules/spaces/components/space-list';
import { useSpaces } from '@/lib/api/get-spaces';
import { useMemory } from '@/lib/api/get-memory';
import { useRetireMemory } from '@/lib/api/retire-memory';
import { useTeams } from '@/lib/api/get-teams';
import { NewSpaceForm } from '@/modules/spaces/components/new-space-form';
import { useBriefVersions } from '@/modules/work-items/api/get-brief-versions';
import { useDiff } from '@/modules/work-items/api/get-diff';
import { useWorkItem } from '@/modules/work-items/api/get-work-item';
import { useMoveWorkItem } from '@/modules/work-items/api/move-work-item';
import { AcceptanceLines } from '@/modules/work-items/components/acceptance-lines';
import { BriefPanel } from '@/modules/work-items/components/brief-panel';
import { MemoryList } from '@/modules/work-items/components/memory-list';
import { WorkItemForm } from '@/modules/work-items/components/work-item-form';
import { ProofPanel } from '@/modules/work-items/components/proof-panel';
import { SubItems } from '@/modules/work-items/components/sub-items';
import { diffCounts } from '@/modules/work-items/utils/diff-counts';
import { statusLabel } from '@/modules/work-items/utils/status-label';
import { useProjects } from '@/modules/projects/api/get-projects';
import { roleFaces } from '@/utils/role-faces';
import { BackButton } from '@/components/back-button';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { Chip } from '@/components/chip';
import { Dialog } from '@/components/dialog';

function ParentLink({ id }: { id: number }) {
  const parent = useWorkItem(id);
  return (
    <Link to={`/work-items/${id}`} className="border-b border-line text-soft">
      {parent.data?.title ?? `work item ${id}`}
    </Link>
  );
}

export function WorkItemPage() {
  const id = Number(useParams().id);
  const workItem = useWorkItem(id);
  const projects = useProjects();
  const briefs = useBriefVersions(id);
  const scope = `work_item:${id}`;
  const memory = useMemory(scope);
  const retire = useRetireMemory(scope);
  const diff = useDiff(id);
  const spaces = useSpaces();
  const teams = useTeams();
  const terminal = useTerminal(true);
  const move = useMoveWorkItem();
  const [dialog, setDialog] = useState<'edit' | 'space' | 'drop' | null>(null);

  if (workItem.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (workItem.isError) return <p className="text-sm text-danger">{workItem.error.message}</p>;

  const f = workItem.data;
  const project = projects.data?.find((p) => p.id === f.projectId);
  const workItemSpaces = spaces.data?.filter((c) => c.workItemId === id) ?? [];
  const latestSpace = workItemSpaces[workItemSpaces.length - 1];
  const team = teams.data?.find((t) => t.id === f.teamId);
  const reviewing = f.parentId === null && f.status === 'review';

  return (
    <div className="space-y-7">
      <header className="flex flex-wrap items-end gap-4">
        <div className="flex-1 md:min-w-[360px]">
          <div className="-ml-2 mb-2 flex items-center gap-1 text-xs text-muted">
            <BackButton fallback="/" />
            <span className="min-w-0">
              {project?.name} · {f.parentId === null ? 'work item' : 'sub-item of '}
              {f.parentId !== null && <ParentLink id={f.parentId} />}
            </span>
          </div>
          <h1 className="text-[40px] leading-[1.05] font-normal tracking-[-0.02em]">{f.title}</h1>
          {f.description && <p className="mt-2 max-w-xl text-sm text-muted">{f.description}</p>}
          <div className="mt-3 flex flex-wrap items-center text-xs text-muted">
            <Chip>{f.type}</Chip>
            <Chip className="ml-2">{statusLabel(f.status)}</Chip>
            {team && <Chip className="ml-2">{team.name}</Chip>}
            <span className="ml-2 font-mono whitespace-nowrap">work-item/{f.id}</span>
            {diff.isError ? (
              <span className="ml-2 whitespace-nowrap text-danger">· {diff.error.message}</span>
            ) : (
              diff.data &&
              (diff.data.length ? (
                <Link
                  to={`/work-items/${id}/review`}
                  className="ml-2 inline-flex min-h-11 items-center gap-1.5 whitespace-nowrap hover:text-fg md:min-h-0"
                >
                  <span aria-hidden="true">·</span>
                  {diff.data.length} file{diff.data.length === 1 ? '' : 's'}
                  <span className="text-done">+{diffCounts(diff.data).added}</span>
                  <span className="text-failed">−{diffCounts(diff.data).removed}</span>
                </Link>
              ) : (
                <span className="ml-2 whitespace-nowrap">· No changes yet</span>
              ))
            )}
          </div>
          {move.isError && <p className="mt-2 text-sm text-danger">{move.error.message}</p>}
        </div>
        <div className="flex gap-2">
          <Button type="button" onClick={() => setDialog('edit')}>
            Edit
          </Button>
          {f.status !== 'dropped' && (
            <Button type="button" disabled={move.isPending} onClick={() => setDialog('drop')}>
              Drop
            </Button>
          )}
        </div>
      </header>
      <div className="flex flex-wrap items-start gap-5">
        <div className="flex min-w-0 flex-[999_1_460px] flex-col gap-5 md:min-w-[460px]">
          {reviewing && (
            <>
              <ReviewPanel workItemId={id} />
              <ProofPanel workItemId={id} />
            </>
          )}
          <AcceptanceLines item={f} />
          {f.parentId === null && <SubItems parent={f} />}
          {f.parentId === null &&
            (briefs.isError ? (
              <p className="text-sm text-danger">{briefs.error.message}</p>
            ) : (
              <BriefPanel versions={briefs.data ?? []} />
            ))}
          {f.parentId === null && !reviewing && <ProofPanel workItemId={id} />}
          {memory.isError ? (
            <p className="text-sm text-danger">{memory.error.message}</p>
          ) : (
            <MemoryList
              items={memory.data ?? []}
              faces={roleFaces(teams.data, latestSpace?.team)}
              onRetire={retire.mutate}
              retiring={retire.isPending}
            />
          )}
          {retire.isError && <p className="text-sm text-danger">{retire.error.message}</p>}
        </div>
        <div className="flex min-w-[300px] flex-1 flex-col gap-5">
          <Card title="Spaces">
            {spaces.isError ? (
              <p className="text-sm text-danger">{spaces.error.message}</p>
            ) : (
              <SpaceList spaces={workItemSpaces} />
            )}
            <Button type="button" className="mt-3" onClick={() => setDialog('space')}>
              New Space
            </Button>
          </Card>
        </div>
      </div>
      <Dialog open={dialog === 'edit'} onClose={() => setDialog(null)} title="Edit Work item">
        {dialog === 'edit' && <WorkItemForm item={f} onDone={() => setDialog(null)} />}
      </Dialog>
      <Dialog open={dialog === 'space'} onClose={() => setDialog(null)} title="New Space">
        {dialog === 'space' && (
          <NewSpaceForm projectId={f.projectId} workItemId={id} onClose={() => setDialog(null)} />
        )}
      </Dialog>
      <Dialog open={dialog === 'drop'} onClose={() => setDialog(null)} title="Drop Work item">
        <p className="mb-5 text-sm text-muted">
          Drop {f.title}? Its Spaces close and their agents stop, and its Team starts the next
          queued item. This cannot be undone.
        </p>
        <div className="flex justify-end gap-2">
          <Button type="button" onClick={() => setDialog(null)}>
            Cancel
          </Button>
          <Button
            type="button"
            variant="primary"
            onClick={() => {
              move.mutate({ id, status: 'dropped' });
              setDialog(null);
            }}
          >
            Drop Work item
          </Button>
        </div>
      </Dialog>
      {terminal.open && (
        <div className="sticky bottom-0 z-10 overflow-hidden rounded-card border border-hairline">
          <WorkItemTerminal key={id} workItemId={id} onHide={terminal.hide} />
        </div>
      )}
    </div>
  );
}

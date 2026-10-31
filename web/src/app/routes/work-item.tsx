import { useState } from 'react';
import { Link, useParams } from 'react-router';
import { WorkItemTerminal } from '@/app/work-item-terminal';
import { ProofPanel } from '@/app/proof-panel';
import { ReviewPanel } from '@/app/review-panel';
import { useTerminal } from '@/app/use-terminal';
import { SpaceList } from '@/modules/spaces/components/space-list';
import { useSpaces } from '@/lib/api/get-spaces';
import { useMemory } from '@/lib/api/get-memory';
import { useRetireMemory } from '@/lib/api/retire-memory';
import { useBots } from '@/lib/api/get-bots';
import { NewSpaceForm } from '@/modules/spaces/components/new-space-form';
import { useBriefVersions } from '@/modules/work-items/api/get-brief-versions';
import { useDiff } from '@/modules/work-items/api/get-diff';
import { useWorkItem } from '@/modules/work-items/api/get-work-item';
import { useMoveWorkItem } from '@/modules/work-items/api/move-work-item';
import { AcceptanceLines } from '@/modules/work-items/components/acceptance-lines';
import { ApproveCard } from '@/modules/work-items/components/approve-card';
import { BriefPanel } from '@/modules/work-items/components/brief-panel';
import { MemoryList } from '@/modules/work-items/components/memory-list';
import { WorkItemForm } from '@/modules/work-items/components/work-item-form';
import { SubItems } from '@/modules/work-items/components/sub-items';
import { TargetsCard } from '@/modules/work-items/components/targets-card';
import { TransferOwnerCard } from '@/modules/work-items/components/transfer-owner-card';
import { diffCounts } from '@/modules/work-items/utils/diff-counts';
import { statusLabel } from '@/modules/work-items/utils/status-label';
import { useProjects } from '@/modules/projects/api/get-projects';
import { memberFaces } from '@/utils/role-faces';
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
  const bots = useBots();
  const terminal = useTerminal(true);
  const move = useMoveWorkItem();
  const [dialog, setDialog] = useState<'edit' | 'space' | 'drop' | null>(null);

  if (workItem.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (workItem.isError) return <p className="text-sm text-danger">{workItem.error.message}</p>;

  const f = workItem.data;
  const project = projects.data?.find((p) => p.id === f.projectId);
  const treeId = f.parentId ?? id;
  const workItemSpaces = spaces.data?.filter((c) => c.workItemId === treeId) ?? [];
  const latestSpace = workItemSpaces[workItemSpaces.length - 1];
  const owner = bots.data?.find((b) => b.id === f.ownerBotId);
  const others = (bots.data ?? []).filter(
    (b) => f.participantBotIds.includes(b.id) && b.id !== f.ownerBotId,
  );
  const proposal = [
    f.proposedBy && `proposed by ${f.proposedBy}`,
    owner && `owner ${owner.name}`,
    others.length > 0 && `with ${others.map((b) => b.name).join(', ')}`,
  ].filter(Boolean);
  const reviewing = f.parentId === null && f.status === 'review';

  return (
    <div className="space-y-7">
      <header className="flex flex-wrap items-end gap-4">
        <div className="min-w-full flex-1 md:min-w-[360px]">
          <div className="-ml-2 mb-2 flex items-center gap-1 text-xs text-muted">
            <BackButton fallback="/" />
            <span className="min-w-0">
              {project?.name} · {f.parentId === null ? 'work item' : 'sub-item of '}
              {f.parentId !== null && <ParentLink id={f.parentId} />}
            </span>
          </div>
          <h1 className="text-[40px] leading-[1.05] font-normal tracking-[-0.02em]">{f.title}</h1>
          {f.description && <p className="mt-2 max-w-xl text-sm text-muted">{f.description}</p>}
          {proposal.length > 0 && <p className="mt-2 text-xs text-muted">{proposal.join(' · ')}</p>}
          <div className="mt-3 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted">
            <Chip>{f.type}</Chip>
            <Chip>{statusLabel(f.status)}</Chip>
            <span className="font-mono whitespace-nowrap">work-item/{treeId}</span>
            {diff.isError ? (
              <span className="whitespace-nowrap text-danger">· {diff.error.message}</span>
            ) : (
              diff.data &&
              (diff.data.length ? (
                <Link
                  to={`/work-items/${id}/review`}
                  className="inline-flex min-h-11 items-center gap-1.5 whitespace-nowrap hover:text-fg md:min-h-0"
                >
                  <span aria-hidden="true">·</span>
                  {diff.data.length} file{diff.data.length === 1 ? '' : 's'}
                  <span className="text-done">+{diffCounts(diff.data).added}</span>
                  <span className="text-failed">−{diffCounts(diff.data).removed}</span>
                </Link>
              ) : (
                <span className="whitespace-nowrap">· No changes yet</span>
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
              faces={memberFaces(bots.data, latestSpace?.members)}
              onRetire={retire.mutate}
              retiring={retire.isPending}
            />
          )}
          {retire.isError && <p className="text-sm text-danger">{retire.error.message}</p>}
        </div>
        <div className="flex min-w-[300px] flex-1 flex-col gap-5">
          {f.parentId === null && f.status === 'backlog' && <ApproveCard item={f} />}
          {f.parentId === null &&
            f.approvedAt !== null &&
            f.status !== 'done' &&
            f.status !== 'dropped' && <TransferOwnerCard item={f} />}
          {f.parentId === null && <TargetsCard item={f} projects={projects.data ?? []} />}
          <Card title="Spaces">
            {spaces.isError ? (
              <p className="text-sm text-danger">{spaces.error.message}</p>
            ) : (
              <SpaceList spaces={workItemSpaces} />
            )}
            {f.parentId === null && (
              <Button type="button" className="mt-3" onClick={() => setDialog('space')}>
                New Space
              </Button>
            )}
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
          Drop {f.title}? Its Spaces close and their agents stop. This cannot be undone.
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

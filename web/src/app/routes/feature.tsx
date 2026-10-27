import { useState } from 'react';
import { Link, useParams } from 'react-router';
import { FeatureTerminal } from '@/app/feature-terminal';
import { ReviewPanel } from '@/app/review-panel';
import { useTerminal } from '@/app/use-terminal';
import { SpaceList } from '@/modules/spaces/components/space-list';
import { useSpaces } from '@/lib/api/get-spaces';
import { useMemory } from '@/lib/api/get-memory';
import { useRetireMemory } from '@/lib/api/retire-memory';
import { useTeams } from '@/lib/api/get-teams';
import { NewSpaceForm } from '@/modules/spaces/components/new-space-form';
import { useBriefVersions } from '@/modules/features/api/get-brief-versions';
import { useDiff } from '@/modules/features/api/get-diff';
import { useFeature } from '@/modules/features/api/get-feature';
import { useSetFeatureClosed } from '@/modules/features/api/set-feature-closed';
import { AcceptanceLines } from '@/modules/features/components/acceptance-lines';
import { BriefPanel } from '@/modules/features/components/brief-panel';
import { MemoryList } from '@/modules/features/components/memory-list';
import { FeatureForm } from '@/modules/features/components/feature-form';
import { ProofPanel } from '@/modules/features/components/proof-panel';
import { SubItems } from '@/modules/features/components/sub-items';
import { diffCounts } from '@/modules/features/utils/diff-counts';
import { statusLabel } from '@/modules/features/utils/status-label';
import { useProjects } from '@/modules/projects/api/get-projects';
import { roleFaces } from '@/utils/role-faces';
import { BackButton } from '@/components/back-button';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { Chip } from '@/components/chip';
import { Dialog } from '@/components/dialog';

function ParentLink({ id }: { id: number }) {
  const parent = useFeature(id);
  return (
    <Link to={`/features/${id}`} className="border-b border-line text-soft">
      {parent.data?.title ?? `work item ${id}`}
    </Link>
  );
}

export function FeaturePage() {
  const id = Number(useParams().id);
  const feature = useFeature(id);
  const projects = useProjects();
  const briefs = useBriefVersions(id);
  const scope = `work_item:${id}`;
  const memory = useMemory(scope);
  const retire = useRetireMemory(scope);
  const diff = useDiff(id);
  const spaces = useSpaces();
  const teams = useTeams();
  const terminal = useTerminal(true);
  const setClosed = useSetFeatureClosed(id);
  const [dialog, setDialog] = useState<'edit' | 'space' | 'close' | null>(null);

  if (feature.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (feature.isError) return <p className="text-sm text-danger">{feature.error.message}</p>;

  const f = feature.data;
  const project = projects.data?.find((p) => p.id === f.projectId);
  const featureSpaces = spaces.data?.filter((c) => c.workItemId === id) ?? [];
  const latestSpace = featureSpaces[featureSpaces.length - 1];
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
                  to={`/features/${id}/review`}
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
              <SpaceList spaces={featureSpaces} />
            )}
            <Button type="button" className="mt-3" onClick={() => setDialog('space')}>
              New Space
            </Button>
          </Card>
        </div>
      </div>
      <Dialog open={dialog === 'edit'} onClose={() => setDialog(null)} title="Edit feature">
        {dialog === 'edit' && <FeatureForm item={f} onDone={() => setDialog(null)} />}
      </Dialog>
      <Dialog open={dialog === 'space'} onClose={() => setDialog(null)} title="New Space">
        {dialog === 'space' && (
          <NewSpaceForm projectId={f.projectId} featureId={id} onClose={() => setDialog(null)} />
        )}
      </Dialog>
      <Dialog open={dialog === 'close'} onClose={() => setDialog(null)} title="Close feature">
        <p className="mb-5 text-sm text-muted">
          Close {f.title}? Its Spaces close too and their agents stop. You can reopen it later.
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

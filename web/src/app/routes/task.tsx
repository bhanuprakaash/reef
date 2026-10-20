import { Link, useParams } from 'react-router';
import { useDiff } from '@/modules/features/api/get-diff';
import { useFeatures } from '@/modules/features/api/get-features';
import { useWorktree } from '@/modules/features/api/get-worktree';
import { diffCounts } from '@/modules/features/utils/diff-counts';
import { useProjects } from '@/modules/projects/api/get-projects';
import { useTask } from '@/modules/tasks/api/get-task';
import { useUpdateTask } from '@/modules/tasks/api/update-task';
import { InlineTaskField } from '@/modules/tasks/components/inline-task-field';
import type { TaskStatus } from '@/types/api';
import { BackButton } from '@/components/back-button';
import { Card } from '@/components/card';
import { SegmentedControl } from '@/components/segmented-control';

const statuses = [
  { value: 'todo', label: 'To do' },
  { value: 'doing', label: 'Doing' },
  { value: 'done', label: 'Done' },
] satisfies { value: TaskStatus; label: string }[];

function FeatureCodeRows({ featureId }: { featureId: number }) {
  const worktree = useWorktree(featureId);
  const diff = useDiff(featureId);

  return (
    <>
      <dt className="text-muted">Worktree</dt>
      <dd className="font-mono break-all">
        {worktree.data?.path || <span className="font-sans text-faint">Not created yet</span>}
      </dd>
      <dt className="text-muted">Changes</dt>
      <dd>
        {diff.data?.length ? (
          <Link to={`/features/${featureId}/review`} className="hover:underline">
            {diff.data.length} file{diff.data.length === 1 ? '' : 's'}{' '}
            <span className="text-done">+{diffCounts(diff.data).added}</span>{' '}
            <span className="text-failed">−{diffCounts(diff.data).removed}</span>
          </Link>
        ) : (
          <span className="text-faint">{diff.isError ? diff.error.message : 'No changes yet'}</span>
        )}
      </dd>
    </>
  );
}

export function TaskPage() {
  const id = Number(useParams().id);
  const task = useTask(id);
  const features = useFeatures();
  const projects = useProjects();
  const update = useUpdateTask();

  if (task.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (task.isError) return <p className="text-sm text-danger">{task.error.message}</p>;

  const t = task.data;
  const feature = features.data?.find((f) => f.id === t.featureId);
  const project = projects.data?.find((p) => p.id === t.projectId);

  return (
    <div className="mx-auto flex w-full max-w-[760px] flex-col gap-5 py-6">
      <header className="flex flex-wrap items-end gap-4">
        <div className="min-w-0 flex-1">
          <div className="-ml-2 mb-2 flex items-center gap-1 text-xs text-muted">
            <BackButton fallback="/tasks" />
            {feature && (
              <Link to={`/features/${feature.id}`} className="hover:text-fg">
                {feature.name}
              </Link>
            )}
          </div>
          <h1 className="flex gap-2.5 text-[32px] leading-[1.1] font-normal tracking-[-0.02em]">
            <span className="text-muted">#{t.id}</span>
            <InlineTaskField task={t} field="title" className="text-left" />
          </h1>
        </div>
      </header>
      <div>
        <SegmentedControl
          label="Status"
          options={statuses}
          value={t.status}
          onChange={(status) => update.mutate({ task: t, status })}
        />
        {update.isError && <p className="mt-2 text-sm text-danger">{update.error.message}</p>}
      </div>
      <InlineTaskField task={t} field="body" multiline className="text-sm whitespace-pre-wrap" />
      <Card title="Details">
        <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
          <dt className="text-muted">Project</dt>
          <dd>{project?.name}</dd>
          <dt className="text-muted">Feature</dt>
          <dd>
            {feature ? (
              <Link to={`/features/${feature.id}`} className="hover:underline">
                {feature.name}
              </Link>
            ) : (
              'None'
            )}
          </dd>
          {feature && (
            <>
              <dt className="text-muted">Branch</dt>
              <dd className="font-mono">feature/{feature.slug}</dd>
              <dt className="text-muted">Base</dt>
              <dd className="font-mono">{project?.baseBranch || 'main'}</dd>
              <FeatureCodeRows featureId={feature.id} />
              {feature.kind === 'journey' && (
                <>
                  <dt className="text-muted">Stage</dt>
                  <dd>{feature.stage}</dd>
                </>
              )}
            </>
          )}
          <dt className="text-muted">Created</dt>
          <dd>{new Date(t.createdAt).toLocaleString()}</dd>
          <dt className="text-muted">Updated</dt>
          <dd>{new Date(t.updatedAt).toLocaleString()}</dd>
        </dl>
      </Card>
    </div>
  );
}

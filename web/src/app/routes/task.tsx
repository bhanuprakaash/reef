import { Link, useParams } from 'react-router';
import { useFeatures } from '@/modules/features/api/get-features';
import { useTask } from '@/modules/tasks/api/get-task';
import { BackButton } from '@/components/back-button';

export function TaskPage() {
  const id = Number(useParams().id);
  const task = useTask(id);
  const features = useFeatures();

  if (task.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (task.isError) return <p className="text-sm text-danger">{task.error.message}</p>;

  const t = task.data;
  const feature = features.data?.find((f) => f.id === t.featureId);

  return (
    <div className="mx-auto flex w-full max-w-[760px] flex-col gap-5 py-6">
      <header>
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
          <span>{t.title}</span>
        </h1>
        <div className="mt-3 flex flex-wrap items-center gap-3 text-xs text-muted">
          <span className="flex h-[22px] items-center rounded-pill bg-bubble px-[9px] text-soft">
            {t.status}
          </span>
          <span>Created {new Date(t.createdAt).toLocaleString()}</span>
          <span>Updated {new Date(t.updatedAt).toLocaleString()}</span>
        </div>
      </header>
      {t.body && <p className="text-sm whitespace-pre-wrap">{t.body}</p>}
    </div>
  );
}

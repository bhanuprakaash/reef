import { useSetSpaceProject, useSpaceProjects } from '@/modules/spaces/api/space-projects';
import type { Project } from '@/types/api';
import { Button } from '@/components/button';
import { Select } from '@/components/select';

type Props = { spaceId: number; projects: Project[]; readOnly: boolean };

export function SpaceProjects({ spaceId, projects, readOnly }: Props) {
  const associated = useSpaceProjects(spaceId);
  const set = useSetSpaceProject(spaceId);
  const current = associated.data ?? [];
  const options = projects
    .filter((p) => !current.some((c) => c.id === p.id))
    .map((p) => ({ value: String(p.id), label: p.name }));

  return (
    <div>
      <h2 className="mb-1.5 text-[13px] text-muted">Projects</h2>
      {associated.isError && <p className="text-sm text-danger">{associated.error.message}</p>}
      {associated.isSuccess && current.length === 0 && (
        <p className="text-[13.5px] text-muted">None</p>
      )}
      <ul>
        {current.map((p) => (
          <li key={p.id} className="flex min-h-9 items-center gap-2 text-[14.5px]">
            <span className="min-w-0 flex-1 truncate">{p.name}</span>
            {!readOnly && (
              <Button
                variant="ghost"
                size="sm"
                aria-label={`Remove ${p.name}`}
                disabled={set.isPending}
                onClick={() => set.mutate({ projectId: p.id, associated: false })}
              >
                Remove
              </Button>
            )}
          </li>
        ))}
      </ul>
      {!readOnly && options.length > 0 && (
        <Select
          aria-label="Add Project"
          variant="pill"
          prefix="Add Project"
          value=""
          options={options}
          disabled={set.isPending}
          onChange={(v) => set.mutate({ projectId: Number(v), associated: true })}
          className="mt-1.5"
        />
      )}
      {set.isError && <p className="text-sm text-danger">{set.error.message}</p>}
    </div>
  );
}

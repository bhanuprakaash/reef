import { useSetSpaceProject, useSpaceProjects } from '@/modules/spaces/api/space-projects';
import type { Project } from '@/types/api';
import { Button } from '@/components/button';
import { SearchPicker } from '@/components/search-picker';

type Props = { spaceId: number; projects: Project[]; readOnly: boolean };

export function SpaceProjects({ spaceId, projects, readOnly }: Props) {
  const associated = useSpaceProjects(spaceId);
  const set = useSetSpaceProject(spaceId);
  const current = associated.data ?? [];

  return (
    <div>
      <h2 className="mb-1.5 text-[13px] text-muted">Projects</h2>
      {associated.isError && <p className="text-sm text-danger">{associated.error.message}</p>}
      {associated.isSuccess && current.length === 0 && (
        <p className="text-[13.5px] text-muted">No Project linked to this Space yet.</p>
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
      {!readOnly && projects.length > 0 && (
        <div className="mt-1.5">
          <SearchPicker
            label="Add Project"
            options={projects.map((p) => ({ value: String(p.id), label: p.name }))}
            selected={current.map((c) => String(c.id))}
            disabled={set.isPending}
            onToggle={(v) =>
              set.mutate({
                projectId: Number(v),
                associated: !current.some((c) => String(c.id) === v),
              })
            }
          />
        </div>
      )}
      {set.isError && <p className="text-sm text-danger">{set.error.message}</p>}
    </div>
  );
}

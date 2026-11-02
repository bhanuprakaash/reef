import { projectIconNames } from '@/components/project-icons';
import { ProjectTile } from '@/components/project-tile';
import { useUpdateProject } from '@/modules/projects/api/update-project';
import type { Project } from '@/types/api';
import { palette, projectTile } from '@/utils/project-tile';

export function ProjectIconPicker({ project }: { project: Project }) {
  const update = useUpdateProject(project);
  const tile = projectTile(project);

  return (
    <div
      role="dialog"
      aria-label="Project icon"
      className="absolute top-full left-0 z-20 mt-2 w-[296px] rounded-menu border border-line bg-card p-3 shadow-menu"
    >
      <div className="grid grid-cols-7 gap-0.5">
        {projectIconNames.map((n) => (
          <button
            key={n}
            type="button"
            aria-label={n}
            aria-pressed={project.icon === n}
            onClick={() => update.mutate({ icon: n })}
            className="flex size-9 items-center justify-center rounded-lg hover:bg-hover aria-pressed:ring-2 aria-pressed:ring-ring"
          >
            <ProjectTile icon={n} colour={tile.colour} size={30} />
          </button>
        ))}
      </div>
      <button
        type="button"
        aria-pressed={project.icon === ''}
        onClick={() => update.mutate({ icon: '' })}
        className="mt-1.5 h-8 w-full rounded-lg px-2 text-left text-[13px] text-soft hover:bg-hover aria-pressed:bg-hover"
      >
        Letter
      </button>
      <div className="mt-2 flex justify-between border-t border-line pt-3">
        {palette.map((c) => (
          <button
            key={c}
            type="button"
            aria-label={`Colour ${c}`}
            aria-pressed={tile.colour === c}
            onClick={() => update.mutate({ colour: c })}
            style={{ background: c }}
            className="size-6 rounded-full aria-pressed:ring-2 aria-pressed:ring-fg aria-pressed:ring-offset-2 aria-pressed:ring-offset-card"
          />
        ))}
      </div>
      {update.isError && <p className="mt-2 text-sm text-danger">{update.error.message}</p>}
    </div>
  );
}

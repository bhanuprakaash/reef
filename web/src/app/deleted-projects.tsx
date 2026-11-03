import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';
import { spacesQuery } from '@/lib/api/get-spaces';
import { useDeletedProjects } from '@/modules/projects/api/get-projects';
import type { Project } from '@/types/api';

function DeletedProject({ project }: { project: Project }) {
  const [open, setOpen] = useState(false);
  const spaces = useQuery({ ...spacesQuery(project.id), enabled: open });
  return (
    <li>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="px-4 py-1 text-soft"
      >
        {project.name}
      </button>
      {open && (
        <ul className="pl-7">
          {spaces.data?.map((space) => (
            <li key={space.id}>
              <Link className="block py-1 text-muted" to={`/spaces/${space.id}`}>
                {space.name}
              </Link>
            </li>
          ))}
        </ul>
      )}
      {spaces.isError && <p className="text-danger">{spaces.error.message}</p>}
    </li>
  );
}

export function DeletedProjects() {
  const projects = useDeletedProjects();
  const [open, setOpen] = useState(false);
  return (
    <section aria-label="Deleted projects">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="px-2 py-2 text-xs text-muted"
      >
        Deleted projects
      </button>
      {open && (
        <ul>
          {projects.data?.map((project) => (
            <DeletedProject key={project.id} project={project} />
          ))}
        </ul>
      )}
      {projects.isError && <p className="text-danger">{projects.error.message}</p>}
    </section>
  );
}

import { useRef, useState } from 'react';
import { useNavigate } from 'react-router';
import { Dialog } from '@/components/dialog';
import { Menu, MenuItem, MenuSeparator } from '@/components/menu';
import { ProjectTile } from '@/components/project-tile';
import { useCurrentProject } from '@/lib/current-project';
import { useDismiss } from '@/lib/use-dismiss';
import { ProjectIconPicker } from '@/modules/projects/components/project-icon-picker';
import { AddProjectForm } from '@/modules/projects/components/add-project-form';
import type { Project } from '@/types/api';
import { projectTile } from '@/utils/project-tile';

function Tile({ project, size }: { project?: Project; size?: number }) {
  return <ProjectTile {...(project && projectTile(project))} size={size} />;
}

function Check() {
  return (
    <svg
      width="15"
      height="15"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className="ml-auto flex-none"
    >
      <path d="M5 12.5l4.5 4.5L19 7.5" />
    </svg>
  );
}

export function ProjectSwitcher({ projects }: { projects: Project[] }) {
  const [current, onPick] = useCurrentProject(projects);
  const [open, setOpen] = useState(false);
  const [adding, setAdding] = useState(false);
  const [picking, setPicking] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();

  useDismiss(open || picking, ref, () => {
    setOpen(false);
    setPicking(false);
  });

  function pick(project: Project | null) {
    onPick(project);
    setOpen(false);
  }

  return (
    <div ref={ref} className="relative mt-3.5 mb-1.5">
      <button
        type="button"
        aria-label="Switch project"
        aria-expanded={open}
        title={current?.path}
        onClick={() => setOpen(!open)}
        className="flex h-9 w-full items-center gap-2.5 rounded-lg px-2 text-left text-sm hover:bg-hover"
      >
        <Tile project={current} />
        <span className="min-w-0 truncate font-medium">{current?.name ?? 'All projects'}</span>
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
          className="ml-auto flex-none text-muted"
        >
          <path d="M8 9l4-4 4 4M8 15l4 4 4-4" />
        </svg>
      </button>
      {open && (
        <Menu className="absolute inset-x-0 top-full z-10 mt-1">
          <MenuItem onClick={() => pick(null)}>
            <Tile size={20} />
            All projects
            {!current && <Check />}
          </MenuItem>
          {projects.map((p) => (
            <MenuItem key={p.id} onClick={() => pick(p)}>
              <Tile project={p} size={20} />
              {p.name}
              {current?.id === p.id && <Check />}
            </MenuItem>
          ))}
          <MenuSeparator />
          {current && (
            <>
              <MenuItem
                onClick={() => {
                  void navigate(`/projects/${current.id}`);
                  setOpen(false);
                }}
              >
                Open project
              </MenuItem>
              <MenuItem
                onClick={() => {
                  setPicking(true);
                  setOpen(false);
                }}
              >
                Change icon
              </MenuItem>
            </>
          )}
          <MenuItem
            onClick={() => {
              setAdding(true);
              setOpen(false);
            }}
          >
            New project…
          </MenuItem>
        </Menu>
      )}
      {picking && current && <ProjectIconPicker project={current} />}
      <Dialog open={adding} onClose={() => setAdding(false)} title="New project">
        {adding && <AddProjectForm onClose={() => setAdding(false)} onCreated={pick} />}
      </Dialog>
    </div>
  );
}

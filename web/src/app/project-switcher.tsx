import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router';
import { Logo } from '@/components/logo';
import { Menu, MenuItem, MenuSeparator } from '@/components/menu';
import { AddProjectForm } from '@/modules/projects/components/add-project-form';
import type { Project } from '@/types/api';

type Props = { projects: Project[]; current: Project; onPick: (project: Project) => void };

export function ProjectSwitcher({ projects, current, onPick }: Props) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();

  useEffect(() => {
    if (!open) return;
    const close = (e: Event) => {
      if (
        e instanceof KeyboardEvent ? e.key === 'Escape' : !ref.current?.contains(e.target as Node)
      )
        setOpen(false);
    };
    document.addEventListener('pointerdown', close);
    document.addEventListener('keydown', close);
    return () => {
      document.removeEventListener('pointerdown', close);
      document.removeEventListener('keydown', close);
    };
  }, [open]);

  function pick(project: Project) {
    onPick(project);
    setOpen(false);
  }

  return (
    <div ref={ref} className="relative mt-3.5 mb-1.5">
      <button
        type="button"
        aria-label="Switch project"
        aria-expanded={open}
        title={current.path}
        onClick={() => setOpen(!open)}
        className="flex h-[30px] w-full items-center gap-2 rounded-lg px-2 text-left text-[13px] hover:bg-hover"
      >
        <Logo size={18} />
        {current.name}
        <svg
          width="12"
          height="12"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.2"
          strokeLinecap="round"
          aria-hidden="true"
          className="text-muted"
        >
          <path d="M6 9l6 6 6-6" />
        </svg>
      </button>
      {open && (
        <Menu className="absolute inset-x-0 top-full z-10 mt-1">
          {projects.map((p) => (
            <MenuItem key={p.id} onClick={() => pick(p)}>
              {p.name}
            </MenuItem>
          ))}
          <MenuSeparator />
          <MenuItem
            onClick={() => {
              void navigate(`/projects/${current.id}`);
              setOpen(false);
            }}
          >
            Open project
          </MenuItem>
          <div className="p-1.5">
            <AddProjectForm onCreated={pick} />
          </div>
        </Menu>
      )}
    </div>
  );
}

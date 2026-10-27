import { useState } from 'react';
import { FeatureSpaces } from '@/app/feature-spaces';
import { Dialog } from '@/components/dialog';
import { TextField } from '@/components/text-field';
import { NewSpaceForm } from '@/modules/spaces/components/new-space-form';
import { useProjects } from '@/modules/projects/api/get-projects';
import type { Project } from '@/types/api';

export function SpacesPage() {
  const projects = useProjects();
  const [query, setQuery] = useState('');
  const [creating, setCreating] = useState<Project | null>(null);

  return (
    <div className="mx-auto flex w-full max-w-[820px] flex-col gap-4 py-6">
      <h1 className="text-[32px] font-normal tracking-[-0.02em]">Spaces</h1>
      <TextField
        type="search"
        aria-label="Search"
        placeholder="Search"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      {projects.isError && <p className="text-sm text-danger">{projects.error.message}</p>}
      {projects.data?.map((p) => (
        <div key={p.id}>
          <div className="flex items-center justify-between px-2.5">
            <h2 className="text-[15px]">{p.name}</h2>
            <button
              type="button"
              aria-label={`New quick session in ${p.name}`}
              onClick={() => setCreating(p)}
              className="flex size-11 items-center justify-center rounded-full bg-bubble text-soft"
            >
              <svg
                width="20"
                height="20"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
                aria-hidden="true"
              >
                <path d="M12 5v14M5 12h14" />
              </svg>
            </button>
          </div>
          <FeatureSpaces project={p} query={query} />
        </div>
      ))}
      <Dialog open={creating !== null} onClose={() => setCreating(null)} title="New quick session">
        {creating && (
          <NewSpaceForm
            projectId={creating.id}
            featureId={null}
            onClose={() => setCreating(null)}
          />
        )}
      </Dialog>
    </div>
  );
}

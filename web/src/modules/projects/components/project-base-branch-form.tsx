import { useState } from 'react';
import { useUpdateProject } from '@/modules/projects/api/update-project';
import { TextField } from '@/components/text-field';
import type { Project } from '@/types/api';

export function ProjectBaseBranchForm({ project }: { project: Project }) {
  const update = useUpdateProject(project);
  const [draft, setDraft] = useState<string | null>(null);
  const value = draft ?? project.baseBranch;

  function save() {
    const baseBranch = value.trim();
    if (draft === null || baseBranch === project.baseBranch) return setDraft(null);
    update.mutate({ baseBranch }, { onSuccess: () => setDraft(null) });
  }

  return (
    <div>
      <div className="mb-1.5 flex items-center text-xs text-muted">
        <label htmlFor="project-base-branch">Base branch</label>
        {update.isSuccess && draft === null && <span className="ml-auto text-done">Saved</span>}
      </div>
      <TextField
        id="project-base-branch"
        className="w-full border-transparent font-mono hover:border-line"
        placeholder="default (origin HEAD)"
        value={value}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={save}
        onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
      />
      <p className="mt-1.5 text-[12.5px] text-muted">
        New Work items start here, and PRs target it.
      </p>
      {update.isError && <p className="mt-1.5 text-sm text-danger">{update.error.message}</p>}
    </div>
  );
}

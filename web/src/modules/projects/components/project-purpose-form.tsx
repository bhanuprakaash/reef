import { useState } from 'react';
import { useUpdateProject } from '@/modules/projects/api/update-project';
import { TextArea } from '@/components/text-area';
import type { Project } from '@/types/api';

export function ProjectPurposeForm({ project }: { project: Project }) {
  const update = useUpdateProject(project);
  const [draft, setDraft] = useState<string | null>(null);
  const value = draft ?? project.purpose;

  function save() {
    const purpose = value.trim();
    if (draft === null || purpose === project.purpose) return setDraft(null);
    update.mutate({ purpose }, { onSuccess: () => setDraft(null) });
  }

  return (
    <div>
      <div className="mb-1.5 flex items-center text-xs text-muted">
        <label htmlFor="project-purpose">Purpose</label>
        {update.isSuccess && draft === null && <span className="ml-auto text-done">Saved</span>}
      </div>
      <TextArea
        id="project-purpose"
        className="w-full resize-none border-transparent leading-[1.45] hover:border-line"
        rows={2}
        placeholder="What this Project is for"
        value={value}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={save}
      />
      {update.isError && <p className="mt-1.5 text-sm text-danger">{update.error.message}</p>}
    </div>
  );
}

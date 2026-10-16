import { useState } from 'react';
import { useUpdateProject } from '@/modules/projects/api/update-project';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { TextField } from '@/components/text-field';
import type { Project } from '@/types/api';

export function ProjectBaseBranchForm({ project }: { project: Project }) {
  const update = useUpdateProject(project);
  const [draft, setDraft] = useState<string | null>(null);
  const value = draft ?? project.baseBranch;

  return (
    <Card title="Base branch">
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          update.mutate(value.trim(), { onSuccess: () => setDraft(null) });
        }}
      >
        <TextField
          aria-label="Base branch"
          className="w-full font-mono"
          placeholder="default (origin HEAD)"
          value={value}
          onChange={(e) => setDraft(e.target.value)}
        />
        <p className="text-[12.5px] text-muted">New features start here, and PRs target it.</p>
        {update.isError && <p className="text-sm text-danger">{update.error.message}</p>}
        <div className="flex items-center justify-end gap-2.5">
          {update.isSuccess && draft === null && (
            <span className="text-[12.5px] text-done">Saved</span>
          )}
          <Button variant="primary" type="submit" loading={update.isPending}>
            Save
          </Button>
        </div>
      </form>
    </Card>
  );
}

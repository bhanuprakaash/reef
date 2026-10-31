import { useState } from 'react';
import { useUpdateProject } from '@/modules/projects/api/update-project';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { TextArea } from '@/components/text-area';
import type { Project } from '@/types/api';

export function ProjectPurposeForm({ project }: { project: Project }) {
  const update = useUpdateProject(project);
  const [draft, setDraft] = useState<string | null>(null);
  const value = draft ?? project.purpose;

  return (
    <Card title="Purpose">
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          update.mutate({ purpose: value.trim() }, { onSuccess: () => setDraft(null) });
        }}
      >
        <TextArea
          aria-label="Purpose"
          className="w-full"
          rows={3}
          placeholder="What this Project is for"
          value={value}
          onChange={(e) => setDraft(e.target.value)}
        />
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

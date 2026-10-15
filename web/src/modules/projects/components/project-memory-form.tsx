import { useState } from 'react';
import { useProjectMemory } from '@/modules/projects/api/get-project-memory';
import { useUpdateProjectMemory } from '@/modules/projects/api/update-project-memory';
import { Button } from '@/components/button';
import { TextArea } from '@/components/text-area';

export function ProjectMemoryForm({ projectId }: { projectId: number }) {
  const memory = useProjectMemory(projectId);
  const update = useUpdateProjectMemory(projectId);
  const [body, setBody] = useState<string | null>(null);

  if (memory.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (memory.isError) return <p className="text-sm text-danger">{memory.error.message}</p>;

  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault();
        update.mutate(body ?? memory.data.body);
      }}
    >
      <label className="block space-y-1">
        <span className="text-sm text-muted">Project memory</span>
        <TextArea
          className="w-full"
          rows={6}
          value={body ?? memory.data.body}
          onChange={(e) => setBody(e.target.value)}
        />
      </label>
      <div className="flex items-center gap-3">
        <Button variant="primary" type="submit" loading={update.isPending}>
          Save
        </Button>
        {update.isSuccess && <span className="text-sm text-muted">Saved</span>}
        {update.isError && <span className="text-sm text-danger">{update.error.message}</span>}
      </div>
    </form>
  );
}

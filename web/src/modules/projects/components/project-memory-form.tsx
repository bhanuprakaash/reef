import { useState } from 'react';
import { useProjectMemory } from '@/modules/projects/api/get-project-memory';
import { useUpdateProjectMemory } from '@/modules/projects/api/update-project-memory';
import { buttonClass, errorClass, inputClass, mutedClass } from '@/utils/classes';

export function ProjectMemoryForm({ projectId }: { projectId: number }) {
  const memory = useProjectMemory(projectId);
  const update = useUpdateProjectMemory(projectId);
  const [body, setBody] = useState<string | null>(null);

  if (memory.isPending) return <p className={mutedClass}>Loading...</p>;
  if (memory.isError) return <p className={errorClass}>{memory.error.message}</p>;

  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault();
        update.mutate(body ?? memory.data.body);
      }}
    >
      <label className="block space-y-1">
        <span className={mutedClass}>Project memory</span>
        <textarea
          className={`${inputClass} w-full`}
          rows={6}
          value={body ?? memory.data.body}
          onChange={(e) => setBody(e.target.value)}
        />
      </label>
      <div className="flex items-center gap-3">
        <button type="submit" className={buttonClass} disabled={update.isPending}>
          Save
        </button>
        {update.isSuccess && <span className={mutedClass}>Saved</span>}
        {update.isError && <span className={errorClass}>{update.error.message}</span>}
      </div>
    </form>
  );
}

import { useState } from 'react';
import { useProjectMemory } from '@/modules/projects/api/get-project-memory';
import { useUpdateProjectMemory } from '@/modules/projects/api/update-project-memory';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { SegmentedControl } from '@/components/segmented-control';

export function ProjectMemoryForm({ projectId }: { projectId: number }) {
  const memory = useProjectMemory(projectId);
  const update = useUpdateProjectMemory(projectId);
  const [body, setBody] = useState<string | null>(null);
  const [mode, setMode] = useState<'write' | 'preview'>('write');

  if (memory.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (memory.isError) return <p className="text-sm text-danger">{memory.error.message}</p>;

  const text = body ?? memory.data.body;
  const words = text.trim() === '' ? 0 : text.trim().split(/\s+/).length;

  return (
    <Card
      title="Project memory"
      flush
      aside={
        <>
          <span className="text-[12.5px] text-muted">loaded into every session in this repo</span>
          {update.isSuccess && <span className="ml-auto text-[12.5px] text-done">Saved</span>}
          {update.isError && (
            <span className="ml-auto text-[12.5px] text-danger">{update.error.message}</span>
          )}
        </>
      }
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          update.mutate(text);
        }}
      >
        <div className="border-b border-hairline px-3.5 pb-2">
          <SegmentedControl
            label="Memory view"
            options={[
              { value: 'write', label: 'Write' },
              { value: 'preview', label: 'Preview' },
            ]}
            value={mode}
            onChange={setMode}
          />
        </div>
        {mode === 'write' ? (
          <textarea
            aria-label="Project memory"
            rows={14}
            value={text}
            onChange={(e) => setBody(e.target.value)}
            className="block w-full resize-y bg-transparent px-[18px] py-3.5 font-mono text-[12.5px] leading-[1.7] text-soft outline-none"
          />
        ) : (
          <div className="min-h-[18.5rem] px-[18px] py-3.5 text-[13.5px] leading-[1.6] whitespace-pre-wrap text-soft">
            {text}
          </div>
        )}
        <div className="flex items-center gap-2.5 border-t border-hairline px-[18px] py-3">
          <span className="text-[12.5px] text-muted">
            {words} words · keep it short, every turn pays for it
          </span>
          <Button variant="primary" type="submit" loading={update.isPending} className="ml-auto">
            Save
          </Button>
        </div>
      </form>
    </Card>
  );
}

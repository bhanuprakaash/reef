import { useState } from 'react';
import { useAddMemory } from '@/lib/api/add-memory';
import { useMemory } from '@/lib/api/get-memory';
import { useRetireMemory } from '@/lib/api/retire-memory';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { Select } from '@/components/select';
import { TextArea } from '@/components/text-area';

const KINDS = ['decision', 'convention', 'finding', 'fact', 'lesson'].map((k) => ({
  value: k,
  label: k,
}));

export function ProjectMemoryForm({ projectId }: { projectId: number }) {
  const scope = `project:${projectId}`;
  const items = useMemory(scope);
  const add = useAddMemory(scope);
  const retire = useRetireMemory(scope);
  const [kind, setKind] = useState('convention');
  const [body, setBody] = useState('');

  if (items.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (items.isError) return <p className="text-sm text-danger">{items.error.message}</p>;

  return (
    <Card
      title="Project memory"
      flush
      aside={
        <>
          <span className="text-[12.5px] text-muted">read by every session in this repo</span>
          {(add.isError || retire.isError) && (
            <span className="ml-auto text-[12.5px] text-danger">
              {(add.error ?? retire.error)?.message}
            </span>
          )}
        </>
      }
    >
      {items.data.length > 0 && (
        <ul>
          {items.data.map((item) => (
            <li key={item.id} className="flex items-start gap-3.5 border-t border-hairline pl-6">
              <span className="w-20 shrink-0 pt-3.5 text-xs text-muted">{item.kind}</span>
              <span className="min-w-0 flex-1 py-3.5 text-sm whitespace-pre-wrap">{item.body}</span>
              <Button
                variant="ghost"
                size="sm"
                className="mt-2.5 mr-3 shrink-0"
                disabled={retire.isPending}
                aria-label={`Retire ${item.kind} ${item.id}`}
                onClick={() => retire.mutate(item.id)}
              >
                Retire
              </Button>
            </li>
          ))}
        </ul>
      )}
      <form
        className="flex flex-col gap-2.5 border-t border-hairline px-[18px] py-3.5"
        onSubmit={(e) => {
          e.preventDefault();
          add.mutate({ kind, body: body.trim() }, { onSuccess: () => setBody('') });
        }}
      >
        <TextArea
          aria-label="New project memory"
          rows={3}
          placeholder="Keep it short, every session reads it"
          value={body}
          onChange={(e) => setBody(e.target.value)}
        />
        <div className="flex items-center gap-2.5">
          <Select aria-label="Kind" value={kind} onChange={setKind} options={KINDS} />
          <Button
            variant="primary"
            type="submit"
            loading={add.isPending}
            disabled={body.trim() === ''}
            className="ml-auto"
          >
            Add
          </Button>
        </div>
      </form>
    </Card>
  );
}

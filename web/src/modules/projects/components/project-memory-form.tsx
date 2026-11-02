import { useState } from 'react';
import { useAddMemory } from '@/lib/api/add-memory';
import { useMemory } from '@/lib/api/get-memory';
import { useRetireMemory } from '@/lib/api/retire-memory';
import { Button } from '@/components/button';
import { Select } from '@/components/select';
import { TextField } from '@/components/text-field';

const KINDS = ['decision', 'convention', 'finding', 'fact', 'lesson'].map((k) => ({
  value: k,
  label: k,
}));

const SHOWN = 3;

export function ProjectMemoryForm({ projectId }: { projectId: number }) {
  const scope = `project:${projectId}`;
  const items = useMemory(scope);
  const add = useAddMemory(scope);
  const retire = useRetireMemory(scope);
  const [kind, setKind] = useState('convention');
  const [body, setBody] = useState('');
  const [all, setAll] = useState(false);

  if (items.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (items.isError) return <p className="text-sm text-danger">{items.error.message}</p>;

  const shown = all ? items.data : items.data.slice(0, SHOWN);

  return (
    <section aria-label="Project memory">
      <div className="flex items-center gap-2">
        <h2 className="text-[14.5px]">Memory</h2>
        <span className="text-xs text-faint">{items.data.length}</span>
        <span className="ml-auto text-xs text-muted">read by every session</span>
      </div>
      {(add.isError || retire.isError) && (
        <p className="mt-1 text-[12.5px] text-danger">{(add.error ?? retire.error)?.message}</p>
      )}
      {shown.length > 0 && (
        <ul className="mt-1">
          {shown.map((item) => (
            <li key={item.id} className="group relative border-b border-hairline py-2.5 pr-9">
              <span className="block text-[10px] tracking-[0.08em] text-faint uppercase">
                {item.kind}
              </span>
              <span className="mt-0.5 block text-[13px] whitespace-pre-wrap text-soft">
                {item.body}
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="absolute top-1.5 right-0 md:opacity-0 md:group-hover:opacity-100 md:focus-visible:opacity-100"
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
      {items.data.length > SHOWN && (
        <Button variant="ghost" size="sm" className="mt-1" onClick={() => setAll(!all)}>
          {all ? 'Show less' : `Show all ${items.data.length}`}
        </Button>
      )}
      <form
        className="mt-2.5 grid grid-cols-[1fr_auto] gap-1.5"
        onSubmit={(e) => {
          e.preventDefault();
          add.mutate({ kind, body: body.trim() }, { onSuccess: () => setBody('') });
        }}
      >
        <TextField
          aria-label="New project memory"
          className="col-span-2 w-full"
          placeholder="Add a memory item"
          value={body}
          onChange={(e) => setBody(e.target.value)}
        />
        <Select aria-label="Kind" value={kind} onChange={setKind} options={KINDS} />
        <Button
          variant="primary"
          type="submit"
          loading={add.isPending}
          disabled={body.trim() === ''}
        >
          Add
        </Button>
      </form>
      {add.data?.similar?.map((s) => (
        <p key={s.id} className="mt-1.5 text-sm text-muted">
          Looks like an existing item ({s.id}): {s.body}
        </p>
      ))}
      {add.data?.embedError && (
        <p className="mt-1.5 text-sm text-muted">Saved without a vector: {add.data.embedError}</p>
      )}
    </section>
  );
}

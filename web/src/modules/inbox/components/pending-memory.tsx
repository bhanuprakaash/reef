import { Button } from '@/components/button';
import { useDecideMemory } from '@/lib/api/decide-memory';
import { useMemory } from '@/lib/api/get-memory';
import { usePendingMemory } from '@/lib/api/get-pending-memory';
import type { PendingMemory } from '@/types/api';

function Replaced({ scope, id }: { scope: string; id: number }) {
  const items = useMemory(scope);
  const old = items.data?.find((i) => i.id === id);
  return (
    <div className="rounded-field border border-hairline px-3 py-2.5">
      <div className="text-xs text-muted">Would retire memory {id}</div>
      <div className="mt-1 whitespace-pre-wrap text-sm text-soft">{old?.body ?? '...'}</div>
    </div>
  );
}

function PendingRow({ item }: { item: PendingMemory }) {
  const decide = useDecideMemory();
  const replaces = item.links?.find((l) => l.rel === 'replaces')?.to.split(':')[1];
  const source = item.links?.find((l) => l.rel === 'source')?.to;
  return (
    <li className="flex flex-col gap-3 rounded-card border border-line bg-card px-5 py-[18px]">
      <div className="flex flex-wrap items-baseline gap-2 text-xs text-muted">
        <span className="uppercase tracking-wider">{item.kind}</span>
        <span>
          {item.author} · {item.scope}
          {source && ` · from ${source.replace(':', ' ')}`}
        </span>
      </div>
      <div className={replaces ? 'grid gap-3 md:grid-cols-2' : ''}>
        {replaces && <Replaced scope={item.scope} id={Number(replaces)} />}
        <div className="whitespace-pre-wrap text-[15px]">{item.body}</div>
      </div>
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="primary"
          disabled={decide.isPending}
          onClick={() => decide.mutate({ id: item.id, decision: 'approve' })}
        >
          Approve
        </Button>
        <Button
          type="button"
          variant="ghost"
          disabled={decide.isPending}
          onClick={() => decide.mutate({ id: item.id, decision: 'reject' })}
        >
          Reject
        </Button>
        {decide.isError && <span className="text-sm text-danger">{decide.error.message}</span>}
      </div>
    </li>
  );
}

export function PendingMemoryList() {
  const pending = usePendingMemory();
  if (pending.isError) return <p className="text-sm text-danger">{pending.error.message}</p>;
  if (!pending.data?.length) return null;
  return (
    <section aria-labelledby="inbox-memory" className="flex flex-col gap-3">
      <div>
        <h2 id="inbox-memory" className="text-[15px]">
          Pending memory <span className="font-normal text-muted">· {pending.data.length}</span>
        </h2>
        <p className="text-[13px] text-muted">
          Bots proposed these for shared memory. Nothing changes until you approve.
        </p>
      </div>
      <ul className="flex flex-col gap-3">
        {pending.data.map((item) => (
          <PendingRow key={item.id} item={item} />
        ))}
      </ul>
    </section>
  );
}

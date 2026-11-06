import { useState } from 'react';
import type { MemoryItem } from '@/types/api';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { Creature, faceOf, type Faces } from '@/components/creature';
import { EmptyState } from '@/components/empty-state';

type Props = {
  items: MemoryItem[];
  faces: Faces;
  onRetire: (id: number) => void;
  retiring?: boolean;
};

const SHOWN = 5;

function MemoryRow({
  item,
  faces,
  onRetire,
  retiring,
}: { item: MemoryItem; faces: Faces } & Pick<Props, 'onRetire' | 'retiring'>) {
  const [open, setOpen] = useState(false);
  const author = item.author === 'user' ? 'You' : item.author;
  return (
    <li className="flex items-start border-t border-hairline">
      <button
        type="button"
        aria-expanded={open}
        className="flex min-w-0 flex-1 gap-3.5 py-3.5 pl-6 text-left"
        onClick={() => setOpen(!open)}
      >
        <span className="w-16 shrink-0 pt-0.5 text-xs text-muted">{item.kind}</span>
        <span className="min-w-0 flex-1 select-text">
          <span className={`block text-sm ${open ? '' : 'truncate'}`}>{item.body}</span>
          {open && item.why && <span className="mt-1 block text-sm text-soft">{item.why}</span>}
          <span className="mt-0.5 block text-xs text-muted">
            <span className="capitalize">{author}</span>
            {' · '}
            {new Date(item.createdAt).toLocaleDateString()}
          </span>
        </span>
        {item.author === 'user' ? (
          <span
            aria-hidden="true"
            className="flex size-[22px] shrink-0 items-center justify-center rounded-full bg-pill text-[11px] text-pill-ink"
          >
            {author[0]}
          </span>
        ) : (
          <Creature look={faceOf(faces, item.botId)} size={22} />
        )}
      </button>
      <Button
        variant="ghost"
        size="sm"
        className="mt-2.5 mr-3 shrink-0"
        disabled={retiring}
        aria-label={`Retire ${item.kind} ${item.id}`}
        onClick={() => onRetire(item.id)}
      >
        Retire
      </Button>
    </li>
  );
}

export function MemoryList({ items, faces, onRetire, retiring }: Props) {
  const [showAll, setShowAll] = useState(false);
  return (
    <Card
      title="Memory"
      flush
      aside={<span className="ml-auto text-xs text-muted tabular-nums">{items.length}</span>}
    >
      {items.length === 0 ? (
        <div className="px-6 pb-5">
          <EmptyState
            title="Nothing remembered yet"
            body="What the Bots save for this Work item lands here."
            face="whale"
            compact
          />
        </div>
      ) : (
        <ul>
          {items.slice(0, showAll ? undefined : SHOWN).map((item) => (
            <MemoryRow
              key={item.id}
              item={item}
              faces={faces}
              onRetire={onRetire}
              retiring={retiring}
            />
          ))}
        </ul>
      )}
      {items.length > SHOWN && (
        <button
          type="button"
          className="w-full border-t border-hairline px-6 py-3.5 text-left text-xs text-muted"
          onClick={() => setShowAll(!showAll)}
        >
          {showAll ? 'Show fewer' : `Show all ${items.length}`}
        </button>
      )}
    </Card>
  );
}

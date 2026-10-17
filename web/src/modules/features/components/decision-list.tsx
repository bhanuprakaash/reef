import { useState } from 'react';
import type { Decision } from '@/types/api';
import { Card } from '@/components/card';
import { Creature } from '@/components/creature';
import { EmptyState } from '@/components/empty-state';

type Faces = Record<string, string | undefined>;
type Props = { decisions: Decision[]; faces: Faces };

const SHOWN = 5;

function DecisionRow({ d, faces }: { d: Decision; faces: Faces }) {
  const [open, setOpen] = useState(false);
  const author = d.author === 'user' ? 'You' : d.author;
  return (
    <li className="border-t border-hairline">
      <button
        type="button"
        aria-expanded={open}
        className="flex w-full gap-3.5 px-6 py-3.5 text-left"
        onClick={() => setOpen(!open)}
      >
        <span className="w-7 pt-0.5 font-mono text-xs text-muted">{d.id}</span>
        <span className="min-w-0 flex-1">
          <span className={`block text-sm ${open ? '' : 'truncate'}`}>{d.body}</span>
          {open && d.why && <span className="mt-1 block text-sm text-soft">{d.why}</span>}
          <span className="mt-0.5 block text-xs text-muted">
            <span className="capitalize">{author}</span>
            {' · '}
            {new Date(d.createdAt).toLocaleDateString()}
            {d.replacedBy !== null && ` · replaced by ${d.replacedBy}`}
          </span>
        </span>
        {d.author === 'user' ? (
          <span
            aria-hidden="true"
            className="flex size-[22px] shrink-0 items-center justify-center rounded-full bg-pill text-[11px] text-pill-ink"
          >
            {author[0]}
          </span>
        ) : (
          <Creature role={d.author} face={faces[d.author]} size={22} />
        )}
      </button>
    </li>
  );
}

export function DecisionList({ decisions, faces }: Props) {
  const [showAll, setShowAll] = useState(false);
  const live = decisions.filter((d) => d.retiredAt === null);
  const retired = decisions.filter((d) => d.retiredAt !== null);
  return (
    <Card
      title="Decisions"
      flush
      aside={<span className="ml-auto text-xs text-muted tabular-nums">{live.length}</span>}
    >
      {live.length === 0 ? (
        retired.length === 0 && (
          <div className="px-6 pb-5">
            <EmptyState
              title="No decisions yet"
              body="Choices the team makes land here."
              role="architect"
              compact
            />
          </div>
        )
      ) : (
        <ul>
          {[...live]
            .reverse()
            .slice(0, showAll ? undefined : SHOWN)
            .map((d) => (
              <DecisionRow key={d.id} d={d} faces={faces} />
            ))}
        </ul>
      )}
      {live.length > SHOWN && (
        <button
          type="button"
          className="w-full border-t border-hairline px-6 py-3.5 text-left text-xs text-muted"
          onClick={() => setShowAll(!showAll)}
        >
          {showAll ? 'Show fewer' : `Show all ${live.length}`}
        </button>
      )}
      {retired.length > 0 && (
        <details className="border-t border-hairline">
          <summary className="cursor-pointer px-6 py-3.5 text-xs text-muted">
            Retired ({retired.length})
          </summary>
          <ul>
            {[...retired].reverse().map((d) => (
              <DecisionRow key={d.id} d={d} faces={faces} />
            ))}
          </ul>
        </details>
      )}
    </Card>
  );
}

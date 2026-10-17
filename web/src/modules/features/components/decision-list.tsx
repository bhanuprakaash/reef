import type { Decision } from '@/types/api';
import { Card } from '@/components/card';
import { Creature } from '@/components/creature';
import { EmptyState } from '@/components/empty-state';

type Faces = Record<string, string | undefined>;
type Props = { decisions: Decision[]; faces: Faces };

function DecisionRow({ d, faces }: { d: Decision; faces: Faces }) {
  const author = d.author === 'user' ? 'You' : d.author;
  return (
    <li className="flex gap-3.5 border-t border-hairline px-6 py-3.5">
      <span className="w-7 pt-0.5 font-mono text-xs text-muted">{d.id}</span>
      <div className="min-w-0 flex-1">
        <div className="text-sm">{d.body}</div>
        <div className="mt-0.5 text-xs text-muted">
          {d.why && `${d.why} · `}
          <span className="capitalize">{author}</span>
          {' · '}
          {new Date(d.createdAt).toLocaleString()}
          {d.replacedBy !== null && ` · replaced by ${d.replacedBy}`}
        </div>
      </div>
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
    </li>
  );
}

export function DecisionList({ decisions, faces }: Props) {
  const live = decisions.filter((d) => d.retiredAt === null);
  const retired = decisions.filter((d) => d.retiredAt !== null);
  return (
    <Card
      title="Decisions"
      flush
      aside={<span className="ml-auto text-xs text-muted tabular-nums">{live.length}</span>}
    >
      {live.length === 0 ? (
        <div className="px-6 pb-5">
          <EmptyState
            title="No decisions yet"
            body="Choices the team makes land here."
            role="architect"
            compact
          />
        </div>
      ) : (
        <ul>
          {[...live].reverse().map((d) => (
            <DecisionRow key={d.id} d={d} faces={faces} />
          ))}
        </ul>
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

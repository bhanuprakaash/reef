import type { Decision } from '@/types/api';
import { Card } from '@/components/card';
import { Creature } from '@/components/creature';
import { EmptyState } from '@/components/empty-state';

type Props = { decisions: Decision[]; faces: Record<string, string | undefined> };

export function DecisionList({ decisions, faces }: Props) {
  return (
    <Card
      title="Decisions"
      flush
      aside={<span className="ml-auto text-xs text-muted tabular-nums">{decisions.length}</span>}
    >
      {decisions.length === 0 ? (
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
          {[...decisions].reverse().map((d) => {
            const author = d.author === 'user' ? 'You' : d.author;
            return (
              <li key={d.id} className="flex gap-3.5 border-t border-hairline px-6 py-3.5">
                <span className="w-7 pt-0.5 font-mono text-xs text-muted">{d.id}</span>
                <div className="min-w-0 flex-1">
                  <div className="text-sm">{d.body}</div>
                  <div className="mt-0.5 text-xs text-muted">
                    {d.why && `${d.why} · `}
                    <span className="capitalize">{author}</span>
                    {' · '}
                    {new Date(d.createdAt).toLocaleString()}
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
          })}
        </ul>
      )}
    </Card>
  );
}

import { Markdown } from '@/components/markdown';
import type { Proposal } from '@/types/api';

export function ProposalBody({ p }: { p: Proposal }) {
  return (
    <>
      <div className="rounded-xl bg-field px-3 py-2.5 text-[13.5px] text-soft">
        <h3 className="text-xs text-muted uppercase">Brief</h3>
        <Markdown>{p.brief}</Markdown>
      </div>
      {(p.items.length > 0 || p.retire.length > 0) && (
        <div className="rounded-xl bg-field px-3 py-2.5 text-[13.5px] text-soft">
          <h3 className="text-xs text-muted uppercase">Memory items</h3>
          <ul className="space-y-2">
            {p.items.map((it, i) => (
              <li key={i}>
                <p>
                  {it.kind} ({it.scope}): {it.body}
                </p>
                {it.why && <p className="text-sm text-muted">Why: {it.why}</p>}
                {it.replaces && it.replaces.length > 0 && (
                  <p className="text-sm text-muted">Replaces: {it.replaces.join(', ')}</p>
                )}
              </li>
            ))}
          </ul>
          {p.retire.length > 0 && (
            <p className="text-sm text-muted">retire memory items: {p.retire.join(', ')}</p>
          )}
        </div>
      )}
    </>
  );
}

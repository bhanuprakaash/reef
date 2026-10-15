import type { Decision } from '@/types/api';

export function DecisionList({ decisions }: { decisions: Decision[] }) {
  if (decisions.length === 0) return <p className="text-sm text-muted">No decisions yet.</p>;

  return (
    <ul className="space-y-2 text-sm">
      {decisions.map((d) => (
        <li key={d.id}>
          <p>{d.body}</p>
          {d.why && <p className="text-sm text-muted">Why: {d.why}</p>}
          <p className="text-xs text-muted">
            {d.author}, {new Date(d.createdAt).toLocaleString()}
          </p>
        </li>
      ))}
    </ul>
  );
}

import type { Decision } from '@/types/api';
import { mutedClass } from '@/utils/classes';

export function DecisionList({ decisions }: { decisions: Decision[] }) {
  if (decisions.length === 0) return <p className={mutedClass}>No decisions yet.</p>;

  return (
    <ul className="space-y-2 text-sm">
      {decisions.map((d) => (
        <li key={d.id}>
          <p>{d.body}</p>
          {d.why && <p className={mutedClass}>Why: {d.why}</p>}
          <p className="text-xs text-neutral-500">
            {d.author}, {new Date(d.createdAt).toLocaleString()}
          </p>
        </li>
      ))}
    </ul>
  );
}

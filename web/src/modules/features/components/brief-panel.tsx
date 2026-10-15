import type { Brief } from '@/types/api';

export function BriefPanel({ versions }: { versions: Brief[] }) {
  if (versions.length === 0) return <p className="text-sm text-muted">No brief yet.</p>;

  const latest = versions[versions.length - 1];
  const older = versions.slice(0, -1).reverse();

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted">
        Version {versions.length} by {latest.author}, {new Date(latest.createdAt).toLocaleString()}
      </p>
      <pre className="font-sans text-sm whitespace-pre-wrap">{latest.body}</pre>
      {older.length > 0 && (
        <details>
          <summary className="cursor-pointer text-sm">History ({older.length} older)</summary>
          <ul className="mt-2 space-y-2">
            {older.map((b, i) => (
              <li key={b.id}>
                <details>
                  <summary className="cursor-pointer text-sm">
                    Version {older.length - i} by {b.author},{' '}
                    {new Date(b.createdAt).toLocaleString()}
                  </summary>
                  <pre className="mt-1 font-sans text-sm whitespace-pre-wrap">{b.body}</pre>
                </details>
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  );
}

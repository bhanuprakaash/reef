import { useState } from 'react';
import type { Brief } from '@/types/api';
import { Card } from '@/components/card';

export function BriefPanel({ versions }: { versions: Brief[] }) {
  const [showHistory, setShowHistory] = useState(false);
  const latest = versions[versions.length - 1];
  const older = versions.slice(0, -1).reverse();

  return (
    <Card
      title="Brief"
      aside={
        latest && (
          <span className="text-xs text-muted">
            Version {versions.length} by <span className="capitalize">{latest.author}</span>
            {' · '}
            {new Date(latest.createdAt).toLocaleString()}
            {older.length > 0 && (
              <>
                {' · '}
                <button
                  type="button"
                  aria-expanded={showHistory}
                  className="border-b border-line text-soft"
                  onClick={() => setShowHistory(!showHistory)}
                >
                  History ({older.length} older)
                </button>
              </>
            )}
          </span>
        )
      }
    >
      {!latest ? (
        <p className="text-sm text-muted">No brief yet.</p>
      ) : (
        <div className="space-y-3">
          <pre className="font-sans text-[15px] leading-relaxed whitespace-pre-wrap text-soft">
            {latest.body}
          </pre>
          {showHistory && (
            <ul className="space-y-2 border-t border-hairline pt-3">
              {older.map((b, i) => (
                <li key={b.id}>
                  <details>
                    <summary className="cursor-pointer text-[13px] text-soft">
                      Version {older.length - i} by <span className="capitalize">{b.author}</span>,{' '}
                      {new Date(b.createdAt).toLocaleString()}
                    </summary>
                    <pre className="mt-1.5 font-sans text-sm whitespace-pre-wrap text-muted">
                      {b.body}
                    </pre>
                  </details>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </Card>
  );
}

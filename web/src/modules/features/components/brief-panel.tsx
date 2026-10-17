import { useLayoutEffect, useRef, useState } from 'react';
import type { Brief } from '@/types/api';
import { Card } from '@/components/card';
import { Markdown } from '@/components/markdown';
import { EmptyState } from '@/components/empty-state';

export function BriefPanel({ versions }: { versions: Brief[] }) {
  const [showHistory, setShowHistory] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [overflows, setOverflows] = useState(false);
  const body = useRef<HTMLDivElement>(null);
  const latest = versions[versions.length - 1];
  const older = versions.slice(0, -1).reverse();

  useLayoutEffect(() => {
    const el = body.current;
    if (el && !expanded) setOverflows(el.scrollHeight > el.clientHeight);
  }, [latest?.body, expanded]);

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
        <EmptyState
          title="No brief yet"
          body="What this feature is for goes here."
          role="architect"
          compact
        />
      ) : (
        <div className="space-y-3">
          <div
            ref={body}
            className={`text-[15px] leading-relaxed text-soft ${expanded ? '' : 'max-h-52 overflow-hidden'} ${overflows && !expanded ? 'mask-[linear-gradient(to_bottom,black_75%,transparent)]' : ''}`}
          >
            <Markdown>{latest.body}</Markdown>
          </div>
          {(overflows || expanded) && (
            <button
              type="button"
              aria-expanded={expanded}
              className="border-b border-line text-xs text-soft"
              onClick={() => setExpanded(!expanded)}
            >
              {expanded ? 'Show less' : 'Show all'}
            </button>
          )}
          {showHistory && (
            <ul className="space-y-2 border-t border-hairline pt-3">
              {older.map((b, i) => (
                <li key={b.id}>
                  <details>
                    <summary className="cursor-pointer text-[13px] text-soft">
                      Version {older.length - i} by <span className="capitalize">{b.author}</span>,{' '}
                      {new Date(b.createdAt).toLocaleString()}
                    </summary>
                    <div className="mt-1.5 text-sm text-muted">
                      <Markdown>{b.body}</Markdown>
                    </div>
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

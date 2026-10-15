import { useState } from 'react';
import { type ReviewComment, useSendReview } from '@/modules/features/api/send-review';
import type { DiffLine, FileDiff } from '@/types/api';
import { Button } from '@/components/button';
import { TextField } from '@/components/text-field';

const lineClass: Record<DiffLine['kind'], string> = {
  add: 'bg-done/15 text-done',
  del: 'bg-failed/15 text-danger',
  ctx: '',
};

export function DiffPanel({ files, featureId }: { files: FileDiff[]; featureId: number }) {
  const [comments, setComments] = useState<ReviewComment[]>([]);
  const [active, setActive] = useState<{ path: string; line: number; removed: boolean } | null>(
    null,
  );
  const [draft, setDraft] = useState('');
  const send = useSendReview(featureId);

  function addComment() {
    if (!active || draft.trim() === '') return;
    setComments((cs) => [
      ...cs,
      { path: active.path, line: active.line, text: draft.trim(), removed: active.removed },
    ]);
    setActive(null);
    setDraft('');
  }

  if (files.length === 0) return <p className="text-sm text-muted">No changes.</p>;

  return (
    <div className="space-y-4">
      {files.map((f) => (
        <div key={f.path} className="overflow-x-auto rounded border border-line">
          <p className="border-b border-line px-2 py-1 text-sm font-medium">
            {f.path} <span className="text-sm text-muted">{f.status}</span>
          </p>
          {f.hunks.length === 0 ? (
            <p className={'text-sm text-muted px-2 py-1'}>No text changes.</p>
          ) : (
            <pre className="font-mono text-xs">
              {f.hunks.map((h) => (
                <div key={h.header}>
                  <div className={'text-sm text-muted px-2'}>{h.header}</div>
                  {h.lines.map((l, i) => {
                    const num = l.new ?? l.old;
                    const removed = l.kind === 'del';
                    return (
                      <div key={i}>
                        <div
                          data-path={f.path}
                          data-line={num}
                          onClick={
                            num === undefined
                              ? undefined
                              : () => setActive({ path: f.path, line: num, removed })
                          }
                          className={`flex gap-2 px-2 ${lineClass[l.kind]} ${num === undefined ? '' : 'cursor-pointer'}`}
                        >
                          <span className="w-4 select-none">
                            {l.kind === 'add' ? '+' : l.kind === 'del' ? '-' : ' '}
                          </span>
                          <span className="whitespace-pre-wrap">{l.text}</span>
                        </div>
                        {active?.path === f.path &&
                          active.line === num &&
                          active.removed === removed && (
                            <div className="flex gap-2 px-2 py-1">
                              <TextField
                                className="flex-1"
                                aria-label={`Comment on ${f.path}:${num}${removed ? ' (removed)' : ''}`}
                                value={draft}
                                onChange={(e) => setDraft(e.target.value)}
                                onKeyDown={(e) => e.key === 'Enter' && addComment()}
                              />
                              <Button type="button" onClick={addComment}>
                                Add comment
                              </Button>
                            </div>
                          )}
                      </div>
                    );
                  })}
                </div>
              ))}
            </pre>
          )}
        </div>
      ))}
      {comments.length > 0 && (
        <div className="space-y-2">
          <ul className="space-y-1 text-sm">
            {comments.map((c, i) => (
              <li key={i}>
                {c.path}:{c.line}
                {c.removed ? ' (removed)' : ''} — {c.text}
              </li>
            ))}
          </ul>
          {send.isError && <p className="text-sm text-danger">{send.error.message}</p>}
          <Button
            type="button"
            disabled={send.isPending}
            onClick={() => send.mutate(comments, { onSuccess: () => setComments([]) })}
          >
            Send to developer
          </Button>
        </div>
      )}
    </div>
  );
}

import { useState } from 'react';
import { type ReviewComment, useSendReview } from '@/modules/features/api/send-review';
import type { DiffLine, FileDiff } from '@/types/api';
import { buttonClass, errorClass, inputClass, mutedClass } from '@/utils/classes';

const lineClass: Record<DiffLine['kind'], string> = {
  add: 'bg-green-50 text-green-900 dark:bg-green-950 dark:text-green-200',
  del: 'bg-red-50 text-red-900 dark:bg-red-950 dark:text-red-200',
  ctx: '',
};

export function DiffPanel({ files, featureId }: { files: FileDiff[]; featureId: number }) {
  const [comments, setComments] = useState<ReviewComment[]>([]);
  const [active, setActive] = useState<{ path: string; line: number } | null>(null);
  const [draft, setDraft] = useState('');
  const send = useSendReview(featureId);

  function addComment() {
    if (!active || draft.trim() === '') return;
    setComments((cs) => [...cs, { path: active.path, line: active.line, text: draft.trim() }]);
    setActive(null);
    setDraft('');
  }

  if (files.length === 0) return <p className={mutedClass}>No changes.</p>;

  return (
    <div className="space-y-4">
      {files.map((f) => (
        <div
          key={f.path}
          className="overflow-x-auto rounded border border-neutral-300 dark:border-neutral-700"
        >
          <p className="border-b border-neutral-300 px-2 py-1 text-sm font-medium dark:border-neutral-700">
            {f.path} <span className={mutedClass}>{f.status}</span>
          </p>
          {f.hunks.length === 0 ? (
            <p className={mutedClass + ' px-2 py-1'}>No text changes.</p>
          ) : (
            <pre className="font-mono text-xs">
              {f.hunks.map((h) => (
                <div key={h.header}>
                  <div className={mutedClass + ' px-2'}>{h.header}</div>
                  {h.lines.map((l, i) => (
                    <div key={i}>
                      <div
                        data-path={f.path}
                        data-line={l.new}
                        onClick={
                          l.new === undefined
                            ? undefined
                            : () => setActive({ path: f.path, line: l.new! })
                        }
                        className={`flex gap-2 px-2 ${lineClass[l.kind]} ${l.new === undefined ? '' : 'cursor-pointer'}`}
                      >
                        <span className="w-4 select-none">
                          {l.kind === 'add' ? '+' : l.kind === 'del' ? '-' : ' '}
                        </span>
                        <span className="whitespace-pre-wrap">{l.text}</span>
                      </div>
                      {active?.path === f.path && active.line === l.new && (
                        <div className="flex gap-2 px-2 py-1">
                          <input
                            className={`${inputClass} flex-1`}
                            aria-label={`Comment on ${f.path}:${l.new}`}
                            value={draft}
                            onChange={(e) => setDraft(e.target.value)}
                            onKeyDown={(e) => e.key === 'Enter' && addComment()}
                          />
                          <button type="button" className={buttonClass} onClick={addComment}>
                            Add comment
                          </button>
                        </div>
                      )}
                    </div>
                  ))}
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
                {c.path}:{c.line} — {c.text}
              </li>
            ))}
          </ul>
          {send.isError && <p className={errorClass}>{send.error.message}</p>}
          <button
            type="button"
            className={buttonClass}
            disabled={send.isPending}
            onClick={() => send.mutate(comments, { onSuccess: () => setComments([]) })}
          >
            Send to developer
          </button>
        </div>
      )}
    </div>
  );
}

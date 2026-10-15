import { type ReactNode, useState } from 'react';
import { Link } from 'react-router';
import { type ReviewComment, useSendReview } from '@/modules/features/api/send-review';
import { diffCounts } from '@/modules/features/utils/diff-counts';
import type { DiffLine, FileDiff } from '@/types/api';
import { Button } from '@/components/button';
import { Creature } from '@/components/creature';
import { EmptyState } from '@/components/empty-state';
import { TextArea } from '@/components/text-area';

const lineClass: Record<DiffLine['kind'], string> = {
  add: 'bg-done/15 text-done',
  del: 'bg-danger-bg text-danger',
  ctx: 'text-soft',
};

const statusLetter: Record<FileDiff['status'], { letter: string; color: string }> = {
  added: { letter: 'A', color: 'text-done' },
  modified: { letter: 'M', color: 'text-paused' },
  deleted: { letter: 'D', color: 'text-failed' },
};

type Target = { path: string; line: number; removed: boolean };

const sameTarget = (a: Pick<ReviewComment, 'path' | 'line' | 'removed'>, b: Target) =>
  a.path === b.path && a.line === b.line && !!a.removed === b.removed;

function Counts({ files }: { files: FileDiff[] }) {
  const { added, removed } = diffCounts(files);
  return (
    <>
      <span className="text-done">+{added}</span>
      <span className="text-failed">−{removed}</span>
    </>
  );
}

type Props = {
  files: FileDiff[];
  featureId: number;
  featureName: string;
  branch: string;
  developerFace?: string;
  actions?: ReactNode;
};

export function DiffPanel({
  files,
  featureId,
  featureName,
  branch,
  developerFace,
  actions,
}: Props) {
  const [comments, setComments] = useState<ReviewComment[]>([]);
  const [selected, setSelected] = useState('');
  const [active, setActive] = useState<Target | null>(null);
  const [draft, setDraft] = useState('');
  const send = useSendReview(featureId);
  const file = files.find((f) => f.path === selected) ?? files[0];

  function addComment() {
    if (!active || draft.trim() === '') return;
    setComments((cs) => [...cs, { ...active, text: draft.trim() }]);
    setActive(null);
    setDraft('');
  }

  return (
    <>
      <header className="flex min-h-14 flex-wrap items-center gap-2.5 border-b border-hairline py-2 pl-3.5 pr-3.5 md:pl-[22px] md:pr-4">
        <Link to={`/features/${featureId}`} className="text-[13px] text-muted hover:text-fg">
          {featureName}
        </Link>
        <svg
          width="12"
          height="12"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          className="text-faint"
          aria-hidden="true"
        >
          <path d="M9 6l6 6-6 6" />
        </svg>
        <h1 className="text-base font-normal">Changes</h1>
        <span className="font-mono text-[12.5px] text-muted max-md:hidden">{branch} vs main</span>
        <span className="flex gap-2.5 text-[12.5px]">
          <Counts files={files} />
        </span>
        <div className="ml-auto flex items-center gap-2">
          <span className="text-[13px] text-muted">
            {comments.length} comment{comments.length === 1 ? '' : 's'}
          </span>
          <Button
            type="button"
            variant="primary"
            className="max-md:fixed max-md:bottom-7 max-md:left-3.5 max-md:z-10 max-md:w-[calc(100%-1.75rem)]"
            disabled={comments.length === 0 || send.isPending}
            onClick={() => send.mutate(comments, { onSuccess: () => setComments([]) })}
          >
            Send to <Creature role="developer" face={developerFace} size={18} /> Developer
          </Button>
          {actions}
        </div>
      </header>
      {send.isError && <p className="px-[22px] pt-3 text-sm text-danger">{send.error.message}</p>}
      {file === undefined ? (
        <div className="p-5">
          <EmptyState
            title="No changes yet"
            body="Files the developer edits show up here."
            role="developer"
          />
        </div>
      ) : (
        <div className="flex min-h-0 flex-1 max-md:flex-col">
          <nav
            aria-label="Files"
            className="flex w-60 shrink-0 flex-col gap-0.5 overflow-y-auto border-r border-hairline px-2 py-3 text-[13px] max-md:w-auto max-md:flex-row max-md:gap-1.5 max-md:overflow-x-auto max-md:border-r-0 max-md:px-3.5"
          >
            {files.map((f) => {
              const drafts = comments.filter((c) => c.path === f.path).length;
              const { letter, color } = statusLetter[f.status];
              return (
                <button
                  key={f.path}
                  type="button"
                  onClick={() => setSelected(f.path)}
                  aria-current={f === file}
                  className={`flex h-8 items-center gap-2 rounded-lg px-2.5 text-left max-md:shrink-0 max-md:whitespace-nowrap max-md:rounded-pill ${f === file ? 'bg-card max-md:bg-pill max-md:text-pill-ink' : 'hover:bg-card max-md:bg-card'} ${f.status === 'deleted' ? 'text-muted' : 'text-soft'}`}
                >
                  <span className={`w-4 font-mono text-[11px] ${color}`}>{letter}</span>
                  <span
                    className={`min-w-0 flex-1 truncate ${f.status === 'deleted' ? 'line-through' : ''}`}
                  >
                    {f.path}
                  </span>
                  {drafts > 0 && (
                    <span className="flex size-[18px] items-center justify-center rounded-full bg-pill text-[11px] text-pill-ink">
                      {drafts}
                    </span>
                  )}
                </button>
              );
            })}
          </nav>
          <section
            aria-label="Diff"
            className="flex min-w-0 flex-1 flex-col gap-3 overflow-y-auto p-3.5 max-md:pb-24 md:p-5"
          >
            <div className="overflow-hidden rounded-2xl border border-hairline bg-sidebar font-mono text-[12.5px] leading-[1.8]">
              <div className="flex items-center gap-2 border-b border-hairline px-3.5 py-2 font-sans text-[13px]">
                <span>{file.path}</span>
                <Counts files={[file]} />
                <span className="ml-auto text-xs text-muted">click a line to comment</span>
              </div>
              {file.hunks.length === 0 && (
                <p className="px-3.5 py-2 font-sans text-sm text-muted">No text changes.</p>
              )}
              {file.hunks.map((h) => (
                <div key={h.header}>
                  <div className="bg-main px-3.5 py-1 text-muted">{h.header}</div>
                  {h.lines.map((l, i) => {
                    const num = l.new ?? l.old;
                    const removed = l.kind === 'del';
                    const target =
                      num === undefined ? null : { path: file.path, line: num, removed };
                    const here = target && comments.filter((c) => sameTarget(c, target));
                    return (
                      <div key={i}>
                        <div
                          data-path={file.path}
                          data-line={num}
                          onClick={target ? () => setActive(target) : undefined}
                          className={`flex ${lineClass[l.kind]} ${target ? 'cursor-pointer hover:brightness-125' : ''} ${active && target && sameTarget(active, target) ? 'ring-1 ring-fg ring-inset' : ''}`}
                        >
                          <span className="w-8 shrink-0 select-none pr-2 text-right md:w-11 text-faint">
                            {l.old}
                          </span>
                          <span className="w-8 shrink-0 select-none pr-3 text-right md:w-11 text-faint">
                            {l.new}
                          </span>
                          <span className="whitespace-pre-wrap">
                            {l.kind === 'add' ? '+' : l.kind === 'del' ? '-' : ' '}
                            {l.text}
                          </span>
                        </div>
                        {here?.map((c, n) => (
                          <div
                            key={n}
                            className="mt-1.5 mr-3.5 mb-2.5 ml-3.5 md:ml-[100px] flex items-start gap-2 rounded-xl bg-field px-3 py-2.5 font-sans text-[13.5px] leading-normal text-fg"
                          >
                            <span className="flex size-5 flex-none items-center justify-center rounded-full bg-line text-[9px] text-soft">
                              You
                            </span>
                            <span className="flex-1">{c.text}</span>
                            <span className="text-xs text-muted">draft</span>
                          </div>
                        ))}
                        {active && target && sameTarget(active, target) && (
                          <div className="mt-1.5 mr-3.5 mb-2.5 ml-3.5 md:ml-[100px] flex flex-col gap-2 rounded-xl bg-card px-3 py-2.5 font-sans text-[13.5px] leading-normal">
                            <label htmlFor="review-comment" className="text-xs text-muted">
                              Comment on {file.path}:{num}
                              {removed ? ' (removed)' : ''}
                            </label>
                            <TextArea
                              id="review-comment"
                              rows={2}
                              autoFocus
                              className="resize-y"
                              value={draft}
                              onChange={(e) => setDraft(e.target.value)}
                              onKeyDown={(e) => {
                                if (e.key === 'Enter' && !e.shiftKey) {
                                  e.preventDefault();
                                  addComment();
                                }
                              }}
                            />
                            <div className="flex justify-end gap-2">
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                onClick={() => {
                                  setActive(null);
                                  setDraft('');
                                }}
                              >
                                Cancel
                              </Button>
                              <Button
                                type="button"
                                variant="primary"
                                size="sm"
                                onClick={addComment}
                              >
                                Add comment
                              </Button>
                            </div>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              ))}
            </div>
            <p className="text-[12.5px] text-muted">
              Comments are drafts until you send. Sending posts them as one message in the
              Developer&apos;s latest thread, each as path:line and your text.
            </p>
          </section>
        </div>
      )}
    </>
  );
}

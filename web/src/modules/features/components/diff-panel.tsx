import { Fragment, type ReactNode, useState } from 'react';
import { Link } from 'react-router';
import { type DiffScope } from '@/modules/features/api/get-diff';
import { type ReviewComment, useSendReview } from '@/modules/features/api/send-review';
import { diffCounts } from '@/modules/features/utils/diff-counts';
import { buildFileTree, type TreeNode } from '@/modules/features/utils/file-tree';
import type { DiffLine, FileDiff } from '@/types/api';
import { Button } from '@/components/button';
import { Creature } from '@/components/creature';
import { EmptyState } from '@/components/empty-state';
import { SegmentedControl } from '@/components/segmented-control';
import { TextArea } from '@/components/text-area';
import { useMediaQuery } from '@/lib/use-media-query';
import { useStoredFlag } from '@/lib/use-stored-flag';

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
  baseBranch: string;
  scope: DiffScope;
  onScopeChange: (scope: DiffScope) => void;
  developerFace?: string;
  leading?: ReactNode;
  actions?: (drafts: number) => ReactNode;
};

export function DiffPanel({
  files,
  featureId,
  featureName,
  branch,
  baseBranch,
  scope,
  onScopeChange,
  developerFace,
  leading,
  actions,
}: Props) {
  const [comments, setComments] = useState<ReviewComment[]>([]);
  const [selected, setSelected] = useState('');
  const [active, setActive] = useState<Target | null>(null);
  const [draft, setDraft] = useState('');
  const [treeView, toggleTreeView] = useStoredFlag('reef.diffFileTree');
  const [collapsed, setCollapsed] = useState<string[]>([]);
  const wide = useMediaQuery('(min-width: 768px)');
  const send = useSendReview(featureId);
  const file = files.find((f) => f.path === selected) ?? files[0];

  function addComment() {
    if (!active || draft.trim() === '') return;
    setComments((cs) => [...cs, { ...active, text: draft.trim() }]);
    setActive(null);
    setDraft('');
  }

  function fileRow(f: FileDiff, name: string, depth = 0) {
    const drafts = comments.filter((c) => c.path === f.path).length;
    const { letter, color } = statusLetter[f.status];
    return (
      <button
        key={f.path}
        type="button"
        onClick={() => setSelected(f.path)}
        aria-current={f === file}
        style={depth > 0 ? { paddingLeft: 10 + depth * 14 } : undefined}
        className={`flex h-8 items-center gap-2 rounded-lg px-2.5 text-left max-md:shrink-0 max-md:whitespace-nowrap max-md:rounded-pill ${f === file ? 'bg-card max-md:bg-pill max-md:text-pill-ink' : 'hover:bg-card max-md:bg-card'} ${f.status === 'deleted' ? 'text-muted' : 'text-soft'}`}
      >
        <span className={`w-4 font-mono text-[11px] ${color}`}>{letter}</span>
        <span className={`min-w-0 flex-1 truncate ${f.status === 'deleted' ? 'line-through' : ''}`}>
          {name}
        </span>
        {drafts > 0 && (
          <span className="flex size-[18px] items-center justify-center rounded-full bg-pill text-[11px] text-pill-ink">
            {drafts}
          </span>
        )}
      </button>
    );
  }

  function treeRows(nodes: TreeNode[], depth: number): ReactNode {
    return nodes.map((n) => {
      if (n.kind === 'file') return fileRow(n.file, n.name, depth);
      const open = !collapsed.includes(n.path);
      return (
        <Fragment key={n.path}>
          <button
            type="button"
            aria-expanded={open}
            onClick={() =>
              setCollapsed((c) => (open ? [...c, n.path] : c.filter((p) => p !== n.path)))
            }
            style={{ paddingLeft: 10 + depth * 14 }}
            className="flex h-8 items-center gap-2 rounded-lg px-2.5 text-left text-soft hover:bg-card"
          >
            <svg
              width="12"
              height="12"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              className={`w-4 shrink-0 text-faint ${open ? 'rotate-90' : ''}`}
              aria-hidden="true"
            >
              <path d="M9 6l6 6-6 6" />
            </svg>
            <span className="min-w-0 flex-1 truncate">{n.name}</span>
          </button>
          {open && treeRows(n.children, depth + 1)}
        </Fragment>
      );
    });
  }

  return (
    <>
      <header
        className={`flex min-h-14 flex-wrap items-center gap-2.5 border-b border-hairline py-2 pl-3.5 pr-3.5 md:pl-[22px] md:pr-4 ${actions ? '' : 'in-data-desktop:pr-[88px]!'}`}
      >
        {leading}
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
        <span className="font-mono text-[12.5px] text-muted max-md:hidden">
          {scope === 'full' ? `${branch} vs ${baseBranch}` : 'uncommitted'}
        </span>
        <SegmentedControl
          label="Diff scope"
          options={[
            { value: 'current', label: 'Current changes' },
            { value: 'full', label: 'Full diff' },
          ]}
          value={scope}
          onChange={onScopeChange}
        />
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
          {actions?.(comments.length)}
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
            {wide && (
              <SegmentedControl
                label="File list view"
                className="mb-1.5 self-start"
                options={[
                  {
                    value: 'list',
                    ariaLabel: 'List view',
                    label: (
                      <svg
                        width="14"
                        height="14"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="2"
                        strokeLinecap="round"
                        aria-hidden="true"
                      >
                        <path d="M4 6h16M4 12h16M4 18h16" />
                      </svg>
                    ),
                  },
                  {
                    value: 'tree',
                    ariaLabel: 'Tree view',
                    label: (
                      <svg
                        width="14"
                        height="14"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="2"
                        strokeLinecap="round"
                        aria-hidden="true"
                      >
                        <path d="M4 5h8M8 12h12M8 19h12M4 5v14M4 12h4M4 19h4" />
                      </svg>
                    ),
                  },
                ]}
                value={treeView ? 'tree' : 'list'}
                onChange={(v) => (v === 'tree') !== treeView && toggleTreeView()}
              />
            )}
            {wide && treeView
              ? treeRows(buildFileTree(files), 0)
              : files.map((f) => fileRow(f, f.path))}
          </nav>
          <section
            aria-label="Diff"
            className="flex min-w-0 flex-1 flex-col gap-3 overflow-y-auto p-3.5 max-md:pb-24 md:p-5"
          >
            <div className="shrink-0 overflow-hidden rounded-2xl border border-hairline bg-sidebar font-mono text-[12.5px] leading-[1.8]">
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
                            <span className="flex-1 whitespace-pre-wrap">{c.text}</span>
                            <span className="text-xs text-muted">draft</span>
                            <button
                              type="button"
                              aria-label="Edit comment"
                              title="Edit"
                              onClick={() => {
                                setComments((cs) => cs.filter((x) => x !== c));
                                setActive(target);
                                setDraft(c.text);
                              }}
                              className="-my-1 flex size-6 flex-none items-center justify-center rounded-md text-muted hover:bg-hover hover:text-fg max-md:size-11"
                            >
                              <svg
                                width="13"
                                height="13"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                strokeWidth="1.8"
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                aria-hidden="true"
                              >
                                <path d="M4 20h4L19 9l-4-4L4 16z" />
                              </svg>
                            </button>
                            <button
                              type="button"
                              aria-label="Delete comment"
                              title="Delete"
                              onClick={() => setComments((cs) => cs.filter((x) => x !== c))}
                              className="-my-1 flex size-6 flex-none items-center justify-center rounded-md text-muted hover:bg-hover hover:text-danger max-md:size-11"
                            >
                              <svg
                                width="13"
                                height="13"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                strokeWidth="1.8"
                                strokeLinecap="round"
                                aria-hidden="true"
                              >
                                <path d="M6 6l12 12M18 6L6 18" />
                              </svg>
                            </button>
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

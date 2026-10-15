import { useState } from 'react';
import { Button, buttonClass } from '@/components/button';
import { Dialog } from '@/components/dialog';
import { StageChip } from '@/components/stage-chip';
import { useFeaturePR } from '@/modules/channels/api/get-feature-pr';
import { useMergeFeaturePR } from '@/modules/channels/api/merge-feature-pr';
import type { FeaturePR } from '@/types/api';

const REVIEW: Record<string, string> = {
  APPROVED: 'Approved',
  CHANGES_REQUESTED: 'Changes requested',
  REVIEW_REQUIRED: 'Review required',
};

const STATE: Record<FeaturePR['state'], { label: string; color: string }> = {
  OPEN: { label: 'Open', color: 'var(--color-done)' },
  MERGED: { label: 'Merged', color: 'var(--color-architect)' },
  CLOSED: { label: 'Closed', color: 'var(--color-failed)' },
};

const CHECK_ICON: Record<
  FeaturePR['checks'][number]['state'],
  { label: string; cls: string; d: string }
> = {
  pass: { label: 'passed', cls: 'text-done', d: 'M5 12.5l4.5 4.5L19 7' },
  fail: { label: 'failed', cls: 'text-failed', d: 'M6 6l12 12M18 6L6 18' },
  pending: {
    label: 'pending',
    cls: 'text-paused',
    d: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 7.5V12l3 2',
  },
};

export function PrPanel({ featureId, onClose }: { featureId: number; onClose: () => void }) {
  const query = useFeaturePR(featureId);
  const merge = useMergeFeaturePR(featureId);
  const [confirming, setConfirming] = useState(false);
  const pr = query.data?.pr;

  return (
    <aside aria-label="Pull request" className="space-y-[18px]">
      <div className="flex items-center gap-2">
        <h2 className="text-[15px] font-normal">Pull request</h2>
        <Button
          variant="ghost"
          size="icon"
          type="button"
          aria-label="Close pull request"
          className="ml-auto"
          onClick={onClose}
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            aria-hidden="true"
          >
            <path d="M6 6l12 12M18 6L6 18" />
          </svg>
        </Button>
      </div>
      {query.isPending && <p className="text-sm text-muted">Loading...</p>}
      {query.isError && <p className="text-sm text-danger">{query.error.message}</p>}
      {query.data && !pr && (
        <p className="text-sm text-muted">No pull request for this feature yet.</p>
      )}
      {pr && (
        <>
          <div className="space-y-2">
            <div className="text-[15px]">{pr.title}</div>
            <div className="flex items-center gap-2 text-[12.5px] text-muted">
              <span>#{pr.number}</span>
              <StageChip
                label={pr.isDraft && pr.state === 'OPEN' ? 'Draft' : STATE[pr.state].label}
                color={STATE[pr.state].color}
              />
            </div>
            <div className="text-[12.5px] text-muted">
              <span className="font-mono">{pr.base}</span> ←{' '}
              <span className="font-mono">{pr.head}</span>
            </div>
          </div>
          <div className="flex items-center justify-between rounded-menu bg-field px-3 py-2.5 text-[13.5px]">
            <span className="text-muted">Review</span>
            <span>{REVIEW[pr.reviewDecision] ?? 'No reviews'}</span>
          </div>
          <section aria-label="Checks" className="space-y-1.5">
            <h3 className="text-[12.5px] font-normal text-muted">Checks</h3>
            {pr.checks.length === 0 && <p className="text-[13.5px] text-muted">No checks.</p>}
            <ul className="space-y-1.5">
              {pr.checks.map((c, i) => {
                const icon = CHECK_ICON[c.state];
                return (
                  <li
                    key={i}
                    className="flex items-center gap-2.5 rounded-menu bg-field px-3 py-2 text-[13.5px]"
                  >
                    <svg
                      role="img"
                      aria-label={icon.label}
                      width="14"
                      height="14"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2.4"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      className={`shrink-0 ${icon.cls}`}
                    >
                      <path d={icon.d} />
                    </svg>
                    <span className="min-w-0 truncate">{c.name}</span>
                  </li>
                );
              })}
            </ul>
          </section>
          <div className="flex flex-wrap items-center gap-2">
            <a href={pr.url} target="_blank" rel="noreferrer" className={buttonClass()}>
              Open on GitHub
            </a>
            {pr.state === 'OPEN' && (
              <Button
                variant="primary"
                type="button"
                disabled={!pr.canMerge}
                onClick={() => setConfirming(true)}
              >
                Merge
              </Button>
            )}
          </div>
          {pr.state === 'OPEN' && !pr.canMerge && (
            <p className="text-[12.5px] text-muted">GitHub does not allow a merge yet.</p>
          )}
          <Dialog
            open={confirming}
            onClose={() => {
              setConfirming(false);
              merge.reset();
            }}
            title="Merge pull request"
          >
            <div className="space-y-4">
              <p className="text-sm text-soft">
                Merge #{pr.number} into <span className="font-mono">{pr.base}</span> with a merge
                commit?
              </p>
              {merge.isError && <p className="text-sm text-danger">{merge.error.message}</p>}
              <div className="flex justify-end gap-2">
                <Button type="button" onClick={() => setConfirming(false)}>
                  Cancel
                </Button>
                <Button
                  variant="primary"
                  type="button"
                  loading={merge.isPending}
                  onClick={() => merge.mutate(undefined, { onSuccess: () => setConfirming(false) })}
                >
                  Merge
                </Button>
              </div>
            </div>
          </Dialog>
        </>
      )}
    </aside>
  );
}

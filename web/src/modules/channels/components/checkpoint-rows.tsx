import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { Menu, MenuItem } from '@/components/menu';
import { useTurns } from '@/modules/channels/api/get-turns';
import { useRollbackTurn } from '@/modules/channels/api/rollback-turn';
import { checkpointItemLabel, checkpointSummary } from '@/modules/channels/utils/checkpoint-label';
import { confirmRollback } from '@/modules/channels/utils/confirm-rollback';

const GAP = 8;

export function CheckpointRows({ channelId, running }: { channelId: number; running: boolean }) {
  const turns = useTurns(channelId);
  const rollback = useRollbackTurn(channelId);
  const trigger = useRef<HTMLButtonElement>(null);
  const popover = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const checkpoints = turns.data ?? [];

  useLayoutEffect(() => {
    const t = trigger.current;
    const el = popover.current;
    if (!open || !t || !el) return;
    const r = t.getBoundingClientRect();
    el.style.maxHeight = '';
    const below = window.innerHeight - r.bottom - GAP;
    const above = r.top - GAP;
    const flip = el.offsetHeight > below && above > below;
    el.style.maxHeight = `${Math.max((flip ? above : below) - 4, 96)}px`;
    const centred = r.left + r.width / 2 - el.offsetWidth / 2;
    el.style.left = `${Math.max(GAP, Math.min(centred, window.innerWidth - el.offsetWidth - GAP))}px`;
    el.style.top = flip ? '' : `${r.bottom + 4}px`;
    el.style.bottom = flip ? `${window.innerHeight - r.top + 4}px` : '';
    el.style.visibility = 'visible';
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const close = () => setOpen(false);
    const outside = (e: Event) => {
      const target = e.target;
      if (!(target instanceof Node)) return;
      if (popover.current?.contains(target) || trigger.current?.contains(target)) return;
      close();
    };
    const escape = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return;
      e.stopPropagation();
      close();
      trigger.current?.focus();
    };
    document.addEventListener('mousedown', outside);
    document.addEventListener('keydown', escape, true);
    window.addEventListener('resize', close);
    window.addEventListener('scroll', close, true);
    return () => {
      document.removeEventListener('mousedown', outside);
      document.removeEventListener('keydown', escape, true);
      window.removeEventListener('resize', close);
      window.removeEventListener('scroll', close, true);
    };
  }, [open]);

  if (checkpoints.length === 0) return null;

  return (
    <section aria-label="Checkpoints" className="space-y-2 text-sm">
      <div className="flex items-center gap-2.5 text-[13px] text-muted">
        <span className="h-px flex-1 bg-track" />
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.8"
          strokeLinecap="round"
          aria-hidden="true"
        >
          <circle cx="12" cy="12" r="4" />
          <path d="M12 2v6M12 16v6" />
        </svg>
        <span>
          {checkpointSummary(checkpoints)} ·{' '}
          <button
            ref={trigger}
            type="button"
            aria-haspopup="menu"
            aria-expanded={open}
            className="border-b border-line text-fg disabled:cursor-not-allowed disabled:text-faint"
            disabled={running || rollback.isPending}
            onClick={() => setOpen((o) => !o)}
          >
            Roll back…
          </button>
        </span>
        <span className="h-px flex-1 bg-track" />
      </div>
      {open && (
        <Menu
          ref={popover}
          aria-label="Checkpoints"
          style={{ visibility: 'hidden' }}
          className="fixed z-50 min-w-[260px] overflow-y-auto"
        >
          {[...checkpoints].reverse().map((t) => (
            <MenuItem
              key={t.id}
              onClick={() => {
                setOpen(false);
                if (confirmRollback(t.id)) rollback.mutate(t.id);
              }}
            >
              {checkpointItemLabel(t)}
            </MenuItem>
          ))}
        </Menu>
      )}
      {rollback.isError && <p className="text-sm text-danger">{rollback.error.message}</p>}
    </section>
  );
}

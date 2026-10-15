import { useState } from 'react';

type Props = {
  label: string;
  hint?: string;
  values: string[];
  mono?: boolean;
  empty?: string;
  missing?: (value: string) => boolean;
  onRemove: (index: number) => void;
  onAdd?: (value: string) => void;
  addLabel?: string;
};

export function ChipList({
  label,
  hint,
  values,
  mono,
  empty,
  missing,
  onRemove,
  onAdd,
  addLabel,
}: Props) {
  const [draft, setDraft] = useState('');

  function add() {
    const value = draft.trim();
    if (value && !values.includes(value)) onAdd?.(value);
    setDraft('');
  }

  return (
    <div className="min-w-0">
      <div className="mb-2 text-xs text-muted">
        {label}
        {hint && <span className="text-asleep"> ({hint})</span>}
      </div>
      <div className="flex flex-wrap gap-1.5">
        {values.map((value, i) => (
          <span
            key={`${value}-${i}`}
            className={`inline-flex h-[26px] items-center gap-1 rounded-pill bg-track pr-1 pl-2.5 text-xs text-soft ${mono ? 'font-mono' : ''}`}
          >
            {value}
            {missing?.(value) && <span className="font-sans text-danger">missing</span>}
            <button
              type="button"
              aria-label={`Remove ${value}`}
              onClick={() => onRemove(i)}
              className="flex size-[18px] items-center justify-center rounded-full text-muted hover:bg-hover hover:text-fg"
            >
              <svg
                className="size-2.5"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2.6"
                strokeLinecap="round"
                aria-hidden="true"
              >
                <path d="M6 6l12 12M18 6L6 18" />
              </svg>
            </button>
          </span>
        ))}
        {values.length === 0 && empty && <span className="text-xs text-muted">{empty}</span>}
        {onAdd && (
          <input
            aria-label={addLabel}
            placeholder="+ Add"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={add}
            onKeyDown={(e) => {
              if (e.key !== 'Enter') return;
              e.preventDefault();
              add();
            }}
            className="h-[26px] w-24 rounded-pill border border-dashed border-line bg-transparent px-2.5 text-xs text-fg outline-none placeholder:text-muted focus-visible:border-fg"
          />
        )}
      </div>
    </div>
  );
}

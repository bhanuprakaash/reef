import { useState } from 'react';
import { Button } from '@/components/button';
import { TextField } from '@/components/text-field';
import type { SelectOption } from '@/components/select';

type Props = {
  label: string;
  'aria-label'?: string;
  options: SelectOption[];
  selected: string[];
  disabled?: boolean;
  onToggle: (value: string) => void;
};

export function SearchPicker({
  label,
  'aria-label': name = label,
  options,
  selected,
  disabled,
  onToggle,
}: Props) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const needle = query.trim().toLowerCase();
  const named = (o: SelectOption) => o.label.toLowerCase().includes(needle);
  const shown = options
    .filter((o) => named(o) || o.hint?.toLowerCase().includes(needle))
    .sort((a, b) => Number(named(b)) - Number(named(a)));

  return (
    <div className="min-w-0">
      <Button
        type="button"
        size="sm"
        aria-expanded={open}
        disabled={disabled}
        onClick={() => setOpen(!open)}
      >
        {open ? 'Done' : label}
      </Button>
      {open && (
        <div className="mt-2 space-y-2">
          <TextField
            autoFocus
            className="w-full"
            type="search"
            aria-label={`${name} search`}
            placeholder="Search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key !== 'Escape') return;
              e.stopPropagation();
              setOpen(false);
            }}
          />
          <div
            role="listbox"
            aria-multiselectable="true"
            aria-label={`${name} options`}
            className="max-h-56 overflow-y-auto rounded-field border border-line p-1"
          >
            {shown.map((o) => {
              const on = selected.includes(o.value);
              return (
                <button
                  key={o.value}
                  type="button"
                  role="option"
                  aria-selected={on}
                  disabled={o.disabled}
                  onClick={() => onToggle(o.value)}
                  className="flex min-h-9 w-full items-start gap-2.5 rounded-lg px-2.5 py-1.5 text-left outline-none hover:bg-hover focus-visible:ring-3 focus-visible:ring-ring disabled:cursor-not-allowed disabled:text-faint"
                >
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm">{o.label}</span>
                    {o.hint && (
                      <span className="line-clamp-2 text-xs text-muted" title={o.hint}>
                        {o.hint}
                      </span>
                    )}
                  </span>
                  <svg
                    className={`mt-0.5 size-3.5 shrink-0 ${on ? '' : 'invisible'}`}
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2.4"
                    strokeLinecap="round"
                    aria-hidden="true"
                  >
                    <path d="M5 12l5 5 9-10" />
                  </svg>
                </button>
              );
            })}
            {shown.length === 0 && <p className="px-2.5 py-2 text-xs text-muted">No match</p>}
          </div>
        </div>
      )}
    </div>
  );
}

import { useEffect, useId, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react';
import { fieldClass } from '@/components/text-field';

export type SelectOption = {
  value: string;
  label: string;
  hint?: string;
  group?: string;
  disabled?: boolean;
};

type Props = {
  value: string;
  onChange: (value: string) => void;
  options: SelectOption[];
  'aria-label': string;
  disabled?: boolean;
  className?: string;
  variant?: 'field' | 'pill' | 'bare';
  prefix?: string;
};

const triggerBase =
  'flex w-full items-center gap-2 text-left outline-none disabled:cursor-not-allowed';
const focusRing = 'focus-visible:ring-3 focus-visible:ring-ring';
const triggerClass = {
  field: `${fieldClass} ${triggerBase} h-11 aria-expanded:border-fg md:h-[38px]`,
  pill: `${triggerBase} ${focusRing} h-11 rounded-pill border border-line px-3 text-[13px] text-soft disabled:text-faint md:h-8`,
  bare: `${triggerBase} ${focusRing} h-11 gap-1 rounded-field px-1 text-xs text-muted disabled:text-faint md:h-6`,
};

const GAP = 8;

export function Select({
  value,
  onChange,
  options,
  disabled,
  className,
  variant = 'field',
  prefix,
  'aria-label': label,
}: Props) {
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const popover = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  const selected = options.find((o) => o.value === value);

  function show() {
    setActive(options.findIndex((o) => o.value === value && !o.disabled));
    setOpen(true);
  }

  function pick(i: number) {
    const option = options[i];
    if (!option || option.disabled) return;
    setOpen(false);
    if (option.value !== value) onChange(option.value);
  }

  function move(from: number, step: 1 | -1) {
    for (let i = from + step; i >= 0 && i < options.length; i += step) {
      if (!options[i].disabled) return i;
    }
    return from;
  }

  useLayoutEffect(() => {
    const t = trigger.current;
    const el = popover.current;
    if (!open || !t || !el) return;
    const r = t.getBoundingClientRect();
    el.style.minWidth = `${Math.max(r.width, 150)}px`;
    el.style.maxHeight = '';
    const height = el.offsetHeight;
    const below = window.innerHeight - r.bottom - GAP;
    const above = r.top - GAP;
    const flip = height > below && above > below;
    const room = flip ? above : below;
    el.style.maxHeight = `${Math.max(room - 4, 96)}px`;
    el.style.left = `${Math.max(GAP, Math.min(r.left, window.innerWidth - el.offsetWidth - GAP))}px`;
    el.style.top = flip ? '' : `${r.bottom + 4}px`;
    el.style.bottom = flip ? `${window.innerHeight - r.top + 4}px` : '';
    el.style.visibility = 'visible';
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const close = (e: Event) => {
      if (e.target instanceof Node && popover.current?.contains(e.target)) return;
      setOpen(false);
    };
    window.addEventListener('resize', close);
    window.addEventListener('scroll', close, true);
    return () => {
      window.removeEventListener('resize', close);
      window.removeEventListener('scroll', close, true);
    };
  }, [open]);

  useEffect(() => {
    if (open) popover.current?.querySelector('[data-active]')?.scrollIntoView({ block: 'nearest' });
  }, [open, active]);

  function onKeyDown(e: KeyboardEvent) {
    if (!open) {
      if (['Enter', ' ', 'ArrowDown', 'ArrowUp'].includes(e.key)) {
        e.preventDefault();
        show();
      }
      return;
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      setOpen(false);
    } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      setActive(move(active, e.key === 'ArrowDown' ? 1 : -1));
    } else if (e.key === 'Home' || e.key === 'End') {
      e.preventDefault();
      setActive(e.key === 'Home' ? move(-1, 1) : move(options.length, -1));
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      pick(active);
    }
  }

  return (
    <div className={`relative inline-flex ${className ?? ''}`}>
      <button
        ref={trigger}
        type="button"
        role="combobox"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? `${id}-list` : undefined}
        aria-activedescendant={open && active >= 0 ? `${id}-${active}` : undefined}
        aria-label={label}
        disabled={disabled}
        className={triggerClass[variant]}
        onClick={() => (open ? setOpen(false) : show())}
        onKeyDown={onKeyDown}
        onBlur={() => setOpen(false)}
      >
        <span className="truncate">
          {prefix}
          {selected?.label}
        </span>
        <svg
          className="ml-auto size-3 shrink-0 text-soft"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.4"
          strokeLinecap="round"
          aria-hidden="true"
        >
          <path d="M6 9l6 6 6-6" />
        </svg>
      </button>
      {open && (
        <div
          ref={popover}
          id={`${id}-list`}
          role="listbox"
          aria-label={label}
          style={{ visibility: 'hidden' }}
          onMouseDown={(e) => e.preventDefault()}
          onClick={(e) => e.preventDefault()}
          className="fixed z-50 max-w-[min(420px,calc(100vw-16px))] overflow-y-auto rounded-menu border border-line bg-card p-1.5 text-sm text-fg shadow-menu"
        >
          {options.map((o, i) => (
            <div key={o.value} role="presentation">
              {o.group !== undefined && o.group !== options[i - 1]?.group && (
                <>
                  {i > 0 && <div className="mx-1 my-1.5 h-px bg-line" />}
                  <div className="px-2.5 pt-1.5 pb-1 text-[11.5px] text-muted">{o.group}</div>
                </>
              )}
              <div
                id={`${id}-${i}`}
                role="option"
                aria-selected={o.value === value}
                aria-disabled={o.disabled || undefined}
                data-active={i === active || undefined}
                onMouseMove={() => !o.disabled && setActive(i)}
                onClick={() => pick(i)}
                className={`flex h-9 cursor-pointer items-center gap-2.5 rounded-lg px-2.5 aria-disabled:cursor-not-allowed aria-disabled:text-faint ${i === active ? 'bg-hover' : ''}`}
              >
                <span className="max-w-full shrink-0 truncate">{o.label}</span>
                {o.hint && (
                  <span title={o.hint} className="ml-auto min-w-0 truncate pl-2 text-xs text-muted">
                    {o.hint}
                  </span>
                )}
                {o.value === value && (
                  <svg
                    className={`size-3.5 shrink-0 ${o.hint ? '' : 'ml-auto'}`}
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2.4"
                    strokeLinecap="round"
                    aria-hidden="true"
                  >
                    <path d="M5 12l5 5 9-10" />
                  </svg>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

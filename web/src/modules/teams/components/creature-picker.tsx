import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react';
import {
  bodyColours,
  Creature,
  creatureName,
  creatureNames,
  type Look,
} from '@/components/creature';

type Props = { value: Look; onChange: (look: Look) => void; size?: number };

const label = (name: string) => `${name[0].toUpperCase()}${name.slice(1).replace('-', ' ')}`;

export function CreaturePicker({ value, onChange, size = 44 }: Props) {
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const popover = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);

  useLayoutEffect(() => {
    const t = trigger.current;
    const el = popover.current;
    if (!open || !t || !el) return;
    const r = t.getBoundingClientRect();
    el.style.left = `${Math.max(8, Math.min(r.left, window.innerWidth - el.offsetWidth - 8))}px`;
    el.style.top = `${r.bottom + 6}px`;
    el.style.visibility = 'visible';
    el.querySelector<HTMLElement>('[aria-pressed="true"]')?.focus({ preventScroll: true });
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const outside = (e: Event) => {
      if (e.target instanceof Node && !root.current?.contains(e.target)) setOpen(false);
    };
    const close = (e: Event) => {
      if (e.target instanceof Node && popover.current?.contains(e.target)) return;
      setOpen(false);
    };
    document.addEventListener('pointerdown', outside);
    window.addEventListener('resize', close);
    window.addEventListener('scroll', close, true);
    return () => {
      document.removeEventListener('pointerdown', outside);
      window.removeEventListener('resize', close);
      window.removeEventListener('scroll', close, true);
    };
  }, [open]);

  function onKeyDown(e: KeyboardEvent) {
    if (!open || e.key !== 'Escape') return;
    e.preventDefault();
    e.stopPropagation();
    setOpen(false);
    trigger.current?.focus();
  }

  return (
    <div ref={root} className="relative shrink-0" onKeyDown={onKeyDown}>
      <button
        ref={trigger}
        type="button"
        aria-label={`Change creature (${label(creatureName(value.face))})`}
        aria-haspopup="true"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        style={{ width: size, height: size }}
        className="flex items-center justify-center rounded-2xl border border-line bg-bubble outline-none hover:border-faint focus-visible:ring-3 focus-visible:ring-ring aria-expanded:border-fg"
      >
        <Creature look={value} size={Math.round(size * 0.65)} animate />
      </button>
      {open && (
        <div
          ref={popover}
          style={{ visibility: 'hidden' }}
          className="fixed z-50 space-y-2 rounded-menu border border-line bg-card p-2 shadow-menu"
        >
          <div className="grid grid-cols-4 gap-1.5">
            {creatureNames.map((name) => (
              <button
                key={name}
                type="button"
                aria-label={label(name)}
                aria-pressed={name === creatureName(value.face)}
                onClick={() => {
                  setOpen(false);
                  trigger.current?.focus();
                  onChange({ ...value, face: name });
                }}
                className={`flex size-10 items-center justify-center rounded-xl bg-bubble outline-none focus-visible:ring-3 focus-visible:ring-ring ${name === creatureName(value.face) ? 'border-2 border-fg' : 'border border-line hover:border-faint'}`}
              >
                <Creature look={{ ...value, face: name }} size={26} />
              </button>
            ))}
          </div>
          <div className="grid grid-cols-4 gap-1.5 border-t border-hairline pt-2">
            {Object.entries(bodyColours).map(([name, hex]) => (
              <button
                key={name}
                type="button"
                aria-label={`${label(name)} colour`}
                aria-pressed={name === value.colour}
                onClick={() => {
                  setOpen(false);
                  trigger.current?.focus();
                  onChange({ ...value, colour: name });
                }}
                style={{ backgroundColor: hex }}
                className={`h-6 rounded-full outline-none focus-visible:ring-3 focus-visible:ring-ring ${name === value.colour ? 'ring-2 ring-fg ring-offset-2 ring-offset-card' : ''}`}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

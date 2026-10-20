import {
  type FocusEvent,
  type PointerEvent,
  type ReactNode,
  useEffect,
  useId,
  useState,
} from 'react';
import { createPortal } from 'react-dom';
import { useLocation } from 'react-router';
import { createHoverIntent } from '@/components/hover-intent';

export type HovercardTrigger = {
  'aria-describedby': string | undefined;
  onPointerEnter: (e: PointerEvent<Element>) => void;
  onPointerLeave: () => void;
  onFocus: (e: FocusEvent<Element>) => void;
  onBlur: () => void;
  onClick: () => void;
};

const GAP = 8;

// The card sits beside the trigger, level with its top edge, or with its bottom edge when the
// trigger is in the lower half of the window, so it never covers the pointer or runs off screen.
export function HovercardPanel({
  id,
  anchor,
  onPointerEnter,
  onPointerLeave,
  children,
}: {
  id: string;
  anchor: DOMRect;
  onPointerEnter?: () => void;
  onPointerLeave?: () => void;
  children: ReactNode;
}) {
  const below = anchor.top > window.innerHeight / 2;
  return (
    <div
      id={id}
      role="tooltip"
      onPointerEnter={onPointerEnter}
      onPointerLeave={onPointerLeave}
      style={{
        left: anchor.right + GAP,
        ...(below ? { bottom: window.innerHeight - anchor.bottom } : { top: anchor.top }),
      }}
      className="fixed z-50 w-72 rounded-menu border border-line bg-card p-3.5 text-sm text-fg shadow-menu [--creature-ring:var(--color-card)]"
    >
      {children}
    </div>
  );
}

// children render the trigger and spread the props on its focusable element. The card is
// not shown on touch, below the md breakpoint, or on focus that is not from the keyboard.
export function Hovercard({
  content,
  delay = 400,
  children,
}: {
  content: ReactNode;
  delay?: number;
  children: (trigger: HovercardTrigger) => ReactNode;
}) {
  const id = useId();
  const { key } = useLocation();
  const [anchor, setAnchor] = useState<{ el: Element; rect: DOMRect } | null>(null);
  const [open, setOpen] = useState(false);
  const [intent] = useState(() => createHoverIntent(setOpen, delay));

  useEffect(() => intent.dismiss(), [key, intent]);
  useEffect(() => intent.cancel, [intent]);
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && intent.dismiss();
    const onScroll = (e: Event) =>
      e.target instanceof Node && anchor && e.target.contains(anchor.el) && intent.dismiss();
    document.addEventListener('keydown', onKey);
    document.addEventListener('scroll', onScroll, true);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('scroll', onScroll, true);
    };
  }, [open, anchor, intent]);

  const wide = () => window.matchMedia('(min-width: 768px)').matches;
  return (
    <>
      {children({
        'aria-describedby': open ? id : undefined,
        onPointerEnter: (e) => {
          if (e.pointerType === 'touch' || !wide()) return;
          setAnchor({ el: e.currentTarget, rect: e.currentTarget.getBoundingClientRect() });
          intent.enter();
        },
        onPointerLeave: intent.leave,
        onFocus: (e) => {
          if (!e.currentTarget.matches(':focus-visible') || !wide()) return;
          setAnchor({ el: e.currentTarget, rect: e.currentTarget.getBoundingClientRect() });
          intent.focus();
        },
        onBlur: intent.blur,
        onClick: intent.dismiss,
      })}
      {open &&
        anchor &&
        createPortal(
          <HovercardPanel
            id={id}
            anchor={anchor.rect}
            onPointerEnter={intent.enter}
            onPointerLeave={intent.leave}
          >
            {content}
          </HovercardPanel>,
          document.body,
        )}
    </>
  );
}

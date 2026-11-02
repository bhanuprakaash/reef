import { useEffect, useRef, type ReactNode } from 'react';

type Props = { open: boolean; onClose: () => void; title: string; children: ReactNode };

export function Dialog({ open, onClose, title, children }: Props) {
  const ref = useRef<HTMLDialogElement>(null);
  const opener = useRef<Element | null>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (open && !el.open) {
      opener.current = document.activeElement;
      el.showModal();
    }
    if (!open && el.open) el.close();
  }, [open]);

  function closed() {
    if (opener.current instanceof HTMLElement) {
      opener.current.blur();
      opener.current.focus({ focusVisible: false });
    }
    onClose();
  }

  return (
    <dialog
      ref={ref}
      aria-label={title}
      onClose={closed}
      className="m-auto w-full max-w-[420px] rounded-card border border-line bg-card p-[22px] text-fg shadow-dialog backdrop:bg-scrim/70"
    >
      <div className="mb-3.5 flex items-center gap-2">
        <h2 className="min-w-0 flex-1 text-lg">{title}</h2>
        <button
          type="button"
          aria-label="Close"
          onClick={() => ref.current?.close()}
          className="-mr-2 flex size-9 shrink-0 items-center justify-center rounded-field text-soft hover:bg-hover"
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
        </button>
      </div>
      {children}
    </dialog>
  );
}

import { useEffect, useRef, type ReactNode } from 'react';

type Props = { open: boolean; onClose: () => void; title: string; children: ReactNode };

export function Dialog({ open, onClose, title, children }: Props) {
  const ref = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (open && !el.open) el.showModal();
    if (!open && el.open) el.close();
  }, [open]);

  return (
    <dialog
      ref={ref}
      aria-label={title}
      onClose={onClose}
      className="m-auto w-full max-w-[420px] rounded-card border border-line bg-card p-[22px] text-fg shadow-dialog backdrop:bg-scrim/70"
    >
      <h2 className="mb-3.5 text-lg">{title}</h2>
      {children}
    </dialog>
  );
}

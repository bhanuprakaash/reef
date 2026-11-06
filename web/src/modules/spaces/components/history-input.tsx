import type { ReactNode } from 'react';

export function HistoryInput({
  readOnly,
  reason,
  children,
}: {
  readOnly: boolean;
  reason: string;
  children: ReactNode;
}) {
  if (!readOnly) return children;
  return (
    <p className="text-sm text-muted">
      This history is read-only because the Project was deleted.{reason && ` Paused: ${reason}`}
    </p>
  );
}

import { useEffect, useState } from 'react';
import { Button } from '@/components/button';

export function UpdateCard() {
  const [version, setVersion] = useState<string | null>(null);
  const [dismissed, setDismissed] = useState<string | null>(null);
  useEffect(() => window.reef?.onUpdateReady(setVersion), []);
  if (!version || dismissed === version) return null;

  return (
    <section
      role="status"
      aria-label="Update"
      className="mt-3 shrink-0 rounded-xl border border-line bg-card p-3 motion-safe:animate-rise-in"
    >
      <div className="flex items-center gap-2">
        <span className="size-2 rounded-full bg-working" aria-hidden="true" />
        <span className="text-sm">Update ready</span>
      </div>
      <p className="mt-0.5 pl-4 text-[13px] text-muted">Reef {version}</p>
      <div className="mt-2.5 flex items-center gap-1">
        <Button variant="primary" size="sm" onClick={() => window.reef?.restartToUpdate()}>
          Restart
        </Button>
        <Button variant="ghost" size="sm" onClick={() => setDismissed(version)}>
          Later
        </Button>
      </div>
    </section>
  );
}

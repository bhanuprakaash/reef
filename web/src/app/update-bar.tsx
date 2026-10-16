import { useEffect, useState } from 'react';
import { Button } from '@/components/button';

export function UpdateBar() {
  const [version, setVersion] = useState<string | null>(null);
  useEffect(() => window.reef?.onUpdateReady(setVersion), []);
  if (!version) return null;

  return (
    <div className="app-drag flex h-9 shrink-0 items-center justify-center gap-3 bg-card text-[13px]">
      <span>Reef {version} is ready</span>
      <Button variant="primary" size="sm" onClick={() => window.reef?.restartToUpdate()}>
        Restart
      </Button>
    </div>
  );
}

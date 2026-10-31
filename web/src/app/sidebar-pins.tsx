import { PinButton } from '@/components/pin-button';
import { ResourceRow } from '@/app/resource-row';
import { usePins } from '@/lib/api/pins';

export function SidebarPins() {
  const pins = usePins().data ?? [];
  if (pins.length === 0) return null;
  return (
    <section aria-label="Pinned" className="flex flex-col gap-0.5">
      <h4 className="px-2.5 pt-3.5 pb-1 text-xs text-muted">Pinned</h4>
      <ul className="flex flex-col gap-0.5">
        {pins.map((p) => (
          <li key={`${p.kind}-${p.id}`} className="group relative">
            <ResourceRow resource={p} />
            <PinButton kind={p.kind} id={p.id} overlay />
          </li>
        ))}
      </ul>
    </section>
  );
}

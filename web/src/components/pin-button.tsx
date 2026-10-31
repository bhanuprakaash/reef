import { Button } from '@/components/button';
import { useTogglePin } from '@/lib/api/pins';
import type { PinKind } from '@/types/api';

export function PinButton({
  kind,
  id,
  overlay = false,
}: {
  kind: PinKind;
  id: number;
  overlay?: boolean;
}) {
  const { pinned, toggle } = useTogglePin(kind, id);
  const label = pinned ? 'Unpin' : 'Pin';
  return (
    <Button
      type="button"
      variant="ghost"
      size="iconSm"
      aria-label={label}
      aria-pressed={pinned}
      title={label}
      disabled={toggle.isPending}
      onClick={() => toggle.mutate()}
      className={`hover:text-fg focus-visible:ring-3 focus-visible:ring-ring aria-pressed:text-fg ${overlay ? 'absolute right-2 bottom-1.5 opacity-0 group-hover:opacity-100 focus-visible:opacity-100' : ''}`}
    >
      <svg
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill={pinned ? 'currentColor' : 'none'}
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
      >
        <path d="M9 3h6l-1 6 4 4v2H6v-2l4-4zM12 15v6" />
      </svg>
    </Button>
  );
}

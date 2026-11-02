import { Link } from 'react-router';
import { connectionLabel } from '@/modules/teams/utils/connections';
import type { Connection } from '@/types/api';
import { Toggle } from '@/components/toggle';

type Props = {
  label: string;
  selected: number[];
  available: Connection[] | undefined;
  onChange: (connections: number[]) => void;
};

export function ConnectionPicker({ label, selected, available, onChange }: Props) {
  return (
    <div className="min-w-0 space-y-3">
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="text-[11px] uppercase tracking-[0.08em] text-muted">Connections</h3>
        <span className="text-xs text-muted">{selected.length} selected</span>
      </div>
      {available?.length === 0 && (
        <p className="text-xs text-muted">
          Add connections in{' '}
          <Link to="/settings?section=connections" className="underline">
            Settings
          </Link>
          .
        </p>
      )}
      <div>
        {available?.map((c) => (
          <div
            key={c.id}
            className="flex min-h-11 items-center gap-3 border-b border-hairline last:border-b-0"
          >
            <span className="min-w-0 flex-1">
              <span className="block truncate font-mono text-[13px]">{c.name}</span>
              <span className="block truncate text-xs text-muted">
                {c.url ? 'HTTP' : 'Local process'} · {connectionLabel(c)}
              </span>
            </span>
            <Toggle
              label={`${label} connection ${c.name}`}
              checked={selected.includes(c.id)}
              onChange={(on) =>
                onChange(on ? [...selected, c.id] : selected.filter((id) => id !== c.id))
              }
            />
          </div>
        ))}
      </div>
    </div>
  );
}

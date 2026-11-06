import { useState } from 'react';
import { Link } from 'react-router';
import { connectionLabel } from '@/modules/teams/utils/connections';
import type { Connection } from '@/types/api';
import { Button } from '@/components/button';
import { Toggle } from '@/components/toggle';
import { ChipList } from '@/modules/teams/components/chip-list';

type Props = {
  label: string;
  selected: number[];
  available: Connection[] | undefined;
  onChange: (connections: number[]) => void;
};

export function ConnectionPicker({ label, selected, available, onChange }: Props) {
  const [open, setOpen] = useState(false);
  const chosen = (available ?? []).filter((c) => selected.includes(c.id));

  return (
    <div className="min-w-0">
      <ChipList
        label="Connections"
        mono
        values={chosen.map((c) => c.name)}
        onRemove={(i) => onChange(selected.filter((id) => id !== chosen[i].id))}
      />
      <Button
        type="button"
        size="sm"
        aria-expanded={open}
        disabled={available?.length === 0}
        onClick={() => setOpen(!open)}
      >
        {open ? 'Done' : 'Add connection'}
      </Button>
      {available?.length === 0 && (
        <p className="mt-2 text-xs text-muted">
          Add connections in{' '}
          <Link to="/settings?section=connections" className="underline">
            Settings
          </Link>
          .
        </p>
      )}
      {open && (
        <div className="mt-2">
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
      )}
    </div>
  );
}

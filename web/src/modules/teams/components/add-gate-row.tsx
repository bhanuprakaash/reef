import { useState } from 'react';
import type { Gate } from '@/types/api';
import { Button } from '@/components/button';
import { Select } from '@/components/select';

type Props = { roles: string[]; gates: Gate[]; onAdd: (gate: Gate) => void };

export function AddGateRow({ roles, gates, onAdd }: Props) {
  const [pick, setPick] = useState({ from: '', to: '' });
  const from = roles.includes(pick.from) ? pick.from : roles[0];
  const to = roles.includes(pick.to) ? pick.to : roles[1];
  const options = roles.map((r) => ({ value: r, label: r }));
  const taken = gates.some((g) => g.from === from && g.to === to);

  if (roles.length < 2) return null;
  return (
    <div className="mt-3 flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <Select
          className="min-w-0 flex-1"
          aria-label="Gate from"
          value={from}
          options={options}
          onChange={(v) => setPick({ from: v, to })}
        />
        <span className="text-muted">→</span>
        <Select
          className="min-w-0 flex-1"
          aria-label="Gate to"
          value={to}
          options={options}
          onChange={(v) => setPick({ from, to: v })}
        />
      </div>
      <Button
        type="button"
        disabled={from === to || taken}
        onClick={() => onAdd({ from, to, on: true })}
      >
        Add gate
      </Button>
    </div>
  );
}

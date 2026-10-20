import type { Gate } from '@/types/api';

const defaultGates: Gate[] = [
  { from: 'lead', to: 'developer', on: true },
  { from: 'architect', to: 'developer', on: true },
  { from: 'developer', to: 'tester', on: true },
  { from: 'tester', to: 'developer', on: true },
];

export type GateRow = { gate: Gate; listed: number; isDefault: boolean };

export function gateRows(roles: string[], gates: Gate[]): GateRow[] {
  const isDefault = (g: Gate) => defaultGates.some((d) => d.from === g.from && d.to === g.to);
  const listed = gates.map((gate, i) => ({ gate, listed: i, isDefault: isDefault(gate) }));
  const unlisted = defaultGates
    .filter((d) => roles.includes(d.from) && roles.includes(d.to))
    .filter((d) => !gates.some((g) => g.from === d.from && g.to === d.to))
    .map((gate) => ({ gate, listed: -1, isDefault: true }));
  return [...listed, ...unlisted];
}

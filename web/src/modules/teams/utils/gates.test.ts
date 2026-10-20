import { describe, expect, it } from 'vitest';
import { gateRows } from '@/modules/teams/utils/gates';

const roles = ['lead', 'architect', 'developer', 'tester'];

describe('gateRows', () => {
  it('shows the default gates a team does not list, on', () => {
    expect(gateRows(roles, []).map((r) => [r.gate.from, r.gate.to, r.gate.on, r.listed])).toEqual([
      ['lead', 'developer', true, -1],
      ['architect', 'developer', true, -1],
      ['developer', 'tester', true, -1],
      ['tester', 'developer', true, -1],
    ]);
  });

  it('lets a listed gate replace its default, keeping its place in the list', () => {
    const rows = gateRows(roles, [{ from: 'lead', to: 'developer', on: false }]);
    expect(rows.filter((r) => r.gate.from === 'lead' && r.gate.to === 'developer')).toEqual([
      { gate: { from: 'lead', to: 'developer', on: false }, listed: 0, isDefault: true },
    ]);
    expect(rows).toHaveLength(4);
  });

  it('skips a default gate whose role the team does not have', () => {
    expect(gateRows(['lead', 'architect'], [])).toEqual([]);
  });

  it('lists a gate that is not a default edge as removable', () => {
    expect(gateRows(['lead', 'architect'], [{ from: 'lead', to: 'architect', on: true }])).toEqual([
      { gate: { from: 'lead', to: 'architect', on: true }, listed: 0, isDefault: false },
    ]);
  });
});

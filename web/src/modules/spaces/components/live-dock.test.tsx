import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { LiveDock } from './live-dock';

const { useFailedTurns, useRetryTurn, useRunningTurns } = vi.hoisted(() => ({
  useFailedTurns: vi.fn(),
  useRetryTurn: vi.fn(),
  useRunningTurns: vi.fn(),
}));

vi.mock('@/modules/spaces/api/get-failed-turns', () => ({ useFailedTurns }));
vi.mock('@/lib/api/retry-turn', () => ({ useRetryTurn }));
vi.mock('@/modules/spaces/api/get-turns', () => ({ useRunningTurns }));

const bash = {
  id: 't1',
  name: 'Bash',
  input: { command: 'npm test' },
  done: false,
  output: '',
  isError: false,
  children: [],
};

const dock = (turn: Parameters<typeof LiveDock>[0]['entries'][number]['turn'], role = 'lead') =>
  renderToStaticMarkup(
    <LiveDock spaceId={1} entries={[{ turn, role }]} paused={false} onOpenTurn={() => undefined} />,
  );

describe('LiveDock', () => {
  beforeEach(() => {
    useFailedTurns.mockReturnValue({ data: [] });
    useRetryTurn.mockReturnValue({ isPending: false, mutate: vi.fn() });
    useRunningTurns.mockReturnValue({ data: [] });
  });

  it('names the running tool in the pill', () => {
    const tools = ['Read', 'Edit'].map((name, i) => ({
      ...bash,
      id: `t${i}`,
      name,
      input: { command: `cmd-${name}` },
      done: name !== 'Edit',
    }));
    const markup = dock({
      status: 'running',
      items: tools.map((tool) => ({ type: 'tool' as const, tool })),
    });

    expect(markup).toContain('lead');
    expect(markup).toContain('is working');
    expect(markup).toContain('Editing');
  });

  it('counts the timer from the turn start the server holds', () => {
    useRunningTurns.mockReturnValue({
      data: [{ spaceId: 1, botId: null, startedAt: new Date(Date.now() - 125_000).toISOString() }],
    });

    expect(dock({ status: 'running', items: [] })).toContain('2m 5s');
  });

  it('floats bubbles off a working Bot, slower while it only thinks', () => {
    const tool = { type: 'tool' as const, tool: bash };

    expect(dock({ status: 'running', items: [tool] })).not.toContain('dock-bubbles slow');
    expect(dock({ status: 'running', items: [] })).toContain('dock-bubbles slow');
  });

  it('shows a failed turn with its error and a Retry for the Bot', () => {
    useFailedTurns.mockReturnValue({ data: [{ id: 7, botId: 3, role: 'lead' }] });

    const markup = dock({ status: 'failed', botId: 3, error: 'interrupted', items: [] });

    expect(markup).toContain('Failed: interrupted');
    expect(markup).toContain('Retry');
  });

  it('shows why a Retry from the pill failed', () => {
    useFailedTurns.mockReturnValue({ data: [{ id: 7, botId: 3, role: 'lead' }] });
    useRetryTurn.mockReturnValue({
      isPending: false,
      mutate: vi.fn(),
      isError: true,
      error: new Error('Space is paused'),
    });

    expect(dock({ status: 'failed', botId: 3, error: 'interrupted', items: [] })).toContain(
      'Space is paused',
    );
  });

  it('shows only that a direct turn is working', () => {
    const markup = dock(
      { status: 'running', directMessageId: 1, items: [{ type: 'tool', tool: bash }] },
      'tester',
    );

    expect(markup).toContain('is working');
    expect(markup).not.toContain('Running');
  });

  it('shows nothing while idle', () => {
    expect(dock({ status: 'idle', items: [] })).toBe('');
  });

  it('shows one tide row for three working Bots', () => {
    const turn = { status: 'running' as const, items: [] };
    const markup = renderToStaticMarkup(
      <LiveDock
        spaceId={1}
        entries={['lead', 'dev', 'tester'].map((role, i) => ({
          turn: { ...turn, botId: i },
          role,
        }))}
        paused={false}
        onOpenTurn={() => undefined}
      />,
    );

    expect(markup).toContain('3 working');
    expect(markup.match(/is working/g)).toHaveLength(1);
  });
});

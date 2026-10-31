import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { DirectMessage } from '@/types/api';
import { GuidanceInput } from './guidance-input';

const { useBotActivity } = vi.hoisted(() => ({ useBotActivity: vi.fn() }));

vi.mock('@/modules/spaces/api/get-bot-activity', () => ({ useBotActivity }));
vi.mock('@/modules/spaces/api/guide-bot', () => ({
  useGuideBot: () => ({ isPending: false, isError: false }),
}));
vi.mock('@/modules/spaces/api/stop-space', () => ({ useStopSpace: () => ({ isPending: false }) }));

const guidance = {
  id: 1,
  from: 'user',
  body: 'use sessions',
  kind: 'guidance',
  state: 'waiting',
} as DirectMessage;

function render(kind: string, paused: boolean, running = false) {
  useBotActivity.mockReturnValue({
    data: { label: 'Consultation from Prakash', space: { kind }, exchanges: [guidance] },
  });
  return renderToStaticMarkup(
    <GuidanceInput spaceId={2} botId={3} botName="Alex" paused={paused} running={running} />,
  );
}

describe('GuidanceInput', () => {
  it('names the Bot it guides and says the guidance is queued after the turn', () => {
    const html = render('consultation', false);
    expect(html).toContain('Guide Alex in Consultation from Prakash');
    expect(html).toContain('Queued — after the current turn');
    expect(html).not.toContain('Stop');
  });

  it('keeps the guidance waiting while the Space is paused, and Stop stays while it runs', () => {
    const html = render('work_item', true, true);
    expect(html).toContain('Message Consultation from Prakash');
    expect(html).toContain('Waiting for Space to resume');
    expect(html).toContain('Stop');
  });
});

import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';
import type { Role } from '@/types/api';
import { MentionCard } from './mention-hovercard';

const developer: Role = {
  botId: 1,
  name: 'developer',
  lead: false,
  write: true,
  backend: 'claude',
  permissionMode: 'acceptEdits',
  description: 'You are the developer.\nSecond line stays out.',
  skills: [],
  model: 'sonnet',
  tools: ['Read', 'Edit', 'Write'],
  mcp: [],
};

const text = (markup: string) =>
  markup
    .replace(/<[^>]+>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
const render = (role: Role, working: boolean) =>
  renderToStaticMarkup(
    <MemoryRouter>
      <MentionCard role={role} working={working} />
    </MemoryRouter>,
  );

describe('MentionCard', () => {
  it('shows the role, backend and model, first description line, state, access, counts and Edit', () => {
    const markup = render(developer, true);

    expect(text(markup)).toBe(
      'developer Claude Code · Sonnet Edit You are the developer. State Working Access Can write Tools 3',
    );
    expect(markup).toContain('href="/bots?bot=1"');
  });

  it('shows an idle read-only codex role without tools', () => {
    const markup = render({ ...developer, write: false, backend: 'codex', model: '' }, false);

    expect(text(markup)).toBe(
      'developer Codex · default model Edit You are the developer. State Idle Access Read-only',
    );
  });
});

import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { Tool } from '@/modules/spaces/utils/fold-events';
import { ToolRow } from './tool-row';

const read: Tool = {
  id: 'b1',
  name: 'Read',
  input: { file_path: 'a.go' },
  done: true,
  output: 'package a',
  isError: false,
  children: [],
};

const agent = (done: boolean): Tool => ({
  id: 'a1',
  name: 'Agent',
  input: { description: 'Comment pass', subagent_type: 'comment-sicko', prompt: 'Review' },
  done,
  output: done ? 'nothing removed' : '',
  isError: false,
  children: [read],
});

describe('ToolRow', () => {
  it('labels an Agent call by its type and groups its tools, open while it runs', () => {
    const markup = renderToStaticMarkup(<ToolRow tool={agent(false)} />);

    expect(markup).toContain('comment-sicko · Comment pass');
    expect(markup).toContain('<details open="" class="ml-1.5 mt-1.5">');
    expect(markup).toContain('sub-agent · 1 tool<');
  });

  it('collapses the group once the Agent call is done', () => {
    const markup = renderToStaticMarkup(<ToolRow tool={agent(true)} />);

    expect(markup).toContain('<details class="ml-1.5 mt-1.5">');
  });

  it('renders a tool without children as before', () => {
    const markup = renderToStaticMarkup(<ToolRow tool={read} />);

    expect(markup).toContain('a.go');
    expect(markup).not.toContain('sub-agent');
  });

  it('shows the project a tool call works in', () => {
    const targets = [{ projectId: 3, projectName: 'infra', workspaceId: 2 }];
    const markup = renderToStaticMarkup(<ToolRow tool={{ ...read, targets }} />);

    expect(markup).toContain('>infra<');
    expect(renderToStaticMarkup(<ToolRow tool={read} />)).not.toContain('>infra<');
  });
});

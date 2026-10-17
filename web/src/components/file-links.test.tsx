import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { FileLinkContext } from './file-links';
import { Markdown } from './markdown';

const text = [
  'See `web/src/app.tsx:42`, [the store](store/db.go) and server/http.go.',
  'Not v1.2.3, e.g. this, `npm test` or http://x.io/a.go.',
  '',
  '```',
  'cmd/reef/main.go',
  '```',
].join('\n');

function render(withContext: boolean) {
  const markdown = <Markdown>{text}</Markdown>;
  return renderToStaticMarkup(
    withContext ? (
      <FileLinkContext.Provider value={() => {}}>{markdown}</FileLinkContext.Provider>
    ) : (
      markdown
    ),
  );
}

describe('Markdown file links', () => {
  it('turns inline code, relative links and bare paths into buttons', () => {
    const buttons = render(true).match(/<button[^>]*title="Open[^>]*>.*?<\/button>/g) ?? [];

    expect(buttons).toHaveLength(3);
    expect(buttons[0]).toContain('title="Open web/src/app.tsx"');
    expect(buttons[0]).toContain('<code>web/src/app.tsx:42</code>');
    expect(buttons[1]).toContain('title="Open store/db.go"');
    expect(buttons[1]).toContain('>the store</button>');
    expect(buttons[2]).toContain('>server/http.go</button>');
  });

  it('leaves versions, commands, urls and fenced code alone', () => {
    const html = render(true);

    expect(html).toContain('<code>npm test</code>');
    expect(html).toContain('<a href="http://x.io/a.go"');
    expect(html).toContain('<code>cmd/reef/main.go</code>');
    expect(html).not.toContain('Open cmd/reef');
  });

  it('renders no buttons without a context', () => {
    const html = render(false);

    expect(html).not.toContain('title="Open');
    expect(html).toContain('<code>web/src/app.tsx:42</code>');
  });
});

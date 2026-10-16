import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { Markdown } from './markdown';

const text = [
  '**bold** and `inline`',
  '',
  '- one',
  '- two',
  '',
  '```go',
  'fmt.Println("hi")',
  '```',
  '',
  '| a | b |',
  '| - | - |',
  '| 1 | 2 |',
  '',
  '[docs](https://example.com)',
].join('\n');

describe('Markdown', () => {
  it('renders bold, list, inline code, fenced code, table and link', () => {
    const html = renderToStaticMarkup(<Markdown>{text}</Markdown>);

    expect(html).toContain('<strong>bold</strong>');
    expect(html).toContain('<code>inline</code>');
    expect(html).toContain('<li>two</li>');
    expect(html).toContain(
      '<pre><code class="language-go">fmt.Println(&quot;hi&quot;)\n</code></pre>',
    );
    expect(html).toContain(
      '<a href="https://example.com" target="_blank" rel="noreferrer">docs</a>',
    );
  });

  it('lets a wide table scroll and right-aligns cells that hold only a number', () => {
    const html = renderToStaticMarkup(<Markdown>{text}</Markdown>);

    expect(html).toMatch(/<div class="[^"]*overflow-x-auto[^"]*"><table>/);
    expect(html).toMatch(/<th class="text-left">a<\/th>/);
    expect(html).toMatch(/<td class="[^"]*text-right[^"]*">2<\/td>/);
  });

  it('keeps single newlines as line breaks and does not render raw html', () => {
    const html = renderToStaticMarkup(<Markdown>{'line one\nline two <b>x</b>'}</Markdown>);

    expect(html).toContain('line one<br/>\nline two');
    expect(html).not.toContain('<b>');
  });
});

import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { Markdown } from '@/components/markdown';

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
    expect(html).toContain('<code>fmt.Println(&quot;hi&quot;)</code>');
    expect(html).toContain('>go</span>');
    expect(html).toContain(
      '<a href="https://example.com" target="_blank" rel="noreferrer">docs</a>',
    );
  });

  it('lets a wide table scroll and right-aligns cells that hold only a number', () => {
    const html = renderToStaticMarkup(<Markdown>{text}</Markdown>);

    expect(html).toMatch(/<div class="[^"]*overflow-x-auto[^"]*"><table>/);
    expect(html).toMatch(/<th[^>]*class="text-right"[^>]*>a<\/th>/);
    expect(html).toMatch(/<td class="[^"]*text-right[^"]*">2<\/td>/);
  });

  it('left-aligns the header of a column that holds text', () => {
    const html = renderToStaticMarkup(<Markdown>{'| name | n |\n| - | - |\n| x | 1 |'}</Markdown>);

    expect(html).toMatch(/<th[^>]*class="text-left"[^>]*>name<\/th>/);
    expect(html).toMatch(/<th[^>]*class="text-right"[^>]*>n<\/th>/);
  });

  it('aligns every cell of a numeric column the same way, formatted or not', () => {
    const html = renderToStaticMarkup(<Markdown>{'| n |\n| - |\n| **5** |\n| -3 |'}</Markdown>);

    expect(html).toMatch(/<th[^>]*class="text-right"[^>]*>n<\/th>/);
    expect(html).toContain('<td class="text-right tabular-nums"><strong>5</strong></td>');
    expect(html).toContain('<td class="text-right tabular-nums">-3</td>');
  });

  it('keeps single newlines as line breaks and does not render raw html', () => {
    const html = renderToStaticMarkup(<Markdown>{'line one\nline two <b>x</b>'}</Markdown>);

    expect(html).toContain('line one<br/>\nline two');
    expect(html).not.toContain('<b>');
  });
});

describe('Markdown rich fences', () => {
  it('holds a placeholder, not a note, for a chart that does not parse yet', () => {
    for (const body of ['{"type":"bar","labels":["a"', '{"type":"radar"}']) {
      const html = renderToStaticMarkup(<Markdown>{'```chart\n' + body + '\n```'}</Markdown>);

      expect(html).toContain('aria-hidden="true"');
      expect(html).not.toContain('Not a valid chart');
      expect(html).not.toContain('<canvas');
    }
  });

  it('draws a valid chart on a labelled canvas with a hidden data table', () => {
    const html = renderToStaticMarkup(
      <Markdown>
        {'```chart\n{"type":"line","title":"Load","labels":["a"],"datasets":[{"data":[4]}]}\n```'}
      </Markdown>,
    );

    expect(html).toContain('<canvas role="img" aria-label="Load"></canvas>');
    expect(html).toContain('<tr><th>a</th><td>4</td></tr>');
  });

  it('holds a placeholder for a mermaid diagram until it is drawn', () => {
    const html = renderToStaticMarkup(<Markdown>{'```mermaid\ngraph TD\n```'}</Markdown>);

    expect(html).toContain('aria-hidden="true"');
    expect(html).not.toContain('<pre');
  });

  it('previews html in an iframe with an empty sandbox and a policy first, never inline', () => {
    const html = renderToStaticMarkup(
      <Markdown>{'```html\n<b>x</b><script>alert(1)</script>\n```'}</Markdown>,
    );

    expect(html).toContain(
      '<iframe sandbox="" srcDoc="&lt;meta http-equiv=&quot;Content-Security-Policy&quot; content=&quot;default-src &#x27;none&#x27;; img-src data:; style-src &#x27;unsafe-inline&#x27;&quot;&gt;&lt;b&gt;x&lt;/b&gt;&lt;script&gt;alert(1)&lt;/script&gt;"',
    );
    expect(html).not.toContain('<b>');
    expect(html).not.toContain('<script>');
  });
});

describe('Markdown mentions', () => {
  it('wraps each @mention in a mention span, but not emails, code or links', () => {
    const html = renderToStaticMarkup(
      <Markdown>
        {'@architect and **@dev-1** mail me@x.io `@code` [@l](https://e.co)\n@tester'}
      </Markdown>,
    );

    expect(html).toContain('<span class="mention">@architect</span> and');
    expect(html).toContain('<strong><span class="mention">@dev-1</span></strong>');
    expect(html).toContain('<br/>\n<span class="mention">@tester</span>');
    expect(html).toContain('me@x.io');
    expect(html).toContain('<code>@code</code>');
    expect(html).not.toContain('mention">@l');
    expect(html).not.toContain('mention">@x');
  });
});

import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { MemoryRouter } from 'react-router';
import { Markdown, MentionCards } from './markdown';

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

describe('Markdown mentions', () => {
  it('wraps an @mention that starts a line in a mention span, but not emails, code or links', () => {
    const html = renderToStaticMarkup(
      <Markdown>
        {
          '@architect, @dev-1 do it, @late **@bold** mail me@x.io `@code` [@l](https://e.co)\n@tester'
        }
      </Markdown>,
    );

    expect(html).toContain(
      '<span class="mention">@architect</span>, <span class="mention">@dev-1</span> do it',
    );
    expect(html).toContain('<br/>\n<span class="mention">@tester</span>');
    expect(html).not.toContain('mention">@late');
    expect(html).not.toContain('mention">@bold');
    expect(html).toContain('me@x.io');
    expect(html).toContain('<code>@code</code>');
    expect(html).not.toContain('mention">@l');
    expect(html).not.toContain('mention">@x');
  });

  it('does not highlight an @name after other text or an inline node', () => {
    const html = renderToStaticMarkup(
      <Markdown>{'looks good, @developer please fix\n\n**done** @tester\n\n- @lead'}</Markdown>,
    );

    expect(html).not.toContain('class="mention"');
  });
});

describe('Markdown mention cards', () => {
  it('shows a known Bot as its creature and capitalised name without the @, and leaves other mentions plain', () => {
    const html = renderToStaticMarkup(
      <MemoryRouter>
        <MentionCards.Provider
          value={(name) =>
            name === 'architect'
              ? { look: { face: 'whale', colour: 'violet' }, card: <p>card</p> }
              : null
          }
        >
          <Markdown>{'@architect, @nobody'}</Markdown>
        </MentionCards.Provider>
      </MemoryRouter>,
    );

    expect(html).toMatch(
      /<span class="mention [^"]*" style="[^"]*#8B5CF6[^"]*" tabindex="0"><span[^>]*><svg[^>]*>.*<\/svg><\/span>Architect<\/span>/,
    );
    expect(html).not.toContain('@architect');
    expect(html).toContain('<span class="mention">@nobody</span>');
  });
});

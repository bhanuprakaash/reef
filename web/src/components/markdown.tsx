import { isValidElement, useContext, type ReactNode } from 'react';
import type { Parent, PhrasingContent, Root } from 'mdast';
import type { Components } from 'react-markdown';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { ChartBlock } from '@/components/chart-block';
import { CodeBlock } from '@/components/code-block';
import { DataTable } from '@/components/data-table';
import { HtmlBlock } from '@/components/html-block';
import { FileLink, FileLinkContext, remarkFileLinks } from '@/components/file-links';
import { splitMentions } from '@/utils/split-mentions';

function mentions(nodes: PhrasingContent[]): PhrasingContent[] {
  return nodes.flatMap((node): PhrasingContent[] => {
    if (node.type === 'text') {
      const pieces = splitMentions(node.value);
      if (!pieces.some((piece) => piece.mention)) return [node];
      return pieces.map((piece): PhrasingContent =>
        piece.mention
          ? {
              type: 'text',
              value: piece.value,
              data: { hName: 'span', hProperties: { className: ['mention'] } },
            }
          : { type: 'text', value: piece.value },
      );
    }
    if ('children' in node && node.type !== 'link') {
      (node as Parent).children = mentions((node as Parent).children as PhrasingContent[]);
    }
    return [node];
  });
}

// A single newline in a message is a line break, as in a chat.
function remarkBreaks() {
  return (tree: Root) => {
    const walk = (parent: Parent) => {
      parent.children = parent.children.flatMap((child): typeof parent.children => {
        if (child.type !== 'text') {
          if ('children' in child) walk(child as Parent);
          return [child];
        }
        const lines = child.value.split(/[\t ]*\r?\n[\t ]*/);
        if (lines.length === 1) return [child];
        return lines.flatMap((line, i): typeof parent.children => [
          ...(i > 0 ? [{ type: 'break' as const }] : []),
          ...(line ? [{ type: 'text' as const, value: line }] : []),
        ]);
      });
    };
    walk(tree);
  };
}

function remarkMentions() {
  return (tree: Root) => {
    const walk = (parent: Parent) => {
      parent.children = parent.children.flatMap((child) => {
        if (child.type === 'paragraph' || child.type === 'heading' || child.type === 'tableCell') {
          child.children = mentions(child.children);
        } else if ('children' in child) {
          walk(child as Parent);
        }
        return [child];
      });
    };
    walk(tree);
  };
}

const plugins = [remarkGfm, remarkBreaks, remarkMentions];
const filePlugins = [...plugins, remarkFileLinks];

const style = [
  'flex flex-col gap-[0.7em] break-words leading-[1.62]',

  // Headings need a real scale and air above, or a long reply reads as one wall.
  '[&_h1]:mt-[0.6em] [&_h1]:text-[19px] [&_h1]:font-semibold [&_h1]:leading-snug',
  '[&_h2]:mt-[0.7em] [&_h2]:text-[16.5px] [&_h2]:font-semibold [&_h2]:leading-snug',
  '[&_h3]:mt-[0.6em] [&_h3]:text-[15px] [&_h3]:font-semibold [&_h3]:leading-snug',
  '[&_h4]:mt-[0.5em] [&_h4]:text-[14.5px] [&_h4]:font-medium [&_h4]:text-soft',
  '[&_h5]:mt-[0.5em] [&_h5]:text-[14.5px] [&_h5]:font-medium [&_h5]:text-soft',
  '[&_h6]:mt-[0.5em] [&_h6]:text-[14.5px] [&_h6]:font-medium [&_h6]:text-muted',

  '[&>:is(p,ul,ol,blockquote,h1,h2,h3,h4,h5,h6)]:max-w-[80ch]',

  '[&_strong]:font-semibold',
  '[&_.mention]:text-[color:var(--mention,var(--color-mention))]',
  '[&_p]:my-0 [&_li>p]:my-0',

  '[&_ul]:list-disc [&_ol]:list-decimal [&_ul]:pl-5 [&_ol]:pl-5',
  '[&_li]:my-[0.2em] [&_li]:pl-0.5 [&_li::marker]:text-muted',
  '[&_ul_ul]:my-0.5 [&_ol_ol]:my-0.5',
  '[&_ul.contains-task-list]:list-none [&_ul.contains-task-list]:pl-1',
  '[&_li>input]:mr-1.5 [&_li>input]:align-middle',

  '[&_a]:text-inherit [&_a]:underline [&_a]:decoration-current/40 [&_a]:underline-offset-2 [&_a:hover]:decoration-current',

  '[&_blockquote]:border-l-2 [&_blockquote]:border-current/25 [&_blockquote]:pl-3.5 [&_blockquote]:text-soft',
  '[&_hr]:my-1 [&_hr]:border-current/15',

  '[&_:not(pre)>code]:rounded-md [&_:not(pre)>code]:bg-current/[0.09] [&_:not(pre)>code]:px-1.5 [&_:not(pre)>code]:py-0.5 [&_:not(pre)>code]:font-mono [&_:not(pre)>code]:text-[0.86em]',

  // Tables: row rules and a tinted header, not a four-sided grid on every cell.
  '[&_table]:w-full [&_table]:border-collapse [&_table]:text-[13px]',
  '[&_th]:cursor-pointer [&_th]:select-none [&_th:hover]:text-fg [&_th[aria-sort=ascending]]:after:content-["_↑"] [&_th[aria-sort=descending]]:after:content-["_↓"]',
  '[&_th]:whitespace-nowrap [&_th]:border-b [&_th]:border-current/15 [&_th]:bg-current/[0.06] [&_th]:px-3 [&_th]:py-1.5 [&_th]:text-[11.5px] [&_th]:font-medium [&_th]:uppercase [&_th]:tracking-[0.06em] [&_th]:text-muted',
  '[&_td]:border-t [&_td]:border-current/10 [&_td]:px-3 [&_td]:py-1.5 [&_td]:align-top',
  '[&_tbody_tr:nth-child(even)]:bg-current/[0.025]',

  '[&_img]:max-w-full [&_img]:rounded-lg',
].join(' ');

const components: Components = {
  a: ({ href, children }) => (
    <a href={href} target="_blank" rel="noreferrer">
      {children}
    </a>
  ),
  pre: ({ children }) => {
    const code = isValidElement<{ className?: string; children?: ReactNode }>(children)
      ? children
      : null;
    const lang = /language-([\w+-]+)/.exec(code?.props.className ?? '')?.[1] ?? '';
    const body = String(code?.props.children ?? '').replace(/\n$/, '');
    if (lang === 'chart') return <ChartBlock code={body} />;
    if (lang === 'html') return <HtmlBlock code={body} />;
    return <CodeBlock code={body} lang={lang} />;
  },
  button: FileLink,
  table: DataTable,
  th: ({ children, ...props }) => (
    <th
      tabIndex={0}
      aria-sort={props['aria-sort']}
      className={
        (props as { 'data-numeric'?: boolean })['data-numeric'] ? 'text-right' : 'text-left'
      }
      onKeyDown={(e) => {
        if (e.target !== e.currentTarget || (e.key !== 'Enter' && e.key !== ' ')) return;
        e.preventDefault();
        e.currentTarget.click();
      }}
    >
      {children}
    </th>
  ),
  td: ({ children, ...props }) => (
    <td
      className={
        (props as { 'data-numeric'?: boolean })['data-numeric']
          ? 'text-right tabular-nums'
          : undefined
      }
    >
      {children}
    </td>
  ),
};

export function Markdown({ children }: { children: string }) {
  const openFile = useContext(FileLinkContext);
  return (
    <div className={style}>
      <ReactMarkdown remarkPlugins={openFile ? filePlugins : plugins} components={components}>
        {children}
      </ReactMarkdown>
    </div>
  );
}

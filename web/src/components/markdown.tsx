import type { ReactNode } from 'react';
import type { Components } from 'react-markdown';
import ReactMarkdown from 'react-markdown';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';

const plugins = [remarkGfm, remarkBreaks];

const style = [
  'flex flex-col gap-[0.7em] break-words leading-[1.62]',

  // Headings need a real scale and air above, or a long reply reads as one wall.
  '[&_h1]:mt-[0.6em] [&_h1]:text-[19px] [&_h1]:font-semibold [&_h1]:leading-snug',
  '[&_h2]:mt-[0.7em] [&_h2]:text-[16.5px] [&_h2]:font-semibold [&_h2]:leading-snug',
  '[&_h3]:mt-[0.6em] [&_h3]:text-[15px] [&_h3]:font-semibold [&_h3]:leading-snug',
  '[&_h4]:mt-[0.5em] [&_h4]:text-[14.5px] [&_h4]:font-medium [&_h4]:text-soft',
  '[&_h5]:mt-[0.5em] [&_h5]:text-[14.5px] [&_h5]:font-medium [&_h5]:text-soft',
  '[&_h6]:mt-[0.5em] [&_h6]:text-[14.5px] [&_h6]:font-medium [&_h6]:text-muted',

  '[&_strong]:font-semibold',
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
  '[&_pre]:overflow-x-auto [&_pre]:rounded-lg [&_pre]:border [&_pre]:border-current/10 [&_pre]:bg-current/[0.07] [&_pre]:px-3.5 [&_pre]:py-3 [&_pre]:font-mono [&_pre]:text-[12.5px] [&_pre]:leading-[1.55]',

  // Tables: row rules and a tinted header, not a four-sided grid on every cell.
  '[&_table]:w-full [&_table]:border-collapse [&_table]:text-[13px]',
  '[&_th]:whitespace-nowrap [&_th]:border-b [&_th]:border-current/15 [&_th]:bg-current/[0.06] [&_th]:px-3 [&_th]:py-1.5 [&_th]:text-[11.5px] [&_th]:font-medium [&_th]:uppercase [&_th]:tracking-[0.06em] [&_th]:text-muted',
  '[&_td]:border-t [&_td]:border-current/10 [&_td]:px-3 [&_td]:py-1.5 [&_td]:align-top',
  '[&_tbody_tr:nth-child(even)]:bg-current/[0.025]',

  '[&_img]:max-w-full [&_img]:rounded-lg',
].join(' ');

// A cell holding only a number reads better right-aligned, with figures that line up.
const number = /^[\s$€£+-]*\d[\d,]*(?:\.\d+)?\s*(?:%|ms|s|x|k|m|kb|mb|gb)?$/i;

function numeric(children: ReactNode) {
  return typeof children === 'string' && number.test(children);
}

const components: Components = {
  a: ({ href, children }) => (
    <a href={href} target="_blank" rel="noreferrer">
      {children}
    </a>
  ),
  table: ({ children }) => (
    <div className="my-0.5 overflow-x-auto rounded-lg border border-current/15">
      <table>{children}</table>
    </div>
  ),
  th: ({ children }) => (
    <th className={numeric(children) ? 'text-right' : 'text-left'}>{children}</th>
  ),
  td: ({ children }) => (
    <td className={numeric(children) ? 'text-right tabular-nums' : undefined}>{children}</td>
  ),
};

export function Markdown({ children }: { children: string }) {
  return (
    <div className={style}>
      <ReactMarkdown remarkPlugins={plugins} components={components}>
        {children}
      </ReactMarkdown>
    </div>
  );
}

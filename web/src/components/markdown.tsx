import type { Components } from 'react-markdown';
import ReactMarkdown from 'react-markdown';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';

const plugins = [remarkGfm, remarkBreaks];

const style = [
  'flex flex-col gap-[0.5em] break-words',
  '[&_h1]:text-base [&_h2]:text-base [&_h3]:text-[15px] [&_h4]:text-[15px] [&_h5]:text-[15px] [&_h6]:text-[15px]',
  '[&_h1]:font-medium [&_h2]:font-medium [&_h3]:font-medium [&_h4]:font-medium [&_h5]:font-medium [&_h6]:font-medium',
  '[&_strong]:font-semibold',
  '[&_ul]:list-disc [&_ol]:list-decimal [&_ul]:pl-5 [&_ol]:pl-5 [&_li]:my-[0.15em]',
  '[&_ul.contains-task-list]:list-none [&_ul.contains-task-list]:pl-0 [&_li>input]:mr-1.5',
  '[&_a]:text-inherit [&_a]:underline [&_a]:underline-offset-2',
  '[&_blockquote]:border-l-2 [&_blockquote]:border-current/25 [&_blockquote]:pl-3 [&_blockquote]:opacity-75',
  '[&_hr]:border-current/20',
  '[&_:not(pre)>code]:rounded [&_:not(pre)>code]:bg-current/10 [&_:not(pre)>code]:px-1 [&_:not(pre)>code]:font-mono [&_:not(pre)>code]:text-[0.9em]',
  '[&_pre]:overflow-x-auto [&_pre]:rounded-xl [&_pre]:bg-current/10 [&_pre]:px-3 [&_pre]:py-2.5 [&_pre]:font-mono [&_pre]:text-[12.5px] [&_pre]:leading-normal',
  '[&_table]:border-collapse [&_table]:text-[13px] [&_th]:border [&_td]:border [&_th]:border-current/20 [&_td]:border-current/20 [&_th]:px-2.5 [&_td]:px-2.5 [&_th]:py-1 [&_td]:py-1 [&_th]:text-left [&_th]:font-medium',
].join(' ');

const components: Components = {
  a: ({ href, children }) => (
    <a href={href} target="_blank" rel="noreferrer">
      {children}
    </a>
  ),
  table: ({ children }) => (
    <div className="overflow-x-auto">
      <table>{children}</table>
    </div>
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

import { useLayoutEffect, useRef, useState } from 'react';
import { useSendMessage } from '@/modules/channels/api/send-message';
import { useStopChannel } from '@/modules/channels/api/stop-channel';
import { useMessages } from '@/lib/api/get-messages';
import { useDraft } from '@/modules/channels/hooks/use-draft';
import { stepHistory } from '@/modules/channels/utils/history-step';
import { splitMentions } from '@/utils/split-mentions';
import { findMention, insertMention } from '@/modules/channels/utils/mention';
import { Button } from '@/components/button';
import { Creature, type CreatureState } from '@/components/creature';
import { Menu } from '@/components/menu';

type Props = {
  channelId: number;
  running: boolean;
  paused: boolean;
  parent?: number;
  reason?: string;
  roles?: string[];
  faces?: Record<string, string | undefined>;
  working?: string;
};

const icon = {
  width: 18,
  height: 18,
  viewBox: '0 0 24 24',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 2,
  strokeLinecap: 'round',
  strokeLinejoin: 'round',
  'aria-hidden': true,
} as const;

export function Composer({
  channelId,
  running,
  paused,
  parent,
  reason,
  roles,
  faces,
  working,
}: Props) {
  const [text, setText] = useDraft(`reef.draft.${channelId}.${parent ?? 'main'}`);
  const messages = useMessages(channelId);
  const recalled = useRef<number | null>(null);
  const [caret, setCaret] = useState(0);
  const [active, setActive] = useState(0);
  const [closed, setClosed] = useState(false);
  const field = useRef<HTMLTextAreaElement>(null);
  const mirror = useRef<HTMLDivElement>(null);
  const picked = useRef<number | null>(null);
  const send = useSendMessage(channelId, parent);
  const stop = useStopChannel(channelId);

  const mention = closed || paused ? null : findMention(text, caret);
  const matches = mention
    ? (roles ?? []).filter((name) => name.toLowerCase().startsWith(mention.query.toLowerCase()))
    : [];
  const open = mention !== null && matches.length > 0;
  const highlighted = Math.min(active, matches.length - 1);

  const history = (messages.data ?? [])
    .filter((m) => m.author === 'user' && (m.parentId ?? undefined) === parent)
    .map((m) => m.body)
    .reverse();

  useLayoutEffect(() => {
    if (picked.current === null) return;
    field.current?.setSelectionRange(picked.current, picked.current);
    picked.current = null;
  }, [text]);

  function submit() {
    const trimmed = text.trim();
    if (trimmed === '' || running || paused || send.isPending) return;
    send.mutate(trimmed, { onSuccess: () => setText('') });
  }

  function pick(name: string) {
    if (!mention) return;
    const next = insertMention(text, caret, mention, name);
    setText(next.text);
    setCaret(next.caret);
    picked.current = next.caret;
  }

  const state = (name: string): CreatureState =>
    !running ? 'idle' : name === working ? 'working' : 'asleep';
  const placeholder = paused
    ? reason
      ? `Paused: ${reason}`
      : 'Paused'
    : running
      ? `${working ? working[0].toUpperCase() + working.slice(1) : 'Agent'} is working…`
      : parent !== undefined
        ? 'Reply in thread'
        : roles
          ? 'Message the team, @ to mention'
          : 'Message';

  return (
    <form
      className="relative space-y-2"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      {send.isError && !paused && <p className="text-sm text-danger">{send.error.message}</p>}
      {open && (
        <Menu
          role="listbox"
          aria-label="Mention"
          className="absolute bottom-full left-4 z-10 mb-2 w-[260px]"
        >
          {matches.map((name, i) => (
            <div
              key={name}
              role="option"
              aria-selected={i === highlighted}
              tabIndex={-1}
              className={`flex h-[38px] cursor-pointer items-center gap-2.5 rounded-lg px-2 ${i === highlighted ? 'bg-hover' : ''}`}
              onMouseDown={(e) => {
                e.preventDefault();
                pick(name);
              }}
            >
              <Creature role={name} face={faces?.[name]} size={22} state={state(name)} />
              {name}
              <span className="ml-auto text-xs text-muted">{state(name).replace('-', ' ')}</span>
            </div>
          ))}
        </Menu>
      )}
      <div className="flex min-h-14 items-center gap-2.5 rounded-pill border border-hover bg-field p-1.5">
        <Button type="button" size="icon" aria-label="Attach" disabled className="shrink-0">
          <svg {...icon} strokeWidth={1.7}>
            <path d="M12 5v14M5 12h14" />
          </svg>
        </Button>
        <div className="grid min-w-0 flex-1">
          <div
            ref={mirror}
            aria-hidden
            className={`col-start-1 row-start-1 max-h-40 min-w-0 overflow-hidden whitespace-pre-wrap break-words text-[15.5px] ${paused ? 'text-faint' : 'text-fg'}`}
          >
            {splitMentions(paused ? '' : text).map((piece, i) =>
              piece.mention ? (
                <span key={i} className="text-mention">
                  {piece.value}
                </span>
              ) : (
                piece.value
              ),
            )}
            {'\u200b'}
          </div>
          <textarea
            ref={field}
            rows={1}
            className="field-sizing-content col-start-1 row-start-1 max-h-40 w-full min-w-0 resize-none bg-transparent text-[15.5px] text-transparent caret-fg outline-none placeholder:text-muted disabled:cursor-not-allowed disabled:text-transparent"
            placeholder={placeholder}
            aria-label={parent === undefined ? 'Message' : 'Reply'}
            disabled={paused}
            value={paused ? '' : text}
            onChange={(e) => {
              setText(e.target.value);
              setCaret(e.target.selectionStart);
              setActive(0);
              setClosed(false);
            }}
            onScroll={(e) => {
              if (mirror.current) mirror.current.scrollTop = e.currentTarget.scrollTop;
            }}
            onSelect={(e) => setCaret(e.currentTarget.selectionStart)}
            onKeyDown={(e) => {
              if (e.nativeEvent.isComposing) return;
              if (open) {
                if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
                  e.preventDefault();
                  const step = e.key === 'ArrowDown' ? 1 : -1;
                  setActive((highlighted + step + matches.length) % matches.length);
                  return;
                }
                if (e.key === 'Enter' || e.key === 'Tab') {
                  e.preventDefault();
                  pick(matches[highlighted]);
                  return;
                }
                if (e.key === 'Escape') {
                  e.preventDefault();
                  setClosed(true);
                  return;
                }
              }
              if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
                const step = stepHistory(history, recalled.current, text, e.key);
                if (step) {
                  e.preventDefault();
                  recalled.current = step.index;
                  setText(step.text);
                  setCaret(step.text.length);
                  picked.current = step.text.length;
                  return;
                }
              }
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                submit();
              }
            }}
          />
        </div>
        {running ? (
          <Button
            variant="primary"
            type="button"
            size="icon"
            aria-label="Stop"
            className="shrink-0"
            disabled={stop.isPending}
            onClick={() => stop.mutate()}
          >
            <svg {...icon} width={14} height={14}>
              <rect x="5" y="5" width="14" height="14" rx="3" fill="currentColor" stroke="none" />
            </svg>
          </Button>
        ) : (
          <Button
            variant="primary"
            type="submit"
            size="icon"
            aria-label="Send"
            className="shrink-0"
            disabled={text.trim() === '' || paused || send.isPending}
          >
            <svg {...icon} width={16} height={16} strokeWidth={2.2}>
              <path d="M12 19V5M5 12l7-7 7 7" />
            </svg>
          </Button>
        )}
      </div>
    </form>
  );
}

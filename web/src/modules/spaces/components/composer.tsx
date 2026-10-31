import { useEffect, useLayoutEffect, useRef, useState, type DragEvent } from 'react';
import { useSendMessage } from '@/modules/spaces/api/send-message';
import { useStopSpace } from '@/modules/spaces/api/stop-space';
import { useMessages } from '@/lib/api/get-messages';
import { useMediaQuery } from '@/lib/use-media-query';
import { useDraft } from '@/modules/spaces/hooks/use-draft';
import { stepHistory } from '@/modules/spaces/utils/history-step';
import { splitMentions } from '@/utils/split-mentions';
import { findMention, insertMention } from '@/modules/spaces/utils/mention';
import { insertFiles, withPaths } from '@/modules/spaces/utils/file-path';
import { ClipboardChip } from '@/modules/spaces/components/clipboard-chip';
import { Button } from '@/components/button';
import { Creature, faceOf, type CreatureState, type Faces } from '@/components/creature';
import type { Role } from '@/types/api';
import { Menu } from '@/components/menu';

type Props = {
  spaceId: number;
  running: boolean;
  paused: boolean;
  parent?: number;
  reason?: string;
  roles?: Role[];
  faces?: Faces;
  working?: string;
  workingBotId?: number;
};

type DropEvent = {
  preventDefault: () => void;
  dataTransfer: { files: Iterable<File> & { length: number } };
};

export function filesFromDrop(event: DropEvent, paused: boolean) {
  if (paused || event.dataTransfer.files.length === 0) return [];
  event.preventDefault();
  return [...event.dataTransfer.files];
}

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
  spaceId,
  running,
  paused,
  parent,
  reason,
  roles,
  faces,
  working,
  workingBotId,
}: Props) {
  const [text, setText] = useDraft(`reef.draft.${spaceId}.${parent ?? 'main'}`);
  const messages = useMessages(spaceId);
  const recalled = useRef<number | null>(null);
  const [caret, setCaret] = useState(0);
  const [active, setActive] = useState(0);
  const [closed, setClosed] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [clip, setClip] = useState<string | null>(null);
  const [nameOnly, setNameOnly] = useState(false);
  const field = useRef<HTMLTextAreaElement>(null);
  const mirror = useRef<HTMLDivElement>(null);
  const picked = useRef<number | null>(null);
  const send = useSendMessage(spaceId, parent);
  const stop = useStopSpace(spaceId);

  const mention = closed || paused ? null : findMention(text, caret);
  const matches = mention
    ? (roles ?? []).filter((r) => r.name.toLowerCase().startsWith(mention.query.toLowerCase()))
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

  useEffect(() => {
    const reef = window.reef;
    if (!reef || parent !== undefined) return;
    const check = () =>
      void reef
        .clipboardFile()
        .then(setClip)
        .catch(() => setClip(null));
    check();
    window.addEventListener('focus', check);
    return () => window.removeEventListener('focus', check);
  }, [parent]);

  function attach(files: { path: string; name: string }[]) {
    const next = insertFiles(text, caret, files);
    setText(next.text);
    setCaret(next.caret);
    setNameOnly(next.nameOnly);
    picked.current = next.caret;
    field.current?.focus();
  }

  function dropFiles(e: DragEvent) {
    setDragging(false);
    const files = filesFromDrop(e, paused);
    if (files.length === 0) return;
    attach(withPaths(files, window.reef));
  }

  function hasFiles(e: DragEvent) {
    return !paused && e.dataTransfer.types.includes('Files');
  }

  function submit() {
    const trimmed = text.trim();
    if (trimmed === '' || paused || send.isPending) return;
    send.mutate(trimmed, {
      onSuccess: () => {
        setText('');
        setNameOnly(false);
      },
    });
  }

  function pick(name: string) {
    if (!mention) return;
    const next = insertMention(text, caret, mention, name);
    setText(next.text);
    setCaret(next.caret);
    picked.current = next.caret;
  }

  const narrow = useMediaQuery('(max-width: 767px)');
  const state = (botId: number): CreatureState =>
    !running ? 'idle' : botId === workingBotId ? 'working' : 'asleep';
  const placeholder = paused
    ? reason
      ? `Paused: ${reason}`
      : 'Paused'
    : running
      ? `${working ? working[0].toUpperCase() + working.slice(1) : 'Agent'} is working… your message runs next`
      : parent !== undefined
        ? 'Reply in thread'
        : roles
          ? narrow
            ? 'Message, @ to mention'
            : 'Message the Space, @ to mention'
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
          {matches.map((role, i) => (
            <div
              key={role.botId}
              role="option"
              aria-selected={i === highlighted}
              tabIndex={-1}
              className={`flex h-[38px] cursor-pointer items-center gap-2.5 rounded-lg px-2 ${i === highlighted ? 'bg-hover' : ''}`}
              onMouseDown={(e) => {
                e.preventDefault();
                pick(role.name);
              }}
            >
              <Creature look={faceOf(faces, role.botId)} size={22} state={state(role.botId)} />
              {role.name}
              <span className="ml-auto text-xs text-muted">
                {state(role.botId).replace('-', ' ')}
              </span>
            </div>
          ))}
        </Menu>
      )}
      {clip && !paused && (
        <div className="px-1.5">
          <ClipboardChip
            path={clip}
            onPaste={() => {
              attach([{ path: clip, name: clip }]);
              setClip(null);
            }}
          />
        </div>
      )}
      <div
        className={`flex min-h-14 items-end gap-2.5 rounded-composer border bg-field p-1.5 ${dragging ? 'border-dashed border-mention' : 'border-hover'}`}
        onDragEnter={(e) => {
          if (!hasFiles(e)) return;
          e.preventDefault();
          setDragging(true);
        }}
        onDragOver={(e) => {
          if (!hasFiles(e)) return;
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setDragging(false);
        }}
        onDrop={dropFiles}
      >
        <Button type="button" size="icon" aria-label="Attach" disabled className="shrink-0">
          <svg {...icon} strokeWidth={1.7}>
            <path d="M12 5v14M5 12h14" />
          </svg>
        </Button>
        <div className="grid min-w-0 flex-1 py-2.5 md:py-2">
          <div
            ref={mirror}
            aria-hidden
            className={`col-start-1 row-start-1 max-h-40 min-w-0 overflow-hidden whitespace-pre-wrap break-words [scrollbar-gutter:stable] text-[15.5px] ${paused ? 'text-faint' : 'text-fg'}`}
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
            className="field-sizing-content col-start-1 row-start-1 max-h-40 w-full [scrollbar-gutter:stable] min-w-0 resize-none bg-transparent text-[15.5px] text-transparent caret-fg outline-none placeholder:overflow-hidden placeholder:text-ellipsis placeholder:whitespace-nowrap placeholder:text-muted disabled:cursor-not-allowed disabled:text-transparent"
            placeholder={dragging ? 'Drop to add the file path' : placeholder}
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
            onPaste={(e) => {
              if (e.clipboardData.files.length === 0) return;
              e.preventDefault();
              attach(withPaths([...e.clipboardData.files], window.reef));
            }}
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
                  pick(matches[highlighted].name);
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
        {running && (
          <Button
            variant="secondary"
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
        )}
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
      </div>
      {nameOnly && (
        <p className="px-4 text-xs text-muted">No file path here: only the file name was added.</p>
      )}
    </form>
  );
}

import { useState } from 'react';
import { Link } from 'react-router';
import { Button, buttonClass } from '@/components/button';
import { Creature, faceOf } from '@/components/creature';
import { Markdown } from '@/components/markdown';
import { useContinueSpace } from '@/lib/api/continue-space';
import { useRetryTurn } from '@/lib/api/retry-turn';
import { useBots } from '@/lib/api/get-bots';
import { useDismissItem } from '@/modules/inbox/api/dismiss-item';
import { age } from '@/modules/inbox/utils/age';
import { kindLabels } from '@/modules/inbox/utils/new-items';
import { preview } from '@/modules/inbox/utils/preview';
import type { SpacePreview, InboxItem } from '@/types/api';
import { itemPath } from '@/utils/anchors';
import { memberFaces } from '@/utils/role-faces';

const kindColors: Record<InboxItem['kind'], string> = {
  paused: 'text-paused',
  held: 'text-paused',
  failed: 'text-failed',
  review: 'text-paused',
  question: 'text-working',
  reply: 'text-muted',
  sub_item: 'text-muted',
  proposed: 'text-muted',
  started: 'text-muted',
  delivery: 'text-failed',
  space_proposed: 'text-working',
};

type Props = {
  item: InboxItem;
  space?: SpacePreview;
  seen: boolean;
  onSeen: () => void;
};

export function InboxRow({ item, space, seen, onSeen }: Props) {
  const [open, setOpen] = useState(false);
  const cont = useContinueSpace();
  const retry = useRetryTurn();
  const dismiss = useDismissItem();
  const bots = useBots();
  const pending = cont.isPending || retry.isPending || dismiss.isPending;
  const error = cont.error ?? retry.error ?? dismiss.error;
  const { spaceId, turnId } = item;
  const where =
    item.spaceId === 0 ? `Work item ${item.workItemId}` : (space?.name ?? `Space ${spaceId}`);
  const linkAction = item.kind === 'review' ? 'Review' : item.kind === 'question' ? 'Reply' : null;

  let action = null;
  if (item.kind === 'failed' && turnId !== undefined) {
    action = (
      <Button
        type="button"
        variant="primary"
        size="sm"
        disabled={pending}
        onClick={() => retry.mutate(turnId)}
      >
        Retry
      </Button>
    );
  } else if (item.kind === 'paused' || item.kind === 'held') {
    action = (
      <Button
        type="button"
        variant="primary"
        size="sm"
        disabled={pending}
        onClick={() => cont.mutate(spaceId)}
      >
        Continue
      </Button>
    );
  } else if (linkAction) {
    action = (
      <Link to={itemPath(item)} className={buttonClass('primary', 'sm')}>
        {linkAction}
      </Link>
    );
  }

  const more = (
    <>
      {!linkAction && (
        <Link to={itemPath(item)} className={buttonClass('ghost', 'sm')}>
          {item.group === 'needs_you' ? 'Open Space' : 'Open'}
        </Link>
      )}
      {item.group === 'fyi' && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={pending}
          onClick={() => dismiss.mutate(item)}
        >
          Dismiss
        </Button>
      )}
    </>
  );

  return (
    <li className="group border-b border-hairline">
      <div className="-mx-2.5 flex items-center gap-2 rounded-field px-2.5 hover:bg-card">
        <button
          type="button"
          aria-expanded={open}
          onClick={() => {
            setOpen(!open);
            onSeen();
          }}
          className="flex min-h-[58px] min-w-0 flex-1 items-center gap-3 py-2 text-left"
        >
          <span
            role={seen ? undefined : 'img'}
            aria-label={seen ? undefined : 'Not opened yet'}
            className={`size-[7px] flex-none rounded-full ${seen ? '' : 'bg-working'}`}
          />
          <Creature look={faceOf(memberFaces(bots.data, space?.members), space?.botId)} size={32} />
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-2 text-sm">
              <span className={`truncate ${seen ? 'text-soft' : 'font-medium'}`}>{where}</span>
              <span
                className={`flex-none text-[11.5px] uppercase tracking-wider ${kindColors[item.kind]}`}
              >
                {kindLabels[item.kind]}
              </span>
            </span>
            {!open && (
              <span className="block truncate text-[13.5px] text-muted">{preview(item.text)}</span>
            )}
          </span>
        </button>
        <div className="flex flex-none items-center gap-1.5">
          {action}
          {!open && (
            <span className="hidden items-center gap-1 group-focus-within:flex group-hover:flex">
              {more}
            </span>
          )}
          <span className="w-8 text-right text-xs text-muted">{age(item.at)}</span>
        </div>
      </div>
      {open && (
        <div className="max-w-[720px] pb-4 pl-[60px] text-sm text-soft">
          <Markdown>{item.text}</Markdown>
          <div className="mt-3 flex items-center gap-1.5">{more}</div>
        </div>
      )}
      {error && <p className="pb-2 pl-[60px] text-sm text-danger">{error.message}</p>}
    </li>
  );
}

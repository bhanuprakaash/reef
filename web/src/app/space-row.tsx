import { useLocation } from 'react-router';
import { SpaceCard } from '@/app/space-hovercard';
import { GroupAvatar } from '@/components/creature';
import { PinButton } from '@/components/pin-button';
import { Hovercard } from '@/components/hovercard';
import { Row } from '@/components/row';
import { StatusDot } from '@/components/status-dot';
import { useBots } from '@/lib/api/get-bots';
import { pinKindOf } from '@/lib/api/pins';
import { useTeams } from '@/lib/api/get-teams';
import { useRunningTurns } from '@/modules/spaces/api/get-turns';
import { useWorkItems } from '@/modules/work-items/api/get-work-items';
import { useInbox } from '@/modules/inbox/api/get-inbox';
import type { SpacePreview } from '@/types/api';
import { roleFaces } from '@/utils/role-faces';
import { rowTime } from '@/utils/row-time';

const oneLine = (m: NonNullable<SpacePreview['lastMessage']>) =>
  `${m.author === 'user' ? 'you' : m.author}: ${m.body.replace(/\s+/g, ' ')}`;

export function SpaceRow({ space: c }: { space: SpacePreview }) {
  const { pathname } = useLocation();
  const workItems = useWorkItems();
  const teams = useTeams();
  const bots = useBots();
  const inbox = useInbox();
  const running = useRunningTurns();

  const waiting = inbox.data ?? [];
  const lead = bots.data?.find((b) => b.id === c.botId);
  const faces =
    c.kind === 'conversation' ? (lead ? { [lead.id]: lead } : {}) : roleFaces(teams.data, c.team);
  const roles = Object.keys(faces);
  const turns = (running.data ?? []).filter((t) => t.spaceId === c.id);
  const workingBots = turns.flatMap((t) => bots.data?.find((b) => b.id === t.botId) ?? []);
  const members = { ...faces, ...Object.fromEntries(workingBots.map((b) => [b.id, b])) };
  const workingName = (t: (typeof turns)[number]) =>
    bots.data?.find((b) => b.id === t.botId)?.name ?? t.role;
  const workItem = workItems.data?.find((f) => f.id === c.workItemId);
  const title = c.kind === 'work_item' ? `#${c.name}` : c.name;

  return (
    <div className="group relative">
      <Hovercard
        content={
          <SpaceCard
            space={c}
            workItem={workItem}
            members={members}
            working={turns.map((t) => t.botId)}
            waiting={waiting.filter((i) => i.spaceId === c.id).map((i) => i.kind)}
          />
        }
      >
        {(trigger) => (
          <Row
            trigger={trigger}
            to={`/spaces/${c.id}`}
            active={pathname === `/spaces/${c.id}`}
            avatar={
              <span className="relative flex-none">
                <GroupAvatar roles={roles.length > 0 ? roles : [c.botId]} faces={faces} size={30} />
                {waiting.some((i) => i.spaceId === c.id && i.group === 'needs_you') && (
                  <span
                    role="img"
                    aria-label="needs you"
                    className="absolute -top-0.5 -right-0.5 size-2.5 rounded-full border-2 border-sidebar bg-needs-you"
                  />
                )}
              </span>
            }
            title={title}
            live={
              turns.length > 0 ? (
                <StatusDot
                  status="working"
                  label={`${workingName(turns[turns.length - 1])} is working`}
                />
              ) : undefined
            }
            aside={c.lastMessage && rowTime(c.lastMessage.createdAt)}
            preview={c.lastMessage ? oneLine(c.lastMessage) : 'No messages yet'}
          />
        )}
      </Hovercard>
      <PinButton kind={pinKindOf(c)} id={c.id} overlay />
    </div>
  );
}

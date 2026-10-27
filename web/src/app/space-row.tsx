import { useLocation } from 'react-router';
import { SpaceCard } from '@/app/space-hovercard';
import { GroupAvatar } from '@/components/creature';
import { Hovercard } from '@/components/hovercard';
import { Row } from '@/components/row';
import { StatusDot } from '@/components/status-dot';
import { useTeammates } from '@/lib/api/get-teammates';
import { useTeams } from '@/lib/api/get-teams';
import { useRunningTurns } from '@/modules/spaces/api/get-turns';
import { useWorkItems } from '@/modules/work-items/api/get-work-items';
import { useNeedsYou } from '@/modules/inbox/api/get-inbox';
import type { SpacePreview } from '@/types/api';
import { roleFaces } from '@/utils/role-faces';
import { rowTime } from '@/utils/row-time';

const oneLine = (m: NonNullable<SpacePreview['lastMessage']>) =>
  `${m.author === 'user' ? 'you' : m.author}: ${m.body.replace(/\s+/g, ' ')}`;

export function SpaceRow({ space: c }: { space: SpacePreview }) {
  const { pathname } = useLocation();
  const workItems = useWorkItems();
  const teams = useTeams();
  const teammates = useTeammates();
  const inbox = useNeedsYou();
  const running = useRunningTurns();

  const waiting = inbox.data ?? [];
  const faces =
    c.kind === 'conversation'
      ? { [c.role]: teammates.data?.find((t) => t.name === c.role) }
      : roleFaces(teams.data, c.team);
  const roles = Object.keys(faces);
  const roleNames = (running.data ?? []).filter((t) => t.spaceId === c.id).map((t) => t.role);
  const workItem = workItems.data?.find((f) => f.id === c.workItemId);
  const title = c.kind === 'work_item' ? `#${c.name}` : c.name;

  return (
    <Hovercard
      content={
        <SpaceCard
          space={c}
          workItem={workItem}
          faces={faces}
          working={roleNames}
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
              <GroupAvatar roles={roles.length > 0 ? roles : [c.role]} faces={faces} size={30} />
              {waiting.some((i) => i.spaceId === c.id) && (
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
            roleNames.length > 0 ? (
              <StatusDot status="working" label={`${roleNames.at(-1)} is working`} />
            ) : undefined
          }
          aside={c.lastMessage && rowTime(c.lastMessage.createdAt)}
          preview={c.lastMessage ? oneLine(c.lastMessage) : 'No messages yet'}
        />
      )}
    </Hovercard>
  );
}

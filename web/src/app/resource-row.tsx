import { useLocation } from 'react-router';
import { Creature } from '@/components/creature';
import { Row } from '@/components/row';
import { useBots } from '@/lib/api/get-bots';
import type { FindResult } from '@/types/api';
import { kindLabel, resourcePath } from '@/utils/resource-path';

const icons: Record<Exclude<FindResult['kind'], 'bot'>, string> = {
  chat: 'M21 12a8 8 0 0 1-11.6 7.1L4 20l1-4.6A8 8 0 1 1 21 12z',
  space: 'M4 9h16M4 15h16M10 3L8 21M16 3l-2 18',
  workItem: 'M5 4h14v16H5zM9 9h6M9 13h6',
};

export function ResourceRow({ resource }: { resource: Pick<FindResult, 'kind' | 'id' | 'title'> }) {
  const { pathname } = useLocation();
  const bots = useBots();
  const to = resourcePath(resource);
  return (
    <Row
      to={to}
      active={pathname === to}
      avatar={
        resource.kind === 'bot' ? (
          <Creature look={bots.data?.find((b) => b.id === resource.id) ?? {}} size={30} />
        ) : (
          <span className="flex size-[30px] flex-none items-center justify-center rounded-full bg-bubble">
            <svg
              width="15"
              height="15"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.8"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <path d={icons[resource.kind]} />
            </svg>
          </span>
        )
      }
      title={resource.title}
      preview={kindLabel[resource.kind]}
    />
  );
}

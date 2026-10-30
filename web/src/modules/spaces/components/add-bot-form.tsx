import { useBots } from '@/lib/api/get-bots';
import { useAddBot } from '@/modules/spaces/api/add-bot';
import { useSpaceMembers } from '@/modules/spaces/api/get-space-members';
import { GroupAvatar } from '@/components/creature';
import type { Space } from '@/types/api';

export function AddBotForm({ space, onClose }: { space: Space; onClose: () => void }) {
  const bots = useBots();
  const members = useSpaceMembers(space.id);
  const add = useAddBot(space.id);
  const choices = bots.data?.filter((t) => !members.data?.some((m) => m.botId === t.id)) ?? [];

  return (
    <div className="space-y-3.5">
      <p className="-mt-2.5 text-[13px] text-muted">
        {space.kind === 'conversation'
          ? `${space.role} writes a summary, and #${space.name} becomes a Space with both of them.`
          : `The Bot joins #${space.name}.`}
      </p>
      {choices.length === 0 && <p className="text-sm text-muted">No other Bots yet</p>}
      <div className="grid grid-cols-2 gap-2">
        {choices.map((t) => (
          <button
            key={t.id}
            type="button"
            disabled={add.isPending}
            onClick={() => add.mutate(t.id, { onSuccess: onClose })}
            className="flex min-w-0 items-center gap-2 rounded-menu border border-line p-2.5 text-left text-sm text-soft outline-none hover:text-fg focus-visible:ring-3 focus-visible:ring-ring"
          >
            <GroupAvatar roles={[t.name]} faces={{ [t.name]: t }} size={26} />
            <span className="truncate">{t.name}</span>
          </button>
        ))}
      </div>
      {(bots.isError || add.isError) && (
        <p className="text-sm text-danger">{(bots.error ?? add.error)?.message}</p>
      )}
    </div>
  );
}

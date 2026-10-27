import { useTeammates } from '@/lib/api/get-teammates';
import { useAddTeammate } from '@/modules/spaces/api/add-teammate';
import { GroupAvatar } from '@/components/creature';
import type { Space } from '@/types/api';

export function AddTeammateForm({ space, onClose }: { space: Space; onClose: () => void }) {
  const teammates = useTeammates();
  const add = useAddTeammate(space.id);
  const choices = teammates.data?.filter((t) => t.name !== space.role) ?? [];

  return (
    <div className="space-y-3.5">
      <p className="-mt-2.5 text-[13px] text-muted">
        {space.role} writes a summary, and #{space.name} becomes a Space with both of them.
      </p>
      {choices.length === 0 && <p className="text-sm text-muted">No other Teammates yet</p>}
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
      {(teammates.isError || add.isError) && (
        <p className="text-sm text-danger">{(teammates.error ?? add.error)?.message}</p>
      )}
    </div>
  );
}

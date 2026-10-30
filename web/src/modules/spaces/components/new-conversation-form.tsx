import { useNavigate } from 'react-router';
import { useBots } from '@/lib/api/get-bots';
import { useStartConversation } from '@/modules/spaces/api/start-conversation';
import { GroupAvatar } from '@/components/creature';

type Props = { projectId: number; onClose: () => void };

export function NewConversationForm({ projectId, onClose }: Props) {
  const bots = useBots();
  const create = useStartConversation();
  const navigate = useNavigate();

  return (
    <div className="space-y-3.5">
      {bots.data?.length === 0 && <p className="text-sm text-muted">No Bots yet</p>}
      <div className="grid grid-cols-2 gap-2">
        {bots.data?.map((t) => (
          <button
            key={t.id}
            type="button"
            disabled={create.isPending}
            onClick={() => {
              create.mutate(
                { projectId, bot: t },
                {
                  onSuccess: (space) => {
                    onClose();
                    void navigate(`/spaces/${space.id}`);
                  },
                },
              );
            }}
            className="flex min-w-0 items-center gap-2 rounded-menu border border-line p-2.5 text-left text-sm text-soft outline-none hover:text-fg focus-visible:ring-3 focus-visible:ring-ring"
          >
            <GroupAvatar roles={[t.id]} faces={{ [t.id]: t }} size={26} />
            <span className="truncate">{t.name}</span>
          </button>
        ))}
      </div>
      {(bots.isError || create.isError) && (
        <p className="text-sm text-danger">{(bots.error ?? create.error)?.message}</p>
      )}
    </div>
  );
}

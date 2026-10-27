import { useNavigate } from 'react-router';
import { useTeammates } from '@/lib/api/get-teammates';
import { useCreateSpace } from '@/modules/spaces/api/create-space';
import { GroupAvatar } from '@/components/creature';

type Props = { projectId: number; onClose: () => void };

export function NewConversationForm({ projectId, onClose }: Props) {
  const teammates = useTeammates();
  const create = useCreateSpace();
  const navigate = useNavigate();

  return (
    <div className="space-y-3.5">
      {teammates.data?.length === 0 && <p className="text-sm text-muted">No Teammates yet</p>}
      <div className="grid grid-cols-2 gap-2">
        {teammates.data?.map((t) => (
          <button
            key={t.id}
            type="button"
            disabled={create.isPending}
            onClick={() => {
              const time = new Date().toLocaleTimeString([], {
                hour: '2-digit',
                minute: '2-digit',
              });
              create.mutate(
                {
                  projectId,
                  name: `${t.name} ${time}`,
                  teammateId: t.id,
                  kind: 'conversation',
                },
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
            <GroupAvatar roles={[t.name]} faces={{ [t.name]: t }} size={26} />
            <span className="truncate">{t.name}</span>
          </button>
        ))}
      </div>
      {(teammates.isError || create.isError) && (
        <p className="text-sm text-danger">{(teammates.error ?? create.error)?.message}</p>
      )}
    </div>
  );
}

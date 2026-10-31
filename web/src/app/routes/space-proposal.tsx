import { useNavigate, useParams } from 'react-router';
import { Dialog } from '@/components/dialog';
import { useDismissItem } from '@/modules/inbox/api/dismiss-item';
import { useInbox } from '@/modules/inbox/api/get-inbox';
import { useCurrentProject } from '@/lib/current-project';
import { useProjects } from '@/modules/projects/api/get-projects';
import { NewSpaceForm } from '@/modules/spaces/components/new-space-form';

export function SpaceProposalPage() {
  const { id } = useParams();
  const inbox = useInbox();
  const dismiss = useDismissItem();
  const navigate = useNavigate();
  const projects = useProjects();
  const [project] = useCurrentProject(projects.data);
  const item = inbox.data?.find((i) => i.kind === 'space_proposed' && i.id === Number(id));
  const close = () => void navigate('/inbox');

  if (!item?.proposal) return <p className="text-sm text-muted">No such proposal.</p>;
  return (
    <Dialog open onClose={close} title="New Space">
      <NewSpaceForm
        projectId={project?.id}
        workItemId={null}
        proposal={item.proposal}
        onCreated={() => dismiss.mutate(item)}
        onClose={close}
      />
    </Dialog>
  );
}

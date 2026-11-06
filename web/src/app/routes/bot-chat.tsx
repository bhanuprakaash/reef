import { Navigate, useParams } from 'react-router';
import { useCurrentProject } from '@/lib/current-project';
import { useProjects } from '@/modules/projects/api/get-projects';
import { useBotLatestChat } from '@/modules/teams/api/get-bot-conversations';

// Opens a Bot's latest chat under the selected Project, or under All projects its latest overall.
// With none it opens the Bot page, which offers New chat; it never creates one.
export function BotChatPage() {
  const id = Number(useParams().id);
  const projects = useProjects();
  const [project] = useCurrentProject(projects.data);
  const latest = useBotLatestChat(id, project?.id, projects.isSuccess);

  if (projects.isError) return <p className="text-sm text-danger">{projects.error.message}</p>;
  if (latest.isError) return <p className="text-sm text-danger">{latest.error.message}</p>;
  if (!latest.isSuccess) return <p className="text-sm text-muted">Loading...</p>;
  return <Navigate replace to={latest.data ? `/spaces/${latest.data.id}` : `/bots/${id}`} />;
}

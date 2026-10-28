import { Link, useNavigate, useParams } from 'react-router';
import { useMemory } from '@/lib/api/get-memory';
import { useTeammates } from '@/lib/api/get-teammates';
import { useTeams } from '@/lib/api/get-teams';
import { useCurrentProject } from '@/lib/current-project';
import { useProjects } from '@/modules/projects/api/get-projects';
import { useStartConversation } from '@/modules/spaces/api/start-conversation';
import { useTeammateConversations } from '@/modules/teams/api/get-teammate-conversations';
import { backendLabel } from '@/modules/teams/utils/backend-label';
import { rowTime } from '@/utils/row-time';
import { BackButton } from '@/components/back-button';
import { Button, buttonClass } from '@/components/button';
import { Card } from '@/components/card';
import { Chip } from '@/components/chip';
import { Creature } from '@/components/creature';
import { EmptyState } from '@/components/empty-state';

export function TeammatePage() {
  const id = Number(useParams().id);
  const teammates = useTeammates();
  const teams = useTeams();
  const conversations = useTeammateConversations(id);
  const memory = useMemory(`teammate:${id}`);
  const projects = useProjects();
  const [project] = useCurrentProject(projects.data);
  const create = useStartConversation();
  const navigate = useNavigate();

  if (teammates.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (teammates.isError) return <p className="text-sm text-danger">{teammates.error.message}</p>;
  const teammate = teammates.data.find((t) => t.id === id);
  if (!teammate) return <p className="text-sm text-danger">No teammate {id}</p>;

  const memberships = (teams.data ?? []).flatMap((team) =>
    team.roles.filter((r) => r.teammateId === id).map((role) => ({ team, role })),
  );

  function start() {
    if (!project) return;
    if (!teammate) return;
    create.mutate(
      { projectId: project.id, teammate },
      { onSuccess: (space) => void navigate(`/spaces/${space.id}`) },
    );
  }

  return (
    <div className="mx-auto flex w-full max-w-[820px] flex-col gap-6 py-6">
      <header className="flex flex-wrap items-center gap-4">
        <BackButton fallback="/teammates" />
        <Creature look={teammate} size={64} animate />
        <div className="min-w-0 flex-1 basis-56">
          <h1 className="text-[32px] font-normal tracking-[-0.02em]">{teammate.name}</h1>
          <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
            <Chip>{backendLabel(teammate.backend)}</Chip>
            {teammate.model && <Chip>{teammate.model}</Chip>}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Link to={`/teammates?teammate=${id}`} className={buttonClass('secondary')}>
            Edit
          </Link>
          <Button variant="primary" disabled={!project} loading={create.isPending} onClick={start}>
            Start conversation
          </Button>
        </div>
      </header>
      {create.isError && <p className="text-sm text-danger">{create.error.message}</p>}
      {teammate.persona && <p className="max-w-xl text-sm text-soft">{teammate.persona}</p>}

      <Card title="Teams" flush>
        {memberships.length === 0 ? (
          <div className="px-6 pb-5">
            <EmptyState title="In no Team yet" body="Add it from a Team's page." compact />
          </div>
        ) : (
          <ul>
            {memberships.map(({ team, role }) => (
              <li key={team.name} className="border-t border-hairline">
                <Link
                  to={`/teams?team=${team.name}`}
                  className="flex flex-wrap items-center gap-2 px-6 py-3.5 text-sm"
                >
                  <span>{team.name}</span>
                  <span className="text-muted">{role.name}</span>
                  {role.lead && (
                    <span className="rounded-pill bg-lead px-2 text-[11px] leading-5 text-mark-ink">
                      answers by default
                    </span>
                  )}
                  <span className="ml-auto text-xs text-muted">
                    {role.write ? 'edits code' : 'read-only'}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Card title="Conversations" flush>
        {conversations.isError && (
          <p className="px-6 pb-4 text-sm text-danger">{conversations.error.message}</p>
        )}
        {conversations.data?.length === 0 && (
          <div className="px-6 pb-5">
            <EmptyState title="No conversations yet" body="Start one to talk directly." compact />
          </div>
        )}
        <ul>
          {conversations.data?.map((c) => (
            <li key={c.id} className="border-t border-hairline">
              <Link to={`/spaces/${c.id}`} className="flex items-baseline gap-3 px-6 py-3.5">
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm">{c.name}</span>
                  {c.lastMessage && (
                    <span className="block truncate text-[13px] text-muted">
                      {c.lastMessage.body}
                    </span>
                  )}
                </span>
                <span className="shrink-0 text-xs text-muted">
                  {rowTime(c.lastMessage?.createdAt ?? c.createdAt)}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </Card>

      <Card
        title="Memory"
        flush
        aside={
          <span className="ml-auto text-xs text-muted tabular-nums">{memory.data?.length}</span>
        }
      >
        {memory.isError && <p className="px-6 pb-4 text-sm text-danger">{memory.error.message}</p>}
        {memory.data?.length === 0 && (
          <div className="px-6 pb-5">
            <EmptyState
              title="Nothing remembered yet"
              body={`What ${teammate.name} keeps across every work item lands here.`}
              compact
            />
          </div>
        )}
        <ul>
          {memory.data?.map((item) => (
            <li key={item.id} className="flex gap-3.5 border-t border-hairline px-6 py-3.5">
              <span className="w-16 shrink-0 pt-0.5 text-xs text-muted">{item.kind}</span>
              <span className="min-w-0 flex-1">
                <span className="block text-sm">{item.body}</span>
                {item.why && <span className="mt-1 block text-sm text-soft">{item.why}</span>}
              </span>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}

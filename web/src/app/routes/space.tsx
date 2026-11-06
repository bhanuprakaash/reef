import type { ComponentProps } from 'react';
import { useOutletContext, useParams } from 'react-router';
import type { LayoutContext } from '@/app/layout';
import { PaneToggles } from '@/app/pane-toggles';
import { WorkItemTerminal } from '@/app/work-item-terminal';
import { MentionCardsProvider } from '@/app/mention-hovercard';
import { ReviewDiff } from '@/app/review-diff';
import { SkillDraftCards } from '@/app/skill-draft-cards';
import { TerminalPane } from '@/app/terminal-pane';
import { useTerminal } from '@/app/use-terminal';
import { useSpaceProjects } from '@/modules/spaces/api/space-projects';
import { useSpace } from '@/modules/spaces/api/get-space';
import { SpaceView } from '@/modules/spaces/components/space-view';
import { useBriefVersions } from '@/modules/work-items/api/get-brief-versions';
import { useDiff } from '@/modules/work-items/api/get-diff';
import { useWorkItems } from '@/modules/work-items/api/get-work-items';
import { diffCounts } from '@/modules/work-items/utils/diff-counts';
import { statusLabel } from '@/modules/work-items/utils/status-label';
import { useProjects } from '@/modules/projects/api/get-projects';

type ViewProps = ComponentProps<typeof SpaceView>;

function WorkItemSpaceView({ workItemId, ...props }: ViewProps & { workItemId: number }) {
  const diff = useDiff(workItemId);
  const briefs = useBriefVersions(workItemId);
  return (
    <SpaceView
      {...props}
      diff={diff.data && diffCounts(diff.data)}
      diffPanel={(onClose) => (
        <ReviewDiff id={workItemId} onClose={onClose} defaultScope="current" />
      )}
      brief={briefs.data?.at(-1)?.body}
    />
  );
}

export function SpacePage() {
  const id = Number(useParams().id);
  const space = useSpace(id);
  const workItems = useWorkItems();
  const spaceProjects = useSpaceProjects(id);
  const projects = useProjects();
  const { browserOpen, terminalOpen, toggleBrowser, toggleTerminal, setBrowserDriver } =
    useOutletContext<LayoutContext>();
  const project = projects.data?.find((p) => p.id === space.data?.projectId);
  const terminal = useTerminal(
    space.data !== undefined && (space.data.workItemId !== null || !!project),
  );

  if (space.isPending || spaceProjects.isPending)
    return <p className="text-sm text-muted">Loading...</p>;
  if (spaceProjects.isError)
    return <p className="text-sm text-danger">{spaceProjects.error.message}</p>;
  if (space.isError) return <p className="text-sm text-danger">{space.error.message}</p>;

  const workItemId = space.data.workItemId;
  const workItem = workItems.data?.find((f) => f.id === workItemId);
  const hasBrowser = typeof window !== 'undefined' && window.reef != null;
  const props: ViewProps = {
    space: space.data,
    readOnly:
      spaceProjects.data.length > 0 && spaceProjects.data.every((p) => p.deletingAt !== null),
    workItems: workItems.data ?? [],
    projects: projects.data ?? [],
    status: workItem && statusLabel(workItem.status),
    scope: workItem ? (
      <span className="truncate">{workItem.title}</span>
    ) : (
      [
        space.data.kind === 'free' ? 'free Space' : space.data.kind,
        ...(spaceProjects.data ?? []).map((p) => `${p.name}${p.deletingAt ? ' (deleted)' : ''}`),
      ].join(' · ')
    ),
    drafts: <SkillDraftCards spaceId={id} />,
    browserOpen: hasBrowser && browserOpen,
    tools: hasBrowser ? (
      <PaneToggles
        className="flex gap-0.5 rounded-field border border-line p-0.5"
        browserOpen={browserOpen}
        onBrowser={toggleBrowser}
        terminalOpen={terminalOpen}
        onTerminal={toggleTerminal}
      />
    ) : undefined,
    onBrowserDriver: setBrowserDriver,
  };
  return (
    <MentionCardsProvider spaceId={id}>
      <div className="-m-4 flex h-screen flex-col md:-m-6">
        {workItemId === null ? (
          <SpaceView key={id} {...props} />
        ) : (
          <WorkItemSpaceView key={id} workItemId={workItemId} {...props} />
        )}
        {terminal.open && (
          <div className="shrink-0 border-t border-hairline">
            {workItemId === null ? (
              project && <TerminalPane cwd={project.path} onHide={terminal.hide} />
            ) : (
              <WorkItemTerminal key={workItemId} workItemId={workItemId} onHide={terminal.hide} />
            )}
          </div>
        )}
      </div>
    </MentionCardsProvider>
  );
}

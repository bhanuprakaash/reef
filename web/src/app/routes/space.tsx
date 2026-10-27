import type { ComponentProps } from 'react';
import { useOutletContext, useParams } from 'react-router';
import type { LayoutContext } from '@/app/layout';
import { FeatureTerminal } from '@/app/feature-terminal';
import { MentionCardsProvider } from '@/app/mention-hovercard';
import { ReviewDiff } from '@/app/review-diff';
import { TerminalPane } from '@/app/terminal-pane';
import { useTerminal } from '@/app/use-terminal';
import { useSpace } from '@/modules/spaces/api/get-space';
import { SpaceView } from '@/modules/spaces/components/space-view';
import { useBriefVersions } from '@/modules/features/api/get-brief-versions';
import { useDiff } from '@/modules/features/api/get-diff';
import { useFeatures } from '@/modules/features/api/get-features';
import { diffCounts } from '@/modules/features/utils/diff-counts';
import { useProjects } from '@/modules/projects/api/get-projects';

type ViewProps = ComponentProps<typeof SpaceView>;

function FeatureSpaceView({ featureId, ...props }: ViewProps & { featureId: number }) {
  const diff = useDiff(featureId, 'current');
  const briefs = useBriefVersions(featureId);
  return (
    <SpaceView
      {...props}
      diff={diff.data && diffCounts(diff.data)}
      diffPanel={(onClose) => (
        <ReviewDiff id={featureId} onClose={onClose} defaultScope="current" />
      )}
      brief={briefs.data?.at(-1)?.body}
    />
  );
}

export function SpacePage() {
  const id = Number(useParams().id);
  const space = useSpace(id);
  const features = useFeatures();
  const projects = useProjects();
  const { browserOpen, setBrowserDriver } = useOutletContext<LayoutContext>();
  const project = projects.data?.find((p) => p.id === space.data?.projectId);
  const terminal = useTerminal(
    space.data !== undefined && (space.data.workItemId !== null || !!project),
  );

  if (space.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (space.isError) return <p className="text-sm text-danger">{space.error.message}</p>;

  const featureId = space.data.workItemId;
  const feature = features.data?.find((f) => f.id === featureId);
  const hasBrowser = typeof window !== 'undefined' && window.reef != null;
  const props: ViewProps = {
    space: space.data,
    features: features.data ?? [],
    scope: feature ? <span className="truncate">{feature.title}</span> : 'quick session',
    browserOpen: hasBrowser && browserOpen,
    onBrowserDriver: setBrowserDriver,
  };
  return (
    <MentionCardsProvider spaceId={id} team={space.data.team}>
      <div className="-m-4 flex h-screen flex-col md:-m-6">
        {featureId === null ? (
          <SpaceView key={id} {...props} />
        ) : (
          <FeatureSpaceView key={id} featureId={featureId} {...props} />
        )}
        {terminal.open && (
          <div className="shrink-0 border-t border-hairline">
            {featureId === null ? (
              project && <TerminalPane cwd={project.path} onHide={terminal.hide} />
            ) : (
              <FeatureTerminal key={featureId} featureId={featureId} onHide={terminal.hide} />
            )}
          </div>
        )}
      </div>
    </MentionCardsProvider>
  );
}

import type { ComponentProps } from 'react';
import { useOutletContext, useParams } from 'react-router';
import type { LayoutContext } from '@/app/layout';
import { FeatureTerminal } from '@/app/feature-terminal';
import { ReviewDiff } from '@/app/review-diff';
import { TerminalPane } from '@/app/terminal-pane';
import { useTerminal } from '@/app/use-terminal';
import { useChannel } from '@/modules/channels/api/get-channel';
import { StageChip } from '@/components/stage-chip';
import { ChannelView } from '@/modules/channels/components/channel-view';
import { useBriefVersions } from '@/modules/features/api/get-brief-versions';
import { useDiff } from '@/modules/features/api/get-diff';
import { useFeatures } from '@/modules/features/api/get-features';
import { diffCounts } from '@/modules/features/utils/diff-counts';
import { STAGES } from '@/modules/features/utils/stages';
import { useProjects } from '@/modules/projects/api/get-projects';

type ViewProps = ComponentProps<typeof ChannelView>;

function FeatureChannelView({ featureId, ...props }: ViewProps & { featureId: number }) {
  const diff = useDiff(featureId);
  const briefs = useBriefVersions(featureId);
  return (
    <ChannelView
      {...props}
      diff={diff.data && diffCounts(diff.data)}
      diffPanel={(onClose) => <ReviewDiff id={featureId} onClose={onClose} />}
      brief={briefs.data?.at(-1)?.body}
    />
  );
}

export function ChannelPage() {
  const id = Number(useParams().id);
  const channel = useChannel(id);
  const features = useFeatures();
  const projects = useProjects();
  const { browserOpen, setBrowserDriver } = useOutletContext<LayoutContext>();
  const project = projects.data?.find((p) => p.id === channel.data?.projectId);
  const terminal = useTerminal(
    channel.data !== undefined && (channel.data.featureId !== null || !!project),
  );

  if (channel.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (channel.isError) return <p className="text-sm text-danger">{channel.error.message}</p>;

  const featureId = channel.data.featureId;
  const feature = features.data?.find((f) => f.id === featureId);
  const hasBrowser = typeof window !== 'undefined' && window.reef != null;
  const props: ViewProps = {
    channel: channel.data,
    features: features.data ?? [],
    scope: feature ? (
      <>
        <span className="truncate">{feature.name}</span>
        <StageChip
          label={STAGES.find((s) => s.value === feature.stage)?.label ?? feature.stage}
          color={STAGES.find((s) => s.value === feature.stage)?.color ?? 'var(--color-muted)'}
        />
      </>
    ) : (
      'quick session'
    ),
    browserOpen: hasBrowser && browserOpen,
    onBrowserDriver: setBrowserDriver,
  };
  return (
    <div className="-m-4 flex h-screen flex-col md:-m-6">
      {featureId === null ? (
        <ChannelView key={id} {...props} />
      ) : (
        <FeatureChannelView key={id} featureId={featureId} {...props} />
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
  );
}

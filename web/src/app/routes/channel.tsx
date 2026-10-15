import { type ComponentProps, useEffect, useState } from 'react';
import { useOutletContext, useParams } from 'react-router';
import type { LayoutContext } from '@/app/layout';
import { TerminalPane } from '@/app/terminal-pane';
import { useChannel } from '@/modules/channels/api/get-channel';
import { ChannelView } from '@/modules/channels/components/channel-view';
import { useBriefVersions } from '@/modules/features/api/get-brief-versions';
import { useDiff } from '@/modules/features/api/get-diff';
import { useEnsureWorktree } from '@/modules/features/api/ensure-worktree';
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
      brief={briefs.data?.at(-1)?.body}
    />
  );
}

function FeatureTerminal({ featureId, onHide }: { featureId: number; onHide: () => void }) {
  const { mutate, data } = useEnsureWorktree(featureId);
  useEffect(() => mutate(), [mutate]);
  return data ? <TerminalPane cwd={data.path} onHide={onHide} /> : null;
}

export function ChannelPage() {
  const id = Number(useParams().id);
  const channel = useChannel(id);
  const features = useFeatures();
  const projects = useProjects();
  const [terminalFlipped, setTerminalFlipped] = useState(false);
  const { browserOpen, toggleBrowser, setBrowserDriver } = useOutletContext<LayoutContext>();

  if (channel.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (channel.isError) return <p className="text-sm text-danger">{channel.error.message}</p>;

  const featureId = channel.data.featureId;
  const feature = features.data?.find((f) => f.id === featureId);
  const project = projects.data?.find((p) => p.id === channel.data.projectId);
  const hasTerminal = typeof window !== 'undefined' && window.reef != null;
  const toggleTerminal = () => setTerminalFlipped((flipped) => !flipped);
  const terminalOpen = hasTerminal && (featureId === null) !== terminalFlipped;
  const props: ViewProps = {
    channel: channel.data,
    features: features.data ?? [],
    scope: feature
      ? `${feature.name} · ${STAGES.find((s) => s.value === feature.stage)?.label ?? feature.stage}`
      : 'quick session',
    onTerminal: hasTerminal ? toggleTerminal : undefined,
    browserOpen,
    onBrowser: hasTerminal ? toggleBrowser : undefined,
    onBrowserDriver: setBrowserDriver,
  };
  return (
    <div className="-m-6 flex h-screen flex-col">
      {featureId === null ? (
        <ChannelView key={id} {...props} />
      ) : (
        <FeatureChannelView key={id} featureId={featureId} {...props} />
      )}
      {terminalOpen && (
        <div className="shrink-0 border-t border-hairline">
          {featureId === null ? (
            project && <TerminalPane cwd={project.path} onHide={toggleTerminal} />
          ) : (
            <FeatureTerminal featureId={featureId} onHide={toggleTerminal} />
          )}
        </div>
      )}
    </div>
  );
}

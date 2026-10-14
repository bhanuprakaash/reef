import { useParams } from 'react-router';
import { TerminalPane } from '@/app/terminal-pane';
import { useChannel } from '@/modules/channels/api/get-channel';
import { ChannelView } from '@/modules/channels/components/channel-view';
import { useFeatures } from '@/modules/features/api/get-features';
import { useProjects } from '@/modules/projects/api/get-projects';
import { errorClass, mutedClass } from '@/utils/classes';

export function ChannelPage() {
  const id = Number(useParams().id);
  const channel = useChannel(id);
  const features = useFeatures();
  const projects = useProjects();

  if (channel.isPending) return <p className={mutedClass}>Loading...</p>;
  if (channel.isError) return <p className={errorClass}>{channel.error.message}</p>;

  const feature = features.data?.find((f) => f.id === channel.data.featureId);
  const isQuickSession = channel.data.featureId === null;
  const project = projects.data?.find((p) => p.id === channel.data.projectId);
  return (
    <div className="space-y-4">
      <ChannelView
        key={id}
        channel={channel.data}
        features={features.data ?? []}
        scope={isQuickSession ? 'quick session' : (feature?.name ?? 'feature')}
      />
      {isQuickSession && project && <TerminalPane cwd={project.path} />}
    </div>
  );
}

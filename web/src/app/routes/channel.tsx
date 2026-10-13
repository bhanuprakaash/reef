import { useParams } from 'react-router';
import { useChannel } from '@/modules/channels/api/get-channel';
import { ChannelView } from '@/modules/channels/components/channel-view';
import { useFeatures } from '@/modules/features/api/get-features';
import { errorClass, mutedClass } from '@/utils/classes';

export function ChannelPage() {
  const id = Number(useParams().id);
  const channel = useChannel(id);
  const features = useFeatures();

  if (channel.isPending) return <p className={mutedClass}>Loading...</p>;
  if (channel.isError) return <p className={errorClass}>{channel.error.message}</p>;

  const feature = features.data?.find((f) => f.id === channel.data.featureId);
  return (
    <ChannelView
      key={id}
      channel={channel.data}
      features={features.data ?? []}
      scope={channel.data.featureId === null ? 'quick session' : (feature?.name ?? 'feature')}
    />
  );
}

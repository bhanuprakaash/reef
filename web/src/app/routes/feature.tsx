import { useParams } from 'react-router';
import { ChannelList } from '@/modules/channels/components/channel-list';
import { useChannels } from '@/modules/channels/api/get-channels';
import { useBriefVersions } from '@/modules/features/api/get-brief-versions';
import { useDecisions } from '@/modules/features/api/get-decisions';
import { useFeature } from '@/modules/features/api/get-feature';
import { BriefPanel } from '@/modules/features/components/brief-panel';
import { DecisionList } from '@/modules/features/components/decision-list';
import { FeatureForm } from '@/modules/features/components/feature-form';
import { useTasks } from '@/modules/tasks/api/get-tasks';
import { TaskList } from '@/modules/tasks/components/task-list';
import { errorClass, mutedClass } from '@/utils/classes';

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="space-y-2">
      <h2 className="text-lg font-medium">{title}</h2>
      {children}
    </section>
  );
}

export function FeaturePage() {
  const id = Number(useParams().id);
  const feature = useFeature(id);
  const briefs = useBriefVersions(id);
  const decisions = useDecisions(id);
  const tasks = useTasks({ feature: id });
  const channels = useChannels();

  if (feature.isPending) return <p className={mutedClass}>Loading...</p>;
  if (feature.isError) return <p className={errorClass}>{feature.error.message}</p>;

  return (
    <div className="max-w-3xl space-y-8">
      <h1 className="text-xl font-semibold">{feature.data.name}</h1>
      <FeatureForm key={feature.data.id} feature={feature.data} />
      <Section title="Brief">
        {briefs.isError ? (
          <p className={errorClass}>{briefs.error.message}</p>
        ) : (
          <BriefPanel versions={briefs.data ?? []} />
        )}
      </Section>
      <Section title="Decisions">
        {decisions.isError ? (
          <p className={errorClass}>{decisions.error.message}</p>
        ) : (
          <DecisionList decisions={decisions.data ?? []} />
        )}
      </Section>
      <Section title="Tasks">
        {tasks.isError ? (
          <p className={errorClass}>{tasks.error.message}</p>
        ) : (
          <TaskList tasks={tasks.data ?? []} />
        )}
      </Section>
      <Section title="Channels">
        {channels.isError ? (
          <p className={errorClass}>{channels.error.message}</p>
        ) : (
          <ChannelList channels={channels.data?.filter((c) => c.featureId === id) ?? []} />
        )}
      </Section>
    </div>
  );
}

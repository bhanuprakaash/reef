import { Link } from 'react-router';
import { useChannels } from '@/modules/channels/api/get-channels';
import { NewChannelForm } from '@/modules/channels/components/new-channel-form';
import { useFeatures } from '@/modules/features/api/get-features';
import { AddFeatureForm } from '@/modules/features/components/add-feature-form';
import { useProjects } from '@/modules/projects/api/get-projects';
import { InboxLink } from '@/modules/inbox/components/inbox-link';
import { AddProjectForm } from '@/modules/projects/components/add-project-form';
import { errorClass, mutedClass } from '@/utils/classes';

export function Sidebar() {
  const projects = useProjects();
  const features = useFeatures();
  const channels = useChannels();

  const error = projects.error ?? features.error ?? channels.error;

  return (
    <nav className="space-y-4 text-sm">
      <InboxLink />
      <Link to="/tasks" className="block hover:underline">
        Tasks
      </Link>
      <div className="space-y-3">
        <h2 className="text-xs font-medium uppercase text-neutral-500">Projects</h2>
        {projects.isPending && <p className={mutedClass}>Loading...</p>}
        {error && <p className={errorClass}>{error.message}</p>}
        {projects.data?.map((p) => {
          const projectFeatures = features.data?.filter((f) => f.projectId === p.id) ?? [];
          const quickSessions =
            channels.data?.filter((c) => c.projectId === p.id && c.featureId === null) ?? [];
          return (
            <section key={p.id} className="space-y-1">
              <h3 className="font-medium" title={p.path}>
                {p.name}
              </h3>
              <ul className="space-y-1 pl-2">
                {projectFeatures.map((f) => (
                  <li key={f.id}>
                    <Link to={`/features/${f.id}`} className="hover:underline">
                      {f.name}
                    </Link>
                    <ul className="pl-3 text-neutral-500">
                      {channels.data
                        ?.filter((c) => c.featureId === f.id)
                        .map((c) => (
                          <li key={c.id}>
                            <Link to={`/channels/${c.id}`} className="hover:underline">
                              {c.name}
                            </Link>
                          </li>
                        ))}
                    </ul>
                  </li>
                ))}
              </ul>
              {quickSessions.length > 0 && (
                <div className="pl-2">
                  <h4 className="text-xs uppercase text-neutral-500">Quick sessions</h4>
                  <ul className="pl-3 text-neutral-500">
                    {quickSessions.map((c) => (
                      <li key={c.id}>
                        <Link to={`/channels/${c.id}`} className="hover:underline">
                          {c.name}
                        </Link>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              <div className="space-y-2 pl-2">
                <AddFeatureForm projectId={p.id} />
                <NewChannelForm projectId={p.id} featureId={null} />
              </div>
            </section>
          );
        })}
        <AddProjectForm />
      </div>
    </nav>
  );
}

import { createBrowserRouter } from 'react-router';
import { Layout } from '@/app/layout';
import { CapturePage } from '@/app/routes/capture';
import { ChannelPage } from '@/app/routes/channel';
import { ChannelsPage } from '@/app/routes/channels';
import { FeaturePage } from '@/app/routes/feature';
import { HomePage } from '@/app/routes/home';
import { InboxPage } from '@/app/routes/inbox';
import { ProjectPage } from '@/app/routes/project';
import { ReviewPage } from '@/app/routes/review';
import { SettingsPage } from '@/app/routes/settings';
import { TaskPage } from '@/app/routes/task';
import { TasksPage } from '@/app/routes/tasks';
import { TeamsPage } from '@/app/routes/teams';

export const router = createBrowserRouter([
  { path: '/capture', element: <CapturePage /> },
  {
    element: <Layout />,
    children: [
      { path: '/', element: <HomePage /> },
      { path: '/features/:id', element: <FeaturePage /> },
      { path: '/features/:id/review', element: <ReviewPage /> },
      { path: '/channels', element: <ChannelsPage /> },
      { path: '/channels/:id', element: <ChannelPage /> },
      { path: '/projects/:id', element: <ProjectPage /> },
      { path: '/inbox', element: <InboxPage /> },
      { path: '/tasks', element: <TasksPage /> },
      { path: '/tasks/:id', element: <TaskPage /> },
      { path: '/teams', element: <TeamsPage /> },
      { path: '/settings', element: <SettingsPage /> },
    ],
  },
]);

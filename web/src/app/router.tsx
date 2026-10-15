import { createBrowserRouter } from 'react-router';
import { Layout } from '@/app/layout';
import { CapturePage } from '@/app/routes/capture';
import { ChannelPage } from '@/app/routes/channel';
import { FeaturePage } from '@/app/routes/feature';
import { HomePage } from '@/app/routes/home';
import { InboxPage } from '@/app/routes/inbox';
import { ReviewPage } from '@/app/routes/review';
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
      { path: '/channels/:id', element: <ChannelPage /> },
      { path: '/inbox', element: <InboxPage /> },
      { path: '/tasks', element: <TasksPage /> },
      { path: '/teams', element: <TeamsPage /> },
    ],
  },
]);

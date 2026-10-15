import { createBrowserRouter } from 'react-router';
import { Layout } from '@/app/layout';
import { CapturePage } from '@/app/routes/capture';
import { ChannelPage } from '@/app/routes/channel';
import { FeaturePage } from '@/app/routes/feature';
import { HomePage } from '@/app/routes/home';
import { InboxPage } from '@/app/routes/inbox';
import { TasksPage } from '@/app/routes/tasks';

export const router = createBrowserRouter([
  { path: '/capture', element: <CapturePage /> },
  {
    element: <Layout />,
    children: [
      { path: '/', element: <HomePage /> },
      { path: '/features/:id', element: <FeaturePage /> },
      { path: '/channels/:id', element: <ChannelPage /> },
      { path: '/inbox', element: <InboxPage /> },
      { path: '/tasks', element: <TasksPage /> },
    ],
  },
]);

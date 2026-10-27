import { createBrowserRouter } from 'react-router';
import { Layout } from '@/app/layout';
import { CapturePage } from '@/app/routes/capture';
import { SpacePage } from '@/app/routes/space';
import { SpacesPage } from '@/app/routes/spaces';
import { FeaturePage } from '@/app/routes/feature';
import { HomePage } from '@/app/routes/home';
import { InboxPage } from '@/app/routes/inbox';
import { ProjectPage } from '@/app/routes/project';
import { ReviewPage } from '@/app/routes/review';
import { SettingsPage } from '@/app/routes/settings';
import { TeammatesPage } from '@/app/routes/teammates';
import { TeamsPage } from '@/app/routes/teams';

export const router = createBrowserRouter([
  { path: '/capture', element: <CapturePage /> },
  {
    element: <Layout />,
    children: [
      { path: '/', element: <HomePage /> },
      { path: '/features/:id', element: <FeaturePage /> },
      { path: '/features/:id/review', element: <ReviewPage /> },
      { path: '/spaces', element: <SpacesPage /> },
      { path: '/spaces/:id', element: <SpacePage /> },
      { path: '/projects/:id', element: <ProjectPage /> },
      { path: '/inbox', element: <InboxPage /> },
      { path: '/teams', element: <TeamsPage /> },
      { path: '/teammates', element: <TeammatesPage /> },
      { path: '/settings', element: <SettingsPage /> },
    ],
  },
]);

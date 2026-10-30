import { createBrowserRouter } from 'react-router';
import { Layout } from '@/app/layout';
import { CapturePage } from '@/app/routes/capture';
import { BoardPage } from '@/app/routes/board';
import { SpacePage } from '@/app/routes/space';
import { ConversationsPage, SpacesPage } from '@/app/routes/spaces';
import { WorkItemPage } from '@/app/routes/work-item';
import { HomePage } from '@/app/routes/home';
import { InboxPage } from '@/app/routes/inbox';
import { ProjectPage } from '@/app/routes/project';
import { ReviewPage } from '@/app/routes/review';
import { SettingsPage } from '@/app/routes/settings';
import { BotPage } from '@/app/routes/bot';
import { BotsPage } from '@/app/routes/bots';
import { TeamsPage } from '@/app/routes/teams';

export const router = createBrowserRouter([
  { path: '/capture', element: <CapturePage /> },
  {
    element: <Layout />,
    children: [
      { path: '/', element: <HomePage /> },
      { path: '/board', element: <BoardPage /> },
      { path: '/work-items/:id', element: <WorkItemPage /> },
      { path: '/work-items/:id/review', element: <ReviewPage /> },
      { path: '/spaces', element: <SpacesPage /> },
      { path: '/conversations', element: <ConversationsPage /> },
      { path: '/spaces/:id', element: <SpacePage /> },
      { path: '/projects/:id', element: <ProjectPage /> },
      { path: '/inbox', element: <InboxPage /> },
      { path: '/teams', element: <TeamsPage /> },
      { path: '/bots', element: <BotsPage /> },
      { path: '/bots/:id', element: <BotPage /> },
      { path: '/settings', element: <SettingsPage /> },
    ],
  },
]);

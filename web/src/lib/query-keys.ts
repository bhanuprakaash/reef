export const briefsKey = (workItemId: number) => ['briefs', workItemId];
export const diffKey = (workItemId: number, scope: string) => ['diff', workItemId, scope];
export const memoryRootKey = ['memory'];
export const pendingMemoryKey = [...memoryRootKey, 'pending'];
export const memoryKey = (scope: string) => [...memoryRootKey, scope];
export const inspectorKey = (spaceId: number) => ['inspector', spaceId];
export const skillsRootKey = ['skills'];
export const skillsKey = (projectId?: number) => [...skillsRootKey, projectId ?? 'all'];
export const skillLibraryKey = [...skillsRootKey, 'library'];
export const skillDraftsKey = [...skillsRootKey, 'drafts'];
export const workItemPrKey = (workItemId: number) => ['work-item-pr', workItemId];
export const providersKey = ['providers'];
export const repoConnectionsKey = ['repo-connections'];
export const connectionsKey = ['connections'];
export const availableConnectionsKey = (projectId?: number) => [
  ...connectionsKey,
  'available',
  projectId ?? 'all',
];

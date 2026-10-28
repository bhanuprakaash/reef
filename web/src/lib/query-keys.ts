export const briefsKey = (workItemId: number) => ['briefs', workItemId];
export const diffKey = (workItemId: number, scope: string) => ['diff', workItemId, scope];
export const memoryRootKey = ['memory'];
export const memoryKey = (scope: string) => [...memoryRootKey, scope];
export const inspectorKey = (spaceId: number) => ['inspector', spaceId];
export const skillsKey = (projectId: number) => ['skills', projectId];
export const workItemPrKey = (workItemId: number) => ['work-item-pr', workItemId];
export const providersKey = ['providers'];

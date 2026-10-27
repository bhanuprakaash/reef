export const briefsKey = (featureId: number) => ['briefs', featureId];
export const diffKey = (featureId: number, scope: string) => ['diff', featureId, scope];
export const memoryRootKey = ['memory'];
export const memoryKey = (scope: string) => [...memoryRootKey, scope];
export const inspectorKey = (channelId: number) => ['inspector', channelId];
export const skillsKey = (projectId: number) => ['skills', projectId];
export const featurePrKey = (featureId: number) => ['feature-pr', featureId];
export const providersKey = ['providers'];

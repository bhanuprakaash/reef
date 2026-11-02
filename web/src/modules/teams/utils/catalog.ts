import type { ConnectionInput } from '@/modules/teams/api/connections';

export type CatalogField = {
  key: string;
  label: string;
  placeholder?: string;
  optional?: boolean;
  secret?: boolean;
};

export type CatalogEntry = {
  id: string;
  name: string;
  description: string;
  url: (values: Record<string, string>) => string;
  fields?: CatalogField[];
  defaults?: Record<string, string>;
  clientIdRequired?: boolean;
  oauth?: { clientId: string; callbackPort: number };
  headers?: (values: Record<string, string>) => Record<string, string>;
};

export const catalog: CatalogEntry[] = [
  {
    id: 'gitlab',
    name: 'GitLab',
    description: 'Issues, merge requests and pipelines',
    url: ({ host }) => `https://${host}/api/v4/mcp`,
    fields: [{ key: 'host', label: 'GitLab host', placeholder: 'gitlab.com' }],
    defaults: { host: 'gitlab.com' },
  },
  {
    id: 'github',
    name: 'GitHub',
    description: 'Repositories, issues and pull requests',
    url: () => 'https://api.githubcopilot.com/mcp/',
    fields: [{ key: 'token', label: 'Personal access token', secret: true }],
    headers: ({ token }) => ({ Authorization: `Bearer ${token}` }),
  },
  {
    id: 'slack',
    name: 'Slack',
    description: 'Search and post in channels',
    url: () => 'https://mcp.slack.com/mcp',
    oauth: { clientId: '1601185624273.8899143856786', callbackPort: 3118 },
  },
  {
    id: 'atlassian',
    name: 'Atlassian',
    description: 'Jira issues and Confluence pages',
    url: () => 'https://mcp.atlassian.com/v2/mcp',
  },
  {
    id: 'notion',
    name: 'Notion',
    description: 'Pages and databases',
    url: () => 'https://mcp.notion.com/mcp',
  },
  {
    id: 'sentry',
    name: 'Sentry',
    description: 'Errors and performance issues',
    url: ({ organisation }) =>
      organisation ? `https://mcp.sentry.dev/mcp/${organisation}` : 'https://mcp.sentry.dev/mcp',
    fields: [{ key: 'organisation', label: 'Organisation slug', optional: true }],
  },
  {
    id: 'figma',
    name: 'Figma',
    description: 'Design files and components',
    url: () => 'https://mcp.figma.com/mcp',
  },
];

export function searchCatalog(query: string) {
  const q = query.trim().toLowerCase();
  return catalog.filter(
    (e) => e.name.toLowerCase().includes(q) || e.description.toLowerCase().includes(q),
  );
}

export function entryFields(entry: CatalogEntry): CatalogField[] {
  return entry.clientIdRequired
    ? [...(entry.fields ?? []), { key: 'clientId', label: 'OAuth client id' }]
    : (entry.fields ?? []);
}

export function catalogInput(entry: CatalogEntry, values: Record<string, string>): ConnectionInput {
  const merged = { ...entry.defaults };
  for (const [key, value] of Object.entries(values)) {
    if (value.trim() !== '') merged[key] = value.trim();
  }
  const oauth = entry.oauth ?? (merged.clientId ? { clientId: merged.clientId } : undefined);
  return {
    name: entry.id,
    url: entry.url(merged),
    ...(entry.headers && { headers: entry.headers(merged) }),
    ...(oauth && { oauth }),
  };
}

export function defaultUrl(entry: CatalogEntry) {
  return entry.url(entry.defaults ?? {});
}

export function catalogEntryFor(url: string | undefined) {
  if (!url) return undefined;
  try {
    const u = new URL(url);
    return catalog.find((e) => {
      const base = new URL(defaultUrl(e));
      return e.defaults?.host ? u.pathname === base.pathname : u.host === base.host;
    });
  } catch {
    return undefined;
  }
}

export const builtinTools = [
  'Read',
  'Grep',
  'Glob',
  'Edit',
  'Write',
  'Bash',
  'WebFetch',
  'WebSearch',
];

export const toolHints: Record<string, string> = {
  Read: 'Read files',
  Grep: 'Search file contents',
  Glob: 'Find files by name',
  Edit: 'Change part of a file',
  Write: 'Create or replace files',
  Bash: 'Run shell commands',
  WebFetch: 'Fetch a web page',
  WebSearch: 'Search the web',
};

export function mcpTool(server: string) {
  return `mcp__${server}__*`;
}

export function toggleTool(tools: string[], tool: string, on: boolean) {
  if (!on) return tools.filter((t) => t !== tool);
  return tools.includes(tool) ? tools : [...tools, tool];
}

export function unrestrictedBashBlocked(tool: string, write: boolean, tools: string[]) {
  return tool === 'Bash' && !write && !tools.includes(tool);
}

export function customTools(tools: string[], servers: string[]) {
  const shown = [...builtinTools, ...servers.map(mcpTool)];
  return tools.filter((t) => !shown.includes(t));
}

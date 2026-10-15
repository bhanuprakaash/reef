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

export function mcpTool(server: string) {
  return `mcp__${server}__*`;
}

export function toggleTool(tools: string[], tool: string, on: boolean) {
  if (!on) return tools.filter((t) => t !== tool);
  return tools.includes(tool) ? tools : [...tools, tool];
}

export function customTools(tools: string[], servers: string[]) {
  const shown = [...builtinTools, ...servers.map(mcpTool)];
  return tools.filter((t) => !shown.includes(t));
}

export type FileRef = { path: string; line?: number };

// Extensions a file reference may end in, with the highlight language for each ('' is plain text).
const extensions: Record<string, string> = {
  ts: 'typescript',
  tsx: 'tsx',
  js: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  jsx: 'jsx',
  go: 'go',
  py: 'python',
  rs: 'rust',
  java: 'java',
  md: 'markdown',
  json: 'json',
  yaml: 'yaml',
  yml: 'yaml',
  toml: 'toml',
  css: 'css',
  html: 'html',
  sql: 'sql',
  sh: 'bash',
  diff: 'diff',
  mod: '',
  sum: '',
  txt: '',
};

const names: Record<string, string> = {
  Makefile: '',
  Dockerfile: 'dockerfile',
  LICENSE: '',
  '.gitignore': '',
};

const ref = /^(\/?(?:[\w.@+-]+\/)*([\w.@+-]+))(?::(\d+)(?:[-:]\d+)?|#L(\d+)(?:-L?\d+)?)?$/;

function language(name: string): string | undefined {
  if (name in names) return names[name];
  const ext = /^.+\.(\w+)$/.exec(name)?.[1].toLowerCase();
  return ext === undefined ? undefined : extensions[ext];
}

export function fileLanguage(path: string): string {
  return language(path.split('/').at(-1) ?? '') ?? '';
}

// parseFileRef reads a whole string as a file path with an optional :line, :start-end or #Lline.
export function parseFileRef(text: string): FileRef | null {
  const found = ref.exec(text);
  if (!found) return null;
  const [, path, name, colon, hash] = found;
  if (/^[^/]*@/.test(path) || language(name) === undefined) return null;
  const line = Number(colon ?? hash);
  return line > 0 ? { path, line } : { path };
}

export type FoundRef = FileRef & { start: number; end: number };

// findFileRefs finds paths with a slash in plain text, leaving trailing punctuation outside.
export function findFileRefs(text: string): FoundRef[] {
  const refs: FoundRef[] = [];
  for (const found of text.matchAll(/[^\s()[\]{}<>"'`,;|]+/g)) {
    const token = found[0].replace(/[.:!?]+$/, '');
    const parsed = parseFileRef(token);
    if (parsed?.path.includes('/')) {
      refs.push({ ...parsed, start: found.index, end: found.index + token.length });
    }
  }
  return refs;
}

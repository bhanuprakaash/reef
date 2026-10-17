import type { HighlighterCore, LanguageInput, ThemedToken } from 'shiki/core';

const themes = { light: 'github-light-default', dark: 'github-dark-default' } as const;

export type Line = { content: string; style: Record<string, string> | undefined }[];

// Only these grammars ship. Each is its own chunk, fetched when a reply first uses it.
const grammars: Record<string, () => Promise<LanguageInput>> = {
  bash: () => import('shiki/langs/bash.mjs'),
  css: () => import('shiki/langs/css.mjs'),
  diff: () => import('shiki/langs/diff.mjs'),
  dockerfile: () => import('shiki/langs/dockerfile.mjs'),
  go: () => import('shiki/langs/go.mjs'),
  html: () => import('shiki/langs/html.mjs'),
  java: () => import('shiki/langs/java.mjs'),
  javascript: () => import('shiki/langs/javascript.mjs'),
  json: () => import('shiki/langs/json.mjs'),
  jsx: () => import('shiki/langs/jsx.mjs'),
  markdown: () => import('shiki/langs/markdown.mjs'),
  python: () => import('shiki/langs/python.mjs'),
  rust: () => import('shiki/langs/rust.mjs'),
  sql: () => import('shiki/langs/sql.mjs'),
  toml: () => import('shiki/langs/toml.mjs'),
  tsx: () => import('shiki/langs/tsx.mjs'),
  typescript: () => import('shiki/langs/typescript.mjs'),
  yaml: () => import('shiki/langs/yaml.mjs'),
};

const aliases: Record<string, string> = {
  js: 'javascript',
  ts: 'typescript',
  sh: 'bash',
  shell: 'bash',
  zsh: 'bash',
  yml: 'yaml',
  py: 'python',
  md: 'markdown',
  rs: 'rust',
  jsonc: 'json',
};

let loading: Promise<HighlighterCore> | null = null;

// The highlighter loads on the first fenced block, so a reply with no code pays nothing. The
// JavaScript regex engine avoids shipping the wasm one.
function highlighter() {
  loading ??= Promise.all([
    import('shiki/core'),
    import('shiki/engine/javascript'),
    import('shiki/themes/github-light-default.mjs'),
    import('shiki/themes/github-dark-default.mjs'),
  ]).then(([core, engine, light, dark]) =>
    core.createHighlighterCore({
      themes: [light, dark],
      langs: [],
      engine: engine.createJavaScriptRegexEngine(),
    }),
  );
  return loading;
}

function toLine(tokens: ThemedToken[]): Line {
  return tokens.map((token) => ({
    content: token.content,
    style: token.htmlStyle as Record<string, string> | undefined,
  }));
}

// highlight returns one entry per line with both themes as CSS variables (--shiki-light,
// --shiki-dark), or null for a language it does not ship.
export async function highlight(code: string, lang: string): Promise<Line[] | null> {
  const name = aliases[lang] ?? lang;
  const load = grammars[name];
  if (!load) return null;
  const hl = await highlighter();
  if (!hl.getLoadedLanguages().includes(name)) await hl.loadLanguage(await load());
  const { tokens } = hl.codeToTokens(code, { lang: name, themes, defaultColor: false });
  return tokens.map(toLine);
}

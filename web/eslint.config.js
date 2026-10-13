import { readdirSync } from 'node:fs';
import js from '@eslint/js';
import importPlugin from 'eslint-plugin-import';
import reactHooks from 'eslint-plugin-react-hooks';
import globals from 'globals';
import tseslint from 'typescript-eslint';

const modules = readdirSync('./src/modules');

export default tseslint.config(
  { ignores: ['dist'] },
  js.configs.recommended,
  tseslint.configs.recommended,
  reactHooks.configs.flat.recommended,
  {
    files: ['**/*.{ts,tsx}'],
    languageOptions: { globals: globals.browser },
    plugins: { import: importPlugin },
    settings: { 'import/resolver': { typescript: { project: './tsconfig.json' } } },
    rules: {
      'import/no-restricted-paths': [
        'error',
        {
          zones: modules.map((name) => ({
            target: `./src/modules/${name}`,
            from: './src/modules',
            except: [`./${name}`],
          })),
        },
      ],
    },
  },
  { files: ['*.config.{js,ts}'], languageOptions: { globals: globals.node } },
);

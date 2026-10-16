import { fileURLToPath } from 'node:url';
import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

// A page load of a client route such as /tasks also matches an API path. The Go
// server uses the same rule: a GET that accepts text/html gets the app.
const api = {
  target: 'http://127.0.0.1:7701',
  bypass: (req: { method?: string; headers: { accept?: string } }) =>
    req.method === 'GET' && req.headers.accept?.includes('text/html') ? '/index.html' : undefined,
};

const apiPaths = [
  'projects',
  'profile',
  'features',
  'channels',
  'messages',
  'turns',
  'decisions',
  'tasks',
  'proposals',
  'inbox',
  'health',
];

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: { proxy: Object.fromEntries(apiPaths.map((name) => [`/${name}`, api])) },
});

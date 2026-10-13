# React style

Condensed from [Bulletproof React](https://github.com/alan2207/bulletproof-react/tree/master/docs).
Where this file and the source differ, this file wins. Keep the web app small and organize code
around Reef areas.

## Stack

- Vite, React, TypeScript with `strict: true`, React Router and TanStack Query.
- Tailwind v4 with plain utility classes. Use the system font and `dark:` variants for dark mode.
- npm for packages and scripts.
- No Zod, form library or global store library. Use React state and URL state as described below.

## Structure

The app lives in `web/src`:

```text
app/          App setup, router and composition
components/   Shared UI components
lib/          Configured libraries, including the API client
modules/      Code grouped by Reef area
types/        Shared TypeScript types
utils/        Shared utility functions
```

Area code lives in `web/src/modules/<area>`, for example `modules/channels`,
`modules/features` and `modules/tasks`. Use `modules`, not `features`: Feature is a Reef
product entity. Add only the subfolders an area needs (`api`, `components`, `hooks`, `utils`).

- Name files and folders in kebab-case.
- Use `@/` imports for paths under `web/src`.
- Do not add barrel files; import the file you use directly.
- Modules do not import from other modules. Compose modules in `app/`; shared code belongs in
  `components/`, `lib/`, `types/` or `utils/`.
- Enforce module boundaries with ESLint `import/no-restricted-paths`.

## State

Keep state at its narrowest useful scope.

- Use `useState` for independent component state and `useReducer` when one action updates
  related state. Pass shared component state through props.
- Put navigable or shareable state in the URL using React Router.
- Server data belongs in TanStack Query, not component state.
- Do not add Redux, Zustand or another global store. There is no global client state library.

## API and live output

- Use one fetch-based API client in `web/src/lib/api-client.ts`.
- Declare each request once beside its query or mutation hook in the module's `api/` folder.
  Give each response an explicit TypeScript type.
- Name query keys after the resource, with identifying inputs as needed, such as
  `['channels', featureId]`.
- After a mutation, invalidate the matching query keys so views refresh from the server.
- Each channel receives live agent output over its own Server-Sent Events stream with
  `EventSource`. User messages are sent with HTTP `POST`.

## Checks

- `npm run typecheck` runs TypeScript (`tsc`).
- `npm run lint` runs ESLint. Prettier check must pass.
- Use Vitest for logic that is not UI. Prove UI behavior by driving the real app in a browser.

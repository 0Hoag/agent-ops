# Client code standards (React)

The client app lives in **`0Hoag/agent-ops-client`**, normally checked out as `../agent-ops-client` (ROADMAP 24/10/2026).
This standards reference remains here for cross-repository context.

**Source of truth: `~/Desktop/console`** (company React console). Copy its files and conventions; adapt names
only. Do **not** copy its `packages/*` workspace, host contract or coupling gates: they exist to split a
21-module app and are too heavy here.

| Topic | console file to follow |
| --- | --- |
| Package scripts, engines, packageManager | `package.json` (`dev: RSPACK_MODE=local rspack serve`, `build: tsc --noEmit && NODE_ENV=production RSPACK_MODE=production rspack build`, `typecheck`, `test: vitest run`, `preview`) |
| Node version | `.nvmrc` (22), engines `node >=20.19.0` |
| TypeScript | `tsconfig.json` (ES2020, bundler resolution, strict, noUnused*, `@/*` → `src/*`), `tsconfig.node.json` |
| Bundler + env | `rspack.config.ts` (dotenv: `.env`, `.env.<mode>`, `.env.local` for `local`; HTML template; React refresh) |
| Tests | `vitest.config.mts` |
| App composition | `src/app/` (register-<module>.tsx, paths.ts, route tree) |
| Module shape | `packages/fastchannel/src/{module.ts,route.ts,paths.ts,services/}` → here `src/modules/<m>/` |
| HTTP seam | `packages/fastchannel/src/services/client.ts`: a module reaches the API only through its `services/client.ts` |
| Data hooks | `packages/fastchannel/src/services/fastchannel.ts` (TanStack Query hooks + typed responses in one service file) |
| Code style | single quotes, semicolons, 2-space indent, doc comment on every exported function/type |

## Layout

```
src/
├── main.tsx  App.tsx
├── app/                        # router, paths.ts, register-<module>.tsx, layout/
├── modules/<name>/             # module.ts route.ts paths.ts services/{client.ts,<name>.ts,types.ts} components/<Name>/index.tsx <Page>.tsx
└── shared/                     # http client factory, ui, lib
```

## Rules

- TypeScript strict; every API response typed.
- Modules never import other modules; shared code goes to `src/shared/`.
- TanStack Query for data (array query keys), react-hook-form for forms, TanStack Table for tables.
- Tailwind for styling. Dense ERP dashboard: no record data in headers, one primary button per screen, no forced ALL-CAPS.
- Loading (skeleton), empty and error states on every screen that calls the API.
- ≤200 lines per file. Vitest for pure functions and hooks; a test must be shown to fail once.
- `APP_API_URL` from env (`.env.example` documents every variable).
- It must run: `pnpm build && pnpm preview` serves a page that shows real API data (`scripts/smoke.sh` in agent-ops).

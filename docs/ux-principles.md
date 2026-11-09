# UX principles (owner decision 2026-11-09)

The first real use of the dashboard was judged unusable: raw tables, unclear labels, wrong or empty numbers, and no
reason to open it instead of GitLab. Every client and dashboard-facing API issue in W18 follows this file.

## Why the product exists

GitLab/GitHub already show issues and merge requests in detail. Agent Ops must not copy them; it links out for detail.
It answers three questions GitLab cannot:

1. **What did AI work cost?** Per week, per merge request, per issue, per tool and model.
2. **Is it getting more efficient?** Cache hit rate, cost per merged MR, rework rate, CI first-pass rate.
3. **What needs attention now?** MRs stuck on CI or review, agent sessions that produced no MR, models with no price.

If a screen does not help answer one of these, it does not belong in the main navigation.

## Rules

- Main navigation has four items: **Overview**, **Work**, **Usage**, **Settings**. Coordinator pages (waves, worker
  controls, live logs, budgets, alert history) live under Settings > Advanced.
- Every page opens with one plain sentence or headline number that answers its question, then the evidence.
- No raw UUIDs, no ISO timestamps, no jargon. Use relative time with the exact time in a tooltip, `1.85M` not
  `1846116`, `$12.40` not `12.4`. Replace harness/wave/worker with tool/batch/agent, with a tooltip where needed.
  A run is named like "Claude - feat/hosting-files - 2 h ago".
- Filters are selects, search boxes and date presets (Today, 7 days, 30 days, Custom). Never a free-text box for an ID.
- Never show "Unknown". Show the action that fixes it ("Set a price for claude-sonnet-5-5").
- Every empty state has a next step button. Every error says what happened and how to retry.
- One primary action per screen. Calm and spacious: Inter or system font, 8 px grid, neutral grays and one accent;
  color only encodes status; charts use direct labels, not legends to decode; light and dark themes.
- Works from 390 px to wide desktop with no horizontal scroll.
- Numbers must be correct. A figure that cannot be computed shows how to make it computable, never a placeholder zero.

## Reference products to study (do not copy; if you cannot browse them, follow the rules above)

Linear (calm density, keyboard and command menu), Vercel usage and billing pages (big number, chart, table),
Stripe Dashboard (overview cards with sparklines and a date range), Langfuse and Helicone (LLM cost and usage
dashboards), PostHog (insight cards), shadcn/ui with Radix and Tailwind (components), Recharts (charts).

## Stack decision

The "dense ERP dashboard" guidance in `docs/client-standards.md` is replaced by this file. Allowed additions to the
client: Radix primitives, class-variance-authority, tailwind-merge, lucide-react and Recharts. Keep the existing
conventions (modules, TanStack Query, generated API client, <= 200 lines per file).

## Acceptance for every W18 UI issue

- `pnpm install --frozen-lockfile && pnpm typecheck && pnpm lint && pnpm test && pnpm build` exits 0.
- Playwright screenshots at 1280 px and 390 px are attached to the PR, with no horizontal scroll at 390 px.
- A real-browser run against the local compose stack shows the page with real data; a mutation control that breaks
  the behavior makes that run fail.

# CMS Frontend

Admin/CMS frontend for the IM platform. React 18 SPA.

## Rules (must follow)

- **UI primitives: always use Radix UI** (`@radix-ui/react-*`) for any interactive
  element with behavior/state — dialogs, dropdowns, selects, popovers, tooltips,
  switches, checkboxes, tabs, toasts, etc. Do **not** add another component library
  (MUI, Ant, Chakra, Headless UI, Mantine…) or hand-roll accessible widgets that
  Radix already provides.
- Style Radix with Tailwind + CVA, composed via `cn()`. Reuse the existing
  primitives in `src/components/ui/` before creating new ones; if you must create
  one, follow the `new-component` skill's pattern.
- Every surface must support light **and** dark (`dark:` variants) and reduced motion.

## Stack

- **Build:** Vite 5 + TypeScript 5 (strict), ESM (`"type": "module"`).
- **UI:** Tailwind CSS 3 + Radix UI primitives (shadcn-style), `lucide-react` icons.
- **Component styling:** `class-variance-authority` (CVA) for variants, `clsx` +
  `tailwind-merge` via the `cn()` helper in `src/lib/utils.ts`.
- **Motion:** `framer-motion` v11. Shared tokens live in `src/lib/motion.ts`.
- **Routing:** TanStack Router (`src/routes/index.tsx`).
- **Server state:** TanStack Query. **Client state:** Zustand (`src/store/`).
- **Data:** `graphql-request` + `axios` (`src/lib/graphql.ts`, `src/lib/api.ts`);
  live updates via SSE (`src/lib/sse.ts`).
- **Forms:** `react-hook-form` + `zod` (`@hookform/resolvers`).
- **DnD:** `@dnd-kit`.

## Commands

Package manager is **pnpm** (pinned via `packageManager` + Corepack). Use `pnpm`, not npm/yarn.

- `pnpm dev` — Vite dev server.
- `pnpm build` — `tsc --noEmit` then `vite build`.
- `pnpm type-check` — `tsc --noEmit`.
- `pnpm lint` — ESLint (`--max-warnings 0`).
- `pnpm install --frozen-lockfile` — reproducible install (use in CI).

pnpm settings live in `pnpm-workspace.yaml`: `minimumReleaseAge` (supply-chain delay)
and `allowBuilds` (only listed packages may run install scripts — pnpm blocks them by default).

There is no test runner configured.

## Layout

```
src/
  components/
    ui/      # reusable primitives (Button, Card, MotionButton, …)
    layout/  # AppShell, Header, Sidebar
  pages/     # route-level screens (lazy-loaded)
  routes/    # TanStack Router tree
  store/     # Zustand stores (auth, tenant)
  lib/       # utils, api, graphql, sse, motion
  types/     # shared types
  styles/    # globals.css (Tailwind layers + CSS vars)
```

## Conventions

- **Imports:** use the `@/` alias for `src/` (e.g. `@/lib/utils`). Configured in
  `tsconfig.json` and Vite.
- **Components:** `forwardRef` + `displayName`; variants via CVA; export the cva
  instance as `<name>Variants` when it may be reused. Compose classes with `cn()`.
- **Design tokens:** prefer existing Tailwind tokens over raw values — `brand-*`
  colors, `rounded-xl/2xl`, `shadow-card`/`shadow-glass`, `focus-visible:ring-brand-400`.
  CSS vars (`--sidebar-width`, `--header-height`, `--radius`) are in `globals.css`.
- **Animations:**
  - Micro-interactions (hover/press) → Tailwind transitions, or `MotionButton`
    for spring-based press.
  - Enter/exit + layout → framer-motion (`AnimatePresence`, `layout`).
  - Always import easing/spring/duration from `src/lib/motion.ts` — don't hard-code.
  - Always gate motion with `useReducedMotion()`.
  - `<Card>` fades in on mount and lifts on hover when `interactive`.
- **Glassmorphism + theming (light/dark):**
  - Theme is `class`-based (`dark` on `<html>`), driven by `src/store/theme.store.ts`
    (persisted, applied pre-paint via `initTheme()` in `main.tsx`). Toggle lives in the Header.
  - Frosted surfaces use the `.glass` / `.glass-card` / `.glass-card-elevated`
    utilities in `globals.css` — they already carry `dark:` variants and `backdrop-blur`.
    Prefer these over hand-rolled `bg-white` panels.
  - The ambient gradient background lives on `body` (light + dark) — that's what the
    glass surfaces blur over; keep panels translucent (`bg-white/70`, `dark:bg-slate-900/55`).
  - When adding raw markup, always pair light classes with a `dark:` variant
    (e.g. `text-slate-800 dark:text-slate-100`, `border-slate-100 dark:border-white/10`).
- **TypeScript is strict:** `noUnusedLocals` / `noUnusedParameters` are on — clean up
  unused code or the build fails.

## Automation in this repo

- A `PostToolUse` hook (`.claude/hooks/typecheck-lint.mjs`) lints the edited file and
  type-checks the project after every edit; fix reported errors before moving on.
- `/new-component` skill scaffolds a new `ui/` primitive.
- `ui-reviewer` subagent reviews components for accessibility + motion-safety.

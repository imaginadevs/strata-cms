# Studio Backlog

Pending implementations and follow-ups for the CMS admin frontend (`cms/frontend`).
Keep entries actionable: what, why, and enough context to pick up cold.

Related backend contract: `../backend/docs/relational-items.md`,
`../backend/docs/design-relational-io.md`, `../backend/docs/page-blocks-contract.md`.

---

## Now / high priority

### Verify item + subitems against a running backend
**Status:** Not started · **Why:** the nested editor type-checks but hasn't been
exercised end-to-end.
Run `npm run dev` with the backend up and confirm on a Website tenant:
- Editing a page loads its `pages_blocks` expanded (`?deep=`), reorder persists
  (`sort`), add/remove works, and one save round-trips (nested write).
- Removing a block row + saving deletes it (owned relation); the `blocks`
  definition is untouched.
- Create page with blocks in a single POST.
Files: `src/pages/Content.tsx`, `src/components/content/SubitemsEditor.tsx`,
`src/lib/relational.ts`.

### Fix ESLint config (repo-wide, blocking `npm run lint`)
**Status:** Not started · **Why:** ESLint 9 is installed but the project still uses
a legacy `.eslintrc`, so `npm run lint` errors out for everyone ("couldn't find
eslint.config.js"). Migrate to flat config (`eslint.config.js`) per the ESLint v9
migration guide. Pre-existing, unrelated to the relational work.

---

## Soon

### Block picker for `pages_blocks.block_id`
**Status:** Not started · **Why:** `block_id` renders as a plain UUID input because
it's a `uuid` field, not a `relation` field. Either (a) declare it as a `relation`
field on the backend blueprint so the existing `RelationInput` picker kicks in, or
(b) special-case a block-definition picker in `SubitemsEditor`. (a) is the general
fix and benefits every relation.

### Validate subitem required fields
**Status:** Not started · **Why:** `SubitemsEditor` currently passes empty
`fieldErrors`; only top-level required fields are validated before save. Add
per-row validation (required child fields) and surface errors inline in the row.

---

## Later (blocked on backend)

These depend on backend features tracked in `../backend/docs/BACKLOG.md`.

- **Dotted deep expansion** (`?deep=pages_blocks.block_id`) — inline nested
  definitions once the backend supports multi-level expansion.
- **Explicit-ops / pure-M2M editing** — a link/unlink id-array UI for many-to-many
  relations once the backend nested-write supports it (today only declarative O2M
  arrays are handled).
- **GraphQL-based reads** — if/when GraphQL gets nested resolvers, consider using it
  for the read path instead of REST `?deep=`.

---

## Notes / conventions

- Studio work is best done from a session rooted at `cms/frontend` so its
  `CLAUDE.md`, the `typecheck-lint` hook, `/new-component`, and `ui-reviewer` are
  active.
- Feedback loop until lint is fixed: `npm run type-check`.

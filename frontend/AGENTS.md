# Independent frontend rules

- This app has only a public HTTP API contract and a URL. It must build independently of the backend checkout.
- `src/api/openapi.json` is a vendored snapshot. `src/api/local-openapi.json` explicitly adopts the CLI orchestration contract; regenerate `local-schema.gen.ts` alongside normalization types and never edit it by hand. Refresh via `npm run contract:pull` and regenerate with `npm run contract:generate`; review adoption diffs. Never modify `schema.gen.ts` by hand.
- All API traffic belongs in `src/api/client.ts`; reuse its generated types. No server functions, server routes, filesystem reads, Go imports or sibling source paths.
- Keep TanStack Start SPA mode enabled. Route feature code and API calls execute in the browser; root shell must prerender without a running API. Production contains only `dist/client` and Nginx.
- Runtime browser configuration is `/config.js`, not a Docker DNS name. Never put secrets in browser configuration.
- Keep unknowns explicit, expose errors/conflicts, and retain synthetic coordinate labels. Do not persist raw-address answers in local storage.
- Check `npm run build`, `npm run typecheck` and `npm run test:e2e` against a temporary API overlay when changing the review flow.

## Material Design 3 is the default

- Before composing or changing frontend UI, ask “How does Material Design 3 solve this?” Prefer its official patterns in an opinionated way, including behavior, hierarchy, states, adaptive layouts, and accessibility.
- Read `docs/material-design-3.md` before frontend composition. It records researched sources, project mappings, and deliberate deviations; keep it current when the design system changes.
- Reuse `src/ui/material.tsx` and semantic M3 roles in `src/styles.css`. Do not add arbitrary screen-specific palettes or a competing design system. Change the seed through `scripts/generate-theme.mjs`, run `npm run theme:generate`, and commit the generated colors together.
- User requirements, offline operation, explicit unknowns, synthetic-data labels, and the independent HTTP-only frontend architecture remain authoritative. Document justified departures from M3.

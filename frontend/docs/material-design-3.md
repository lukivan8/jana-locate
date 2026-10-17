# Material Design 3 at Jana Address

Material Design 3 (M3) is the frontend's default design authority. Before composing a screen or inventing an interaction, ask **“How does Material Design 3 solve this?”** Choose its established pattern, then adapt it to the address review task. This applies to layout and behavior as well as appearance. A new library or personal visual preference does not replace this policy.

Research reviewed 16 October 2026. Start with [Google's Material Design 3](https://m3.material.io/). Its current evolution includes M3 Expressive. Our baseline uses stable M3 roles and patterns; expressive shapes and motion are deliberate product decisions, not a requirement to animate every control. Recheck official guidance when adding a component or updating this baseline.

## Philosophy and decision process

Design for people completing a task: submit addresses, understand uncertainty, verify building facts, and export results. Visual emphasis must explain that sequence. Color, shape, type, elevation, and motion form one system rather than independent decoration.

1. Define the person's task, information, and possible outcomes before selecting a component.
2. Look up the official M3 component and its guidelines, accessibility, and specifications. Read behavior, not only screenshots.
3. Choose the least surprising standard pattern and appropriate emphasis. Reuse `src/ui/material.tsx` and existing tokens.
4. Design empty, loading, populated, selected, disabled, validation, failure, and recovery states together.
5. Compose for compact, medium, and expanded windows; preserve meaning and keyboard access.
6. Verify the actual workflow, contrast, focus, text enlargement, and mobile layout in a browser.
7. If M3 does not cover the need, use native web semantics and the same foundations. Document the reason for a deviation here.

User instructions and the repository's API, privacy, synthetic-data, and offline requirements take precedence. Never hide uncertainty or imply a synthetic coordinate has been verified to achieve a cleaner interface.

## Implementation and ownership

This is a local React implementation of M3 patterns, not Google's component library or a claim of complete M3 conformance. Shared primitives live in `src/ui/material.tsx`; layout and component styling live in `src/styles.css`; generated palette roles live in `src/ui/colors.css`. Future repeated controls belong in this shared layer, not copied feature CSS.

Google's [Material Web repository](https://github.com/material-components/material-web) identifies its library as in maintenance mode. We retain native React/HTML controls for this small SPA. Avoid introducing a second competing design system. In particular, a library's “Material” name alone does not establish that its defaults implement M3.

## Color: meaning through roles

Use semantic `--md-sys-color-*` variables, never feature-specific hex values. Match foreground and container roles: `primary` / `on-primary`, `primary-container` / `on-primary-container`, `error-container` / `on-error-container`. Use `on-surface-variant` for secondary text and `outline` for field boundaries; `outline-variant` is for decorative dividers. [Material Web color guidance](https://material-web.dev/theming/color/) describes this role-based scheme and its CSS token convention.

Jana Post's approved brand green is `#117d01` (RGB 17, 125, 1), supplied by the user on 17 October 2026. It is both the palette seed and the exact `primary` role for links, focus and filled actions. Deliberate M3 deviation: the generator preserves this primary instead of substituting the generated tone; white `on-primary` retains WCAG AA contrast. Other roles and surface containers remain derived from this seed. `npm run theme:generate` uses Google's [Material Color Utilities](https://github.com/material-foundation/material-color-utilities) at build time to derive light roles. The generator adds neutral-tone surface containers. Commit generator and generated CSS together. Version 0.3.0 is pinned because 0.4.0's entry point failed under our Node environment due to an extensionless ESM import. Revalidate generation before upgrading.

The frontend uses only the light theme, as requested. The generator emits only light roles and `color-scheme: light`, including for native controls. System `prefers-color-scheme: dark` does not change the interface; no palette is obtained from a service. This fixed brand palette is not operating-system dynamic color. Any future personalized palette must regenerate coherent role pairs, retain error semantics, and pass contrast checks. Never derive colors from private address content.

Our mapping: filled primary actions move work forward; neutral tonal containers group content; tertiary containers call attention to synthetic data and uncertainty; error containers explain failures. Review status uses a green check for matched or manually reviewed records, and an amber warning icon otherwise. Each icon has an accessible name and a hover title. Amber warning roles are generated from a dedicated amber seed; they express operator attention separately from errors and the brand. Color never carries meaning alone. Check at least 4.5:1 for ordinary text and 3:1 for large text and meaningful control boundaries in both schemes. Disabled content is intentionally lower contrast and is never the only way to explain an unavailable action.

## Typography and content

Use the M3 type hierarchy rather than arbitrary sizes. Current role tokens include headline large (32/40), title large (22/28), body large (16/24), body medium (14/20), label large (14/20), and label medium (12/16), expressed as size/line-height in CSS pixels. Use headline for the page, title for section headings, body for descriptions, and label for controls. Extend the shared scale when a new role is needed. See the [official M3 typography scale](https://m3.material.io/styles/typography/type-scale-tokens).

Roboto is preferred when installed; Noto Sans and the local system font are explicit offline fallbacks. No font CDN is used. This means glyph metrics vary by machine; verify Cyrillic as well as English, wrapping, and 200% zoom. If exact Roboto rendering becomes a requirement, vendor licensed font files into this frontend instead of making runtime network requests.

Use short, sentence-case action labels with a verb: “Normalize addresses,” “Export JSON,” “Review.” Keep permanent field labels visible. Supporting text explains format and consequences; placeholders never replace labels. Error copy identifies the problem and a recovery action. Do not introduce claims about accuracy that have not been measured.

## Shape, surfaces, and elevation

The shared corner scale is 4, 8, 12, 16, 28 pixels and full. Assign shape by component: fields use 4px, status labels 8px, content panels 12px, and buttons full. Avoid unrelated radii within one component family. [M3 shape](https://m3.material.io/styles/shape/overview) is a hierarchy, not a command to make everything pill shaped.

Surface-container roles express grouping and depth. This workspace uses filled panels without decorative shadows. Reserve elevation for real separation, such as a floating action or overlapping menu; use a shared elevation token if introduced. A dark theme needs tonal separation as well as any shadow. Do not raise every section or put cards inside cards. See [M3 elevation](https://m3.material.io/styles/elevation/overview).

## Components and interaction

| Need | Default choice | Jana rule |
| --- | --- | --- |
| Complete the current task | Filled button | Normalize or confirm; strongest emphasis in its task region |
| Important secondary action | Outlined button | Export, open review, close editor |
| More prominent secondary action | Filled tonal button | Supported by `Button`; introduce only with a hierarchy reason |
| Low-priority inline action | Text button | Switch from uploaded JSON to text |
| Text or numeric entry | Outlined field | `TextField` / `TextArea`, permanent inset label and native input semantics |
| Independent on/off selection | Checkbox | Needs-review filter; native checked state and associated label |
| Read-only status | Noninteractive status label | Never style it as a clickable filter chip |
| Multi-column records | Semantic table | Preserve headers and horizontal scrolling in a named keyboard-focusable region |
| Persistent actionable failure | Inline error message | `role="alert"`; preserve input and show recovery guidance |
| Successful save | Inline status message | `role="status"`; do not steal focus |

The reusable button supports filled, tonal, outlined, and text variants. [Google's button guidance](https://material-web.dev/components/button/) explains these emphasis levels; elevated buttons should be reserved for contexts that need separation. M3 outlined inputs use labels and optional supporting text; consult [text-field guidance](https://material-web.dev/components/text-field/) before adding error, required, prefix, suffix, or trailing-icon behavior. Our labels remain inset at all times, including empty fields, a deliberate simplification of the floating-label behavior.

Use native disabled controls during requests and visible “Working…” feedback. State layers inherit the current foreground: hover 8%, focus and pressed 10%. Keyboard focus also has a visible outline; a state layer alone is insufficient. Web interaction uses a state overlay rather than a pointer-origin ripple. Controls have a minimum 48px height. Full-target checkbox labels make small native checkboxes easier to use.

For future components, consult the [M3 component catalog](https://m3.material.io/components) first. Use radio buttons for one-of-many choices, switches for immediately applied settings, chips for compact filtering, menus for contextual choices, dialogs for decisions requiring interruption, and snackbars for brief nonblocking feedback. Prefer an inline editor when a task needs continued access to the record. Navigation bars, rails, and drawers are for real top-level destinations; the three demonstration destinations below use persistent native links. Never add a FAB simply to make the app look Material.

## Adaptive composition

Use available window width, not device detection. Our breakpoints follow compact below 600px, medium 600–839px, and expanded from 840px. They are project decisions based on [M3 window-size classes](https://m3.material.io/foundations/layout/applying-layout/window-size-classes), not guarantees that every two-pane arrangement fits at every size.

Expanded windows show batch input beside its processing explanation, free input beside its location preview, and API endpoint navigation beside the reference. Compact and medium windows stack these in task order. Outer margins are 16px on compact and 24px otherwise. Spacing follows an 8px rhythm with 4px adjustments for labels and tight relationships. The editor uses four columns on expanded windows and two on smaller windows. Long addresses wrap; record and schema tables scroll within their containers rather than making the page wider. Keep all catalog information available on mobile.

Adapt navigation, panes, and actions to content as the product grows; do not merely shrink the desktop. Consider text zoom, touch, mouse, keyboard, right-to-left layouts, long translations, and virtual keyboards. See [M3 canonical layouts](https://m3.material.io/foundations/layout/canonical-examples/overview).

## Motion, accessibility, and trustworthy states

Motion explains change. Short state transitions use 150ms; the editor reveal uses 300ms with the M3 standard easing `(0.2, 0, 0, 1)`. These are the project's bounded subset of [M3 motion](https://m3.material.io/styles/motion/overview). Reduced-motion preference disables them. New expressive spring physics require an explicit usability purpose; no infinite decoration or staggered delays across operational content.

Use semantic headings, actual buttons, associated labels, a skip link, and natural reading order. Keyboard users must be able to normalize, review, edit, filter, and export. The offline map's SVG markers are informational; its adjacent location list provides named review buttons in the batch view, and the table provides the same review action. Coordinates remain readable outside the SVG. Preserve native forced-colors support. See [M3 accessible design](https://m3.material.io/foundations/accessible-design/overview).

Unknown, ambiguous, and unmatched are distinct from successful matches. Keep textual warnings, confidence, and synthetic-data disclosure. Do not use a success color to imply catalog verification. A save persists building facts only; delivery details stay in the session export. These domain constraints govern presentation.

## Definition of done for future work

- Reference the applicable M3 pattern; reuse or deliberately extend the shared primitives and role tokens.
- Review all applicable states and explain any departure from M3 in this document.
- Run build and typecheck with only this frontend available; shell prerender must not need the API.
- Run the Playwright review flow against a temporary API catalog overlay.
- Use the installed agent-browser guide, a task-specific session, fresh snapshots, desktop and 390×844 mobile checks, and final screenshots. Check console/errors and framework overlays.
- Inspect keyboard focus, touch targets, long content, light appearance under both system color preferences, text zoom, reduced motion, and contrast for affected controls. Automated checks assist but do not prove accessibility.

The baseline ships no external fonts, icon service, map tiles, theme service, or runtime design dependency. Add new assets locally and keep this application independently buildable.

## Baseline verification (16 October 2026)

`npm run typecheck`, `npm run build`, and the existing `npm run test:e2e` passed. The Playwright flow covered HTTP normalization, confirmation, persistence after reload, filtering, JSON upload, and exported contents. The API ran in the cached Go 1.24 container with an ephemeral `/tmp/catalog.json` overlay and explicit synthetic-catalog opt-in; native NLP models were unavailable, so this verifies the demo workflow rather than production model accuracy.

Agent-browser verified a populated queue and correction editor at 1440×1000 and 390×844, mobile horizontal scrolling, dark appearance, and reduced-motion behavior. The page stayed within the 390px viewport and action targets were at least 48px high. Browser errors were empty and no framework overlay appeared. Axe reported zero violations; SVG and horizontally clipped content produced incomplete contrast checks, so this is not a complete accessibility certification. Screenshots were inspected visually. The mechanical design detector's sole warning was Roboto's familiarity, accepted because the requested M3 typography takes precedence.

## Synthetic example browser

The example explorer extends the existing M3 selection-list and inline-detail pattern ([official list overview](https://m3.material.io/components/lists/overview), reviewed 16 October 2026). Native buttons in a semantic list expose selection through `aria-pressed`; eight records per page keep navigation bounded. A reusable `SelectField` uses the same outlined field roles and native web select behavior. Category, search and combination filters remain above the list. The selected address and result sit beside the list on expanded windows and below it on compact/medium windows. There is no modal or automatic API call on selection.

The new block uses Russian copy for the requested testing workflow. Existing English workspace labels remain. Hidden-character placeholders help inspection without modifying the underlying input. Only actual API output is shown, with explicit nulls and diagnostic warnings. The downloadable fixture and its inferred categories are explicitly synthetic. No expected answer or result cache is shipped.

Previous/next address controls keep single-record exploration in the detail pane; list selection scrolls that pane into view on compact/medium windows. The detail preview is an explicitly named group, and the parsed output is a named section with polite live updates. This avoids unsupported labels on generic containers. Verification covered 1440×1000 and 390×844, exact raw-string transport/export, failure/retry, empty input and filter states. Build/typecheck and the full Playwright review flow passed against a temporary overlay. Model-unavailable warnings were retained; model accuracy was not evaluated.

## Demonstration destinations (16 October 2026)

The workspace now has three real destinations: `/api` for contract and container boundaries, `/` for batch review, and `/geocoder` for arbitrary input. The persistent navigation follows M3 destination navigation with native links and a written active state. Numbering expresses the requested presentation order. On compact screens it becomes three equal touch targets with wrapped labels. It replaces the former single-screen assumption above.

API reference uses the requested Datadog-style endpoint navigation/content structure, while retaining our M3 palette, surfaces and controls. Schema tables come from the frontend's vendored OpenAPI contract; documentation does not import backend sources or make API requests during prerender. Expandable schema details and code scrolling use native web semantics. Synthetic request/response illustrations are labeled separately from live results; copying cURL does not execute it. No second design system or theme was introduced.

Batch input and the processing explanation precede the queue. Non-matched rows explicitly require an operator, with warnings, counts and an inline correction editor. The original synthetic example browser remains available in a disclosure. Selecting a review moves focus to the editor heading. Coordinates and building facts are still the only durable catalog data; delivery fields remain in the exported session.

Free input starts empty, sends the original text through the generated HTTP client, and measures the complete normalization request with `performance.now()`. Editing clears the previous result and pin. Unknown coordinates never receive a marker. The map is an offline country overview, not a street basemap; its local Natural Earth boundary and synthetic catalog coordinate caveat remain visible. No network tile/font service is used. Warnings describe unavailable models without claiming that a separate LLM corrector exists.

### Extension verification

Build and typecheck passed. All six Playwright tests passed in 13.3 seconds against a disposable API overlay with native libpostal and the live internal GLiNER service. Coverage included JSON upload, operator confirmation, export and persistence, synthetic examples, known/unknown free input, error recovery, endpoint schemas and OpenAPI download. This verifies the workflows with available models, not their accuracy on real addresses.

Agent-browser checked all three destinations at 1440×1000 and 390×844, including real batch normalization, the correction editor and live free input. No browser errors, framework overlays or horizontal document overflow appeared. Axe reported zero violations; incomplete SVG and clipped-table contrast checks were manually reviewed. Screenshots were visually inspected and saved under `/home/lukivan8/.codex/browser-artifacts/` as `jana-{api,batch,live}-{desktop,mobile}.png`, with additional editor captures. The independent finish review found no material issues. The detector's sole Roboto familiarity warning remains accepted under the established M3 typography policy.

## Settlement review

The existing inline correction editor now contains a native radio group with source-qualified settlement identities and their provinces. It uses M3 selection controls and existing semantic surface roles; no modal or new palette. Candidates load only for the selected record. Loading, no candidates, errors, retry, stale revisions and unsaved drafts remain explicit. Applying a selection reparses the record and replaces its saved manual corrections; the explanatory copy appears before the action. A city selection does not mark the whole record reviewed or confirm a building. The current choice is restored from the job journal.


## Normalization without geocoding (16 October 2026)

The current workflow follows the Jana Post normalization task: free input, component details and diagnostics. The country map, coordinate inputs, catalog counts and building confirmation action have been removed from free input and batch review. The shared status label `matched` now reads «Нормализован» and reflects complete unambiguous primary address fields, independently of geometry. Native form and radio controls retain the existing M3 roles. Manual field corrections, job restore and export remain available. Earlier map/confirmation descriptions above document the preceding implementation.

## Unified address composer

The main workspace now starts with one centered outlined multiline field and its normalize/import actions. File import is a draft step: TXT lines, JSON `raw_address` values (or strings), and CSV `raw_address` columns are shown as editable lines before submission. Structural quoting is removed; address punctuation and imported IDs remain. An unchanged import retains exact original records, including embedded newlines and blank addresses; edited lines retain IDs by position. Files are limited to 2 MiB and 1,000 records.

The user-requested full-height editor uses a native modal dialog only while expanded, with browser focus containment, Escape to collapse, and scroll locking. In its normal state it is an inline nonmodal dialog. Both states share the same field and draft. The processing explanation sits below the composer/results instead of beside the input. A single result opens the existing inline review editor automatically; multiple results retain the review table. Existing M3 palette, typography, buttons and field roles are unchanged.

## CLI uses the same page

The CLI embeds the frontend's built SPA, rather than maintaining a second HTML/JS/CSS interface. Its runtime config selects a typed local API adapter in `src/api/client.ts`; the workspace, wizard, review table and editor are the same React components. The local orchestration contract is explicitly adopted in `src/api/local-openapi.json`. The embedded artifact is refreshed with `make cli-web`, and `make check` verifies its source manifest.

Local mode adds a PostgreSQL source disclosure within the wizard, source/progress information, pagination of 50 rows, resume, and explicit publication for PostgreSQL runs. Snapshots remain read-only in the composer; reset starts a new draft. File runs retain terminal `jana land` delivery. Source NULL and exact IDs remain in SQLite and exports. The local server supplies config and permits only its own API requests; SPA bootstrap scripts use CSP hashes. Imported CLI links with `?run=...` open this same page. The standalone frontend build never reads CLI files at runtime or build time; contract and release artifact adoption are explicit repository operations.

## Simplified workspace controls

The main page omits job history, saved-job summaries, file-format details and the synthetic example disclosure. Stored jobs remain addressable by URL; the synthetic examples component and fixtures are retained for a future destination. The page no longer fetches job lists. Reset appears only once a normalization job exists, never while typing or importing a draft. The fullscreen control is a 48px icon button with an SVG, accessible name and native tooltip in both expand/collapse states. Existing processing explanations and M3 visual roles remain.


## Pipeline without GLiNER

The current backend uses native libpostal, explicit rules and immutable KATO/OSM references. The second model and its service have been removed. Workspace, free-input explanation and API architecture no longer advertise it. Existing saved warnings with `gliner_unavailable` are rendered as historical diagnostics; new results do not produce this warning or its confidence penalty. Earlier model measurements above are historical.


## Expanded synthetic corpus

The existing example list/detail components are reused on `/examples`, reached from the free-input page. This preserves the compact main wizard. The explorer contains 2000 unlabelled inputs, readable OSM attribution/source links and two downloadable batches of 1000. Category tags describe synthetic mutations, not measured correctness. Native filter, pagination and selection controls retain the current M3 tokens and eight visible examples per page. No expected answers or API calls during prerender are added.

## Expandable review rows

Review opens a detail row directly after its source record, spanning all five table columns. Only one editor is open at a time; the row action exposes its expanded state and toggles to «Свернуть». The row’s «Свернуть» action is the sole close control; collapsing and switching records retain the existing unsaved-change confirmation. On compact screens the table scrolls horizontally while the editor sticks to its left edge and fits the viewport, with a single column of fields.

A confirmed settlement becomes a compact summary with name, type, province and reference identity. «Исправить» reopens the radio choices with the saved selection restored. Failed saves retain the choices and error; only a successful selection collapses them. This extends the existing M3 inline editor and selection controls without introducing new visual roles.

Verified with build, typecheck and all 15 Playwright tests against a disposable API overlay. Agent-browser exercised selection and reselection at 1440×1000 and 390×844; the mobile editor had no document overflow, framework overlay or accessibility violations. The established Roboto font warning remains accepted under the M3 baseline.

The adopted CLI frontend snapshot also passed the standalone Docker smoke: race tests, vet, restart/persistence/export/publication checks and all 10 CLI browser scenarios at desktop and mobile sizes.

## Compact operator review (17 October 2026)

The queue presents one accessible status icon per row. Persisted operator review takes precedence over automatic normalization status for queue presentation and filtering; it does not alter the parser confidence or erase ambiguities. Save completion is reflected in the row icon and closed editor instead of a persistent banner. All diagnostic explanations use one amber list at the top of the editor, before settlement choices. An empty settlement candidate list is omitted; missing-city guidance remains in that diagnostic list. IDs, duplicate review headings/statuses, raw API JSON and verbose save guidance are removed from the review surface. Export retains the complete normalization result.

## File import column selection

CSV and XLSX share a native modal dialog for choosing an address column. Radio choices show column names and up to three sample values; a checkbox supports files without headers. Cancel and Escape preserve the current draft, including when the composer is expanded. A single column imports immediately, with common address headers removed. XLSX is converted locally to CSV from its first worksheet; the worksheet name is disclosed in the dialog. The spreadsheet parser is loaded only when an XLSX file is selected. The dialog uses existing M3 surface roles and buttons, with compact-width wrapping and native focus containment/restoration.

## CLI normalization progress (17 October 2026)

While normalization is running, the queue uses a determinate linear progress indicator, following [Material progress guidance](https://material-web.dev/components/progress/) and the [M3 anatomy](https://m3.material.io/components/progress-indicators/guidelines). The shared React primitive uses primary for the active segment, secondary-container for the remaining track, a 4px gap and a stop indicator. The caption preserves processed/total counts; the accessible progressbar exposes its name, range, current value and Russian value text. SSE updates drive the bar without simulated progress. It disappears when the run leaves normalization, retaining the compact completed queue. Reduced-motion preference disables the short value transition.

## Completed run timing (17 October 2026)

Completed CLI runs show total elapsed normalization time and average elapsed time per snapshot record below the run counts, using secondary text and the body-medium role. The same summary remains available after publication. The CLI stores a separate completion timestamp so operator edits and publication do not affect the measurement. Elapsed time includes snapshot creation, waiting and interruptions; average is elapsed/total, not isolated per-address latency. Legacy runs without a completion timestamp do not display an invented duration.

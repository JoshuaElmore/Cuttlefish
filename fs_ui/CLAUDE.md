# fs_ui

React 19 + TypeScript SPA for Cuttlefish. Built with Vite 8. The compiled output (`build/`) is served as static files by the Go API server.

## Architecture

### File layout

```
src/
  index.tsx           — entry point; wraps app in BrowserRouter
  App.tsx             — root component: auth state machine, sidebar, route outlet
  api.ts              — typed fetch wrappers for all Go API calls
  types.ts            — TypeScript interfaces mirroring Go models
  theme.ts            — "Industry" blueprint design tokens (colors, ramps, fonts)
  format.ts           — formatBytes / formatNumber / formatDate helpers
  icons.tsx           — thin-stroke (Lucide-style) inline SVG icon set

  pages/
    LoginPage.tsx        — handles both local (form) and OIDC (redirect) auth modes
    SplashPage.tsx       — fallback route (*), no sidebar
    FileBrowserPage.tsx  — breadcrumb + table/treemap toggle + stat cards + detail panel
    UserBrowserPage.tsx  — combined user+group storage table (Type tag column)
    SearchPage.tsx       — rule-builder query UI over POST /api/search,
                           with a per-rule NOT dropdown and a results column picker
    ScanHistoryPage.tsx  — fs_indexer + fs_aggregator run history, from GET /api/scans

  components/
    Blueprint.tsx       — shared wireframe primitives: BlueprintFrame (corner marks),
                           PulseDot, IndeterminateBar, Tag, SegmentedToggle,
                           shared input/button styles
    Header.tsx          — File Browser breadcrumb ("/ segment / segment / …")
    FileList.tsx        — sortable table of directory entries (File Browser, table mode)
    TreemapView.tsx     — size-proportional box layout (File Browser, treemap mode)
    DetailPanel.tsx     — shared 380px slide-in right panel (entry or user selection),
                           used by FileBrowserPage, UserBrowserPage and SearchPage

  hooks/
    useFileSystem.ts    — navigation state: currentPath, entries, selectedItem, isLoading, error, sortConfig
    useWindowedRows.ts  — renders only the table rows near the viewport (Search results)
```

Visual design is the "Industry" blueprint system: steel-blue, Barlow/Barlow Condensed, square corners with "+" registration marks on every card (`BlueprintFrame`), no shadows.

**Theming.** `theme.ts` exports each token as a `var(--cf-*)` string; the real light and dark values are CSS custom properties in `index.css`, selected by `data-theme` on `<html>`. Components keep writing `theme.border` in inline styles and the browser resolves the colour at paint time — so switching themes needs no context, no prop drilling and no re-render. Consequences worth knowing:

- **Never hardcode a colour in a component.** A literal like `rgba(29,31,32,0.5)` is invisible in dark mode. If a token is missing, add one (`textFaint`, `dangerSoft` were added for exactly this).
- The dark ramps are *inverted*, not darkened: `neutral100`/`accent100` stay the background end and `neutral800`/`accent800` the text end, so existing pairings keep their contrast direction.
- Preference is stored in `localStorage` under `cuttlefish.theme` (`'light' | 'dark'`), and an inline script in `index.html` applies it before first paint — without it a dark-mode user gets a white flash on every load. That script duplicates `storedThemeMode()`/`systemThemeMode()` logic by necessity; keep the two in step.
- With nothing stored, the app follows `prefers-color-scheme` and keeps following it live (`matchMedia` listener in `App.tsx`). The first explicit toggle ends that.

**There is no Settings page.** An earlier `SettingsPage.tsx` mirrored `fs_config.yml` in a form but never persisted anything — `fs_api/config.go` loads that file once at process start and has no write-back API, and the form surfaced secrets (DB password, session secret, OIDC client secret) to the browser. It was removed rather than left as scaffolding. Config is edited on disk; a real save endpoint would be a separate, security-sensitive change.

### Auth flow

`App.tsx` owns a three-state machine: `'checking' | 'authenticated' | 'unauthenticated'`.

1. On mount, fetches `GET /auth/me`. 200 → `authenticated`; 401 → `unauthenticated`.
2. While `checking`, renders a blank screen (avoids flash of login page).
3. When `unauthenticated`, renders `<LoginPage onLogin={...} />` full-screen.
4. `LoginPage` fetches `GET /auth/mode` to decide which UI to show:
   - `"local"` → username/password form that `POST /auth/login`
   - `"oidc"` → single "Sign in with SSO" link that navigates to `/auth/oidc/start`
5. On success, calls `onLogin()` which sets parent state to `authenticated`, unmounting `LoginPage` and rendering the full app.
6. Logout button in the sidebar calls `POST /auth/logout` then sets state back to `unauthenticated`.

### Data flow

`api.ts` is the single point of contact with the backend:

```
fsApi.listEntries(path)          → GET /api/list?path=…&include_stats=true
fsApi.getEntryStats(path, type)  → GET /api/file/stats?path=… or /api/dir/stats?path=…
```

`useFileSystem` hook wraps these calls and manages all navigation state. `FileBrowserPage` consumes the hook and passes slices of its state down to child components — no global state manager is used.

**Requests are cancelled, not just ignored.** `useFileSystem` keeps an `AbortController` per request kind and aborts the previous one before issuing the next. Listing `/usr` takes longer than listing the small directory clicked after it, so without this the slower reply lands second and the browser shows a directory the user already navigated away from, with the breadcrumb naming the other one. An abort is not an error — the catch returns early on `signal.aborted` rather than reporting a failure.

The hook exposes `error`, which `FileBrowserPage` renders as a banner. It previously swallowed failures into `console.error` and set the selection to `null`, so a failed lookup was indistinguishable from a row that just refused to be selected. `api.ts` throws `ApiError` carrying the HTTP status, so a 404 (this path isn't in the index) can be worded differently from a 500 without parsing message text; when the error banner is up, the "This directory is empty" placeholder is suppressed rather than contradicting it.

`UserBrowserPage` fetches `fsApi.listIdentityStats('uid')` and `('gid')` in parallel (no hook, since that data is self-contained and not navigated) and merges them into one table. `ScanHistoryPage` polls `fsApi.listScans()` every 30s, showing both scan types (`scan_type: 'indexer' | 'aggregator'`) with a `status: 'running' | 'success' | 'failed'` tag; `App.tsx`'s sidebar polls the same endpoint (`listScans(5)`) to show the "Scan running" status card whenever any of the most recent sessions has `status === 'running'`.

`DetailPanel` (used by File Browser, User & Group Usage and Search) takes a `DetailSelection` — `{ kind: 'entry', entry, siblingsTotal? }` or `{ kind: 'user', user }`. For a selected directory it fetches that directory's children itself (`fsApi.listEntries`) to build the top-4-by-size breakdown bar; for a selected file it needs the caller-supplied `siblingsTotal` (sum of the current listing) to show "% of directory" — callers without that context (Search results) get "No aggregate breakdown available" instead. The breakdown section's heading changes with what it's showing: "Contents breakdown" for a directory, "Share of parent directory" for a file, "Breakdown" otherwise.

**Large result sets are windowed.** `SearchPage` renders only the rows near the viewport via `useWindowedRows`, with spacer `<tr>`s standing in for the height of everything above and below. Above `WINDOW_THRESHOLD` (300) rows this is the difference between a usable page and an unusable one — measured at 6 000 rows with 4× CPU throttling, worst frame during:

| interaction | all rows rendered | windowed |
|---|---|---|
| select a row | 88 ms | 17 ms |
| tick/untick a column | 723 ms | 17 ms |
| re-sort by a header | 160 ms | 17 ms |

17 ms is one frame — the interactions stopped dropping any. At 10 000 rows (`MAX_LIMIT`) the numbers are identical, because cost no longer scales with the result count.

Things that were tried and did **not** help, so don't reach for them again: `table-layout: fixed` with an explicit colgroup moved a column toggle 683 ms → 679 ms, because the cost is React reconciling and the browser mutating thousands of `<tr>`/`<td>` nodes, not the table sizing algorithm. `content-visibility` has the same problem — it skips layout and paint but the nodes still get created. Only rendering fewer rows addresses it.

Row heights are **measured, not assumed**. The Path cell wraps, so the same result set is 37 px per row at 1440 px wide and a mix of 37/55/73 px at 900 px. `useWindowedRows` starts from `ROW_HEIGHT_ESTIMATE` and replaces it with the real height as each row is scrolled into view; a `ResizeObserver` on the scroller throws all measurements away when the width changes, since every one of them was taken at the old wrap points.

Windowing costs browser find-in-page and select-all-copy over off-screen rows. That is why it switches on only above the threshold — and why "Export CSV" writes from `results`, never from the DOM.

**Memoisation in the results table is load-bearing, not decoration.** `ResultRow` is `React.memo`'d so selecting a row re-renders the two rows whose highlight changed instead of every row on screen. That only works while its props keep stable identities: `activeColumns` and `cellStyles` are `useMemo`'d on the column set, `onSelect` is the raw `setSelectedPath` setter, and per-cell style objects are built once per column rather than spread inline per cell. Adding an inline arrow or a fresh object to `ResultRow`'s props silently defeats all of it — the page keeps working and just gets slow again.

The same rule applies elsewhere: `FileList` memoises its sort and `TreemapView` its whole squarify layout, both keyed to the data rather than redone on each parent render, because selecting an item re-renders the component that holds the list.

**In-flight search feedback.** While `POST /api/search` is outstanding, the line between the toolbar and the results table swaps the match count for `IndeterminateBar` plus a live elapsed counter, and Run search and the sortable column headers go inert (a second request would race the first, and the later response would win regardless of which query it answered).

The bar is deliberately **indeterminate**. The search is one request whose server-side progress nothing reports — there is no row-count stream, no cursor position, nothing to divide — so a filling bar would be animating a number the client invented. Don't "improve" it into a percentage without a real progress source behind it.

The counter is its own `ElapsedTimer` component rather than state on `SearchPage`, because it ticks at 10 Hz and the previous search's results are still mounted below: hoisting that state would re-render the whole table ten times a second to move one number. `STATUS_SLOT` pins the line's height to the taller of its two states so the table doesn't jump when a search starts or ends. The sweep animation lives in `index.css` as `cf-indeterminate` (inline styles can't declare keyframes, same as `cf-pulse`) and its translate percentages are relative to the *segment's* width, not the track's.

`SearchPage`'s "Show text" panel serializes the whole query — rules, sort, limit, visible columns — to JSON via `serializeQuery`, and `parseQuery` reads it back and runs it. JSON rather than a readable DSL because the text has to round-trip exactly (regex values, paths with spaces) with no hand-written parser to disagree with the writer. `parseQuery` rejects bad structure but clamps individual values to the same allowlists the UI uses, so a query saved by an older build still loads.

`SearchPage`'s result columns come from one `COLUMNS` table at the top of the file, each entry carrying its own `cell` (table) and `csv` (export) renderer — so the CSV always contains exactly the columns on screen, in the same order, and the two can't drift. A `sortKey` marks the columns the API can sort by (`searchSortColumns` in `fs_api/search.go`); `permissions` has none and renders an inert header. The visible set lives in `localStorage` under `cuttlefish.search.columns`, validated against `COLUMNS` on load so a stale key from an older build is dropped rather than crashing a render. Per-rule negation is a `negate` boolean on `SearchRule`, not a second family of `not_*` operators.

For any `entry` selection, a "Size & activity" block sits above the breakdown: a directory gets "This item" (its own `mtime`/`atime`/`ctime`, one line each) stacked above "Contents (N items)" — same three labels, but each expands to an "Oldest: …" / "Newest: …" pair straight off that entry's `aggregates` (`mtime_first`/`mtime_last` etc. from `dir_stats`, not re-derived from the fetched children). Full panel width throughout so full timestamps don't wrap. A file gets just the "This item" section. `UserStats` selections don't get this block.

### Routing

```
/           → redirect to /browser
/browser    → FileBrowserPage
/users      → UserBrowserPage
/search     → SearchPage
/history    → ScanHistoryPage
*           → SplashPage
```

**There is no landing page.** `/` redirects straight to `/browser` (`<Navigate replace>`, so it leaves no history entry to bounce back through). An earlier `HomePage.tsx` sat at `/` with a "Welcome to Cuttlefish" blurb and two buttons that only led where the sidebar already leads; it was one click between signing in and the only thing anyone came for. Because `/` matches no `NAV_ITEMS` path, it also rendered *without* the sidebar, so the first screen after login had no navigation on it at all.

`react-router-dom` v7. The Go server returns `index.html` for any path not matching `/api/`, `/auth/`, or a real static file, so deep links and reloads work correctly.

The active tab in the sidebar is derived from `useLocation()` — no separate tab state.

### Types

`types.ts` mirrors the Go models exactly:

| TypeScript | Go |
|---|---|
| `Entry` | `FileInfo` |
| `UserStats` | `UserStats` |
| `DirAggregates` | `DirAggregates` |
| `ScanSession` | `ScanSession` |

`file_type` values: `1` = file, `2` = directory, `3` = symlink, `0` = other — socket, FIFO or device node (same as Go). `typeLabel` in `format.ts` is the single renderer for these, shared by `SearchPage`'s Type column and `DetailPanel`'s Type metadata row.

Everywhere the UI branches on type it tests `=== 2` (directory) rather than `=== 1` (file), because types `3` and `0` have to fall on the non-directory side: `api.ts`'s `getEntryStats` routes them to `/api/file/stats`, which accepts anything that isn't a directory.

## Build

Use the root Makefile from the repo root:

```bash
make build-ui     # npm install + npm run build, then copies build/ into fs_api/ui/
make run-api      # builds ui, api, and starts the server
make all          # builds everything
```

For local UI development without rebuilding the full stack:

```bash
cd fs_ui && npm run start     # dev server on :5173, proxies /api and /auth to :8080
cd fs_ui && npm run preview   # preview the production build locally
```

Dev proxy is configured in `vite.config.ts` — `/api` and `/auth` requests are forwarded to `http://localhost:8080` so the Go server handles auth and data during development.

## Build hygiene

`make build-ui` deletes `fs_api/ui/build` before copying the new one. Vite hashes each bundle into its filename, so a plain `cp -r` over the top left every previously built `assets/index-*.js` in place — dead weight the Go server still served on request, accumulating one bundle per build.

`fs_api` reads `index.html` **once at startup** (`renderIndexHTML`, for the `ui.title` substitution). Rebuilding the UI under a running server therefore leaves it serving the old bundle's filename: restart `fs_api` after `make build-ui` or you are testing the previous build.

## Key dependencies

Runtime (`dependencies`) is only what ships in the bundle:

- `react` / `react-dom` v19
- `react-router-dom` v7
- `web-vitals` v6 — performance metrics (optional, no-op unless a callback is passed)

Everything else is `devDependencies`: `vite` v8 + `@vitejs/plugin-react` v6, `typescript`, the `@types/*` packages and `@testing-library/*`. They were all in `dependencies` before, which claimed the type checker and the test libraries were part of the shipped app.

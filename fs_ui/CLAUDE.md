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
    useFileSystem.ts    — navigation state: currentPath, entries, selectedItem, isLoading, sortConfig
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

`UserBrowserPage` fetches `fsApi.listIdentityStats('uid')` and `('gid')` in parallel (no hook, since that data is self-contained and not navigated) and merges them into one table. `ScanHistoryPage` polls `fsApi.listScans()` every 30s, showing both scan types (`scan_type: 'indexer' | 'aggregator'`) with a `status: 'running' | 'success' | 'failed'` tag; `App.tsx`'s sidebar polls the same endpoint (`listScans(5)`) to show the "Scan running" status card whenever any of the most recent sessions has `status === 'running'`.

`DetailPanel` (used by File Browser, User & Group Usage and Search) takes a `DetailSelection` — `{ kind: 'entry', entry, siblingsTotal? }` or `{ kind: 'user', user }`. For a selected directory it fetches that directory's children itself (`fsApi.listEntries`) to build the top-4-by-size breakdown bar; for a selected file it needs the caller-supplied `siblingsTotal` (sum of the current listing) to show "% of directory" — callers without that context (Search results) get "No aggregate breakdown available" instead. The breakdown section's heading changes with what it's showing: "Contents breakdown" for a directory, "Share of parent directory" for a file, "Breakdown" otherwise.

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

## Key dependencies

- `react` / `react-dom` v19
- `react-router-dom` v7
- `vite` v8 + `@vitejs/plugin-react` v6 — build tooling
- `web-vitals` v4 — performance metrics (optional, no-op unless a callback is passed)
- `typescript` v5

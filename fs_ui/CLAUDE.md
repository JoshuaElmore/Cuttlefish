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
    HomePage.tsx         — landing page after login (route "/", no sidebar)
    SplashPage.tsx       — fallback route (*), no sidebar
    FileBrowserPage.tsx  — breadcrumb + table/treemap toggle + stat cards + detail panel
    UserBrowserPage.tsx  — combined user+group storage table (Type tag column)
    SearchPage.tsx       — rule-builder query UI over POST /api/search
    ScanHistoryPage.tsx  — fs_indexer + fs_aggregator run history, from GET /api/scans
    SettingsPage.tsx     — fs_config.yml-shaped form; preview only, does not persist (see note below)

  components/
    Blueprint.tsx       — shared wireframe primitives: BlueprintFrame (corner marks),
                           PulseDot, Tag, SegmentedToggle, shared input/button styles
    Header.tsx          — File Browser breadcrumb ("root › segment › …")
    FileList.tsx        — sortable table of directory entries (File Browser, table mode)
    TreemapView.tsx     — size-proportional box layout (File Browser, treemap mode)
    DetailPanel.tsx     — shared 380px slide-in right panel (entry or user selection),
                           used by FileBrowserPage, UserBrowserPage and SearchPage

  hooks/
    useFileSystem.ts    — navigation state: currentPath, entries, selectedItem, isLoading, sortConfig
```

Visual design is the "Industry" blueprint system: light steel-blue theme, Barlow/Barlow Condensed, square corners with "+" registration marks on every card (`BlueprintFrame`), no shadows. Tokens live in `theme.ts`.

**Settings is not wired to a backend.** `fs_config.yml` has no write-back API (`fs_api/config.go` loads it once at process start), and the form surfaces secrets (DB password, session secret, OIDC client secret) — turning it into a real save/run-now endpoint is a separate, security-sensitive change. `SettingsPage.tsx` is preview scaffolding: it holds local state only, and "Save changes" / "Run now" show a brief inline notice instead of persisting anything.

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

For any `entry` selection, a "Size & activity" block sits above the breakdown: a directory gets "This item" (its own `mtime`/`atime`/`ctime`, one line each) stacked above "Contents (N items)" — same three labels, but each expands to an "Oldest: …" / "Newest: …" pair straight off that entry's `aggregates` (`mtime_first`/`mtime_last` etc. from `dir_stats`, not re-derived from the fetched children). Full panel width throughout so full timestamps don't wrap. A file gets just the "This item" section. `UserStats` selections don't get this block.

### Routing

```
/           → HomePage
/browser    → FileBrowserPage
/users      → UserBrowserPage
/search     → SearchPage
/history    → ScanHistoryPage
/settings   → SettingsPage
*           → SplashPage
```

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

`file_type` values: `1` = file, `2` = directory (same as Go).

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

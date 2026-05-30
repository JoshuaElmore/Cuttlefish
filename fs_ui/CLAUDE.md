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
  theme.ts            — design token object (colors, spacing constants)

  pages/
    LoginPage.tsx       — handles both local (form) and OIDC (redirect) auth modes
    HomePage.tsx        — landing/splash after login
    SplashPage.tsx      — fallback route (*)
    FileBrowserPage.tsx — main file explorer: header, file list, detail panel
    UserBrowserPage.tsx — user/group storage breakdown table

  components/
    Header.tsx          — path breadcrumb + navigate-to input
    FileList.tsx        — sortable table of directory entries
    FileDetails.tsx     — metadata panel for a selected file or directory
    UserBrowser.tsx     — sortable table of user/group stats

  hooks/
    useFileSystem.ts    — navigation state: currentPath, entries, selectedItem, isLoading, sortConfig
```

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

`UserBrowserPage` fetches `/api/user/list` and `/api/group/list` directly (no hook), since that data is self-contained and not navigated.

### Routing

```
/           → HomePage
/browser    → FileBrowserPage
/users      → UserBrowserPage
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

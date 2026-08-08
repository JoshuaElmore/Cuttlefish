import React, { useEffect, useState } from 'react';
import { Routes, Route, Navigate, useNavigate, useLocation } from 'react-router-dom';
import {
  theme, ThemeMode, applyThemeMode, storeThemeMode, storedThemeMode, systemThemeMode,
} from './theme';
import { fsApi } from './api';
import { ScanSession } from './types';
import { BlueprintFrame, PulseDot } from './components/Blueprint';
import { FolderIcon, UsersIcon, SearchIcon, HistoryIcon, SunIcon, MoonIcon } from './icons';
import SplashPage from './pages/SplashPage';
import FileBrowserPage from './pages/FileBrowserPage';
import UserBrowserPage from './pages/UserBrowserPage';
import SearchPage from './pages/SearchPage';
import ScanHistoryPage from './pages/ScanHistoryPage';
import LoginPage from './pages/LoginPage';

const NAV_ITEMS = [
  { id: 'browser', path: '/browser', label: 'File Browser', icon: FolderIcon },
  { id: 'users', path: '/users', label: 'User & Group Usage', icon: UsersIcon },
  { id: 'search', path: '/search', label: 'Advanced Search', icon: SearchIcon },
  { id: 'history', path: '/history', label: 'Scan History', icon: HistoryIcon },
] as const;

const CuttlefishExplorer: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();

  type AuthState = 'checking' | 'authenticated' | 'unauthenticated';
  const [authState, setAuthState] = useState<AuthState>('checking');
  const [runningScan, setRunningScan] = useState<ScanSession | null>(null);

  // index.html already applied the mode before first paint; this just reads back
  // the same decision so the button renders the right icon.
  const [themeMode, setThemeMode] = useState<ThemeMode>(() => storedThemeMode() ?? systemThemeMode());

  const toggleTheme = () => {
    const next: ThemeMode = themeMode === 'dark' ? 'light' : 'dark';
    setThemeMode(next);
    applyThemeMode(next);
    storeThemeMode(next);
  };

  // Until the user picks a side, follow the OS — someone whose machine flips to
  // dark in the evening shouldn't have to touch this.
  useEffect(() => {
    if (storedThemeMode() !== null) return;
    const mq = window.matchMedia?.('(prefers-color-scheme: dark)');
    if (!mq) return;
    const onChange = (ev: MediaQueryListEvent) => {
      if (storedThemeMode() !== null) return;
      const next: ThemeMode = ev.matches ? 'dark' : 'light';
      setThemeMode(next);
      applyThemeMode(next);
    };
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, []);

  useEffect(() => {
    fetch('/auth/me')
      .then(r => setAuthState(r.ok ? 'authenticated' : 'unauthenticated'))
      .catch(() => setAuthState('unauthenticated'));
  }, []);

  useEffect(() => {
    if (authState !== 'authenticated') return;
    let cancelled = false;
    const poll = () => fsApi.listScans(5).then(sessions => {
      if (!cancelled) setRunningScan(sessions.find(s => s.status === 'running') ?? null);
    }).catch(() => { /* sidebar status is best-effort */ });
    poll();
    const interval = setInterval(poll, 30000);
    return () => { cancelled = true; clearInterval(interval); };
  }, [authState]);

  // Rendered in the sidebar footer where there is one, and floated top-right on
  // the screens without a sidebar (login, splash) so the control is never
  // out of reach.
  const themeToggle = (floating: boolean) => (
    <div
      onClick={toggleTheme}
      title={themeMode === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}
      style={{
        display: 'inline-flex', alignItems: 'center', gap: 6, padding: '5px 9px', cursor: 'pointer',
        border: `1px solid ${theme.border}`, color: theme.textMuted, fontSize: 12,
        background: theme.bg,
        ...(floating ? { position: 'fixed' as const, top: 16, right: 16, zIndex: 50 } : {}),
      }}
    >
      {themeMode === 'dark' ? <SunIcon size={13} /> : <MoonIcon size={13} />}
      {themeMode === 'dark' ? 'Light' : 'Dark'}
    </div>
  );

  if (authState === 'checking') {
    return <div style={{ height: '100vh', background: theme.bg }} />;
  }

  if (authState === 'unauthenticated') {
    return (
      <>
        <LoginPage onLogin={() => setAuthState('authenticated')} />
        {themeToggle(true)}
      </>
    );
  }

  const activeNav = NAV_ITEMS.find(item => location.pathname === item.path)?.id ?? null;
  const elapsedLabel = (() => {
    if (!runningScan) return '';
    const started = new Date(runningScan.started_at * 1000);
    const mins = Math.floor((Date.now() / 1000 - runningScan.started_at) / 60);
    return `Started ${started.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} · ${mins}m elapsed`;
  })();

  return (
    <div style={{
      display: 'flex', height: '100vh', fontFamily: theme.fontBody,
      background: theme.bg, color: theme.text, overflow: 'hidden', fontSize: 14,
    }}>
      {activeNav && (
        <div style={{ width: 236, flex: 'none', background: theme.surface, borderRight: `1px solid ${theme.border}`, display: 'flex', flexDirection: 'column' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '22px 20px 18px' }}>
            <BlueprintFrame style={{ width: 34, height: 34, flex: 'none', background: theme.accent, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <span style={{ fontFamily: theme.fontHeading, fontWeight: 700, fontSize: 17, color: theme.bg }}>C</span>
            </BlueprintFrame>
            <div>
              <div style={{ fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 18, letterSpacing: '0.2px', lineHeight: 1.1 }}>Cuttlefish</div>
              <div style={{ fontSize: 10, fontWeight: 600, letterSpacing: '0.14em', textTransform: 'uppercase', color: theme.textMuted }}>Admin</div>
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 2, padding: '8px 12px', flex: 1 }}>
            {NAV_ITEMS.map(item => {
              const isActive = activeNav === item.id;
              const Icon = item.icon;
              return (
                <div
                  key={item.id}
                  onClick={() => navigate(item.path)}
                  style={{
                    display: 'flex', alignItems: 'center', gap: 10, padding: '10px 12px', cursor: 'pointer', fontSize: 13.5,
                    border: isActive ? `1px solid ${theme.accent500}` : '1px solid transparent',
                    background: isActive ? theme.accent100 : 'transparent',
                    color: isActive ? theme.accent800 : theme.neutral700,
                    fontWeight: isActive ? 600 : 400,
                  }}
                >
                  <Icon size={17} color={isActive ? theme.accent : theme.neutral600} />
                  <span>{item.label}</span>
                </div>
              );
            })}
          </div>

          {runningScan && (
            <BlueprintFrame style={{ margin: '10px 14px 4px', padding: 12 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 7, fontSize: 11, fontWeight: 600, letterSpacing: '0.05em', textTransform: 'uppercase', color: theme.textMuted2 }}>
                <PulseDot />
                Scan running
              </div>
              <div style={{ fontSize: 12, color: theme.textMuted2, marginTop: 6 }}>{elapsedLabel}</div>
            </BlueprintFrame>
          )}

          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, padding: '10px 20px 16px' }}>
            <div
              onClick={() => fetch('/auth/logout', { method: 'POST' }).then(() => setAuthState('unauthenticated'))}
              style={{ fontSize: 12, color: theme.textMuted, cursor: 'pointer' }}
            >
              Sign out
            </div>
            {themeToggle(false)}
          </div>
        </div>
      )}

      {!activeNav && themeToggle(true)}

      <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
        <Routes>
          {/* No landing page: a signed-in user starts in the File Browser. */}
          <Route path="/" element={<Navigate to="/browser" replace />} />
          <Route path="/browser" element={<FileBrowserPage />} />
          <Route path="/users" element={<UserBrowserPage />} />
          <Route path="/search" element={<SearchPage />} />
          <Route path="/history" element={<ScanHistoryPage />} />
          <Route path="*" element={<SplashPage />} />
        </Routes>
      </div>
    </div>
  );
};

export default CuttlefishExplorer;

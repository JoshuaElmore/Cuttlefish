// "Industry" design tokens — steel-blue blueprint/wireframe system.
// Each token is a `var(--cf-*)` reference; the actual light and dark values live
// in index.css, keyed off `data-theme` on <html>. Components keep using
// `theme.border` exactly as before — the browser resolves the colour at paint
// time, so switching themes needs no context, no props and no re-render.
export const theme = {
  bg: 'var(--cf-bg)',
  surface: 'var(--cf-surface)',
  text: 'var(--cf-text)',
  textMuted: 'var(--cf-text-muted)',
  textMuted2: 'var(--cf-text-muted2)',
  textFaint: 'var(--cf-text-faint)',
  border: 'var(--cf-border)',
  borderSoft: 'var(--cf-border-soft)',
  danger: 'var(--cf-danger)',
  dangerSoft: 'var(--cf-danger-soft)',

  accent: 'var(--cf-accent)',

  neutral100: 'var(--cf-neutral-100)',
  neutral200: 'var(--cf-neutral-200)',
  neutral300: 'var(--cf-neutral-300)',
  neutral400: 'var(--cf-neutral-400)',
  neutral500: 'var(--cf-neutral-500)',
  neutral600: 'var(--cf-neutral-600)',
  neutral700: 'var(--cf-neutral-700)',
  neutral800: 'var(--cf-neutral-800)',
  neutral900: 'var(--cf-neutral-900)',

  accent100: 'var(--cf-accent-100)',
  accent200: 'var(--cf-accent-200)',
  accent300: 'var(--cf-accent-300)',
  accent400: 'var(--cf-accent-400)',
  accent500: 'var(--cf-accent-500)',
  accent600: 'var(--cf-accent-600)',
  accent700: 'var(--cf-accent-700)',
  accent800: 'var(--cf-accent-800)',
  accent900: 'var(--cf-accent-900)',

  fontHeading: "'Barlow Condensed', system-ui, sans-serif",
  fontBody: "'Barlow', system-ui, sans-serif",
};

// Bar colors for the detail panel's size-breakdown segmented bar, brightest to dimmest.
export const breakdownColors = [theme.accent, theme.accent400, theme.neutral400, theme.neutral300];

export const gridTemplate = 'minmax(200px, 2fr) 120px 150px 150px';

// ── Light/dark preference ───────────────────────────────────────────────────
// Stored in localStorage, per browser — there is no server-side user profile to
// hang it on, and it is a per-device choice anyway (a laptop and a wall display
// signed in as the same admin want different answers).
export type ThemeMode = 'light' | 'dark';

export const THEME_STORAGE_KEY = 'cuttlefish.theme';

export const systemThemeMode = (): ThemeMode =>
  typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';

// null means "never chosen" — the app follows the OS until the user picks.
export const storedThemeMode = (): ThemeMode | null => {
  try {
    const raw = localStorage.getItem(THEME_STORAGE_KEY);
    return raw === 'light' || raw === 'dark' ? raw : null;
  } catch {
    return null;
  }
};

export const applyThemeMode = (mode: ThemeMode) => {
  document.documentElement.setAttribute('data-theme', mode);
};

export const storeThemeMode = (mode: ThemeMode) => {
  try {
    localStorage.setItem(THEME_STORAGE_KEY, mode);
  } catch {
    /* private mode — the choice just won't survive the session */
  }
};

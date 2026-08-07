// "Industry" design tokens — steel-blue blueprint/wireframe system.
// Mirrors the token sheet from the bound Claude Design project
// (_ds/industry-*/styles.css): light ground, square corners, hairline borders.
export const theme = {
  bg: '#f2f2f3',
  surface: '#e9e9ea',
  text: '#1d1f20',
  textMuted: 'rgba(29,31,32,0.55)',
  textMuted2: 'rgba(29,31,32,0.7)',
  border: 'rgba(29,31,32,0.16)',
  borderSoft: 'rgba(29,31,32,0.08)',
  danger: '#c0392b',

  accent: '#5980a6',

  neutral100: '#f5f5f8',
  neutral200: '#e7e7ea',
  neutral300: '#d4d4d7',
  neutral400: '#b7b7ba',
  neutral500: '#98989b',
  neutral600: '#7a7a7d',
  neutral700: '#5d5d60',
  neutral800: '#424244',
  neutral900: '#2b2b2d',

  accent100: '#eef6ff',
  accent200: '#d6ebff',
  accent300: '#b5d9fd',
  accent400: '#94bce3',
  accent500: '#749dc4',
  accent600: '#597ea3',
  accent700: '#416180',
  accent800: '#2c455d',
  accent900: '#1d2d3d',

  fontHeading: "'Barlow Condensed', system-ui, sans-serif",
  fontBody: "'Barlow', system-ui, sans-serif",
};

// Bar colors for the detail panel's size-breakdown segmented bar, brightest to dimmest.
export const breakdownColors = [theme.accent, theme.accent400, theme.neutral400, theme.neutral300];

export const gridTemplate = 'minmax(200px, 2fr) 120px 150px 150px';

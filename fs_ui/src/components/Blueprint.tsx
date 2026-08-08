import React from 'react';
import { theme } from '../theme';

// Shared "Industry" design-system primitives: the wireframe corner-registration
// frame every card/figure/status-block wears, plus small building blocks
// (pulse dot, tag, segmented toggle) reused across pages.

const cornerBase: React.CSSProperties = {
  position: 'absolute',
  width: 10,
  height: 10,
  color: theme.textFaint,
};
const cornerV: React.CSSProperties = { position: 'absolute', left: 4, top: 0, width: 1, height: '100%', background: 'currentColor' };
const cornerH: React.CSSProperties = { position: 'absolute', top: 4, left: 0, width: '100%', height: 1, background: 'currentColor' };

const Corner: React.FC<{ pos: React.CSSProperties }> = ({ pos }) => (
  <i style={{ ...cornerBase, ...pos }}>
    <i style={cornerV} />
    <i style={cornerH} />
  </i>
);

export const CornerMarks: React.FC = () => (
  <>
    <Corner pos={{ top: -6, left: -6 }} />
    <Corner pos={{ top: -6, right: -6 }} />
    <Corner pos={{ bottom: -6, left: -6 }} />
    <Corner pos={{ bottom: -6, right: -6 }} />
  </>
);

export const BlueprintFrame: React.FC<{ style?: React.CSSProperties; children: React.ReactNode }> = ({ style, children }) => (
  <div style={{ position: 'relative', border: `1px solid ${theme.border}`, ...style }}>
    <CornerMarks />
    {children}
  </div>
);

export const PulseDot: React.FC<{ size?: number }> = ({ size = 7 }) => (
  <span style={{
    width: size, height: size, borderRadius: '50%', background: theme.danger, flex: 'none',
    animation: 'cf-pulse 1.6s ease-in-out infinite', display: 'inline-block',
  }} />
);

// Indeterminate progress bar. Deliberately not a percentage: the work it covers
// is a single request whose server-side progress nothing reports, so a filling
// bar would be inventing a number. This says "still working" and nothing more.
export const IndeterminateBar: React.FC<{ height?: number; style?: React.CSSProperties }> = ({ height = 3, style }) => (
  <div
    role="progressbar"
    aria-label="Loading"
    style={{ position: 'relative', overflow: 'hidden', height, width: '100%', background: theme.neutral300, ...style }}
  >
    <div style={{
      position: 'absolute', top: 0, bottom: 0, left: 0, width: '26%',
      background: theme.accent, animation: 'cf-indeterminate 1.3s ease-in-out infinite',
    }} />
  </div>
);

export const Tag: React.FC<{ tone?: 'accent' | 'neutral' | 'danger'; children: React.ReactNode; style?: React.CSSProperties }> = ({ tone = 'neutral', children, style }) => {
  const toneStyle: React.CSSProperties =
    tone === 'accent' ? { background: theme.accent100, color: theme.accent800 } :
    tone === 'danger' ? { background: theme.dangerSoft, color: theme.danger } :
    { background: theme.neutral100, color: theme.neutral800 };
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', fontSize: 11, padding: '3px 9px', ...toneStyle, ...style }}>
      {children}
    </span>
  );
};

export const SegmentedToggle = <T extends string>({ options, value, onChange }: {
  options: { value: T; label: string }[];
  value: T;
  onChange: (v: T) => void;
}) => (
  <div style={{ display: 'inline-flex', overflow: 'hidden', border: `1px solid ${theme.border}`, flex: 'none' }}>
    {options.map(opt => (
      <div
        key={opt.value}
        onClick={() => onChange(opt.value)}
        style={{
          padding: '7px 16px', fontSize: 13, cursor: 'pointer',
          background: value === opt.value ? theme.accent : 'transparent',
          color: value === opt.value ? theme.bg : theme.text,
        }}
      >
        {opt.label}
      </div>
    ))}
  </div>
);

export const inputStyle: React.CSSProperties = {
  minHeight: 34,
  padding: '5px 10px',
  fontSize: 13,
  background: theme.surface,
  border: `1px solid ${theme.border}`,
  color: theme.text,
  fontFamily: theme.fontBody,
};

export const btnPrimaryStyle: React.CSSProperties = {
  display: 'inline-flex', alignItems: 'center', justifyContent: 'center', gap: 6,
  cursor: 'pointer', fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 14,
  background: theme.accent, color: theme.bg, border: `1px solid ${theme.accent}`, padding: '8px 18px',
};

export const btnSecondaryStyle: React.CSSProperties = {
  display: 'inline-flex', alignItems: 'center', justifyContent: 'center', gap: 6,
  cursor: 'pointer', fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 14,
  background: 'transparent', color: theme.text, border: `1px solid ${theme.border}`, padding: '8px 18px',
};

export const btnGhostStyle: React.CSSProperties = {
  display: 'inline-flex', alignItems: 'center', gap: 6, cursor: 'pointer',
  color: theme.accent700, fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 14,
  background: 'transparent', border: 'none', padding: 0,
};

import React from 'react';

// Thin-stroke Lucide-style icon set (stroke-width 1.5), matching the paths used
// in the bound "Industry" design system. One component per icon rather than a
// path-table lookup, so each call site stays a plain, typeable JSX element.
export interface IconProps {
  size?: number;
  color?: string;
  strokeWidth?: number;
  style?: React.CSSProperties;
}

const svg = (size: number, color: string, strokeWidth: number, style: React.CSSProperties | undefined, children: React.ReactNode) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" style={{ flex: 'none', ...style }}>
    {children}
  </svg>
);

export const FolderIcon: React.FC<IconProps> = ({ size = 16, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z" />);

export const FileIcon: React.FC<IconProps> = ({ size = 16, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" /><path d="M13 2v6h6" /></>);

export const UsersIcon: React.FC<IconProps> = ({ size = 16, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <>
    <path d="M17 20v-1a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4v1" /><circle cx="9" cy="7" r="3.2" />
    <path d="M22 20v-1a3.5 3.5 0 0 0-2.5-3.35" /><path d="M16 3.4a3.2 3.2 0 0 1 0 6.2" />
  </>);

export const UserIcon: React.FC<IconProps> = ({ size = 16, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <><circle cx="12" cy="8" r="3.4" /><path d="M5 20v-1a5 5 0 0 1 5-5h4a5 5 0 0 1 5 5v1" /></>);

export const SearchIcon: React.FC<IconProps> = ({ size = 16, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <><circle cx="10.5" cy="10.5" r="6.5" /><path d="M20 20l-4.35-4.35" /></>);

export const HistoryIcon: React.FC<IconProps> = ({ size = 16, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <><circle cx="12" cy="12" r="9" /><path d="M12 7v5l4 2" /></>);

export const SettingsIcon: React.FC<IconProps> = ({ size = 16, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <><circle cx="12" cy="12" r="3" /><path d="M12 3v2M12 19v2M4.2 7.8l1.7 1M18.1 15.2l1.7 1M4.2 16.2l1.7-1M18.1 8.8l1.7-1M3 12h2M19 12h2" /></>);

export const ChevronRightIcon: React.FC<IconProps> = ({ size = 13, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <path d="M9 6l6 6-6 6" />);

export const XIcon: React.FC<IconProps> = ({ size = 16, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <path d="M6 6l12 12M18 6L6 18" />);

export const TrashIcon: React.FC<IconProps> = ({ size = 14, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13" />);

export const PlusIcon: React.FC<IconProps> = ({ size = 14, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <path d="M12 5v14M5 12h14" />);

export const CopyIcon: React.FC<IconProps> = ({ size = 14, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <><rect x="8" y="8" width="12" height="12" rx="1" /><path d="M5 15V5a1 1 0 0 1 1-1h10" /></>);

export const RefreshCwIcon: React.FC<IconProps> = ({ size = 14, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <>
    <path d="M20 11A8 8 0 0 0 6.3 6.3L4 8.6" /><path d="M4 4v4.6h4.6" />
    <path d="M4 13a8 8 0 0 0 13.7 4.7L20 15.4" /><path d="M20 20v-4.6h-4.6" />
  </>);

export const SunIcon: React.FC<IconProps> = ({ size = 16, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <>
    <circle cx="12" cy="12" r="4" />
    <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
  </>);

export const MoonIcon: React.FC<IconProps> = ({ size = 16, color = 'currentColor', strokeWidth = 1.5, style }) =>
  svg(size, color, strokeWidth, style, <path d="M20 14.5A8.2 8.2 0 0 1 9.5 4a8.3 8.3 0 1 0 10.5 10.5z" />);

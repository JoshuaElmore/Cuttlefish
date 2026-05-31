import React from 'react';
import { theme } from '../theme';

interface HeaderProps {
  currentPath: string;
  searchPath: string;
  setSearchPath: (val: string) => void;
  onNavigate: (path: string) => void;
}

const Header: React.FC<HeaderProps> = ({ currentPath, searchPath, setSearchPath, onNavigate }) => {
  const segments = currentPath.split('/').filter(Boolean);
  const breadcrumbs = [
    { label: '/', path: '/' },
    ...segments.map((seg, i) => ({
      label: seg,
      path: '/' + segments.slice(0, i + 1).join('/'),
    })),
  ];

  return (
    <>
      <div style={{
        padding: '24px',
        background: 'rgba(0,0,0,0.3)',
        borderBottom: `1px solid ${theme.border}`,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between'
      }}>
        <div style={{ width: '48px' }} />
        <div style={{ display: 'flex', gap: '8px' }}>
          <input
            style={{
              padding: '8px 12px', borderRadius: '6px', border: `1px solid ${theme.border}`,
              background: 'rgba(0,0,0,0.3)', color: 'white', outline: 'none', fontSize: '14px', width: '250px'
            }}
            value={searchPath}
            onChange={e => setSearchPath(e.target.value)}
            placeholder="Jump to path..."
            onKeyDown={e => e.key === 'Enter' && onNavigate(searchPath)}
          />
          <button
            onClick={() => onNavigate(searchPath)}
            style={{
              padding: '8px 16px', borderRadius: '6px', border: 'none',
              background: theme.accentPurple, color: 'white', fontWeight: 600, cursor: 'pointer'
            }}
          >
            Go
          </button>
        </div>
      </div>
      <div style={{
        padding: '10px 16px',
        background: 'rgba(0,0,0,0.1)',
        borderBottom: `1px solid ${theme.border}`,
        fontFamily: 'monospace',
        fontSize: '13px',
        display: 'flex',
        alignItems: 'center',
        flexWrap: 'wrap',
        gap: '2px',
      }}>
        {breadcrumbs.map((crumb, i) => {
          const isLast = i === breadcrumbs.length - 1;
          return (
            <React.Fragment key={crumb.path}>
              {i > 1 && <span style={{ color: theme.textMuted, userSelect: 'none' }}>/</span>}
              <span
                onClick={isLast ? undefined : () => onNavigate(crumb.path)}
                style={{
                  color: isLast ? theme.accentBlue : theme.textMuted,
                  cursor: isLast ? 'default' : 'pointer',
                  padding: '2px 5px',
                  borderRadius: '4px',
                  transition: 'background 0.15s, color 0.15s',
                }}
                onMouseEnter={isLast ? undefined : e => {
                  (e.currentTarget as HTMLElement).style.background = 'rgba(255,255,255,0.08)';
                  (e.currentTarget as HTMLElement).style.color = theme.textMain;
                }}
                onMouseLeave={isLast ? undefined : e => {
                  (e.currentTarget as HTMLElement).style.background = 'transparent';
                  (e.currentTarget as HTMLElement).style.color = theme.textMuted;
                }}
              >
                {crumb.label}
              </span>
            </React.Fragment>
          );
        })}
      </div>
    </>
  );
};

export default Header;

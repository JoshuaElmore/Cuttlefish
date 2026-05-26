import React from 'react';
import { theme } from '../theme';

interface HeaderProps {
  currentPath: string;
  searchPath: string;
  setSearchPath: (val: string) => void;
  onNavigate: (path: string) => void;
}

const Header: React.FC<HeaderProps> = ({ currentPath, searchPath, setSearchPath, onNavigate }) => {
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
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <div style={{ 
            width: 32, height: 32, borderRadius: 8, 
            background: `linear-gradient(135deg, ${theme.accentPurple}, ${theme.accentBlue})`,
            display: 'flex', alignItems: 'center', justifyContent: 'center', fontWeight: 'bold'
          }}>🦑</div >
          <h2 style={{ margin: 0, fontSize: '1.25rem', fontWeight: 700 }}>Cuttlefish</h2>
        </div >
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
        </div >
      </div >
      <div style={{ padding: '16px', background: 'rgba(0,0,0,0.1)', borderBottom: `1px solid ${theme.border}`, fontFamily: 'monospace', fontSize: '13px', color: theme.accentBlue }}>
        {currentPath}
      </div >
    </>
  );
};

export default Header;

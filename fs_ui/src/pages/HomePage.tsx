import React from 'react';
import { theme } from '../theme';
import { useNavigate } from 'react-router-dom';

const HomePage: React.FC = () => {
  const navigate = useNavigate();

  return (
    <div style={{ 
      flex: 1, 
      display: 'flex', 
      flexDirection: 'column', 
      alignItems: 'center', 
      justifyContent: 'center', 
      background: theme.panelBg,
      backdropFilter: 'blur(10px)',
      color: theme.textMain,
      textAlign: 'center',
      padding: '40px'
    }}>
      <div style={{ 
        fontSize: '80px', 
        marginBottom: '24px', 
        filter: 'drop-shadow(0 0 20px rgba(0,150,255,0.3))' 
      }}>🦑</div>
      <h1 style={{ 
        fontSize: '32px', 
        fontWeight: 700, 
        marginBottom: '16px' 
      }}>Welcome to Cuttlefish</h1>
      <p style={{ 
        fontSize: '16px', 
        color: theme.textMuted, 
        maxWidth: '500px', 
        marginBottom: '32px', 
        lineHeight: '1.6' 
      }}>
        Manage and explore your high-performance filesystem index. 
        Quickly browse directories, analyze usage, and find files across billions of inodes.
      </p>
      <div style={{ display: 'flex', gap: '16px' }}>
        <button 
          onClick={() => navigate('/browser')}
          style={{ 
            padding: '12px 24px', 
            fontSize: '14px', 
            fontWeight: 600, 
            borderRadius: '8px', 
            border: 'none', 
            background: theme.accentPurple, 
            color: 'white', 
            cursor: 'pointer',
            transition: 'all 0.2s'
          }}
          onMouseEnter={(e) => e.currentTarget.style.opacity = '0.9'}
          onMouseLeave={(e) => e.currentTarget.style.opacity = '1'}
        >
          Open File Browser
        </button>
        <button 
          onClick={() => navigate('/users')}
          style={{ 
            padding: '12px 24px', 
            fontSize: '14px', 
            fontWeight: 600, 
            borderRadius: '8px', 
            border: `1px solid ${theme.border}`, 
            background: 'rgba(255,255,255,0.05)', 
            color: theme.textMain, 
            cursor: 'pointer',
            transition: 'all 0.2s'
          }}
          onMouseEnter={(e) => e.currentTarget.style.background = 'rgba(255,255,255,0.1)'}
          onMouseLeave={(e) => e.currentTarget.style.background = 'rgba(255,255,255,0.05)'}
        >
          View User Stats
        </button>
      </div>
    </div>
  );
};

export default HomePage;

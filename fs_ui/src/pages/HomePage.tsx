import React from 'react';
import { useNavigate } from 'react-router-dom';
import { theme } from '../theme';
import { BlueprintFrame, btnPrimaryStyle, btnSecondaryStyle } from '../components/Blueprint';

const HomePage: React.FC = () => {
  const navigate = useNavigate();

  return (
    <div style={{
      flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
      background: theme.bg, color: theme.text, textAlign: 'center', padding: 40,
    }}>
      <BlueprintFrame style={{ width: 56, height: 56, background: theme.accent, display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 28 }}>
        <span style={{ fontFamily: theme.fontHeading, fontWeight: 700, fontSize: 26, color: theme.bg }}>C</span>
      </BlueprintFrame>
      <h1 style={{ fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 32, margin: '0 0 16px' }}>Welcome to Cuttlefish</h1>
      <p style={{ fontSize: 16, color: theme.textMuted2, maxWidth: 500, marginBottom: 32, lineHeight: 1.6 }}>
        Manage and explore your high-performance filesystem index.
        Quickly browse directories, analyze usage, and find files across billions of inodes.
      </p>
      <div style={{ display: 'flex', gap: 12 }}>
        <div style={btnPrimaryStyle} onClick={() => navigate('/browser')}>Open File Browser</div>
        <div style={btnSecondaryStyle} onClick={() => navigate('/users')}>View User Stats</div>
      </div>
    </div>
  );
};

export default HomePage;

import React from 'react';
import { useNavigate } from 'react-router-dom';
import { theme } from '../theme';
import { BlueprintFrame, btnSecondaryStyle } from '../components/Blueprint';

const SplashPage: React.FC = () => {
  const navigate = useNavigate();

  return (
    <div style={{
      flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
      background: theme.bg, color: theme.text, textAlign: 'center', padding: 40,
    }}>
      <BlueprintFrame style={{ width: 80, height: 80, background: theme.accent, display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 28 }}>
        <span style={{ fontFamily: theme.fontHeading, fontWeight: 700, fontSize: 38, color: theme.bg }}>C</span>
      </BlueprintFrame>
      <h1 style={{ fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 44, margin: '0 0 16px' }}>Cuttlefish Explorer</h1>
      <p style={{ fontSize: 18, color: theme.textMuted2, maxWidth: 600, marginBottom: 36, lineHeight: 1.6 }}>
        A high-performance filesystem indexer and aggregator.
        Explore billions of inodes with near-instant precision.
      </p>
      <div style={btnSecondaryStyle} onClick={() => navigate('/browser')}>Enter Explorer</div>
    </div>
  );
};

export default SplashPage;

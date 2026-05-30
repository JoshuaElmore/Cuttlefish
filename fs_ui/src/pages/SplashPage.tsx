import React from 'react';
import { theme } from '../theme';
import { useNavigate } from 'react-router-dom';

const SplashPage: React.FC = () => {
  const navigate = useNavigate();

  return (
    <div style={{ 
      flex: 1, 
      display: 'flex', 
      flexDirection: 'column', 
      alignItems: 'center', 
      justifyContent: 'center', 
      background: theme.panelBg,
      backdropFilter: 'blur(20px)',
      color: theme.textMain,
      textAlign: 'center',
      padding: '40px'
    }}>
      <img
        src="/logo.svg"
        alt="Cuttlefish"
        style={{
          width: '160px',
          height: '160px',
          objectFit: 'contain',
          marginBottom: '24px',
          filter: 'drop-shadow(0 0 30px rgba(0,150,255,0.5))',
          animation: 'float 6s ease-in-out infinite'
        }}
      />
      <h1 style={{ 
        fontSize: '48px', 
        fontWeight: 800, 
        marginBottom: '16px', 
        background: 'linear-gradient(to right, #fff, #888)', 
        WebkitBackgroundClip: 'text', 
        WebkitTextFillColor: 'transparent' 
      }}>
        Cuttlefish Explorer
      </h1>
      <p style={{ 
        fontSize: '18px', 
        color: theme.textMuted, 
        maxWidth: '600px', 
        marginBottom: '40px', 
        lineHeight: '1.6' 
      }}>
        A high-performance filesystem indexer and aggregator. 
        Explore billions of inodes with near-instant precision.
      </p>
      <button 
        onClick={() => navigate('/browser')}
        style={{ 
          padding: '16px 32px', 
          fontSize: '16px', 
          fontWeight: 600, 
          borderRadius: '12px', 
          border: `1px solid ${theme.border}`, 
          background: 'rgba(255,255,255,0.1)', 
          color: theme.textMain, 
          cursor: 'pointer', 
          transition: 'all 0.2s',
          backdropFilter: 'blur(10px)'
        }}
        onMouseEnter={(e) => e.currentTarget.style.background = 'rgba(255,255,255,0.2)'}
        onMouseLeave={(e) => e.currentTarget.style.background = 'rgba(255,255,255,0.1)'}
      >
        Enter Explorer
      </button>
      <style>{`
        @keyframes float {
          0%, 100% { transform: translateY(0); }
          50% { transform: translateY(-20px); }
        }
      `}</style>
    </div>
  );
};

export default SplashPage;

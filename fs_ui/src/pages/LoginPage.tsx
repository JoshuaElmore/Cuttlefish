import React, { useEffect, useState } from 'react';
import { theme } from '../theme';

interface LoginPageProps {
  onLogin: () => void;
}

const LoginPage: React.FC<LoginPageProps> = ({ onLogin }) => {
  const [mode, setMode] = useState<'local' | 'oidc' | null>(null);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    fetch('/auth/mode')
      .then(r => r.json())
      .then(d => setMode(d.mode))
      .catch(() => setMode('local'));
  }, []);

  const handleLocalLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      const r = await fetch('/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      });
      if (r.ok) {
        onLogin();
      } else {
        const d = await r.json();
        setError(d.error || 'Login failed');
      }
    } catch {
      setError('Network error');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{
      height: '100vh',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      background: theme.bgGradient,
      fontFamily: '"Inter", system-ui, -apple-system, sans-serif',
    }}>
      <div style={{
        width: 360,
        padding: '40px',
        background: theme.cardBg,
        border: `1px solid ${theme.border}`,
        borderRadius: '20px',
        boxShadow: '0 20px 60px rgba(0,0,0,0.4)',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        gap: '24px',
      }}>
        <img src="/logo.svg" alt="Cuttlefish" style={{ width: 64, height: 64, objectFit: 'contain' }} />
        <div style={{ textAlign: 'center' }}>
          <h1 style={{ margin: 0, fontSize: '1.5rem', fontWeight: 700, color: theme.textMain }}>Cuttlefish</h1>
          <p style={{ margin: '4px 0 0', fontSize: '13px', color: theme.textMuted }}>Sign in to continue</p>
        </div>

        {mode === null && (
          <p style={{ color: theme.textMuted, fontSize: '14px' }}>Loading...</p>
        )}

        {mode === 'oidc' && (
          <a
            href="/auth/oidc/start"
            style={{
              display: 'block',
              width: '100%',
              padding: '12px',
              textAlign: 'center',
              borderRadius: '10px',
              border: `1px solid ${theme.border}`,
              background: 'rgba(255,255,255,0.08)',
              color: theme.textMain,
              fontWeight: 600,
              fontSize: '15px',
              textDecoration: 'none',
              cursor: 'pointer',
              transition: 'background 0.2s',
            }}
            onMouseEnter={e => (e.currentTarget.style.background = 'rgba(255,255,255,0.15)')}
            onMouseLeave={e => (e.currentTarget.style.background = 'rgba(255,255,255,0.08)')}
          >
            Sign in with SSO
          </a>
        )}

        {mode === 'local' && (
          <form onSubmit={handleLocalLogin} style={{ width: '100%', display: 'flex', flexDirection: 'column', gap: '12px' }}>
            <input
              type="text"
              placeholder="Username"
              autoComplete="username"
              value={username}
              onChange={e => setUsername(e.target.value)}
              style={inputStyle}
            />
            <input
              type="password"
              placeholder="Password"
              autoComplete="current-password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              style={inputStyle}
            />
            {error && (
              <p style={{ margin: 0, fontSize: '13px', color: '#f87171', textAlign: 'center' }}>{error}</p>
            )}
            <button
              type="submit"
              disabled={loading}
              style={{
                padding: '12px',
                borderRadius: '10px',
                border: 'none',
                background: theme.accentPurple,
                color: 'white',
                fontWeight: 600,
                fontSize: '15px',
                cursor: loading ? 'not-allowed' : 'pointer',
                opacity: loading ? 0.7 : 1,
                transition: 'opacity 0.2s',
              }}
            >
              {loading ? 'Signing in...' : 'Sign in'}
            </button>
          </form>
        )}
      </div>
    </div>
  );
};

const inputStyle: React.CSSProperties = {
  padding: '10px 14px',
  borderRadius: '8px',
  border: '1px solid rgba(255,255,255,0.15)',
  background: 'rgba(0,0,0,0.3)',
  color: 'white',
  fontSize: '14px',
  outline: 'none',
  width: '100%',
  boxSizing: 'border-box',
};

export default LoginPage;

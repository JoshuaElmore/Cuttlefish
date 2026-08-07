import React, { useEffect, useState } from 'react';
import { theme } from '../theme';
import { BlueprintFrame, btnPrimaryStyle, inputStyle } from '../components/Blueprint';

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
      height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: theme.bg, fontFamily: theme.fontBody,
    }}>
      <BlueprintFrame style={{ width: 360, padding: 40, background: theme.surface, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 24 }}>
        <div style={{ width: 48, height: 48, background: theme.accent, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <span style={{ fontFamily: theme.fontHeading, fontWeight: 700, fontSize: 22, color: theme.bg }}>C</span>
        </div>
        <div style={{ textAlign: 'center' }}>
          <h1 style={{ margin: 0, fontFamily: theme.fontHeading, fontSize: '1.4rem', fontWeight: 600, color: theme.text }}>Cuttlefish</h1>
          <p style={{ margin: '4px 0 0', fontSize: 13, color: theme.textMuted2 }}>Sign in to continue</p>
        </div>

        {mode === null && <p style={{ color: theme.textMuted2, fontSize: 14 }}>Loading...</p>}

        {mode === 'oidc' && (
          <a href="/auth/oidc/start" style={{ ...btnPrimaryStyle, width: '100%', textDecoration: 'none', padding: 12 }}>
            Sign in with SSO
          </a>
        )}

        {mode === 'local' && (
          <form onSubmit={handleLocalLogin} style={{ width: '100%', display: 'flex', flexDirection: 'column', gap: 12 }}>
            <input
              type="text" placeholder="Username" autoComplete="username"
              value={username} onChange={e => setUsername(e.target.value)}
              style={{ ...inputStyle, width: '100%', minHeight: 40 }}
            />
            <input
              type="password" placeholder="Password" autoComplete="current-password"
              value={password} onChange={e => setPassword(e.target.value)}
              style={{ ...inputStyle, width: '100%', minHeight: 40 }}
            />
            {error && <p style={{ margin: 0, fontSize: 13, color: theme.danger, textAlign: 'center' }}>{error}</p>}
            <button
              type="submit" disabled={loading}
              style={{ ...btnPrimaryStyle, width: '100%', padding: 12, opacity: loading ? 0.6 : 1, cursor: loading ? 'not-allowed' : 'pointer', border: `1px solid ${theme.accent}` }}
            >
              {loading ? 'Signing in...' : 'Sign in'}
            </button>
          </form>
        )}
      </BlueprintFrame>
    </div>
  );
};

export default LoginPage;

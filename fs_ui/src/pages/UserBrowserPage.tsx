import React, { useState, useEffect } from 'react';
import { UserStats } from '../types';
import { theme } from '../theme';

interface UserBrowserPageProps {
  formatSize: (bytes: number) => string;
  formatNumber: (num: number) => string;
}

const UserBrowserPage: React.FC<UserBrowserPageProps> = ({ formatSize, formatNumber }) => {
  const [activeType, setActiveType] = useState<'uid' | 'gid'>('uid');
  const [stats, setStats] = useState<UserStats[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchStats = async () => {
      setIsLoading(true);
      setError(null);
      try {
        const endpoint = activeType === 'uid' ? '/api/user/list' : '/api/group/list';
        const response = await fetch(endpoint);
        if (!response.ok) throw new Error(`API error: ${response.statusText}`);
        const data = await response.json();
        setStats(data);
      } catch (err: any) {
        setError(err.message);
      } finally {
        setIsLoading(false);
      }
    };

    fetchStats();
  }, [activeType]);

  return (
    <div style={{ 
      flex: 1, 
      display: 'flex', 
      flexDirection: 'column', 
      background: theme.panelBg,
      backdropFilter: 'blur(10px)',
      color: theme.textMain 
    }}>
      <div style={{ 
        padding: '24px', 
        borderBottom: `1px solid ${theme.border}`, 
        display: 'flex', 
        justifyContent: 'space-between', 
        alignItems: 'center',
        background: 'rgba(0,0,0,0.1)' 
      }}>
        <h2 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          {activeType === 'uid' ? '👤 User Storage Usage' : '👥 Group Storage Usage'}
        </h2>
        
        <div style={{ display: 'flex', gap: '8px', background: 'rgba(0,0,0,0.3)', padding: '4px', borderRadius: '8px' }}>
          {(['uid', 'gid'] as const).map(type => (
            <div 
              key={type}
              onClick={() => setActiveType(type)}
              style={{ 
                padding: '6px 16px', 
                cursor: 'pointer', 
                borderRadius: '6px', 
                fontSize: '13px', 
                fontWeight: 500,
                transition: 'all 0.2s',
                background: activeType === type ? 'rgba(255,255,255,0.1)' : 'transparent',
                color: activeType === type ? theme.textMain : theme.textMuted,
                border: activeType === type ? `1px solid ${theme.border}` : '1px solid transparent'
              }}
            >
              {type === 'uid' ? 'Users' : 'Groups'}
            </div>
          ))}
        </div>
      </div>

      <div style={{ flex: 1, overflowY: 'auto', padding: '24px' }}>
        {isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', color: theme.textMuted }}>
            Loading statistics...
          </div>
        ) : error ? (
          <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', color: '#ff6b6b' }}>
            Error: {error}
          </div>
        ) : (
          <table style={{ 
            width: '100%', 
            borderCollapse: 'collapse', 
            textAlign: 'left', 
            fontSize: '14px',
            borderSpacing: 0
          }}>
            <thead>
              <tr style={{ color: theme.textMuted, borderBottom: `2px solid ${theme.border}` }}>
                <th style={{ padding: '12px', fontWeight: 500 }}>Name / ID</th>
                <th style={{ padding: '12px', fontWeight: 500, textAlign: 'right' }}>Total Size</th>
                <th style={{ padding: '12px', fontWeight: 500, textAlign: 'right' }}>File Count</th>
                <th style={{ padding: '12px', fontWeight: 500, textAlign: 'right' }}>Avg. File Size</th>
              </tr>
            </thead>
            <tbody>
              {stats.map((s, i) => (
                <tr key={i} style={{ 
                  borderBottom: `1px solid ${theme.border}`, 
                  transition: 'background 0.2s',
                  cursor: 'default'
                }}
                onMouseEnter={(e) => e.currentTarget.style.background = 'rgba(255,255,255,0.03)'}
                onMouseLeave={(e) => e.currentTarget.style.background = 'transparent'}
                >
                  <td style={{ padding: '12px', fontWeight: 500 }}>{s.name}</td>
                  <td style={{ padding: '12px', textAlign: 'right', fontFamily: 'monospace' }}>{formatSize(s.total_size_bytes)}</td>
                  <td style={{ padding: '12px', textAlign: 'right', fontFamily: 'monospace' }}>{formatNumber(s.file_count)}</td>
                  <td style={{ padding: '12px', textAlign: 'right', fontFamily: 'monospace', color: theme.textMuted }}>
                    {s.file_count > 0 ? formatSize(Math.floor(s.total_size_bytes / s.file_count)) : '0 B'}
                  </td>
                </tr>
              ))}
              {stats.length === 0 && (
                <tr>
                  <td colSpan={4} style={{ textAlign: 'center', padding: '40px', color: theme.textMuted }}>
                    No usage statistics found.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
};

export default UserBrowserPage;

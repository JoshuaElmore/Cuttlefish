import React, { useEffect, useState } from 'react';
import { UserStats } from '../types';
import { theme } from '../theme';
import { formatBytes, formatNumber } from '../format';
import { fsApi } from '../api';
import { Tag } from '../components/Blueprint';
import DetailPanel, { DetailSelection } from '../components/DetailPanel';

const th: React.CSSProperties = {
  textAlign: 'left', fontSize: 11, letterSpacing: '0.08em', textTransform: 'uppercase',
  color: theme.textMuted, padding: 8, borderBottom: `1px solid ${theme.border}`,
};
const td: React.CSSProperties = { padding: '9px 8px', borderBottom: `1px solid ${theme.borderSoft}` };

const UserBrowserPage: React.FC = () => {
  const [stats, setStats] = useState<UserStats[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  useEffect(() => {
    const fetchStats = async () => {
      setIsLoading(true);
      setError(null);
      try {
        const [users, groups] = await Promise.all([
          fsApi.listIdentityStats('uid'),
          fsApi.listIdentityStats('gid'),
        ]);
        setStats([...users, ...groups].sort((a, b) => b.total_size_bytes - a.total_size_bytes));
      } catch (err: any) {
        setError(err.message);
      } finally {
        setIsLoading(false);
      }
    };
    fetchStats();
  }, []);

  const keyOf = (s: UserStats) => `${s.id_type}:${s.id_value}`;
  const selected: DetailSelection | null = selectedKey
    ? { kind: 'user', user: stats.find(s => keyOf(s) === selectedKey)! }
    : null;

  return (
    <div style={{ display: 'flex', flex: 1, overflow: 'hidden', background: theme.bg, color: theme.text }}>
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, overflow: 'hidden' }}>
        <div style={{ flex: 'none', padding: '16px 28px', borderBottom: `1px solid ${theme.border}`, fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 19 }}>
          User &amp; Group Usage
        </div>

        <div style={{ flex: 1, overflow: 'auto', padding: '24px 28px' }}>
          {isLoading ? (
            <div style={{ textAlign: 'center', padding: 80, color: theme.textMuted }}>Loading statistics...</div>
          ) : error ? (
            <div style={{ textAlign: 'center', padding: 80, color: theme.danger }}>Error: {error}</div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
              <thead>
                <tr>
                  <th style={th}>Name</th>
                  <th style={th}>Type</th>
                  <th style={{ ...th, textAlign: 'right' }}>Total Size</th>
                  <th style={{ ...th, textAlign: 'right' }}>Files</th>
                </tr>
              </thead>
              <tbody>
                {stats.map(s => {
                  const key = keyOf(s);
                  const isSelected = selectedKey === key;
                  return (
                    <tr key={key} onClick={() => setSelectedKey(key)} style={{ cursor: 'pointer', background: isSelected ? theme.accent100 : 'transparent' }}>
                      <td style={{ ...td, fontWeight: 500 }}>{s.name}</td>
                      <td style={td}><Tag tone={s.id_type === 'uid' ? 'accent' : 'neutral'}>{s.id_type.toUpperCase()}</Tag></td>
                      <td style={{ ...td, textAlign: 'right', fontVariantNumeric: 'tabular-nums', color: theme.textMuted2 }}>{formatBytes(s.total_size_bytes)}</td>
                      <td style={{ ...td, textAlign: 'right', fontVariantNumeric: 'tabular-nums', color: theme.textMuted2 }}>{formatNumber(s.file_count)}</td>
                    </tr>
                  );
                })}
                {stats.length === 0 && (
                  <tr><td colSpan={4} style={{ textAlign: 'center', padding: 40, color: theme.textMuted }}>No usage statistics found.</td></tr>
                )}
              </tbody>
            </table>
          )}
        </div>
      </div>

      <DetailPanel selected={selected} onClose={() => setSelectedKey(null)} />
    </div>
  );
};

export default UserBrowserPage;

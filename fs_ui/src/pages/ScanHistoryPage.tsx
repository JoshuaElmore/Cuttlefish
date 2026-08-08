import React, { useEffect, useState } from 'react';
import { theme } from '../theme';
import { formatNumber } from '../format';
import { fsApi } from '../api';
import { ScanSession, ScanStatus, ScanType } from '../types';
import { PulseDot, Tag } from '../components/Blueprint';

const th: React.CSSProperties = {
  textAlign: 'left', fontSize: 11, letterSpacing: '0.08em', textTransform: 'uppercase',
  color: theme.textMuted, padding: 8, borderBottom: `1px solid ${theme.border}`,
};
const td: React.CSSProperties = { padding: '9px 8px', borderBottom: `1px solid ${theme.borderSoft}` };

const SCAN_TYPE_LABEL: Record<ScanType, string> = { indexer: 'Indexer', aggregator: 'Aggregator' };
const STATUS_LABEL: Record<ScanStatus, string> = { running: 'Running', success: 'Completed', failed: 'Failed' };
const STATUS_TONE: Record<ScanStatus, 'accent' | 'neutral' | 'danger'> = { running: 'neutral', success: 'accent', failed: 'danger' };

const formatDuration = (startedAt: number, endedAt: number | undefined, status: ScanStatus) => {
  const running = status === 'running';
  const seconds = running ? Math.floor(Date.now() / 1000) - startedAt : (endedAt ?? startedAt) - startedAt;
  if (seconds < 0) return '—';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  const label = h > 0 ? `${h}h ${m}m` : m > 0 ? `${m}m ${s}s` : `${s}s`;
  return running ? `${label} elapsed` : label;
};

const ScanHistoryPage: React.FC = () => {
  const [sessions, setSessions] = useState<ScanSession[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const load = () => fsApi.listScans().then(data => { if (!cancelled) { setSessions(data); setIsLoading(false); } }).catch(err => { if (!cancelled) { setError(err.message); setIsLoading(false); } });
    load();
    const interval = setInterval(load, 30000);
    return () => { cancelled = true; clearInterval(interval); };
  }, []);

  return (
    <div style={{ display: 'flex', flex: 1, overflow: 'hidden', background: theme.bg, color: theme.text }}>
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, overflow: 'hidden' }}>
        <div style={{ flex: 'none', padding: '16px 28px', borderBottom: `1px solid ${theme.border}`, fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 19 }}>
          Scan History
        </div>

        <div style={{ flex: 1, overflow: 'auto', padding: '24px 28px' }}>
          {isLoading ? (
            <div style={{ textAlign: 'center', padding: 80, color: theme.textMuted }}>Loading scan history...</div>
          ) : error ? (
            <div style={{ textAlign: 'center', padding: 80, color: theme.danger }}>Error: {error}</div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
              <thead>
                <tr>
                  <th style={th}>Started</th>
                  <th style={th}>Type</th>
                  <th style={th}>Duration</th>
                  <th style={{ ...th, textAlign: 'right' }}>Files Scanned</th>
                  <th style={th}>Status</th>
                </tr>
              </thead>
              <tbody>
                {sessions.map(s => (
                  <tr key={s.session_id}>
                    <td style={td}>{new Date(s.started_at * 1000).toLocaleString()}</td>
                    <td style={td}><Tag tone={s.scan_type === 'indexer' ? 'accent' : 'neutral'}>{SCAN_TYPE_LABEL[s.scan_type]}</Tag></td>
                    <td style={{ ...td, color: theme.textMuted2 }}>{formatDuration(s.started_at, s.ended_at, s.status)}</td>
                    <td style={{ ...td, textAlign: 'right', fontVariantNumeric: 'tabular-nums', color: theme.textMuted2 }}>{formatNumber(s.files_scanned)}</td>
                    <td style={td}>
                      <Tag tone={STATUS_TONE[s.status]}>
                        {s.status === 'running' && <PulseDot size={6} />}
                        <span style={{ marginLeft: s.status === 'running' ? 5 : 0 }}>{STATUS_LABEL[s.status]}</span>
                      </Tag>
                    </td>
                  </tr>
                ))}
                {sessions.length === 0 && (
                  <tr><td colSpan={5} style={{ textAlign: 'center', padding: 40, color: theme.textMuted }}>No scans recorded yet.</td></tr>
                )}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  );
};

export default ScanHistoryPage;

import React from 'react';
import { Entry } from '../types';
import { theme } from '../theme';

interface FileDetailsProps {
  item: Entry;
  formatSize: (bytes: number) => string;
  formatNumber: (num: number) => string;
}

const FileDetails: React.FC<FileDetailsProps> = ({ item, formatSize, formatNumber }) => {
  return (
    <div style={{ 
      background: theme.cardBg, padding: '30px', borderRadius: '20px', 
      boxShadow: '0 10px 30px rgba(0,0,0,0.2)', border: `1px solid ${theme.border}`,
      animation: 'fadeIn 0.3s ease-out'
    }}>
      <h3 style={{ marginTop: 0, fontSize: '1.5rem', marginBottom: '24px', display: 'flex', alignItems: 'center', gap: '12px' }}>
        <span style={{ fontSize: '1.8rem' }}>{item.file_type === 2 ? '📁' : '📄'}</span> 
        {item.file_type === 2 ? 'Directory Info' : 'File Metadata'}
      </h3>
      <div style={{ marginBottom: '24px', padding: '12px', background: 'rgba(0,0,0,0.3)', borderRadius: '10px', border: `1px solid ${theme.border}`, fontFamily: 'monospace', fontSize: '13px', color: theme.accentBlue, wordBreak: 'break-all' }}>
        {item.path}
      </div>
      
      <div style={{ marginBottom: '32px' }}>
        <h4 style={{ fontSize: '0.9rem', color: theme.textMuted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '16px', borderBottom: `1px solid ${theme.border}`, paddingBottom: '8px' }}>
          👤 Identity & Ownership
        </h4>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: '15px' }}>
          <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: '10px', border: `1px solid ${theme.border}` }}>
            <strong style={{ display: 'block', fontSize: '11px', color: theme.textMuted, textTransform: 'uppercase' }}>User</strong>
            <div style={{ fontSize: '16px', fontWeight: 600 }}>{item.user} <span style={{ color: theme.textMuted, fontSize: '12px' }}>(UID: {item.uid})</span></div >
          </div>
          <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: '10px', border: `1px solid ${theme.border}` }}>
            <strong style={{ display: 'block', fontSize: '11px', color: theme.textMuted, textTransform: 'uppercase' }}>Group</strong>
            <div style={{ fontSize: '16px', fontWeight: 600 }}>{item.group} <span style={{ color: theme.textMuted, fontSize: '12px' }}>(GID: {item.gid})</span></div >
          </div>
          <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: '10px', border: `1px solid ${theme.border}` }}>
            <strong style={{ display: 'block', fontSize: '11px', color: theme.textMuted, textTransform: 'uppercase' }}>Permissions</strong>
            <div style={{ fontSize: '16px', fontWeight: 600, color: theme.accentPurple }}>{item.permissions}</div>
          </div>
        </div>
      </div>

      <div style={{ marginBottom: '32px' }}>
        <h4 style={{ fontSize: '0.9rem', color: theme.textMuted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '16px', borderBottom: `1px solid ${theme.border}`, paddingBottom: '8px' }}>
          🕒 Timestamps
        </h4>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: '15px' }}>
          <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: '10px', border: `1px solid ${theme.border}` }}>
            <strong style={{ display: 'block', fontSize: '11px', color: theme.textMuted, textTransform: 'uppercase' }}>Modified</strong>
            <div style={{ fontSize: '14px' }}>{new Date(item.mtime * 1000).toLocaleString()}</div>
          </div>
          <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: '10px', border: `1px solid ${theme.border}` }}>
            <strong style={{ display: 'block', fontSize: '11px', color: theme.textMuted, textTransform: 'uppercase' }}>Accessed</strong>
            <div style={{ fontSize: '14px' }}>{new Date(item.atime * 1000).toLocaleString()}</div>
          </div>
          <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: '10px', border: `1px solid ${theme.border}` }}>
            <strong style={{ display: 'block', fontSize: '11px', color: theme.textMuted, textTransform: 'uppercase' }}>Changed</strong>
            <div style={{ fontSize: '14px' }}>{new Date(item.ctime * 1000).toLocaleString()}</div>
          </div>
        </div>
      </div>

      {item.file_type === 2 && item.aggregates && (
        <div style={{ marginBottom: '32px' }}>
          <h4 style={{ fontSize: '0.9rem', color: theme.textMuted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '16px', borderBottom: `1px solid ${theme.border}`, paddingBottom: '8px' }}>
            📦 Aggregated Stats
          </h4>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: '20px' }}>
            <div style={{ padding: '15px', background: 'rgba(0,0,0,0.2)', borderRadius: '12px', border: `1px solid ${theme.border}` }}>
              <strong style={{ display: 'block', fontSize: '12px', color: theme.textMuted, textTransform: 'uppercase' }}>Total Size</strong>
              <span style={{ fontSize: '20px', fontWeight: 'bold', color: theme.accentPurple }}>{formatSize(item.aggregates.total_size_bytes)}</span>
            </div>
            <div style={{ padding: '15px', background: 'rgba(0,0,0,0.2)', borderRadius: '12px', border: `1px solid ${theme.border}` }}>
              <strong style={{ display: 'block', fontSize: '12px', color: theme.textMuted, textTransform: 'uppercase' }}>Total Files</strong>
              <span style={{ fontSize: '20px', fontWeight: 'bold', color: theme.accentBlue }}>{formatNumber(item.aggregates.file_count)}</span>
            </div>
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '10px', marginTop: '15px' }}>
            <div style={{ padding: '10px', background: 'rgba(0,0,0,0.2)', borderRadius: '8px', fontSize: '12px' }}>
              <strong style={{ display: 'block', color: theme.textMuted }}>MTime Range</strong>
              <div>{new Date(item.aggregates.mtime_first * 1000).toLocaleDateString()} → {new Date(item.aggregates.mtime_last * 1000).toLocaleDateString()}</div>
            </div>
            <div style={{ padding: '10px', background: 'rgba(0,0,0,0.2)', borderRadius: '8px', fontSize: '12px' }}>
              <strong style={{ display: 'block', color: theme.textMuted }}>ATime Range</strong>
              <div>{new Date(item.aggregates.atime_first * 1000).toLocaleDateString()} → {new Date(item.aggregates.atime_last * 1000).toLocaleDateString()}</div>
            </div>
            <div style={{ padding: '10px', background: 'rgba(0,0,0,0.2)', borderRadius: '8px', fontSize: '12px' }}>
              <strong style={{ display: 'block', color: theme.textMuted }}>CTime Range</strong>
              <div>{new Date(item.aggregates.ctime_first * 1000).toLocaleDateString()} → {new Date(item.aggregates.ctime_last * 1000).toLocaleDateString()}</div>
            </div>
          </div>
        </div>
      )}

      <div style={{ padding: '15px', background: 'rgba(0,0,0,0.2)', borderRadius: '10px', border: `1px solid ${theme.border}` }}>
        <strong style={{ display: 'block', fontSize: '11px', color: theme.textMuted, textTransform: 'uppercase' }}>Size (Actual)</strong>
        <div style={{ fontSize: '16px', fontWeight: 600 }}>{formatSize(item.size_bytes)}</div>
      </div>
    </div>
  );
};

export default FileDetails;

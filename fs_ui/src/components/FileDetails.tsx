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
      background: theme.cardBg, padding: '16px', borderRadius: '12px',
      boxShadow: '0 4px 16px rgba(0,0,0,0.2)', border: `1px solid ${theme.border}`,
      animation: 'fadeIn 0.3s ease-out'
    }}>
      <h3 style={{ marginTop: 0, fontSize: '1rem', marginBottom: '10px', display: 'flex', alignItems: 'center', gap: '8px' }}>
        <span style={{ fontSize: '1.1rem' }}>{item.file_type === 2 ? '📁' : '📄'}</span>
        {item.file_type === 2 ? 'Directory Info' : 'File Metadata'}
      </h3>

      <div style={{ marginBottom: '12px', padding: '8px 10px', background: 'rgba(0,0,0,0.3)', borderRadius: '6px', border: `1px solid ${theme.border}`, fontFamily: 'monospace', fontSize: '12px', color: theme.accentBlue, wordBreak: 'break-all' }}>
        {item.path}
      </div>

      <div style={{ marginBottom: '12px' }}>
        <h4 style={{ fontSize: '0.75rem', color: theme.textMuted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '8px', borderBottom: `1px solid ${theme.border}`, paddingBottom: '4px', marginTop: 0 }}>
          Identity &amp; Ownership
        </h4>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))', gap: '8px' }}>
          <div style={{ padding: '8px', background: 'rgba(0,0,0,0.2)', borderRadius: '6px', border: `1px solid ${theme.border}` }}>
            <strong style={{ display: 'block', fontSize: '10px', color: theme.textMuted, textTransform: 'uppercase' }}>User</strong>
            <div style={{ fontSize: '13px', fontWeight: 600 }}>{item.user} <span style={{ color: theme.textMuted, fontSize: '11px' }}>(UID: {item.uid})</span></div>
          </div>
          <div style={{ padding: '8px', background: 'rgba(0,0,0,0.2)', borderRadius: '6px', border: `1px solid ${theme.border}` }}>
            <strong style={{ display: 'block', fontSize: '10px', color: theme.textMuted, textTransform: 'uppercase' }}>Group</strong>
            <div style={{ fontSize: '13px', fontWeight: 600 }}>{item.group} <span style={{ color: theme.textMuted, fontSize: '11px' }}>(GID: {item.gid})</span></div>
          </div>
          <div style={{ padding: '8px', background: 'rgba(0,0,0,0.2)', borderRadius: '6px', border: `1px solid ${theme.border}` }}>
            <strong style={{ display: 'block', fontSize: '10px', color: theme.textMuted, textTransform: 'uppercase' }}>Permissions</strong>
            <div style={{ fontSize: '13px', fontWeight: 600, color: theme.accentPurple }}>{item.permissions}</div>
          </div>
        </div>
      </div>

      <div style={{ marginBottom: '12px' }}>
        <h4 style={{ fontSize: '0.75rem', color: theme.textMuted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '8px', borderBottom: `1px solid ${theme.border}`, paddingBottom: '4px', marginTop: 0 }}>
          Timestamps
        </h4>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))', gap: '8px' }}>
          <div style={{ padding: '8px', background: 'rgba(0,0,0,0.2)', borderRadius: '6px', border: `1px solid ${theme.border}` }}>
            <strong style={{ display: 'block', fontSize: '10px', color: theme.textMuted, textTransform: 'uppercase' }}>Modified</strong>
            <div style={{ fontSize: '12px' }}>{new Date(item.mtime * 1000).toLocaleString()}</div>
          </div>
          <div style={{ padding: '8px', background: 'rgba(0,0,0,0.2)', borderRadius: '6px', border: `1px solid ${theme.border}` }}>
            <strong style={{ display: 'block', fontSize: '10px', color: theme.textMuted, textTransform: 'uppercase' }}>Accessed</strong>
            <div style={{ fontSize: '12px' }}>{new Date(item.atime * 1000).toLocaleString()}</div>
          </div>
          <div style={{ padding: '8px', background: 'rgba(0,0,0,0.2)', borderRadius: '6px', border: `1px solid ${theme.border}` }}>
            <strong style={{ display: 'block', fontSize: '10px', color: theme.textMuted, textTransform: 'uppercase' }}>Changed</strong>
            <div style={{ fontSize: '12px' }}>{new Date(item.ctime * 1000).toLocaleString()}</div>
          </div>
        </div>
      </div>

      {item.file_type === 2 && item.aggregates && (
        <div style={{ marginBottom: '12px' }}>
          <h4 style={{ fontSize: '0.75rem', color: theme.textMuted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '8px', borderBottom: `1px solid ${theme.border}`, paddingBottom: '4px', marginTop: 0 }}>
            Aggregated Stats
          </h4>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))', gap: '8px' }}>
            <div style={{ padding: '10px', background: 'rgba(0,0,0,0.2)', borderRadius: '8px', border: `1px solid ${theme.border}` }}>
              <strong style={{ display: 'block', fontSize: '10px', color: theme.textMuted, textTransform: 'uppercase' }}>Total Size</strong>
              <span style={{ fontSize: '16px', fontWeight: 'bold', color: theme.accentPurple }}>{formatSize(item.aggregates.total_size_bytes)}</span>
            </div>
            <div style={{ padding: '10px', background: 'rgba(0,0,0,0.2)', borderRadius: '8px', border: `1px solid ${theme.border}` }}>
              <strong style={{ display: 'block', fontSize: '10px', color: theme.textMuted, textTransform: 'uppercase' }}>Total Files</strong>
              <span style={{ fontSize: '16px', fontWeight: 'bold', color: theme.accentBlue }}>{formatNumber(item.aggregates.file_count)}</span>
            </div>
          </div>
          <div style={{ marginTop: '8px', marginBottom: '6px', fontSize: '11px', color: theme.textMuted, fontStyle: 'italic' }}>
            The earliest and latest dates each action was performed on any file or folder within this directory.
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '8px' }}>
            {[
              { label: 'Access Recent',  val: item.aggregates.atime_last  },
              { label: 'Modify Recent',  val: item.aggregates.mtime_last  },
              { label: 'Create Recent',  val: item.aggregates.ctime_last  },
              { label: 'Access Oldest',  val: item.aggregates.atime_first },
              { label: 'Modify Oldest',  val: item.aggregates.mtime_first },
              { label: 'Create Oldest',  val: item.aggregates.ctime_first },
            ].map(({ label, val }) => (
              <div key={label} style={{ padding: '8px', background: 'rgba(0,0,0,0.2)', borderRadius: '6px', fontSize: '11px' }}>
                <strong style={{ display: 'block', color: theme.textMuted }}>{label}</strong>
                <div>{val === 0 ? <span style={{ color: theme.textMuted, fontStyle: 'italic' }}>Too Old</span> : new Date(val * 1000).toLocaleDateString()}</div>
              </div>
            ))}
          </div>
        </div>
      )}

      <div style={{ padding: '8px 10px', background: 'rgba(0,0,0,0.2)', borderRadius: '6px', border: `1px solid ${theme.border}` }}>
        <strong style={{ display: 'block', fontSize: '10px', color: theme.textMuted, textTransform: 'uppercase' }}>Size (Actual)</strong>
        <div style={{ fontSize: '13px', fontWeight: 600 }}>{formatSize(item.size_bytes)}</div>
      </div>
    </div>
  );
};

export default FileDetails;

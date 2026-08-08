import React from 'react';
import { Entry, SortConfig } from '../types';
import { theme } from '../theme';
import { formatBytes } from '../format';
import { FolderIcon, FileIcon } from '../icons';

interface FileListProps {
  entries: Entry[];
  selectedPath: string | null;
  sortConfig: SortConfig;
  onSelect: (entry: Entry) => void;
  onNavigate: (path: string) => void;
  onSort: (key: keyof Entry) => void;
}

const th: React.CSSProperties = {
  fontSize: 11, letterSpacing: '0.08em', textTransform: 'uppercase', color: theme.textMuted,
  padding: 8, borderBottom: `1px solid ${theme.border}`, cursor: 'pointer', userSelect: 'none',
};
const td: React.CSSProperties = { padding: '9px 8px', borderBottom: `1px solid ${theme.borderSoft}` };

const FileList: React.FC<FileListProps> = ({ entries, selectedPath, sortConfig, onSelect, onNavigate, onSort }) => {
  const sortIndicator = (key: keyof Entry) => (sortConfig.key === key ? (sortConfig.direction === 'asc' ? ' ▲' : ' ▼') : '');

  const sorted = [...entries].sort((a, b) => {
    if (!sortConfig.key) return 0;
    let aVal: any = a[sortConfig.key];
    let bVal: any = b[sortConfig.key];
    if (sortConfig.key === 'size_bytes') {
      aVal = a.file_type === 2 ? (a.aggregates?.total_size_bytes ?? 0) : a.size_bytes;
      bVal = b.file_type === 2 ? (b.aggregates?.total_size_bytes ?? 0) : b.size_bytes;
    }
    if (aVal < bVal) return sortConfig.direction === 'asc' ? -1 : 1;
    if (aVal > bVal) return sortConfig.direction === 'asc' ? 1 : -1;
    return 0;
  });

  return (
    <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
      <thead>
        <tr>
          <th style={{ ...th, textAlign: 'left' }} onClick={() => onSort('path')}>Name{sortIndicator('path')}</th>
          <th style={{ ...th, textAlign: 'right' }} onClick={() => onSort('size_bytes')}>Size{sortIndicator('size_bytes')}</th>
          <th style={{ ...th, textAlign: 'left' }} onClick={() => onSort('user')}>Owner{sortIndicator('user')}</th>
          <th style={{ ...th, textAlign: 'left' }} onClick={() => onSort('mtime')}>Modified{sortIndicator('mtime')}</th>
        </tr>
      </thead>
      <tbody>
        {sorted.map(e => {
          const isDir = e.file_type === 2;
          const name = e.path.split('/').filter(Boolean).pop() || e.path;
          const isSelected = selectedPath === e.path;
          return (
            <tr
              key={e.path}
              onClick={() => onSelect(e)}
              onDoubleClick={() => { if (isDir) onNavigate(e.path); }}
              style={{ cursor: 'pointer', background: isSelected ? theme.accent100 : 'transparent' }}
            >
              <td style={td}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  {isDir ? <FolderIcon color={theme.accent} /> : <FileIcon color={theme.textMuted} />}
                  <span
                    onClick={e2 => { e2.stopPropagation(); if (isDir) onNavigate(e.path); else onSelect(e); }}
                    style={{ cursor: 'pointer' }}
                  >
                    {name}
                  </span>
                </div>
              </td>
              <td style={{ ...td, textAlign: 'right', fontVariantNumeric: 'tabular-nums', color: theme.textMuted2 }}>
                {formatBytes(isDir ? (e.aggregates?.total_size_bytes ?? 0) : e.size_bytes)}
              </td>
              <td style={{ ...td, color: theme.textMuted2 }}>{e.user}</td>
              <td style={{ ...td, color: theme.textMuted2 }}>{new Date(e.mtime * 1000).toLocaleString()}</td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
};

export default FileList;

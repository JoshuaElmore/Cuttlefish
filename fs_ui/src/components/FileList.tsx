import React, { useMemo } from 'react';
import { Entry, SortConfig } from '../types';
import { theme } from '../theme';
import { formatBytes } from '../format';
import { FolderIcon, FileIcon } from '../icons';
import { btnReset } from './Blueprint';

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
  const sortHeader = (key: keyof Entry, label: string, align: 'left' | 'right') => (
    <th
      style={{ ...th, textAlign: align }}
      aria-sort={sortConfig.key === key ? (sortConfig.direction === 'asc' ? 'ascending' : 'descending') : undefined}
    >
      <button type="button" onClick={() => onSort(key)} style={btnReset}>
        {label}{sortIndicator(key)}
      </button>
    </th>
  );

  // Keyed to the listing and the sort, not redone whenever the parent renders:
  // selecting a row re-renders this component, and /usr/lib is tens of
  // thousands of entries to re-sort for a highlight that moved one row.
  const sorted = useMemo(() => [...entries].sort((a, b) => {
    if (!sortConfig.key) return 0;
    let aVal: any = a[sortConfig.key];
    let bVal: any = b[sortConfig.key];
    if (sortConfig.key === 'size_bytes') {
      aVal = a.file_type === 2 ? (a.aggregates?.total_size_bytes ?? 0) : a.size_bytes;
      bVal = b.file_type === 2 ? (b.aggregates?.total_size_bytes ?? 0) : b.size_bytes;
    }
    if (typeof aVal === 'string' && typeof bVal === 'string') {
      const cmp = aVal.localeCompare(bVal, undefined, { sensitivity: 'base', numeric: true });
      return sortConfig.direction === 'asc' ? cmp : -cmp;
    }
    if (aVal < bVal) return sortConfig.direction === 'asc' ? -1 : 1;
    if (aVal > bVal) return sortConfig.direction === 'asc' ? 1 : -1;
    return 0;
  }), [entries, sortConfig]);

  return (
    <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
      <thead>
        <tr>
          {sortHeader('path', 'Name', 'left')}
          {sortHeader('size_bytes', 'Size', 'right')}
          {sortHeader('user', 'Owner', 'left')}
          {sortHeader('mtime', 'Modified', 'left')}
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
              tabIndex={0}
              aria-selected={isSelected}
              onClick={() => onSelect(e)}
              onDoubleClick={() => { if (isDir) onNavigate(e.path); }}
              // Only when the row itself has focus: Enter on the name button is its own action.
              onKeyDown={ev => { if (ev.target === ev.currentTarget && (ev.key === 'Enter' || ev.key === ' ')) { ev.preventDefault(); onSelect(e); } }}
              style={{ cursor: 'pointer', background: isSelected ? theme.accent100 : 'transparent' }}
            >
              <td style={td}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  {isDir ? <FolderIcon color={theme.accent} /> : <FileIcon color={theme.textMuted} />}
                  <button
                    type="button"
                    onClick={e2 => { e2.stopPropagation(); if (isDir) onNavigate(e.path); else onSelect(e); }}
                    style={btnReset}
                  >
                    {name}
                  </button>
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

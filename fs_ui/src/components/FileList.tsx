import React from 'react';
import { Entry, SortConfig } from '../types';
import { theme, gridTemplate } from '../theme';

interface FileListProps {
  entries: Entry[];
  selectedPath: string | null;
  sortConfig: SortConfig;
  onSelect: (entry: Entry) => void;
  onNavigate: (path: string) => void;
  onSort: (key: keyof Entry) => void;
  isLoading: boolean;
  formatSize: (bytes: number) => string;
}

const FileList: React.FC<FileListProps> = ({ 
  entries, selectedPath, sortConfig, onSelect, onNavigate, onSort, isLoading, formatSize 
}) => {
  const getSortIcon = (key: keyof Entry) => {
    if (sortConfig.key !== key) return '↕️';
    return sortConfig.direction === 'asc' ? '🔼' : '🔽';
  };

  const sortedEntries = [...entries].sort((a, b) => {
    if (!sortConfig.key) return 0;
    let aVal: any = a[sortConfig.key];
    let bVal: any = b[sortConfig.key];
    if (sortConfig.key === 'size_bytes') {
      aVal = a.aggregates ? a.aggregates.total_size_bytes : a.size_bytes;
      bVal = b.aggregates ? b.aggregates.total_size_bytes : b.size_bytes;
    }
    if (aVal < bVal) return sortConfig.direction === 'asc' ? -1 : 1;
    if (aVal > bVal) return sortConfig.direction === 'asc' ? 1 : -1;
    return 0;
  });

  return (
    <div style={{ overflowY: 'auto', flex: 1 }}>
      {isLoading && entries.length === 0 ? (
        <div style={{ padding: '40px', textAlign: 'center', color: theme.textMuted }}>Loading...</div>
      ) : (
        <>
          <div style={{ 
            display: 'grid', 
            gridTemplateColumns: gridTemplate, 
            borderBottom: `1px solid ${theme.border}`,
            background: 'rgba(0,0,0,0.2)',
            fontWeight: 600,
            fontSize: '12px',
            color: theme.textMuted,
            textTransform: 'uppercase',
            letterSpacing: '0.05em'
          }}>
            <div onClick={() => onSort('path')} style={{ padding: '12px 20px', cursor: 'pointer', userSelect: 'none' }}>
              Name {getSortIcon('path')}
            </div>
            <div onClick={() => onSort('file_type')} style={{ padding: '12px 20px', cursor: 'pointer', userSelect: 'none' }}>
              Type {getSortIcon('file_type')}
            </div>
            <div onClick={() => onSort('size_bytes')} style={{ padding: '12px 20px', textAlign: 'right', cursor: 'pointer', userSelect: 'none' }}>
              Size {getSortIcon('size_bytes')}
            </div>
            <div onClick={() => onSort('uid')} style={{ padding: '12px 20px', textAlign: 'right', cursor: 'pointer', userSelect: 'none' }}>
              Owner {getSortIcon('uid')}
            </div>
            <div onClick={() => onSort('mtime')} style={{ padding: '12px 20px', textAlign: 'right', cursor: 'pointer', userSelect: 'none' }}>
              Modified {getSortIcon('mtime')}
            </div>
            <div style={{ padding: '12px 20px', textAlign: 'center' }}>Action</div>
          </div>
          {sortedEntries.map((e, i) => (
            <div 
              key={i} 
              onClick={() => onSelect(e)}
              onDoubleClick={() => {
                if (e.file_type === 2) onNavigate(e.path);
              }}
              style={{ 
                display: 'grid', 
                gridTemplateColumns: gridTemplate, 
                padding: '12px 20px', borderBottom: `1px solid ${theme.border}`, 
                cursor: 'pointer', alignItems: 'center', transition: 'background 0.2s',
                fontSize: '14px',
                backgroundColor: selectedPath === e.path ? theme.selected : 'transparent'
              }}
              onMouseEnter={(ev) => {
                if (selectedPath !== e.path) ev.currentTarget.style.backgroundColor = theme.hover;
              }}
              onMouseLeave={(ev) => {
                ev.currentTarget.style.backgroundColor = selectedPath === e.path ? theme.selected : 'transparent';
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {e.file_type === 2 ? '📁' : e.file_type === 1 ? '📄' : '⚙️'} {e.path.split('/').pop()}
              </div>
              <div style={{ color: theme.textMuted, fontSize: '13px' }}>
                {e.file_type === 2 ? 'Directory' : e.file_type === 1 ? 'File' : 'System' }
              </div>
              <div style={{ textAlign: 'right', fontFamily: 'monospace', color: theme.textMuted, fontSize: '13px' }}>
                {e.file_type === 2 && e.aggregates ? formatSize(e.aggregates.total_size_bytes) : formatSize(e.size_bytes)}
              </div>
              <div style={{ textAlign: 'right', fontFamily: 'monospace', color: theme.textMuted, fontSize: '13px' }}>
                {e.user}
              </div>
              <div style={{ textAlign: 'right', color: theme.textMuted, fontSize: '13px' }}>
                {new Date(e.mtime * 1000).toLocaleDateString()}
              </div>
              <div style={{ display: 'flex', justifyContent: 'center' }}>
                {e.file_type === 2 && (
                  <button 
                    onClick={(ev) => {
                      ev.stopPropagation();
                      onNavigate(e.path);
                    }}
                    style={{ 
                      padding: '4px 8px', borderRadius: '4px', border: 'none', 
                      background: theme.accentBlue, color: 'white', fontSize: '11px', 
                      fontWeight: 600, cursor: 'pointer', transition: 'opacity 0.2s'
                    }}
                  >
                    Enter
                  </button>
                )}
              </div>
            </div>
          ))}
          {entries.length === 0 && !isLoading && <div style={{ padding: '40px', textAlign: 'center', color: theme.textMuted }}>No entries found.</div>}
        </>
      )}
    </div>
  );
};

export default FileList;

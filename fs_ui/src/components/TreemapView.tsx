import React from 'react';
import { Entry } from '../types';
import { theme } from '../theme';
import { formatBytes } from '../format';
import { FolderIcon, FileIcon } from '../icons';

interface TreemapViewProps {
  entries: Entry[];
  selectedPath: string | null;
  onSelect: (entry: Entry) => void;
}

// Box side scales with relative size within the current directory, same curve
// as the design reference: 90px..250px based on size / max(size).
const TreemapView: React.FC<TreemapViewProps> = ({ entries, selectedPath, onSelect }) => {
  const effectiveSize = (e: Entry) => (e.file_type === 2 ? (e.aggregates?.total_size_bytes ?? 0) : e.size_bytes);
  const maxSize = Math.max(1, ...entries.map(effectiveSize));

  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, alignContent: 'flex-start' }}>
      {entries.map(e => {
        const isDir = e.file_type === 2;
        const name = e.path.split('/').filter(Boolean).pop() || e.path;
        const size = effectiveSize(e);
        const pct = size / maxSize;
        const side = 90 + pct * 160;
        const isSelected = selectedPath === e.path;
        return (
          <div
            key={e.path}
            onClick={() => onSelect(e)}
            style={{
              width: side, height: side * 0.65, padding: '10px 12px',
              background: isDir ? theme.accent100 : theme.neutral100,
              border: isSelected ? `1.5px solid ${theme.accent}` : `1px solid ${theme.border}`,
              color: isDir ? theme.accent800 : theme.neutral800,
              cursor: 'pointer', fontSize: 13,
              display: 'flex', flexDirection: 'column', justifyContent: 'flex-end',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontWeight: 600, overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' }}>
              {isDir ? <FolderIcon size={14} color="currentColor" /> : <FileIcon size={14} color="currentColor" />}
              <span>{name}</span>
            </div>
            <div style={{ fontSize: 12, opacity: 0.75, marginTop: 4 }}>{formatBytes(size)}</div>
          </div>
        );
      })}
    </div>
  );
};

export default TreemapView;

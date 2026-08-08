import React, { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useFileSystem } from '../hooks/useFileSystem';
import { theme } from '../theme';
import { formatBytes, formatNumber } from '../format';
import { BlueprintFrame, SegmentedToggle, inputStyle, btnPrimaryStyle } from '../components/Blueprint';
import Header from '../components/Header';
import FileList from '../components/FileList';
import TreemapView from '../components/TreemapView';
import DetailPanel, { DetailSelection } from '../components/DetailPanel';
import { Entry } from '../types';
import { FolderIcon } from '../icons';

const FileBrowserPage: React.FC = () => {
  const [searchParams] = useSearchParams();
  const initialPath = searchParams.get('path') || '/';

  const {
    currentPath, entries, selectedItem, isLoading, error,
    sortConfig, setSortConfig, navigateTo, selectItem, setSelectedItem,
  } = useFileSystem(initialPath);

  const [viewMode, setViewMode] = useState<'table' | 'treemap'>('table');
  const [jumpPath, setJumpPath] = useState('');

  const requestSort = (key: keyof Entry) => {
    const direction = sortConfig.key === key && sortConfig.direction === 'asc' ? 'desc' : 'asc';
    setSortConfig({ key, direction });
  };

  const effectiveSize = (e: Entry) => (e.file_type === 2 ? (e.aggregates?.total_size_bytes ?? 0) : e.size_bytes);

  // Two passes over every entry in the directory, which is thousands of items
  // in the busy ones — keyed to the listing rather than redone on each render.
  const { totalSize, largest } = useMemo(() => ({
    totalSize: entries.reduce((a, e) => a + effectiveSize(e), 0),
    largest: entries.length ? entries.reduce((a, e) => (effectiveSize(e) > effectiveSize(a) ? e : a)) : null,
  }), [entries]);

  const selected: DetailSelection | null = useMemo(() => (selectedItem
    ? { kind: 'entry', entry: selectedItem, siblingsTotal: selectedItem.file_type === 2 ? undefined : totalSize }
    : null), [selectedItem, totalSize]);

  return (
    <div style={{ display: 'flex', flex: 1, overflow: 'hidden', background: theme.bg, color: theme.text }}>
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, overflow: 'hidden' }}>
        <div style={{ flex: 'none', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 16, padding: '16px 28px', borderBottom: `1px solid ${theme.border}` }}>
          <Header currentPath={currentPath} onNavigate={navigateTo} />
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <input
              value={jumpPath}
              onChange={e => setJumpPath(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter' && jumpPath) navigateTo(jumpPath); }}
              placeholder="Jump to path..."
              style={{ ...inputStyle, width: 200 }}
            />
            <div onClick={() => jumpPath && navigateTo(jumpPath)} style={{ ...btnPrimaryStyle, padding: '7px 14px' }}>Go</div>
            <SegmentedToggle
              options={[{ value: 'table', label: 'Table' }, { value: 'treemap', label: 'Treemap' }]}
              value={viewMode}
              onChange={setViewMode}
            />
          </div>
        </div>

        <div style={{ flex: 1, overflow: 'auto', padding: '24px 28px' }}>
          {error && (
            <div style={{ marginBottom: 16, padding: '10px 14px', fontSize: 13, color: theme.danger, border: `1px solid ${theme.danger}` }}>
              {error}
            </div>
          )}
          <div style={{ display: 'flex', gap: 14, marginBottom: 20 }}>
            {[
              { label: 'Total Size', value: formatBytes(totalSize) },
              { label: 'Items', value: formatNumber(entries.length) },
              { label: 'Largest Item', value: largest ? (largest.path.split('/').filter(Boolean).pop() || largest.path) : '—' },
            ].map(stat => (
              <BlueprintFrame key={stat.label} style={{ padding: '14px 16px', flex: 1 }}>
                <div style={{ fontSize: 10, fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', color: theme.accent }}>{stat.label}</div>
                <div style={{ fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 24, marginTop: 4 }}>{stat.value}</div>
              </BlueprintFrame>
            ))}
          </div>

          {isLoading && entries.length === 0 ? (
            <div style={{ padding: 80, textAlign: 'center', color: theme.textMuted }}>Loading...</div>
          ) : entries.length === 0 && error ? (
            // The banner above already says what went wrong; claiming the
            // directory is empty on top of it would contradict it.
            null
          ) : entries.length === 0 ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 10, padding: '80px 0', color: theme.textFaint }}>
              <FolderIcon size={40} color="currentColor" strokeWidth={1.3} />
              <div style={{ fontSize: 14 }}>This directory is empty</div>
            </div>
          ) : viewMode === 'table' ? (
            <FileList
              entries={entries}
              selectedPath={selectedItem?.path || null}
              sortConfig={sortConfig}
              onSelect={selectItem}
              onNavigate={navigateTo}
              onSort={requestSort}
            />
          ) : (
            <TreemapView
              entries={entries}
              selectedPath={selectedItem?.path || null}
              onSelect={selectItem}
              onNavigate={navigateTo}
            />
          )}
        </div>
      </div>

      <DetailPanel selected={selected} onClose={() => setSelectedItem(null)} />
    </div>
  );
};

export default FileBrowserPage;

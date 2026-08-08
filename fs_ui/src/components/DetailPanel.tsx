import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Entry, UserStats } from '../types';
import { theme, breakdownColors } from '../theme';
import { formatBytes, formatNumber, formatDate } from '../format';
import { fsApi } from '../api';
import { FolderIcon, FileIcon, UserIcon, XIcon, CopyIcon } from '../icons';

// The right-hand slide-in panel shared by File Browser, User & Group Usage and
// Advanced Search. `siblingsTotal` (sum of sizes in the entry's parent listing)
// lets a selected *file* show "% of directory" — callers that don't have that
// context (e.g. search results) simply omit it and the panel falls back to
// "No aggregate breakdown available".
export type DetailSelection =
  | { kind: 'entry'; entry: Entry; siblingsTotal?: number }
  | { kind: 'user'; user: UserStats };

interface DetailPanelProps {
  selected: DetailSelection | null;
  onClose: () => void;
  /** Overrides the default "View in Search" footer action (used on pages other than Advanced Search). */
  secondaryAction?: { label: string; onClick: (entry: Entry) => void };
}

interface BreakdownItem { name: string; pctLabel: string; color: string; flex: number; }
interface TimeRow { label: string; value: string; }
interface RangeRow { label: string; oldest: string; newest: string; }
interface Compare {
  showContents: boolean;
  ownSizeLabel: string;
  ownTimes: TimeRow[];
  contentsSizeLabel?: string;
  contentsCountLabel?: string;
  contentsTimes?: RangeRow[];
}

const rowStyle: React.CSSProperties = { display: 'flex', justifyContent: 'space-between', fontSize: 13, gap: 12 };

// Oldest/newest for a directory's recursive activity range (dir_stats'
// *_first/*_last), which fs_aggregator already computes across every
// descendant — no extra fetch needed.
const rangeRow = (label: string, first: number | undefined, last: number | undefined): RangeRow => ({
  label,
  oldest: first ? formatDate(first) : '—',
  newest: last ? formatDate(last) : '—',
});

const DetailPanel: React.FC<DetailPanelProps> = ({ selected, onClose, secondaryAction }) => {
  const navigate = useNavigate();
  const [children, setChildren] = useState<Entry[] | null>(null);

  const entryPath = selected?.kind === 'entry' ? selected.entry.path : null;
  const isDir = selected?.kind === 'entry' && selected.entry.file_type === 2;

  useEffect(() => {
    setChildren(null);
    if (!isDir || !entryPath) return;
    let cancelled = false;
    fsApi.listEntries(entryPath, true)
      .then(data => { if (!cancelled) setChildren(data); })
      .catch(() => { if (!cancelled) setChildren([]); });
    return () => { cancelled = true; };
  }, [isDir, entryPath]);

  let breakdownItems: BreakdownItem[] = [];
  let breakdownHeading = 'Contents breakdown';
  if (selected?.kind === 'entry' && isDir && children) {
    const sized = children
      .map(c => ({ name: c.path.split('/').filter(Boolean).pop() || c.path, size: c.file_type === 2 ? (c.aggregates?.total_size_bytes ?? 0) : c.size_bytes }))
      .sort((a, b) => b.size - a.size);
    const total = selected.entry.aggregates?.total_size_bytes || sized.reduce((a, c) => a + c.size, 0) || 1;
    breakdownItems = sized.slice(0, 4).map((c, i) => ({
      name: c.name,
      pctLabel: Math.round((c.size / total) * 100) + '%',
      color: breakdownColors[i % breakdownColors.length],
      flex: Math.max(c.size, 1),
    }));
  } else if (selected?.kind === 'entry' && !isDir && selected.siblingsTotal) {
    breakdownHeading = 'Share of parent directory';
    const size = selected.entry.size_bytes;
    const total = Math.max(selected.siblingsTotal, size, 1);
    breakdownItems = [
      { name: selected.entry.path.split('/').filter(Boolean).pop() || selected.entry.path, pctLabel: Math.round((size / total) * 100) + '% of directory', color: breakdownColors[0], flex: Math.max(size, 1) },
      { name: 'Rest of directory', pctLabel: '', color: theme.neutral300, flex: Math.max(total - size, 1) },
    ];
  } else {
    breakdownHeading = 'Breakdown';
  }
  const breakdownEmpty = breakdownItems.length === 0;

  const metadataRows: { label: string; value: string }[] = [];
  if (selected?.kind === 'user') {
    metadataRows.push(
      { label: 'ID type', value: selected.user.id_type.toUpperCase() },
      { label: 'Total size', value: formatBytes(selected.user.total_size_bytes) },
      { label: 'File count', value: formatNumber(selected.user.file_count) },
    );
  } else if (selected?.kind === 'entry') {
    const e = selected.entry;
    metadataRows.push(
      { label: 'Owner', value: `${e.user}:${e.group}` },
      { label: 'UID / GID', value: `${e.uid} / ${e.gid}` },
      { label: 'Permissions', value: e.permissions || '—' },
    );
  }

  const name = selected?.kind === 'user'
    ? selected.user.name
    : selected?.kind === 'entry'
      ? (selected.entry.path.split('/').filter(Boolean).pop() || selected.entry.path)
      : '';

  // "Size & activity" comparison block (entry selections only): the item's own
  // size/timestamps next to its contents' — for a directory that's the recursive
  // total/time-range straight off dir_stats, no extra fetch beyond what selecting
  // it already triggers for the breakdown above.
  let compare: Compare | null = null;
  if (selected?.kind === 'entry') {
    const e = selected.entry;
    const ownTimes: TimeRow[] = [
      { label: 'Modified', value: formatDate(e.mtime) },
      { label: 'Accessed', value: formatDate(e.atime) },
      { label: 'Created', value: formatDate(e.ctime) },
    ];
    if (isDir) {
      const contentsTotal = children?.reduce((a, c) => a + (c.file_type === 2 ? (c.aggregates?.total_size_bytes ?? 0) : c.size_bytes), 0);
      compare = {
        showContents: true,
        ownSizeLabel: formatBytes(e.aggregates?.total_size_bytes ?? 0),
        ownTimes,
        contentsSizeLabel: contentsTotal !== undefined ? formatBytes(contentsTotal) : '…',
        contentsCountLabel: children ? `${formatNumber(children.length)} ${children.length === 1 ? 'item' : 'items'}` : '…',
        contentsTimes: [
          rangeRow('Modified', e.aggregates?.mtime_first, e.aggregates?.mtime_last),
          rangeRow('Accessed', e.aggregates?.atime_first, e.aggregates?.atime_last),
          rangeRow('Created', e.aggregates?.ctime_first, e.aggregates?.ctime_last),
        ],
      };
    } else {
      compare = { showContents: false, ownSizeLabel: formatBytes(e.size_bytes), ownTimes };
    }
  }

  return (
    <div style={{
      flex: 'none', width: selected ? 380 : 0,
      borderLeft: selected ? `1px solid ${theme.border}` : 'none',
      background: theme.surface, overflow: 'hidden', transition: 'width 0.18s ease',
    }}>
      {selected && (
        <div style={{ width: 380, padding: 20, display: 'flex', flexDirection: 'column', gap: 18, height: '100%', overflow: 'auto', fontFamily: theme.fontBody }}>
          <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
              {selected.kind === 'user' && <UserIcon size={18} color={theme.accent} />}
              {selected.kind === 'entry' && isDir && <FolderIcon size={18} color={theme.accent} />}
              {selected.kind === 'entry' && !isDir && <FileIcon size={18} color={theme.textMuted2} />}
              <div style={{ fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 16, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{name}</div>
            </div>
            <span onClick={onClose} style={{ cursor: 'pointer', flex: 'none', padding: 4, color: theme.textMuted }}>
              <XIcon size={16} />
            </span>
          </div>
          {selected.kind === 'entry' && (
            <div style={{ fontFamily: 'ui-monospace,monospace', fontSize: 12, color: theme.textMuted, wordBreak: 'break-all', marginTop: -10 }}>
              {selected.entry.path}
            </div>
          )}

          {compare && (
            <div>
              <div style={{ fontSize: 10, fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', color: theme.textMuted, marginBottom: 8 }}>
                Size &amp; activity
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
                <div>
                  <div style={{ fontSize: 11, color: theme.textMuted, marginBottom: 4 }}>This item</div>
                  <div style={{ fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 22, marginBottom: 6 }}>{compare.ownSizeLabel}</div>
                  {compare.ownTimes.map((t, i) => (
                    <div key={i} style={{ display: 'flex', justifyContent: 'space-between', gap: 8, fontSize: 12, color: theme.textMuted2, marginTop: 3 }}>
                      <span>{t.label}</span><span style={{ fontFamily: 'ui-monospace,monospace', fontSize: 11.5 }}>{t.value}</span>
                    </div>
                  ))}
                </div>
                {compare.showContents && (
                  <div style={{ borderTop: `1px solid ${theme.border}`, paddingTop: 14 }}>
                    <div style={{ fontSize: 11, color: theme.accent700, marginBottom: 4 }}>Contents ({compare.contentsCountLabel})</div>
                    <div style={{ fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 22, color: theme.accent800, marginBottom: 6 }}>{compare.contentsSizeLabel}</div>
                    {compare.contentsTimes?.map((t, i) => (
                      <div key={i} style={{ marginTop: i === 0 ? 0 : 10, fontSize: 12 }}>
                        <div style={{ color: theme.textMuted2, marginBottom: 2 }}>{t.label}</div>
                        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                          <span style={{ color: theme.textMuted }}>Oldest:</span>
                          <span style={{ fontFamily: 'ui-monospace,monospace', fontSize: 11.5 }}>{t.oldest}</span>
                        </div>
                        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, marginTop: 2 }}>
                          <span style={{ color: theme.textMuted }}>Newest:</span>
                          <span style={{ fontFamily: 'ui-monospace,monospace', fontSize: 11.5 }}>{t.newest}</span>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          )}

          <div>
            <div style={{ fontSize: 10, fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', color: theme.textMuted, marginBottom: 8 }}>
              {breakdownHeading}
            </div>
            <div style={{ display: 'flex', height: 10, width: '100%', overflow: 'hidden', border: `1px solid ${theme.border}` }}>
              {breakdownItems.map((b, i) => (
                <div key={i} style={{ flex: b.flex, background: b.color }} />
              ))}
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 5, marginTop: 8 }}>
              {breakdownItems.map((b, i) => (
                <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 7, fontSize: 12 }}>
                  <span style={{ width: 8, height: 8, flex: 'none', background: b.color }} />
                  <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{b.name}</span>
                  <span style={{ color: theme.textMuted, fontVariantNumeric: 'tabular-nums' }}>{b.pctLabel}</span>
                </div>
              ))}
            </div>
            {breakdownEmpty && <div style={{ fontSize: 12, color: theme.textFaint }}>No aggregate breakdown available</div>}
          </div>

          <div style={{ height: 1, background: theme.border }} />

          <div>
            <div style={{ fontSize: 10, fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', color: theme.textMuted, marginBottom: 8 }}>
              Metadata
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {metadataRows.map((m, i) => (
                <div key={i} style={rowStyle}>
                  <span style={{ color: theme.textMuted, flex: 'none' }}>{m.label}</span>
                  <span style={{ textAlign: 'right', fontFamily: 'ui-monospace,monospace', fontSize: 12.5, overflow: 'hidden', textOverflow: 'ellipsis' }}>{m.value}</span>
                </div>
              ))}
            </div>
          </div>

          {selected.kind === 'entry' && (
            <div style={{ display: 'flex', gap: 8, marginTop: 'auto' }}>
              <div
                onClick={() => navigator.clipboard?.writeText(selected.entry.path)}
                style={{ flex: 1, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', gap: 6, padding: '8px 12px', fontSize: 13, cursor: 'pointer', border: `1px solid ${theme.border}` }}
              >
                <CopyIcon size={14} />
                Copy path
              </div>
              <div
                onClick={() => secondaryAction
                  ? secondaryAction.onClick(selected.entry)
                  : navigate(`/search?path=${encodeURIComponent(selected.entry.path)}`)}
                style={{ flex: 1, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '8px 12px', fontSize: 13, cursor: 'pointer', color: theme.accent700 }}
              >
                {secondaryAction?.label ?? 'View in Search'}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export default DetailPanel;

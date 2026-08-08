import React, { useLayoutEffect, useMemo, useRef, useState } from 'react';
import { Entry } from '../types';
import { theme } from '../theme';
import { formatBytes } from '../format';
import { FolderIcon, FileIcon } from '../icons';

interface TreemapViewProps {
  entries: Entry[];
  selectedPath: string | null;
  onSelect: (entry: Entry) => void;
  onNavigate: (path: string) => void;
}

type Sized = { entry: Entry; size: number; area: number };
type Tile = { entry: Entry; size: number; x: number; y: number; w: number; h: number };

// Smallest tile worth drawing, in px². Below this a tile is a hairline the eye
// can't hit or read, and squarify would start losing them to rounding.
const MIN_TILE_AREA = 100;

const effectiveSize = (e: Entry) => (e.file_type === 2 ? (e.aggregates?.total_size_bytes ?? 0) : e.size_bytes);

const entryName = (e: Entry) => e.path.split('/').filter(Boolean).pop() || e.path;

// Aspect ratio of the worst tile in a candidate row — the quantity the
// squarified algorithm minimises to keep tiles close to square.
const worstRatio = (areas: number[], side: number): number => {
  const sum = areas.reduce((a, b) => a + b, 0);
  if (sum <= 0 || side <= 0) return Infinity;
  const max = Math.max(...areas);
  const min = Math.min(...areas);
  const s2 = side * side;
  const sum2 = sum * sum;
  return Math.max((s2 * max) / sum2, sum2 / (s2 * min));
};

// Squarified treemap (Bruls, Huizing & van Wijk, 2000). Items must be sorted
// largest-first with `area` already scaled so the areas sum to w*h — then each
// tile's AREA is proportional to its size, which is the whole point: the
// previous version scaled the box's SIDE from 90px to 250px, so a 176 GB
// directory came out barely larger than an 18-byte file.
const squarify = (items: Sized[], x0: number, y0: number, w0: number, h0: number): Tile[] => {
  const tiles: Tile[] = [];
  let queue = items;
  let x = x0, y = y0, w = w0, h = h0;

  while (queue.length > 0 && w > 0.01 && h > 0.01) {
    const side = Math.min(w, h);

    // Grow the row while adding the next item improves the worst aspect ratio.
    let row = [queue[0]];
    let i = 1;
    while (i < queue.length) {
      const candidate = [...row, queue[i]];
      if (worstRatio(candidate.map(t => t.area), side) > worstRatio(row.map(t => t.area), side)) break;
      row = candidate;
      i++;
    }

    const rowArea = row.reduce((s, t) => s + t.area, 0);
    if (w >= h) {
      const rowW = rowArea / h;           // row runs down the left edge
      let oy = y;
      for (const t of row) {
        const th = t.area / rowW;
        tiles.push({ entry: t.entry, size: t.size, x, y: oy, w: rowW, h: th });
        oy += th;
      }
      x += rowW;
      w -= rowW;
    } else {
      const rowH = rowArea / w;           // row runs across the top edge
      let ox = x;
      for (const t of row) {
        const tw = t.area / rowH;
        tiles.push({ entry: t.entry, size: t.size, x: ox, y, w: tw, h: rowH });
        ox += tw;
      }
      y += rowH;
      h -= rowH;
    }
    queue = queue.slice(i);
  }

  return tiles;
};

const TreemapView: React.FC<TreemapViewProps> = ({ entries, selectedPath, onSelect, onNavigate }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);

  useLayoutEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    setWidth(el.getBoundingClientRect().width);
    const ro = new ResizeObserver(obs => setWidth(obs[0].contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Landscape rect that grows with the pane but stays a sane reading height.
  const height = Math.round(Math.min(Math.max(width * 0.5, 320), 680));

  // The whole layout — sort, threshold, squarify — depends only on the entries
  // and the rectangle, so it is computed once for those rather than on every
  // render. Selecting a tile re-renders this component, and re-running an
  // O(n log n) sort plus the squarify pass to move one highlight was the
  // difference between a snappy click and a visible stall on a big directory.
  const { tiles, hidden, hiddenBytes, grandTotal } = useMemo(() => {
    const sized = entries
      .map(entry => ({ entry, size: effectiveSize(entry) }))
      .sort((a, b) => b.size - a.size);

    // A directory spans ten orders of magnitude (176 GB next to an 18-byte
    // dotfile), so the tail is genuinely sub-pixel at true area. Those are
    // pulled out and listed below rather than drawn: left in, they became
    // slivers the layout eventually dropped on the floor, so entries silently
    // vanished.
    const area = width * height;
    const total = sized.reduce((s, e) => s + e.size, 0);
    const drawn = area > 0 && total > 0
      ? sized.filter(s => s.size > 0 && (s.size / total) * area >= MIN_TILE_AREA)
      : [];
    const drawnPaths = new Set(drawn.map(s => s.entry.path));
    const rest = sized.filter(s => !drawnPaths.has(s.entry.path));

    // Normalised over the drawn items so the tiles fill the rect exactly.
    const drawnTotal = drawn.reduce((s, e) => s + e.size, 0);
    return {
      tiles: drawnTotal > 0
        ? squarify(drawn.map(s => ({ ...s, area: (s.size / drawnTotal) * area })), 0, 0, width, height)
        : [],
      hidden: rest,
      hiddenBytes: rest.reduce((s, e) => s + e.size, 0),
      grandTotal: total,
    };
  }, [entries, width, height]);

  return (
    <div ref={containerRef}>
      <div style={{ position: 'relative', width: '100%', height, border: `1px solid ${theme.border}`, overflow: 'hidden' }}>
        {tiles.map(t => {
          const isDir = t.entry.file_type === 2;
          const isSelected = selectedPath === t.entry.path;
          const name = entryName(t.entry);
          // A tile only gets a label if the label would fit — below that the
          // tooltip carries the name, and the map stays readable.
          const showName = t.w >= 54 && t.h >= 22;
          const showSize = t.w >= 74 && t.h >= 42;
          const showIcon = t.w >= 96 && t.h >= 30;
          return (
            <div
              key={t.entry.path}
              onClick={() => onSelect(t.entry)}
              onDoubleClick={() => { if (isDir) onNavigate(t.entry.path); }}
              title={`${name} — ${formatBytes(t.size)}${isDir ? ' (double-click to open)' : ''}`}
              style={{
                position: 'absolute',
                left: t.x, top: t.y, width: t.w, height: t.h,
                boxSizing: 'border-box',
                padding: showName ? '6px 8px' : 0,
                background: isSelected ? theme.accent200 : isDir ? theme.accent100 : theme.neutral100,
                border: isSelected ? `1.5px solid ${theme.accent}` : `1px solid ${theme.border}`,
                color: isDir ? theme.accent800 : theme.neutral800,
                cursor: 'pointer', fontSize: 13, overflow: 'hidden',
                display: 'flex', flexDirection: 'column', justifyContent: 'flex-end',
              }}
            >
              {showName && (
                <div style={{ display: 'flex', alignItems: 'center', gap: 5, fontWeight: 600, overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' }}>
                  {showIcon && (isDir ? <FolderIcon size={13} color="currentColor" /> : <FileIcon size={13} color="currentColor" />)}
                  <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{name}</span>
                </div>
              )}
              {showSize && <div style={{ fontSize: 11, opacity: 0.75, marginTop: 2 }}>{formatBytes(t.size)}</div>}
            </div>
          );
        })}

        {tiles.length === 0 && (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: theme.textMuted, fontSize: 14 }}>
            Nothing here has a measurable size.
          </div>
        )}
      </div>

      {hidden.length > 0 && (
        <div style={{ marginTop: 14 }}>
          <div style={{ fontSize: 11, fontWeight: 600, letterSpacing: '0.06em', textTransform: 'uppercase', color: theme.textMuted, marginBottom: 8 }}>
            {hidden.length} {hidden.length === 1 ? 'entry' : 'entries'} too small to draw
            {grandTotal > 0 && ` — ${formatBytes(hiddenBytes)}, under ${((hiddenBytes / grandTotal) * 100).toFixed(1)}% of this directory`}
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
            {hidden.map(({ entry, size }) => {
              const isDir = entry.file_type === 2;
              const isSelected = selectedPath === entry.path;
              return (
                <div
                  key={entry.path}
                  onClick={() => onSelect(entry)}
                  onDoubleClick={() => { if (isDir) onNavigate(entry.path); }}
                  title={`${entryName(entry)} — ${formatBytes(size)}${isDir ? ' (double-click to open)' : ''}`}
                  style={{
                    display: 'flex', alignItems: 'center', gap: 5, padding: '5px 9px', maxWidth: 260,
                    background: isSelected ? theme.accent200 : isDir ? theme.accent100 : theme.neutral100,
                    border: isSelected ? `1.5px solid ${theme.accent}` : `1px solid ${theme.border}`,
                    color: isDir ? theme.accent800 : theme.neutral800,
                    cursor: 'pointer', fontSize: 12, whiteSpace: 'nowrap', overflow: 'hidden',
                  }}
                >
                  {isDir ? <FolderIcon size={12} color="currentColor" /> : <FileIcon size={12} color="currentColor" />}
                  <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{entryName(entry)}</span>
                  <span style={{ opacity: 0.6, flexShrink: 0 }}>{formatBytes(size)}</span>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
};

export default TreemapView;

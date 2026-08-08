import { RefObject, useCallback, useLayoutEffect, useRef, useState } from 'react';

// Renders only the rows near the viewport, so a 10 000-row result set costs the
// same as a screenful.
//
// Why this exists: every interaction on a big search — selecting a row, ticking
// a column, re-sorting — re-renders the results table, and at 6 000 rows that
// was a 0.7 s frame. Measurement showed the cost was React reconciling and the
// browser mutating thousands of <tr>/<td> nodes, *not* table layout (forcing
// `table-layout: fixed` moved a column toggle 683 ms → 679 ms). Rendering fewer
// rows is the only thing that addresses it.
//
// Heights are measured rather than assumed. The Path column wraps, so rows are
// 37 px at a wide viewport but 55 or 73 px at a narrow one, in no fixed pattern
// — a constant row height would put the spacers wrong and make the scrollbar
// drift. Unmeasured rows use `estimateHeight` until they are scrolled into view
// once.

type Options = {
  count: number;
  /** Height assumed for rows that have not been on screen yet. */
  estimateHeight: number;
  /** Rows drawn beyond each edge of the viewport, to cover fast scrolling. */
  overscan?: number;
  /** Below this many rows, windowing is skipped entirely (see WINDOW_THRESHOLD). */
  threshold?: number;
  /** The scrolling ancestor. */
  scrollRef: RefObject<HTMLElement | null>;
  /** Element the rows start at; its offset inside the scroller anchors the maths. */
  anchorRef: RefObject<HTMLElement | null>;
  /** Changing this discards every measurement (different rows, different columns). */
  resetKey: unknown;
};

export type WindowedRows = {
  start: number;
  end: number;
  padTop: number;
  padBottom: number;
  /** Ref callback for row `index`, so its real height replaces the estimate. */
  rowRef: (index: number) => (el: HTMLElement | null) => void;
};

// Under a few hundred rows the table is cheap to render whole, and windowing
// only takes things away (browser find-in-page, select-all-and-copy). Above it,
// those are already unreliable — the user cannot see 6 000 rows either — and
// interactivity matters more.
export const WINDOW_THRESHOLD = 300;

export const useWindowedRows = ({
  count, estimateHeight, overscan = 10, threshold = WINDOW_THRESHOLD, scrollRef, anchorRef, resetKey,
}: Options): WindowedRows => {
  const heightsRef = useRef<number[]>([]);
  const rowElsRef = useRef(new Map<number, HTMLElement>());
  const [range, setRange] = useState({ start: 0, end: 0 });

  const windowed = count > threshold;

  // Walks the height table from the top on each recompute. That is O(count) per
  // scroll frame, but it is a tight add-and-compare loop over a plain number[]
  // — a few tens of microseconds at 10 000 rows, well under the cost of the
  // binary search plus prefix-sum array it would take to avoid it.
  const compute = useCallback(() => {
    const scroller = scrollRef.current;
    const anchor = anchorRef.current;
    if (!scroller || !anchor) return;

    const heights = heightsRef.current;
    const anchorTop = anchor.getBoundingClientRect().top
      - scroller.getBoundingClientRect().top
      + scroller.scrollTop;
    const viewTop = scroller.scrollTop - anchorTop;
    const viewBottom = viewTop + scroller.clientHeight;

    let start = 0;
    let acc = 0;
    while (start < count && acc + (heights[start] ?? estimateHeight) < viewTop) {
      acc += heights[start] ?? estimateHeight;
      start++;
    }
    let end = start;
    let accEnd = acc;
    while (end < count && accEnd < viewBottom) {
      accEnd += heights[end] ?? estimateHeight;
      end++;
    }

    const nextStart = Math.max(0, start - overscan);
    const nextEnd = Math.min(count, end + overscan);
    setRange(prev => (prev.start === nextStart && prev.end === nextEnd ? prev : { start: nextStart, end: nextEnd }));
  }, [count, estimateHeight, overscan, scrollRef, anchorRef]);

  // A new result set or a different column set invalidates every measurement:
  // the same row index is now a different row, or the same row is a different
  // height. Runs before paint so no frame is drawn against stale offsets.
  useLayoutEffect(() => {
    heightsRef.current = [];
    rowElsRef.current.clear();
    setRange({ start: 0, end: 0 });
    if (windowed) compute();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [count, resetKey, windowed]);

  useLayoutEffect(() => {
    if (!windowed) return;
    const scroller = scrollRef.current;
    if (!scroller) return;

    let frame = 0;
    const onScroll = () => {
      if (frame) return;
      frame = requestAnimationFrame(() => { frame = 0; compute(); });
    };
    scroller.addEventListener('scroll', onScroll, { passive: true });
    // Resizing changes both the viewport and how many rows wrap, so every
    // measurement taken at the old width is suspect.
    const ro = new ResizeObserver(() => { heightsRef.current = []; compute(); });
    ro.observe(scroller);
    compute();

    return () => {
      scroller.removeEventListener('scroll', onScroll);
      ro.disconnect();
      if (frame) cancelAnimationFrame(frame);
    };
  }, [windowed, compute, scrollRef]);

  // Replace estimates with what the rows actually measured. Deliberately has no
  // dependency array: it has to run after every commit, because that is when
  // newly windowed-in rows exist to measure. It only recomputes the range when
  // a height actually moved, so it settles after one extra pass instead of
  // looping.
  useLayoutEffect(() => {
    if (!windowed) return;
    let changed = false;
    rowElsRef.current.forEach((el, index) => {
      const h = el.getBoundingClientRect().height;
      if (h > 0 && Math.abs((heightsRef.current[index] ?? estimateHeight) - h) > 0.5) {
        heightsRef.current[index] = h;
        changed = true;
      }
    });
    if (changed) compute();
  });

  const rowRef = useCallback((index: number) => (el: HTMLElement | null) => {
    if (el) rowElsRef.current.set(index, el);
    else rowElsRef.current.delete(index);
  }, []);

  if (!windowed) {
    return { start: 0, end: count, padTop: 0, padBottom: 0, rowRef };
  }

  const heights = heightsRef.current;
  const sum = (from: number, to: number) => {
    let total = 0;
    for (let i = from; i < to; i++) total += heights[i] ?? estimateHeight;
    return total;
  };
  const end = Math.min(range.end, count);
  const start = Math.min(range.start, end);

  return { start, end, padTop: sum(0, start), padBottom: sum(end, count), rowRef };
};

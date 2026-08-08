import { useState, useEffect, useCallback, useRef } from 'react';
import { Entry, SortConfig } from '../types';
import { ApiError, fsApi } from '../api';

// A 404 here is the ordinary case of asking for something the last scan didn't
// cover, not a fault — say that instead of showing the user a status code.
const describe = (e: unknown, path: string, noun: string) => {
  if (e instanceof ApiError && e.status === 404) {
    return `${path} is not in the index. It may have been added since the last scan, or never scanned.`;
  }
  return e instanceof Error ? e.message : `Could not load this ${noun}.`;
};

export const useFileSystem = (initialPath = '/') => {
  const [currentPath, setCurrentPath] = useState(initialPath);
  const [entries, setEntries] = useState<Entry[]>([]);
  const [selectedItem, setSelectedItem] = useState<Entry | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sortConfig, setSortConfig] = useState<SortConfig>({ key: 'path', direction: 'asc' });

  // One controller per kind of request. Listing a big directory can take longer
  // than listing the small one clicked after it, so without this the slower
  // reply lands second and the browser shows a directory the user has already
  // navigated away from — with the breadcrumb still naming the new one.
  const listAbort = useRef<AbortController | null>(null);
  const detailAbort = useRef<AbortController | null>(null);

  useEffect(() => () => {
    listAbort.current?.abort();
    detailAbort.current?.abort();
  }, []);

  const navigateTo = useCallback(async (path: string) => {
    listAbort.current?.abort();
    const controller = new AbortController();
    listAbort.current = controller;

    setCurrentPath(path);
    setSelectedItem(null);
    setError(null);
    setIsLoading(true);
    try {
      const data = await fsApi.listEntries(path, true, controller.signal);
      setEntries(data);
    } catch (e) {
      if (controller.signal.aborted) return;   // superseded, not a failure
      setEntries([]);
      setError(describe(e, path, 'directory'));
    } finally {
      if (!controller.signal.aborted) setIsLoading(false);
    }
  }, []);

  // Read through a ref so re-clicking the selected row can be skipped without
  // making selectItem depend on the selection — as a prop of memoised rows it
  // has to keep a stable identity.
  const selectedPathRef = useRef<string | null>(null);
  selectedPathRef.current = selectedItem?.path ?? null;

  const selectItem = useCallback(async (entry: Entry) => {
    if (selectedPathRef.current === entry.path) return;
    detailAbort.current?.abort();
    const controller = new AbortController();
    detailAbort.current = controller;

    setError(null);
    setIsLoading(true);
    try {
      const data = await fsApi.getEntryStats(entry.path, entry.file_type, true, controller.signal);
      setSelectedItem(data);
    } catch (e) {
      if (controller.signal.aborted) return;
      setSelectedItem(null);
      // Previously swallowed into console.error, which made a failed lookup
      // look like a row that simply refused to be selected.
      setError(describe(e, entry.path, 'entry'));
    } finally {
      if (!controller.signal.aborted) setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    navigateTo(initialPath);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [navigateTo]);

  return {
    currentPath,
    setCurrentPath,
    entries,
    selectedItem,
    setSelectedItem,
    isLoading,
    error,
    sortConfig,
    setSortConfig,
    navigateTo,
    selectItem,
  };
};

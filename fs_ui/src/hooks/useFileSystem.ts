import { useState, useEffect, useCallback } from 'react';
import { Entry, SortConfig } from '../types';
import { fsApi } from '../api';

export const useFileSystem = () => {
  const [currentPath, setCurrentPath] = useState('/');
  const [entries, setEntries] = useState<Entry[]>([]);
  const [selectedItem, setSelectedItem] = useState<Entry | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [sortConfig, setSortConfig] = useState<SortConfig>({ key: 'path', direction: 'asc' });

  const navigateTo = useCallback(async (path: string) => {
    setCurrentPath(path);
    setSelectedItem(null);
    setIsLoading(true);
    try {
      const data = await fsApi.listEntries(path);
      setEntries(data);
    } catch (e) {
      console.error("Navigation error", e);
      setEntries([]);
    } finally {
      setIsLoading(false);
    }
  }, []);

  const selectItem = async (entry: Entry) => {
    if (selectedItem && selectedItem.path === entry.path) return;
    setIsLoading(true);
    try {
      const data = await fsApi.getEntryStats(entry.path, entry.file_type);
      setSelectedItem(data);
    } catch (e) {
      console.error("Detail error", e);
      setSelectedItem(null);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    navigateTo('/');
  }, [navigateTo]);

  return {
    currentPath,
    setCurrentPath,
    entries,
    selectedItem,
    setSelectedItem,
    isLoading,
    sortConfig,
    setSortConfig,
    navigateTo,
    selectItem
  };
};

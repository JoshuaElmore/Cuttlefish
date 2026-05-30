import React, { useState } from 'react';
import { useFileSystem } from '../hooks/useFileSystem';
import { theme } from '../theme';
import Header from '../components/Header';
import FileList from '../components/FileList';
import FileDetails from '../components/FileDetails';
import { Entry } from '../types';

const FileBrowserPage: React.FC = () => {
  const {
    currentPath,
    entries,
    selectedItem,
    isLoading,
    sortConfig,
    setSortConfig,
    navigateTo,
    selectItem
  } = useFileSystem();

  const [searchPath, setSearchPath] = useState('');

  const formatNumber = (num: number) => num.toLocaleString();
  const formatSize = (bytes: number) => {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const requestSort = (key: keyof Entry) => {
    const direction = (sortConfig.key === key && sortConfig.direction === 'asc') ? 'desc' : 'asc';
    setSortConfig({ key, direction });
  };

  return (
    <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
      <div style={{ 
        flex: 2, 
        display: 'flex', 
        flexDirection: 'column', 
        borderRight: `1px solid ${theme.border}`,
        background: theme.panelBg,
        backdropFilter: 'blur(10px)'
      }}>
        <Header 
          currentPath={currentPath} 
          searchPath={searchPath} 
          setSearchPath={setSearchPath} 
          onNavigate={navigateTo} 
        />
        
        <div style={{ flex: 1, overflowY: 'auto' }}>
          {currentPath !== '/' && (
            <div 
              onClick={() => {
                const parts = currentPath.split('/').filter(Boolean);
                parts.pop();
                navigateTo('/' + parts.join('/'));
              }}
              style={{ 
                padding: '12px 20px', cursor: 'pointer', background: 'rgba(255,255,255,0.05)', 
                fontWeight: 600, borderBottom: `1px solid ${theme.border}`, color: theme.accentBlue,
                transition: 'background 0.2s'
              }}
            >
              <span style={{ cursor: 'pointer' }}>⬅️ .. (Parent)</span>
            </div>
          )}
          <FileList 
            entries={entries} 
            selectedPath={selectedItem?.path || null}
            sortConfig={sortConfig} 
            onSelect={selectItem} 
            onNavigate={navigateTo} 
            onSort={requestSort} 
            isLoading={isLoading}
            formatSize={formatSize}
          />
        </div>
      </div>

      <div style={{ 
        flex: 1, 
        padding: '40px', 
        overflowY: 'auto', 
        background: 'rgba(0,0,0,0.1)',
        display: 'flex', 
        flexDirection: 'column', 
        gap: '24px' 
      }}>
        {selectedItem ? (
          <FileDetails 
            item={selectedItem} 
            formatSize={formatSize} 
            formatNumber={formatNumber} 
          />
        ) : (
          <div style={{ textAlign: 'center', color: theme.textMuted, marginTop: '20vh' }}>
            <h3 style={{ fontWeight: 400, opacity: 0.6 }}>Select a file or folder to view its details</h3>
          </div>
        )}
      </div>
    </div>
  );
};

export default FileBrowserPage;

import React, { useState } from 'react';
import { useFileSystem } from './hooks/useFileSystem';
import { theme } from './theme';
import Header from './components/Header';
import FileList from './components/FileList';
import FileDetails from './components/FileDetails';
import UserBrowser from './components/UserBrowser';
import { Entry } from './types';

const CuttlefishExplorer: React.FC = () => {
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
  const [activeTab, setActiveTab] = useState<'fileBrowser' | 'userBrowser'>('fileBrowser');

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
    <div style={{ 
      display: 'flex', 
      height: '100vh', 
      fontFamily: '"Inter", system-ui, -apple-system, sans-serif', 
      background: theme.bgGradient, 
      color: theme.textMain,
      overflow: 'hidden'
    }}>
      {/* Left Sidebar - Now used for Navigation Tabs */}
      <div style={{ 
        width: 260, 
        background: 'rgba(0,0,0,0.2)', 
        borderRight: `1px solid ${theme.border}`,
        display: 'flex',
        flexDirection: 'column'
      }}>
        <div style={{ padding: '24px', fontSize: '14px', fontWeight: 600, opacity: 0.5, textAlign: 'center', marginBottom: '20px' }}>
          EXPLORER
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', padding: '0 12px' }}>
          {[
            { id: 'fileBrowser', label: '📁 File Browser' },
            { id: 'userBrowser', label: '👤 User Browser' }
          ].map(tab => (
            <div 
              key={tab.id}
              onClick={() => setActiveTab(tab.id as any)}
              style={{ 
                padding: '12px 16px', 
                cursor: 'pointer', 
                borderRadius: '8px',
                background: activeTab === tab.id ? 'rgba(255,255,255,0.1)' : 'transparent',
                color: activeTab === tab.id ? theme.textMain : theme.textMuted,
                fontWeight: activeTab === tab.id ? 600 : 400,
                transition: 'all 0.2s',
                fontSize: '14px',
                border: activeTab === tab.id ? `1px solid ${theme.border}` : '1px solid transparent'
              }}
            >
              {tab.label}
            </div>
          ))}
        </div>
      </div>

      {/* Main Content Area */}
      <div style={{ 
        flex: 1, 
        display: 'flex', 
        overflow: 'hidden' 
      }}>
        {activeTab === 'fileBrowser' ? (
          <>
            {/* File Browser Center Panel */}
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

            {/* File Browser Right Bar */}
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
                <div style={{ textAlign: 'center', color: theme.textMuted, marginTop: '20vh', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '20px' }}>
                  <span style={{ fontSize: '64px', opacity: 0.3 }}>🦑</span>
                  <h3 style={{ fontWeight: 400, opacity: 0.6 }}>Select a file or folder to view its details</h3>
                </div>
              )}
            </div>
          </>
        ) : (
          <UserBrowser formatSize={formatSize} formatNumber={formatNumber} />
        )}
      </div>

      <style>{`
        @keyframes fadeIn {
          from { opacity: 0; transform: translateY(10px); }
          to { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </div>
  );
};

export default CuttlefishExplorer;

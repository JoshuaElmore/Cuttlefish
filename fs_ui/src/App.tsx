import React, { useState, useEffect, useCallback } from 'react';

interface DirAggregates {
  total_size_bytes: number;
  file_count: number;
  mtime_first: number;
  mtime_last: number;
  atime_first: number;
  atime_last: number;
  ctime_first: number;
  ctime_last: number;
}

interface Entry {
  path: string;
  size_bytes: number;
  file_type: number;
  permissions: string;
  uid: number;
  gid: number;
  user: string;
  group: string;
  mtime: number;
  atime: number;
  ctime: number;
  aggregates?: DirAggregates;
}

type SortConfig = {
  key: keyof Entry | null;
  direction: 'asc' | 'desc';
};

const CuttlefishExplorer: React.FC = () => {
  const [currentPath, setCurrentPath] = useState('/');
  const [entries, setEntries] = useState<Entry[]>([]);
  const [selectedItem, setSelectedItem] = useState<Entry | null>(null);
  const [searchPath, setSearchPath] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [sortConfig, setSortConfig] = useState<SortConfig>({ key: 'path', direction: 'asc' });

  const formatNumber = (num: number) => {
    return num.toLocaleString();
  };

  const formatSize = (bytes: number) => {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const navigateTo = useCallback(async (path: string) => {
    setCurrentPath(path);
    setSelectedItem(null);
    setIsLoading(true);
    try {
      const res = await fetch(`/api/list?path=${encodeURIComponent(path)}&include_stats=true`);
      if (!res.ok) throw new Error(`Server error: ${res.status}`);
      const data = await res.json();
      setEntries(Array.isArray(data) ? data : []);
    } catch (e) {
      console.error("Navigation error", e);
      setEntries([]);
    } finally {
      setIsLoading(false);
    }
  }, []);

  const selectPath = async (entry: Entry) => {
    if (selectedItem && selectedItem.path === entry.path) return;

    setIsLoading(true);
    try {
      const endpoint = entry.file_type === 2 ? '/api/dir/stats' : '/api/file/stats';
      const res = await fetch(`${endpoint}?path=${encodeURIComponent(entry.path)}&include_stats=true`);
      
      if (!res.ok) {
        throw new Error(`API error: ${res.status}`);
      }
      
      const data = await res.json();
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

  const theme = {
    bg: '#0f0c29',
    bgGradient: 'linear-gradient(to bottom, #0f0c29, #302b63, #24243e)',
    panelBg: 'rgba(20, 20, 40, 0.6)',
    accentPurple: '#a855f7',
    accentBlue: '#3b82f6',
    textMain: '#e2e8f0',
    textMuted: '#94a3b8',
    cardBg: 'rgba(30, 41, 59, 0.7)',
    border: 'rgba(148, 163, 184, 0.2)',
    hover: 'rgba(168, 85, 247, 0.2)',
    selected: 'rgba(168, 85, 247, 0.4)',
  };

  const gridTemplate = 'minmax(200px, 2fr) 120px 150px 100px 150px 80px';

  const requestSort = (key: keyof Entry) => {
    let direction: 'asc' | 'desc' = 'asc';
    if (sortConfig.key === key && sortConfig.direction === 'asc') {
      direction = 'desc';
    }
    setSortConfig({ key, direction });
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

  const getSortIcon = (key: keyof Entry) => {
    if (sortConfig.key !== key) return '↕️';
    return sortConfig.direction === 'asc' ? '🔼' : '🔽';
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
      <div style={{ 
        width: 260, 
        background: 'rgba(0,0,0,0.2)', 
        borderRight: `1px solid ${theme.border}`,
      }}>
        <div style={{ padding: '24px', opacity: 0.3, fontSize: '12px', textAlign: 'center', marginTop: '20px' }}>
          Sidebar Space
        </div>
      </div>

      <div style={{ 
        flex: 2, 
        display: 'flex', 
        flexDirection: 'column', 
        borderRight: `1px solid ${theme.border}`,
        background: theme.panelBg,
        backdropFilter: 'blur(10px)'
      }}>
        <div style={{ 
          padding: '24px', 
          background: 'rgba(0,0,0,0.3)', 
          borderBottom: `1px solid ${theme.border}`,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <div style={{ 
              width: 32, height: 32, borderRadius: 8, 
              background: `linear-gradient(135deg, ${theme.accentPurple}, ${theme.accentBlue})`,
              display: 'flex', alignItems: 'center', justifyContent: 'center', fontWeight: 'bold'
            }}>🦑</div>
            <h2 style={{ margin: 0, fontSize: '1.25rem', fontWeight: 700 }}>Cuttlefish</h2>
          </div>
          <div style={{ display: 'flex', gap: '8px' }}>
            <input 
              style={{ 
                padding: '8px 12px', borderRadius: '6px', border: `1px solid ${theme.border}`, 
                background: 'rgba(0,0,0,0.3)', color: 'white', outline: 'none', fontSize: '14px', width: '250px'
              }} 
              value={searchPath} 
              onChange={e => setSearchPath(e.target.value)} 
              placeholder="Jump to path..." 
              onKeyDown={e => e.key === 'Enter' && navigateTo(searchPath)}
            />
            <button 
              onClick={() => navigateTo(searchPath)}
              style={{ 
                padding: '8px 16px', borderRadius: '6px', border: 'none', 
                background: theme.accentPurple, color: 'white', fontWeight: 600, cursor: 'pointer'
              }}
            >
              Go
            </button>
          </div>
        </div>

        <div style={{ padding: '16px', background: 'rgba(0,0,0,0.1)', borderBottom: `1px solid ${theme.border}`, fontFamily: 'monospace', fontSize: '13px', color: theme.accentBlue }}>
          {currentPath}
        </div>

        <div style={{ overflowY: 'auto', flex: 1 }}>
          {isLoading && entries.length === 0 ? (
            <div style={{ padding: '40px', textAlign: 'center', color: theme.textMuted }}>Loading...</div>
          ) : (
            <>
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
                <div onClick={() => requestSort('path')} style={{ padding: '12px 20px', cursor: 'pointer', userSelect: 'none' }}>
                  Name {getSortIcon('path')}
                </div>
                <div onClick={() => requestSort('file_type')} style={{ padding: '12px 20px', cursor: 'pointer', userSelect: 'none' }}>
                  Type {getSortIcon('file_type')}
                </div>
                <div onClick={() => requestSort('size_bytes')} style={{ padding: '12px 20px', textAlign: 'right', cursor: 'pointer', userSelect: 'none' }}>
                  Size {getSortIcon('size_bytes')}
                </div>
                <div onClick={() => requestSort('uid')} style={{ padding: '12px 20px', textAlign: 'right', cursor: 'pointer', userSelect: 'none' }}>
                  Owner {getSortIcon('uid')}
                </div>
                <div onClick={() => requestSort('mtime')} style={{ padding: '12px 20px', textAlign: 'right', cursor: 'pointer', userSelect: 'none' }}>
                  Modified {getSortIcon('mtime')}
                </div>
                <div style={{ padding: '12px 20px', textAlign: 'center' }}>Action</div>
              </div>
              {(sortedEntries || []).map((e, i) => (
                <div 
                  key={i} 
                  onClick={() => selectPath(e)}
                  onDoubleClick={() => {
                    if (e.file_type === 2) navigateTo(e.path);
                  }}
                  style={{ 
                    display: 'grid', 
                    gridTemplateColumns: gridTemplate, 
                    padding: '12px 20px', borderBottom: `1px solid ${theme.border}`, 
                    cursor: 'pointer', alignItems: 'center', transition: 'background 0.2s',
                    fontSize: '14px',
                    backgroundColor: selectedItem?.path === e.path ? theme.selected : 'transparent'
                  }}
                  onMouseEnter={(ev) => {
                    if (selectedItem?.path !== e.path) {
                      ev.currentTarget.style.backgroundColor = theme.hover;
                    }
                  }}
                  onMouseLeave={(ev) => {
                    ev.currentTarget.style.backgroundColor = selectedItem?.path === e.path ? theme.selected : 'transparent';
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {e.file_type === 2 ? '📁' : '📄'} {e.path.split('/').pop()}
                  </div>
                  <div style={{ color: theme.textMuted, fontSize: '13px' }}>
                    {e.file_type === 2 ? 'Directory' : 'File' }
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
                          navigateTo(e.path);
                        }}
                        style={{ 
                          padding: '4px 8px', borderRadius: '4px', border: 'none', 
                          background: theme.accentBlue, color: 'white', fontSize: '11px', 
                          fontWeight: 600, cursor: 'pointer', transition: 'opacity 0.2s'
                        }}
                        onMouseEnter={(ev) => ev.currentTarget.style.opacity = '0.8'}
                        onMouseLeave={(ev) => ev.currentTarget.style.opacity = '1'}
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
        {selectedItem && (
          <div style={{ 
            background: theme.cardBg, padding: '30px', borderRadius: '20px', 
            boxShadow: '0 10px 30px rgba(0,0,0,0.2)', border: `1px solid ${theme.border}`,
            animation: 'fadeIn 0.3s ease-out'
          }}>
            <h3 style={{ marginTop: 0, fontSize: '1.5rem', marginBottom: '24px', display: 'flex', alignItems: 'center', gap: '12px' }}>
              <span style={{ fontSize: '1.8rem' }}>{selectedItem.file_type === 2 ? '📁' : '📄'}</span> 
              {selectedItem.file_type === 2 ? 'Directory Info' : 'File Metadata'}
            </h3>
            <div style={{ marginBottom: '24px', padding: '12px', background: 'rgba(0,0,0,0.3)', borderRadius: '10px', border: `1px solid ${theme.border}`, fontFamily: 'monospace', fontSize: '13px', color: theme.accentBlue, wordBreak: 'break-all' }}>
              {selectedItem.path}
            </div>
            
            <div style={{ marginBottom: '32px' }}>
              <h4 style={{ fontSize: '0.9rem', color: theme.textMuted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '16px', borderBottom: `1px solid ${theme.border}`, paddingBottom: '8px' }}>
                👤 Identity & Ownership
              </h4>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: '15px' }}>
                <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: '10px', border: `1px solid ${theme.border}` }}>
                  <strong style={{ display: 'block', fontSize: '11px', color: theme.textMuted, textTransform: 'uppercase' }}>User</strong>
                  <div style={{ fontSize: '16px', fontWeight: 600 }}>{selectedItem.user} <span style={{ color: theme.textMuted, fontSize: '12px' }}>(UID: {selectedItem.uid})</span></div>
                </div>
                <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: '10px', border: `1px solid ${theme.border}` }}>
                  <strong style={{ display: 'block', fontSize: '11px', color: theme.textMuted, textTransform: 'uppercase' }}>Group</strong>
                  <div style={{ fontSize: '16px', fontWeight: 600 }}>{selectedItem.group} <span style={{ color: theme.textMuted, fontSize: '12px' }}>(GID: {selectedItem.gid})</span></div>
                </div>
                <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: '10px', border: `1px solid ${theme.border}` }}>
                  <strong style={{ display: 'block', fontSize: '11px', color: theme.textMuted, textTransform: 'uppercase' }}>Permissions</strong>
                  <div style={{ fontSize: '16px', fontWeight: 600, color: theme.accentPurple }}>{selectedItem.permissions}</div>
                </div>
              </div>
            </div>

            <div style={{ marginBottom: '32px' }}>
              <h4 style={{ fontSize: '0.9rem', color: theme.textMuted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '16px', borderBottom: `1px solid ${theme.border}`, paddingBottom: '8px' }}>
                🕒 Timestamps
              </h4>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: '15px' }}>
                <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: '10px', border: `1px solid ${theme.border}` }}>
                  <strong style={{ display: 'block', fontSize: '11px', color: theme.textMuted, textTransform: 'uppercase' }}>Modified</strong>
                  <div style={{ fontSize: '14px' }}>{new Date(selectedItem.mtime * 1000).toLocaleString()}</div>
                </div>
                <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: '10px', border: `1px solid ${theme.border}` }}>
                  <strong style={{ display: 'block', fontSize: '11px', color: theme.textMuted, textTransform: 'uppercase' }}>Accessed</strong>
                  <div style={{ fontSize: '14px' }}>{new Date(selectedItem.atime * 1000).toLocaleString()}</div>
                </div>
                <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: '10px', border: `1px solid ${theme.border}` }}>
                  <strong style={{ display: 'block', fontSize: '11px', color: theme.textMuted, textTransform: 'uppercase' }}>Changed</strong>
                  <div style={{ fontSize: '14px' }}>{new Date(selectedItem.ctime * 1000).toLocaleString()}</div>
                </div>
              </div>
            </div>

            {selectedItem.file_type === 2 && selectedItem.aggregates && (
              <div style={{ marginBottom: '32px' }}>
                <h4 style={{ fontSize: '0.9rem', color: theme.textMuted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '16px', borderBottom: `1px solid ${theme.border}`, paddingBottom: '8px' }}>
                  📦 Aggregated Stats
                </h4>
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: '20px' }}>
                  <div style={{ padding: '15px', background: 'rgba(0,0,0,0.2)', borderRadius: '12px', border: `1px solid ${theme.border}` }}>
                    <strong style={{ display: 'block', fontSize: '12px', color: theme.textMuted, textTransform: 'uppercase' }}>Total Size</strong>
                    <span style={{ fontSize: '20px', fontWeight: 'bold', color: theme.accentPurple }}>{formatSize(selectedItem.aggregates.total_size_bytes)}</span>
                  </div>
                  <div style={{ padding: '15px', background: 'rgba(0,0,0,0.2)', borderRadius: '12px', border: `1px solid ${theme.border}` }}>
                    <strong style={{ display: 'block', fontSize: '12px', color: theme.textMuted, textTransform: 'uppercase' }}>Total Files</strong>
                    <span style={{ fontSize: '20px', fontWeight: 'bold', color: theme.accentBlue }}>{formatNumber(selectedItem.aggregates.file_count)}</span>
                  </div>
                </div>
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '10px', marginTop: '15px' }}>
                  <div style={{ padding: '10px', background: 'rgba(0,0,0,0.2)', borderRadius: '8px', fontSize: '12px' }}>
                    <strong style={{ display: 'block', color: theme.textMuted }}>MTime Range</strong>
                    <div>{new Date(selectedItem.aggregates.mtime_first * 1000).toLocaleDateString()} → {new Date(selectedItem.aggregates.mtime_last * 1000).toLocaleDateString()}</div>
                  </div>
                  <div style={{ padding: '10px', background: 'rgba(0,0,0,0.2)', borderRadius: '8px', fontSize: '12px' }}>
                    <strong style={{ display: 'block', color: theme.textMuted }}>ATime Range</strong>
                    <div>{new Date(selectedItem.aggregates.atime_first * 1000).toLocaleDateString()} → {new Date(selectedItem.aggregates.atime_last * 1000).toLocaleDateString()}</div>
                  </div>
                  <div style={{ padding: '10px', background: 'rgba(0,0,0,0.2)', borderRadius: '8px', fontSize: '12px' }}>
                    <strong style={{ display: 'block', color: theme.textMuted }}>CTime Range</strong>
                    <div>{new Date(selectedItem.aggregates.ctime_first * 1000).toLocaleDateString()} → {new Date(selectedItem.aggregates.ctime_last * 1000).toLocaleDateString()}</div>
                  </div>
                </div>
              </div>
            )}

            <div style={{ padding: '15px', background: 'rgba(0,0,0,0.2)', borderRadius: '10px', border: `1px solid ${theme.border}` }}>
              <strong style={{ display: 'block', fontSize: '11px', color: theme.textMuted, textTransform: 'uppercase' }}>Size (Actual)</strong>
              <div style={{ fontSize: '16px', fontWeight: 600 }}>{formatSize(selectedItem.size_bytes)}</div>
            </div>
          </div>
        )}
        
        {!selectedItem && (
          <div style={{ textAlign: 'center', color: theme.textMuted, marginTop: '20vh', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '20px' }}>
            <span style={{ fontSize: '64px', opacity: 0.3 }}>🦑</span>
            <h3 style={{ fontWeight: 400, opacity: 0.6 }}>Select a file or folder to view its details</h3>
          </div>
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

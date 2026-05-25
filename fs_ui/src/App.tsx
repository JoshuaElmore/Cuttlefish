import React, { useState, useEffect } from 'react';

interface Entry {
  path: string;
  name: string;
  size: number;
  type: number;
  mtime: number;
}

interface DirStats {
  total_size_bytes: number;
  file_count: number;
  last_modified: number;
}

interface FileStats {
  size_bytes: number;
  file_type: number;
  permissions: string;
  uid: number;
  gid: number;
  mtime: number;
  metadata: string;
}

const FSExplorer: React.FC = () => {
  const [currentPath, setCurrentPath] = useState('/');
  const [entries, setEntries] = useState<Entry[]>([]);
  const [selectedFile, setSelectedFile] = useState<FileStats | null>(null);
  const [selectedDir, setSelectedDir] = useState<DirStats | null>(null);
  const [searchPath, setSearchPath] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  const formatSize = (bytes: number) => {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const navigateTo = async (path: string) => {
    setCurrentPath(path);
    setSelectedFile(null);
    setSelectedDir(null);
    setIsLoading(true);
    try {
      const res = await fetch(`/api/v1/list?path=${encodeURIComponent(path)}`);
      if (!res.ok) throw new Error(`Server error: ${res.status}`);
      const data = await res.json();
      // CRITICAL FIX: Ensure entries is always an array
      setEntries(Array.isArray(data) ? data : []);
    } catch (e) {
      console.error("Navigation error", e);
      setEntries([]);
    } finally {
      setIsLoading(false);
    }
  };

  const selectPath = async (path: string) => {
    setSelectedFile(null);
    setSelectedDir(null);
    try {
      const dirRes = await fetch(`/api/v1/dir?path=${encodeURIComponent(path)}`);
      if (dirRes.ok) {
        const data = await dirRes.json();
        setSelectedDir(data);
      }

      const fileRes = await fetch(`/api/v1/file?path=${encodeURIComponent(path)}`);
      if (fileRes.ok) {
        const data = await fileRes.json();
        setSelectedFile(data);
      }
    } catch (e) {
      console.error("Detail error", e);
    }
  };

  useEffect(() => {
    navigateTo('/');
  }, []);

  return (
    <div style={{ display: 'flex', height: '100vh', fontFamily: 'sans-serif', backgroundColor: '#f8f9fa' }}>
      <div style={{ width: 350, background: '#fff', borderRight: '1px solid #ddd', display: 'flex', flexDirection: 'column' }}>
        <div style={{ padding: 20, background: '#2c3e50', color: 'white' }}><h2 style={{ margin: 0 }}>📁 FS Explorer</h2></div>
        <div style={{ padding: 10, background: '#eee', display: 'flex', gap: 10, borderBottom: '1px solid #ddd' }}>
          <input 
            style={{ flex: 1, padding: 5 }} 
            value={searchPath} 
            onChange={e => setSearchPath(e.target.value)} 
            placeholder="Jump to path..." 
          />
          <button onClick={() => navigateTo(searchPath)}>Go</button>
        </div>
        <div style={{ overflowY: 'auto', flex: 1 }}>
          {isLoading ? (
            <div style={{ padding: 20, color: '#888' }}>Loading...</div>
          ) : (
            <>
              {currentPath !== '/' && (
                <div 
                  onClick={() => {
                    const parts = currentPath.split('/').filter(Boolean);
                    parts.pop();
                    navigateTo('/' + parts.join('/'));
                  }}
                  style={{ padding: '10px 20px', cursor: 'pointer', background: '#f0f0f0', fontWeight: 'bold', borderBottom: '1px solid #ddd' }}
                >
                  .. (Parent)
                </div>
              )}
              {(entries || []).map((e, i) => (
                <div 
                  key={i} 
                  onClick={() => {
                    if (e.type === 2) navigateTo(e.path);
                    else selectPath(e.path);
                  }}
                  style={{ padding: '10px 20px', borderBottom: '1px solid #eee', cursor: 'pointer', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}
                >
                  <div style={{ display: 'flex', flexDirection: 'column' }}>
                    <span style={{ fontWeight: 500 }}>{e.type === 2 ? '📁' : '📄'} {e.name}</span>
                    <span style={{ fontSize: 11, color: '#999' }}>{new Date(e.mtime * 1000).toLocaleDateString()}</span>
                  </div>
                  <span style={{ fontSize: 12, color: '#666', fontFamily: 'monospace' }}>{formatSize(e.size)}</span>
                </div>
              ))}
              {entries.length === 0 && !isLoading && <div style={{ padding: 20, color: '#999' }}>No entries found.</div>}
            </>
          )}
        </div>
      </div>
      <div style={{ flex: 1, padding: 30, overflowY: 'auto' }}>
        <div style={{ padding: 10, background: '#eee', marginBottom: 20, borderRadius: 8, fontFamily: 'monospace', fontSize: 14 }}>
          {currentPath}
        </div>
        
        {selectedDir && (
          <div style={{ background: 'white', padding: 20, borderRadius: 12, boxShadow: '0 2px 10px rgba(0,0,0,0.05)', marginBottom: 20 }}>
            <h3 style={{ marginTop: 0 }}>📊 Directory Summary</h3>
            <div style={{ display: 'flex', gap: 20 }}>
              <div><strong style={{ display: 'block', fontSize: 12, color: '#888' }}>Total Size</strong><span style={{ fontSize: 18, fontWeight: 'bold' }}>{formatSize(selectedDir.total_size_bytes)}</span></div>
              <div><strong style={{ display: 'block', fontSize: 12, color: '#888' }}>Files</strong><span style={{ fontSize: 18, fontWeight: 'bold' }}>{selectedDir.file_count}</span></div>
              <div><strong style={{ display: 'block', fontSize: 12, color: '#888' }}>Modified</strong><span style={{ fontSize: 18, fontWeight: 'bold' }}>{new Date(selectedDir.last_modified * 1000).toLocaleString()}</span></div>
            </div>
          </div>
        )}
        
        {selectedFile && (
          <div style={{ background: 'white', padding: 20, borderRadius: 12, boxShadow: '0 2px 10px rgba(0,0,0,0.05)' }}>
            <h3 style={{ marginTop: 0 }}>📄 File Metadata</h3>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 15, marginBottom: 20 }}>
              <div><strong style={{ fontSize: 12, color: '#888' }}>Size</strong><div style={{ fontSize: 16 }}>{selectedFile.size_bytes} B</div></div>
              <div><strong style={{ fontSize: 12, color: '#888' }}>Type</strong><div style={{ fontSize: 16 }}>{selectedFile.file_type}</div></div>
              <div><strong style={{ fontSize: 12, color: '#888' }}>Permissions</strong><div style={{ fontSize: 16 }}>{selectedFile.permissions}</div></div>
              <div><strong style={{ fontSize: 12, color: '#888' }}>Modified</strong><div style={{ fontSize: 16 }}>{new Date(selectedFile.mtime * 1000).toLocaleString()}</div></div>
            </div>
            <h3 style={{ fontSize: 16 }}>🛠️ Extended Metadata</h3>
            <pre style={{ background: '#1e272e', color: '#d2dae2', padding: 15, borderRadius: 8, overflowX: 'auto', fontSize: 13, fontFamily: 'monospace' }}>{selectedFile.metadata || '{}'}</pre>
          </div>
        )}
        
        {!selectedDir && !selectedFile && (
          <div style={{ textAlign: 'center', color: '#888', marginTop: 100 }}>
            <h3>Select a file or folder to view details</h3>
          </div>
        )}
      </div>
    </div>
  );
};

export default FSExplorer;

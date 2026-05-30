import React, { useState } from 'react';
import { Routes, Route, useNavigate, useLocation } from 'react-router-dom';
import { theme } from './theme';
import HomePage from './pages/HomePage';
import SplashPage from './pages/SplashPage';
import FileBrowserPage from './pages/FileBrowserPage';
import UserBrowserPage from './pages/UserBrowserPage';

const CuttlefishExplorer: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();

  const formatNumber = (num: number) => num.toLocaleString();
  const formatSize = (bytes: number) => {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  // Determine which tab is active based on URL
  const activeTab = location.pathname === '/users' ? 'userBrowser' : 
                    location.pathname === '/browser' ? 'fileBrowser' : 'none';

  return (
    <div style={{ 
      display: 'flex', 
      height: '100vh', 
      fontFamily: '"Inter", system-ui, -apple-system, sans-serif', 
      background: theme.bgGradient, 
      color: theme.textMain,
      overflow: 'hidden'
    }}>
      {/* Sidebar - Only show if not on Splash page */}
      {activeTab !== 'none' && (
        <div style={{ 
          width: 260, 
          background: 'rgba(0,0,0,0.2)', 
          borderRight: `1px solid ${theme.border}`,
          display: 'flex',
          flexDirection: 'column'
        }}>        <div style={{ padding: '24px', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '12px', marginBottom: '20px' }}>
          <img
            src="/logo.svg"
            alt="Cuttlefish"
            style={{ width: 48, height: 48, borderRadius: 12, boxShadow: '0 0 20px rgba(0,0,0,0.5)' }}
          />
          <h2 style={{ margin: 0, fontSize: '1.1rem', fontWeight: 700, letterSpacing: '1px', textAlign: 'center' }}>Cuttlefish</h2>
          <div style={{ fontSize: '10px', fontWeight: 600, opacity: 0.4, textTransform: 'uppercase', letterSpacing: '2px' }}>Explorer</div>
        </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', padding: '0 12px' }}>
            {[
              { id: 'fileBrowser', path: '/browser', label: '📁 File Browser' },
              { id: 'userBrowser', path: '/users', label: '👤 User Browser' }
            ].map(tab => (
              <div 
                key={tab.id}
                onClick={() => navigate(tab.path)}
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
      )}

      {/* Main View */}
      <div style={{ 
        flex: 1, 
        display: 'flex', 
        overflow: 'hidden' 
      }}>
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/browser" element={<FileBrowserPage />} />
          <Route path="/users" element={<UserBrowserPage formatSize={formatSize} formatNumber={formatNumber} />} />
          {/* Fallback to splash */}
          <Route path="*" element={<SplashPage />} />
        </Routes>
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

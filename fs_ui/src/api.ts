import { Entry, ScanSession, SearchRequest, UserStats } from './types';

const API_BASE = '/api';

export const fsApi = {
  async listEntries(path: string, includeStats = true): Promise<Entry[]> {
    const res = await fetch(`${API_BASE}/list?path=${encodeURIComponent(path)}&include_stats=${includeStats}`);
    if (!res.ok) throw new Error(`Server error: ${res.status}`);
    const data = await res.json();
    return Array.isArray(data) ? data : [];
  },

  async getEntryStats(path: string, fileType: number, includeStats = true): Promise<Entry> {
    const endpoint = fileType === 2 ? '/api/dir/stats' : '/api/file/stats';
    const res = await fetch(`${endpoint}?path=${encodeURIComponent(path)}&include_stats=${includeStats}`);
    if (!res.ok) throw new Error(`API error: ${res.status}`);
    return await res.json();
  },

  async search(request: SearchRequest): Promise<Entry[]> {
    const res = await fetch(`${API_BASE}/search`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(request),
    });
    if (!res.ok) {
      let message = `Server error: ${res.status}`;
      try {
        const body = await res.json();
        if (body?.error) message = body.error;
      } catch {
        /* non-JSON error body */
      }
      throw new Error(message);
    }
    const data = await res.json();
    return Array.isArray(data) ? data : [];
  },

  async listIdentityStats(idType: 'uid' | 'gid'): Promise<UserStats[]> {
    const endpoint = idType === 'uid' ? '/api/user/list' : '/api/group/list';
    const res = await fetch(endpoint);
    if (!res.ok) throw new Error(`API error: ${res.status}`);
    const data = await res.json();
    return Array.isArray(data) ? data : [];
  },

  async listScans(limit = 50): Promise<ScanSession[]> {
    const res = await fetch(`${API_BASE}/scans?limit=${limit}`);
    if (!res.ok) throw new Error(`API error: ${res.status}`);
    const data = await res.json();
    return Array.isArray(data) ? data : [];
  },
};

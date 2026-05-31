import { Entry, SearchRequest } from './types';

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
  }
};

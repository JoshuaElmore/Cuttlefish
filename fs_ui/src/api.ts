import { Entry, ScanSession, SearchRequest, UserStats } from './types';

const API_BASE = '/api';

// Carries the HTTP status so callers can tell "this path isn't indexed" (404)
// from "the server broke" (500) without parsing the message text back apart.
export class ApiError extends Error {
  readonly status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

export const fsApi = {
  // `signal` lets a caller drop a reply it no longer wants — see useFileSystem,
  // where a slow listing must not overwrite the directory navigated to since.
  async listEntries(path: string, includeStats = true, signal?: AbortSignal): Promise<Entry[]> {
    const res = await fetch(`${API_BASE}/list?path=${encodeURIComponent(path)}&include_stats=${includeStats}`, { signal });
    if (!res.ok) throw new ApiError(`Server error: ${res.status}`, res.status);
    const data = await res.json();
    return Array.isArray(data) ? data : [];
  },

  // Directories go to /dir/stats; everything else — files, symlinks, sockets,
  // FIFOs, device nodes — to /file/stats, which accepts any non-directory.
  async getEntryStats(path: string, fileType: number, includeStats = true, signal?: AbortSignal): Promise<Entry> {
    const endpoint = fileType === 2 ? `${API_BASE}/dir/stats` : `${API_BASE}/file/stats`;
    const res = await fetch(`${endpoint}?path=${encodeURIComponent(path)}&include_stats=${includeStats}`, { signal });
    if (!res.ok) throw new ApiError(`API error: ${res.status}`, res.status);
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
      throw new ApiError(message, res.status);
    }
    const data = await res.json();
    return Array.isArray(data) ? data : [];
  },

  async listIdentityStats(idType: 'uid' | 'gid'): Promise<UserStats[]> {
    const endpoint = idType === 'uid' ? `${API_BASE}/user/list` : `${API_BASE}/group/list`;
    const res = await fetch(endpoint);
    if (!res.ok) throw new ApiError(`API error: ${res.status}`, res.status);
    const data = await res.json();
    return Array.isArray(data) ? data : [];
  },

  async listScans(limit = 50): Promise<ScanSession[]> {
    const res = await fetch(`${API_BASE}/scans?limit=${limit}`);
    if (!res.ok) throw new ApiError(`API error: ${res.status}`, res.status);
    const data = await res.json();
    return Array.isArray(data) ? data : [];
  },
};

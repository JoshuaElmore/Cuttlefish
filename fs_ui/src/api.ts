import { AuthSession, Entry, Grant, ScanSession, SearchRequest, UserStats } from './types';

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

// App registers this so a 401 from any call (e.g. an admin revoked the session)
// drops straight back to the login page.
let onUnauthorized = () => {};
export const setOnUnauthorized = (fn: () => void) => { onUnauthorized = fn; };

// Every fsApi call funnels non-2xx replies through here: fires the 401 hook and
// surfaces the server's `{error}` message when it sent one.
const fail = async (res: Response, fallback: string): Promise<never> => {
  if (res.status === 401) onUnauthorized();
  let message = `${fallback}: ${res.status}`;
  try {
    const body = await res.json();
    if (body?.error) message = body.error;
  } catch {
    /* non-JSON error body */
  }
  throw new ApiError(message, res.status);
};

export const fsApi = {
  // `signal` lets a caller drop a reply it no longer wants — see useFileSystem,
  // where a slow listing must not overwrite the directory navigated to since.
  async listEntries(path: string, includeStats = true, signal?: AbortSignal): Promise<Entry[]> {
    const res = await fetch(`${API_BASE}/list?path=${encodeURIComponent(path)}&include_stats=${includeStats}`, { signal });
    if (!res.ok) await fail(res, 'Server error');
    const data = await res.json();
    return Array.isArray(data) ? data : [];
  },

  // Directories go to /dir/stats; everything else — files, symlinks, sockets,
  // FIFOs, device nodes — to /file/stats, which accepts any non-directory.
  async getEntryStats(path: string, fileType: number, includeStats = true, signal?: AbortSignal): Promise<Entry> {
    const endpoint = fileType === 2 ? `${API_BASE}/dir/stats` : `${API_BASE}/file/stats`;
    const res = await fetch(`${endpoint}?path=${encodeURIComponent(path)}&include_stats=${includeStats}`, { signal });
    if (!res.ok) await fail(res, 'API error');
    return await res.json();
  },

  async search(request: SearchRequest): Promise<Entry[]> {
    const res = await fetch(`${API_BASE}/search`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(request),
    });
    if (!res.ok) await fail(res, 'Server error');
    const data = await res.json();
    return Array.isArray(data) ? data : [];
  },

  async listIdentityStats(idType: 'uid' | 'gid'): Promise<UserStats[]> {
    const endpoint = idType === 'uid' ? `${API_BASE}/user/list` : `${API_BASE}/group/list`;
    const res = await fetch(endpoint);
    if (!res.ok) await fail(res, 'API error');
    const data = await res.json();
    return Array.isArray(data) ? data : [];
  },

  async listScans(limit = 50): Promise<ScanSession[]> {
    const res = await fetch(`${API_BASE}/scans?limit=${limit}`);
    if (!res.ok) await fail(res, 'API error');
    const data = await res.json();
    return Array.isArray(data) ? data : [];
  },

  async listGrants(): Promise<Grant[]> {
    const res = await fetch(`${API_BASE}/admin/grants`);
    if (!res.ok) await fail(res, 'API error');
    const data = await res.json();
    return Array.isArray(data) ? data : [];
  },

  async addGrant(group_name: string, path_prefix: string): Promise<Grant> {
    const res = await fetch(`${API_BASE}/admin/grants`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ group_name, path_prefix }),
    });
    if (!res.ok) await fail(res, 'API error');
    return await res.json();
  },

  async deleteGrant(id: number): Promise<void> {
    const res = await fetch(`${API_BASE}/admin/grants?id=${id}`, { method: 'DELETE' });
    if (!res.ok) await fail(res, 'API error');
  },

  async listSessions(): Promise<AuthSession[]> {
    const res = await fetch(`${API_BASE}/admin/sessions`);
    if (!res.ok) await fail(res, 'API error');
    const data = await res.json();
    return Array.isArray(data) ? data : [];
  },

  async revokeSessions(username: string): Promise<void> {
    const res = await fetch(`${API_BASE}/admin/sessions?user=${encodeURIComponent(username)}`, { method: 'DELETE' });
    if (!res.ok) await fail(res, 'API error');
  },
};

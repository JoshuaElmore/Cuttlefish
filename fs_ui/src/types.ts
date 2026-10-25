export interface DirAggregates {
  total_size_bytes: number;
  file_count: number;
  mtime_first: number;
  mtime_last: number;
  atime_first: number;
  atime_last: number;
  ctime_first: number;
  ctime_last: number;
}

export interface UserStats {
  id_type: 'uid' | 'gid';
  id_value: number;
  total_size_bytes: number;
  file_count: number;
  name: string;
}

export interface Entry {
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

export type SortConfig = {
  key: keyof Entry | null;
  direction: 'asc' | 'desc';
};

export type SearchField =
  | 'path' | 'uid' | 'gid' | 'user' | 'group' | 'file_type' | 'size_bytes'
  | 'mtime' | 'atime' | 'ctime'
  | 'dir_total_size' | 'dir_file_count'
  | 'dir_mtime_first' | 'dir_mtime_last'
  | 'dir_atime_first' | 'dir_atime_last'
  | 'dir_ctime_first' | 'dir_ctime_last';
export type SearchConnector = 'AND' | 'OR';

export interface SearchRule {
  field: SearchField;
  operator: string;
  value: string;
  connector: SearchConnector;
  negate?: boolean;   // inverts the condition: "path NOT contains foo"
}

export interface SearchRequest {
  rules: SearchRule[];
  sort_by: string;
  order: 'ASC' | 'DESC';
  limit: number;
  offset: number;
}

export type ScanType = 'indexer' | 'aggregator';
export type ScanStatus = 'running' | 'success' | 'failed';

export interface ScanSession {
  session_id: string;
  scan_type: ScanType;
  status: ScanStatus;
  started_at: number;
  ended_at?: number;
  files_scanned: number;
}

export type Role = 'admin' | 'viewer' | 'restricted';

// GET /auth/me. `prefixes` only constrains a `restricted` user.
export interface Me {
  user: string;
  role: Role;
  prefixes: string[];
  unix_user?: string;   // restricted users only: user mapped through the identity rules
}

export interface IdentityRule {
  id: number;
  match_suffix: string;
  replace_with: string;
  created_by: string;
  created_at: string;   // ISO
}

export type GroupType = 'idp' | 'unix';

export interface MatchedGrant {
  group_type: GroupType;
  group_name: string;
  path_prefix: string;  // as stored, possibly with {user}
  path: string;         // what the user gets; '' when {user} can't be filled safely
}

export interface IdentityPreview {
  identity: string;
  unix_user: string;
  uid: number | null;   // null when no scanned file is owned by that user
  groups: string[];     // Unix groups
  idp_groups: string[];
  idp_groups_source: 'given' | 'session' | 'none';
  role: 'admin' | 'viewer' | 'restricted';
  grants: MatchedGrant[];  // empty unless role is restricted
}

export interface Grant {
  id: number;
  group_type: GroupType;
  group_name: string;
  path_prefix: string;
  created_by: string;
  created_at: string;   // ISO
}

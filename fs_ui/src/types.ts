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

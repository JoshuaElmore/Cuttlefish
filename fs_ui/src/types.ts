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

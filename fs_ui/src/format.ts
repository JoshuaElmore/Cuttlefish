export const formatBytes = (bytes: number): string => {
  if (!bytes || bytes <= 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

export const formatNumber = (num: number): string => num.toLocaleString();

// A nonzero share too small to show at one decimal reads "<0.1%", not "0%".
export const formatPercent = (ratio: number): string => {
  const pct = ratio * 100;
  return pct > 0 && pct < 0.1 ? '<0.1%' : parseFloat(pct.toFixed(1)) + '%';
};

export const formatDate = (epochSeconds: number): string =>
  epochSeconds ? new Date(epochSeconds * 1000).toLocaleString() : '—';

// `file_type` as written by fs_indexer (st_mode & S_IFMT). 0 covers sockets,
// FIFOs and device nodes, which the indexer doesn't distinguish further.
export const typeLabel = (fileType: number): string =>
  fileType === 2 ? 'Directory' : fileType === 1 ? 'File' : fileType === 3 ? 'Symlink' : 'Other';

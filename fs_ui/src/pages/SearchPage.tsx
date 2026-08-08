import React, { useEffect, useRef, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { theme } from '../theme';
import { formatBytes, formatDate, formatNumber } from '../format';
import { fsApi } from '../api';
import { DirAggregates, Entry, SearchField, SearchRule, SearchConnector } from '../types';
import { BlueprintFrame, inputStyle, btnPrimaryStyle, btnSecondaryStyle, btnGhostStyle } from '../components/Blueprint';
import { PlusIcon, TrashIcon } from '../icons';
import DetailPanel, { DetailSelection } from '../components/DetailPanel';

// Field definitions drive which operators and which value input are shown.
type FieldKind = 'text' | 'number' | 'size' | 'timestamp' | 'fileType';

const FIELDS: { value: SearchField; label: string; kind: FieldKind }[] = [
  { value: 'path',           label: 'Path',              kind: 'text'      },
  { value: 'uid',            label: 'UID',               kind: 'number'    },
  { value: 'user',           label: 'Username',          kind: 'text'      },
  { value: 'gid',            label: 'GID',               kind: 'number'    },
  { value: 'group',          label: 'Group Name',        kind: 'text'      },
  { value: 'file_type',      label: 'Type',              kind: 'fileType'  },
  { value: 'size_bytes',     label: 'Size',              kind: 'size'      },
  { value: 'mtime',          label: 'Modified',          kind: 'timestamp' },
  { value: 'atime',          label: 'Accessed',          kind: 'timestamp' },
  { value: 'ctime',          label: 'Changed',           kind: 'timestamp' },
  { value: 'dir_total_size', label: 'Dir: Total Size',   kind: 'size'      },
  { value: 'dir_file_count', label: 'Dir: File Count',   kind: 'number'    },
  { value: 'dir_mtime_first', label: 'Dir: Earliest Modified', kind: 'timestamp' },
  { value: 'dir_mtime_last',  label: 'Dir: Latest Modified',   kind: 'timestamp' },
  { value: 'dir_atime_first', label: 'Dir: Earliest Accessed', kind: 'timestamp' },
  { value: 'dir_atime_last',  label: 'Dir: Latest Accessed',   kind: 'timestamp' },
  { value: 'dir_ctime_first', label: 'Dir: Earliest Changed',  kind: 'timestamp' },
  { value: 'dir_ctime_last',  label: 'Dir: Latest Changed',    kind: 'timestamp' },
];

// Negation is a separate dropdown (the NOT column), not a second set of
// operators — one "contains" entry that can be flipped, rather than
// "contains" and "does not contain" sitting next to each other in one list.
const TEXT_OPERATORS = [
  { value: 'contains', label: 'contains' },
  { value: 'equals', label: 'equals' },
  { value: 'starts_with', label: 'starts with' },
  { value: 'regex', label: 'regex' },
  { value: 'regex_i', label: 'regex (ignore case)' },
];

const NUMBER_OPERATORS = [
  { value: 'equals', label: '=' },
  { value: 'gt', label: '>' },
  { value: 'lt', label: '<' },
];

const TIMESTAMP_OPERATORS = [
  { value: 'gt', label: 'after' },
  { value: 'lt', label: 'before' },
  { value: 'equals', label: 'equals' },
];

const FILE_TYPE_OPERATORS = [
  { value: 'equals', label: 'is' },
];

const FILE_TYPES = [
  { value: '1', label: 'File' },
  { value: '2', label: 'Directory' },
  { value: '3', label: 'Symlink' },
  { value: '0', label: 'Other' },
];

const SORT_OPTIONS = [
  { value: 'path',            label: 'Path'                   },
  { value: 'size_bytes',      label: 'Size'                   },
  { value: 'uid',             label: 'UID'                    },
  { value: 'gid',             label: 'GID'                    },
  { value: 'file_type',       label: 'Type'                   },
  { value: 'mtime',           label: 'Modified'               },
  { value: 'atime',           label: 'Accessed'               },
  { value: 'ctime',           label: 'Changed'                },
  { value: 'dir_total_size',  label: 'Dir: Total Size'         },
  { value: 'dir_file_count',  label: 'Dir: File Count'        },
  { value: 'dir_mtime_first', label: 'Dir: Earliest Modified' },
  { value: 'dir_mtime_last',  label: 'Dir: Latest Modified'   },
  { value: 'dir_atime_first', label: 'Dir: Earliest Accessed' },
  { value: 'dir_atime_last',  label: 'Dir: Latest Accessed'   },
  { value: 'dir_ctime_first', label: 'Dir: Earliest Changed'  },
  { value: 'dir_ctime_last',  label: 'Dir: Latest Changed'    },
];

const typeLabel = (t: number) => (t === 2 ? 'Directory' : t === 1 ? 'File' : t === 3 ? 'Symlink' : 'Other');

// A directory's meaningful size is its recursive total from dir_stats; a file's
// is its own. Both the table cell and the CSV cell go through this.
const effectiveSize = (e: Entry) => (e.file_type === 2 ? e.aggregates?.total_size_bytes ?? e.size_bytes : e.size_bytes);

const isoDate = (epoch: number) => (epoch ? new Date(epoch * 1000).toISOString() : '');

// Result columns. `sortKey` is the API's sort_by value — columns without one
// (permissions) are not sortable server-side and render an inert header.
// `cell` drives the table, `csv` the export, so the two can never drift.
type ResultColumn = {
  key: string;
  label: string;
  csvLabel?: string;          // when the export needs a unit the header lacks
  align?: 'left' | 'right';
  sortKey?: string;
  cell: (e: Entry) => React.ReactNode;
  csv: (e: Entry) => string | number;
};

// The six dir_stats min/max timestamps are all the same shape: a directory-only
// epoch that falls back to "—" on files. Column key doubles as the API sort key.
const dirTimeColumn = (key: string, label: string, pick: (a: DirAggregates) => number): ResultColumn => ({
  key,
  label,
  sortKey: key,
  cell: e => (e.aggregates ? formatDate(pick(e.aggregates)) : '—'),
  csv: e => (e.aggregates ? isoDate(pick(e.aggregates)) : ''),
});

const COLUMNS: ResultColumn[] = [
  { key: 'path', label: 'Path', sortKey: 'path',
    cell: e => e.path, csv: e => e.path },
  { key: 'file_type', label: 'Type', sortKey: 'file_type',
    cell: e => typeLabel(e.file_type), csv: e => typeLabel(e.file_type) },
  { key: 'size_bytes', label: 'Size', csvLabel: 'Size (bytes)', align: 'right', sortKey: 'size_bytes',
    cell: e => formatBytes(effectiveSize(e)), csv: e => effectiveSize(e) },
  { key: 'user', label: 'Owner', sortKey: 'uid', cell: e => e.user, csv: e => e.user },
  { key: 'uid', label: 'UID', align: 'right', sortKey: 'uid', cell: e => e.uid, csv: e => e.uid },
  { key: 'group', label: 'Group', sortKey: 'gid', cell: e => e.group, csv: e => e.group },
  { key: 'gid', label: 'GID', align: 'right', sortKey: 'gid', cell: e => e.gid, csv: e => e.gid },
  { key: 'permissions', label: 'Permissions', cell: e => e.permissions, csv: e => e.permissions },
  { key: 'mtime', label: 'Modified', sortKey: 'mtime', cell: e => formatDate(e.mtime), csv: e => isoDate(e.mtime) },
  { key: 'atime', label: 'Accessed', sortKey: 'atime', cell: e => formatDate(e.atime), csv: e => isoDate(e.atime) },
  { key: 'ctime', label: 'Changed', sortKey: 'ctime', cell: e => formatDate(e.ctime), csv: e => isoDate(e.ctime) },

  // dir_stats aggregates: present only for directories, so files render "—".
  // Ordered to match SORT_OPTIONS — own columns first, then the dir: block.
  { key: 'dir_total_size', label: 'Dir: Total Size', csvLabel: 'Dir Total Size (bytes)', align: 'right', sortKey: 'dir_total_size',
    cell: e => (e.aggregates ? formatBytes(e.aggregates.total_size_bytes) : '—'),
    csv: e => e.aggregates?.total_size_bytes ?? '' },
  { key: 'dir_file_count', label: 'Dir: File Count', align: 'right', sortKey: 'dir_file_count',
    cell: e => (e.aggregates ? formatNumber(e.aggregates.file_count) : '—'),
    csv: e => e.aggregates?.file_count ?? '' },
  dirTimeColumn('dir_mtime_first', 'Dir: Earliest Modified', a => a.mtime_first),
  dirTimeColumn('dir_mtime_last',  'Dir: Latest Modified',   a => a.mtime_last),
  dirTimeColumn('dir_atime_first', 'Dir: Earliest Accessed', a => a.atime_first),
  dirTimeColumn('dir_atime_last',  'Dir: Latest Accessed',   a => a.atime_last),
  dirTimeColumn('dir_ctime_first', 'Dir: Earliest Changed',  a => a.ctime_first),
  dirTimeColumn('dir_ctime_last',  'Dir: Latest Changed',    a => a.ctime_last),
];

const DEFAULT_COLUMNS = ['path', 'file_type', 'size_bytes', 'user'];
const COLUMNS_STORAGE_KEY = 'cuttlefish.search.columns';

// Stored selection is filtered against COLUMNS so a renamed or dropped column
// in a newer build can't resurrect itself out of an old browser's localStorage.
const loadColumns = (): string[] => {
  try {
    const raw = localStorage.getItem(COLUMNS_STORAGE_KEY);
    if (!raw) return DEFAULT_COLUMNS;
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return DEFAULT_COLUMNS;
    const valid = parsed.filter((k: unknown) => COLUMNS.some(c => c.key === k));
    return valid.length ? valid : DEFAULT_COLUMNS;
  } catch {
    return DEFAULT_COLUMNS;
  }
};

const fieldKind = (field: SearchField): FieldKind =>
  FIELDS.find(f => f.value === field)?.kind ?? 'text';

const operatorsFor = (field: SearchField) => {
  const kind = fieldKind(field);
  if (kind === 'text') return TEXT_OPERATORS;
  if (kind === 'timestamp') return TIMESTAMP_OPERATORS;
  if (kind === 'fileType') return FILE_TYPE_OPERATORS;
  return NUMBER_OPERATORS;
};

const defaultOperator = (field: SearchField) => operatorsFor(field)[0].value;

const newRule = (connector: SearchConnector = 'AND'): SearchRule => ({
  field: 'path',
  operator: 'contains',
  value: '',
  connector,
  negate: false,
});

const th: React.CSSProperties = {
  padding: '10px 12px', fontWeight: 600, fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.05em',
  cursor: 'pointer', userSelect: 'none', borderBottom: `1px solid ${theme.border}`, whiteSpace: 'nowrap', color: theme.textMuted,
};
const td: React.CSSProperties = { padding: '9px 12px', borderBottom: `1px solid ${theme.borderSoft}` };

const SearchPage: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  const [rules, setRules] = useState<SearchRule[]>([newRule()]);
  const [sortBy, setSortBy] = useState('path');
  const [order, setOrder] = useState<'ASC' | 'DESC'>('ASC');
  const [limit, setLimit] = useState(200);

  const [results, setResults] = useState<Entry[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasSearched, setHasSearched] = useState(false);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);

  const [columnKeys, setColumnKeys] = useState<string[]>(loadColumns);
  const [columnsOpen, setColumnsOpen] = useState(false);
  const columnsRef = useRef<HTMLDivElement>(null);

  // Keep COLUMNS' order regardless of the order boxes were ticked in, so the
  // table and CSV always read Path → Type → Size → … .
  const activeColumns = COLUMNS.filter(c => columnKeys.includes(c.key));

  const toggleColumn = (key: string) => {
    setColumnKeys(prev => {
      const next = prev.includes(key) ? prev.filter(k => k !== key) : [...prev, key];
      if (next.length === 0) return prev;   // never leave a headerless table
      const ordered = COLUMNS.filter(c => next.includes(c.key)).map(c => c.key);
      try { localStorage.setItem(COLUMNS_STORAGE_KEY, JSON.stringify(ordered)); } catch { /* private mode */ }
      return ordered;
    });
  };

  useEffect(() => {
    if (!columnsOpen) return;
    const onDown = (ev: MouseEvent) => {
      if (!columnsRef.current?.contains(ev.target as Node)) setColumnsOpen(false);
    };
    document.addEventListener('mousedown', onDown);
    return () => document.removeEventListener('mousedown', onDown);
  }, [columnsOpen]);

  const updateRule = (index: number, patch: Partial<SearchRule>) => {
    setRules(prev => prev.map((r, i) => (i === index ? { ...r, ...patch } : r)));
  };

  const changeField = (index: number, field: SearchField) => {
    const operator = defaultOperator(field);
    const value = fieldKind(field) === 'fileType' ? '2' : '';
    updateRule(index, { field, operator, value });
  };

  const addRule = () => setRules(prev => [...prev, newRule('AND')]);
  const removeRule = (index: number) =>
    setRules(prev => (prev.length === 1 ? prev : prev.filter((_, i) => i !== index)));

  const executeSearch = async (rulesToRun: SearchRule[], nextSortBy: string, nextOrder: 'ASC' | 'DESC') => {
    const activeRules = rulesToRun
      .filter(r => fieldKind(r.field) === 'fileType' || r.value.trim() !== '')
      .map(r => {
        if (fieldKind(r.field) === 'timestamp' && r.value) {
          const epoch = Math.floor(new Date(r.value).getTime() / 1000);
          return { ...r, value: String(epoch) };
        }
        return r;
      });
    if (activeRules.length === 0) {
      setError('Enter at least one condition with a value.');
      return;
    }

    setIsLoading(true);
    setError(null);
    setHasSearched(true);
    setSelectedPath(null);
    try {
      const data = await fsApi.search({ rules: activeRules, sort_by: nextSortBy, order: nextOrder, limit, offset: 0 });
      setResults(data);
    } catch (err: any) {
      setError(err.message);
      setResults([]);
    } finally {
      setIsLoading(false);
    }
  };

  const runSearch = () => executeSearch(rules, sortBy, order);

  const sortByColumn = (key: string) => {
    const nextOrder: 'ASC' | 'DESC' = sortBy === key && order === 'ASC' ? 'DESC' : 'ASC';
    setSortBy(key);
    setOrder(nextOrder);
    executeSearch(rules, key, nextOrder);
  };

  // A DetailPanel's "Open in Search" link lands here with ?path= — seed a single
  // exact-match rule and run it immediately.
  useEffect(() => {
    const path = searchParams.get('path');
    if (!path) return;
    const rule: SearchRule = { field: 'path', operator: 'equals', value: path, connector: 'AND' };
    setRules([rule]);
    executeSearch([rule], sortBy, order);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Exports exactly the columns on screen, in the same order.
  const exportCsv = () => {
    const escape = (v: string | number) => {
      const s = String(v);
      return s.includes(',') || s.includes('"') || s.includes('\n') ? `"${s.replace(/"/g, '""')}"` : s;
    };
    const headers = activeColumns.map(c => c.csvLabel ?? c.label);
    const rows = results.map(e => activeColumns.map(c => c.csv(e)).map(escape).join(','));
    const csv = [headers.join(','), ...rows].join('\n');
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `cuttlefish-search-${new Date().toISOString().slice(0, 19).replace(/:/g, '-')}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const selectedEntry = results.find(r => r.path === selectedPath) || null;
  const selected: DetailSelection | null = selectedEntry ? { kind: 'entry', entry: selectedEntry } : null;

  const openInBrowser = (e: Entry) => {
    const targetDir = e.file_type === 2 ? e.path : e.path.substring(0, e.path.lastIndexOf('/')) || '/';
    navigate(`/browser?path=${encodeURIComponent(targetDir)}`);
  };

  return (
    <div style={{ display: 'flex', flex: 1, overflow: 'hidden', background: theme.bg, color: theme.text }}>
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, overflow: 'hidden' }}>
        <div style={{ flex: 'none', padding: '16px 28px', borderBottom: `1px solid ${theme.border}`, fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 19 }}>
          Advanced Search
        </div>

        <div style={{ flex: 1, overflow: 'auto', padding: '24px 28px', display: 'flex', flexDirection: 'column', gap: 20 }}>
          <BlueprintFrame style={{ padding: 16 }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {rules.map((rule, i) => {
                const kind = fieldKind(rule.field);
                return (
                  <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                    <div style={{ width: 70 }}>
                      {i === 0 ? (
                        <span style={{ color: theme.textMuted, fontSize: 13, paddingLeft: 4 }}>Where</span>
                      ) : (
                        <select value={rule.connector} onChange={e => updateRule(i, { connector: e.target.value as SearchConnector })} style={{ ...inputStyle, width: 70, fontWeight: 600 }}>
                          <option value="AND">AND</option>
                          <option value="OR">OR</option>
                        </select>
                      )}
                    </div>

                    <select value={rule.field} onChange={e => changeField(i, e.target.value as SearchField)} style={{ ...inputStyle, width: 150 }}>
                      {FIELDS.map(f => <option key={f.value} value={f.value}>{f.label}</option>)}
                    </select>

                    <select
                      value={rule.negate ? 'NOT' : ''}
                      onChange={e => updateRule(i, { negate: e.target.value === 'NOT' })}
                      title="Negate this condition"
                      style={{ ...inputStyle, width: 70, fontWeight: rule.negate ? 600 : 400, color: rule.negate ? theme.danger : theme.textMuted }}
                    >
                      <option value="">—</option>
                      <option value="NOT">NOT</option>
                    </select>

                    <select value={rule.operator} onChange={e => updateRule(i, { operator: e.target.value })} style={{ ...inputStyle, width: 150 }}>
                      {operatorsFor(rule.field).map(op => <option key={op.value} value={op.value}>{op.label}</option>)}
                    </select>

                    {kind === 'fileType' ? (
                      <select value={rule.value || '2'} onChange={e => updateRule(i, { value: e.target.value })} style={{ ...inputStyle, flex: 1, minWidth: 160 }}>
                        {FILE_TYPES.map(t => <option key={t.value} value={t.value}>{t.label}</option>)}
                      </select>
                    ) : kind === 'timestamp' ? (
                      <input
                        type="datetime-local" value={rule.value}
                        onChange={e => updateRule(i, { value: e.target.value })}
                        onKeyDown={e => { if (e.key === 'Enter') runSearch(); }}
                        style={{ ...inputStyle, flex: 1, minWidth: 200 }}
                      />
                    ) : (
                      <input
                        type={kind === 'number' ? 'number' : 'text'}
                        value={rule.value}
                        placeholder={kind === 'size' ? 'e.g. 1G, 500M, 2T' : kind === 'number' ? 'number' : 'value or regex'}
                        onChange={e => updateRule(i, { value: e.target.value })}
                        onKeyDown={e => { if (e.key === 'Enter') runSearch(); }}
                        style={{ ...inputStyle, flex: 1, minWidth: 160 }}
                      />
                    )}

                    <span
                      onClick={() => removeRule(i)}
                      title="Remove condition"
                      style={{ cursor: rules.length === 1 ? 'not-allowed' : 'pointer', padding: 6, color: rules.length === 1 ? theme.neutral400 : theme.danger }}
                    >
                      <TrashIcon />
                    </span>
                  </div>
                );
              })}

              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 6 }}>
                <div onClick={addRule} style={btnGhostStyle}>
                  <PlusIcon />
                  Add rule
                </div>
                <div style={btnPrimaryStyle} onClick={runSearch}>
                  {isLoading ? 'Searching…' : 'Run search'}
                </div>
              </div>
            </div>
          </BlueprintFrame>

          <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
            <label style={{ fontSize: 13, color: theme.textMuted }}>Sort by</label>
            <select value={sortBy} onChange={e => setSortBy(e.target.value)} style={{ ...inputStyle, width: 150 }}>
              {SORT_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
            </select>
            <select value={order} onChange={e => setOrder(e.target.value as 'ASC' | 'DESC')} style={{ ...inputStyle, width: 130 }}>
              <option value="ASC">Ascending</option>
              <option value="DESC">Descending</option>
            </select>
            <label style={{ fontSize: 13, color: theme.textMuted }}>Limit</label>
            <input type="number" value={limit} min={1} max={1000} onChange={e => setLimit(Math.max(1, Math.min(1000, Number(e.target.value) || 1)))} style={{ ...inputStyle, width: 90 }} />
            <div style={{ marginLeft: 'auto', display: 'flex', gap: 8, alignItems: 'center' }}>
              <div ref={columnsRef} style={{ position: 'relative' }}>
                <div style={btnSecondaryStyle} onClick={() => setColumnsOpen(o => !o)}>
                  Columns ({activeColumns.length})
                </div>
                {columnsOpen && (
                  <div style={{
                    position: 'absolute', top: 'calc(100% + 4px)', right: 0, zIndex: 20,
                    background: theme.bg, border: `1px solid ${theme.border}`,
                    padding: '8px 4px', minWidth: 210, maxHeight: 'min(60vh, 520px)', overflowY: 'auto',
                  }}>
                    {COLUMNS.map(col => {
                      const checked = columnKeys.includes(col.key);
                      const isLastOne = checked && activeColumns.length === 1;
                      return (
                        <label
                          key={col.key}
                          title={isLastOne ? 'At least one column must stay visible' : undefined}
                          style={{
                            display: 'flex', alignItems: 'center', gap: 8, padding: '5px 10px',
                            fontSize: 13, cursor: isLastOne ? 'not-allowed' : 'pointer',
                            opacity: isLastOne ? 0.5 : 1, whiteSpace: 'nowrap',
                          }}
                        >
                          <input type="checkbox" checked={checked} disabled={isLastOne} onChange={() => toggleColumn(col.key)} />
                          {col.label}
                        </label>
                      );
                    })}
                  </div>
                )}
              </div>
              <div
                onClick={results.length ? exportCsv : undefined}
                style={{ ...btnSecondaryStyle, cursor: results.length ? 'pointer' : 'default', opacity: results.length ? 1 : 0.4 }}
              >
                Export CSV
              </div>
            </div>
          </div>

          {error && <div style={{ color: theme.danger, fontSize: 13 }}>Error: {error}</div>}

          <div style={{ fontSize: 12, color: theme.textMuted }}>
            {hasSearched && !isLoading && `Showing ${results.length} match${results.length === 1 ? '' : 'es'}${results.length >= limit ? ` (limited to ${limit})` : ''}`}
          </div>

          <div style={{ border: `1px solid ${theme.border}`, overflowX: 'auto' }}>
            <table style={{ width: '100%', minWidth: 640, borderCollapse: 'collapse', fontSize: 14 }}>
              <thead>
                <tr>
                  {activeColumns.map(col => (
                    <th
                      key={col.key}
                      onClick={col.sortKey ? () => sortByColumn(col.sortKey!) : undefined}
                      style={{ ...th, textAlign: col.align ?? 'left', cursor: col.sortKey ? 'pointer' : 'default' }}
                    >
                      {col.label}{col.sortKey && sortBy === col.sortKey ? (order === 'ASC' ? ' ▲' : ' ▼') : ''}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {results.map(e => {
                  const isSelected = selectedPath === e.path;
                  return (
                    <tr key={e.path} onClick={() => setSelectedPath(e.path)} style={{ cursor: 'pointer', background: isSelected ? theme.accent100 : 'transparent' }}>
                      {activeColumns.map(col => (
                        <td
                          key={col.key}
                          style={{
                            ...td,
                            textAlign: col.align ?? 'left',
                            ...(col.key === 'path'
                              ? { fontFamily: 'ui-monospace,monospace', fontSize: 13, wordBreak: 'break-all' }
                              : { whiteSpace: 'nowrap' }),
                            ...(col.align === 'right' ? { fontVariantNumeric: 'tabular-nums', color: theme.textMuted2 } : {}),
                            ...(col.key === 'file_type' ? { color: theme.textMuted2 } : {}),
                          }}
                        >
                          {col.cell(e)}
                        </td>
                      ))}
                    </tr>
                  );
                })}
                {!isLoading && hasSearched && results.length === 0 && !error && (
                  <tr><td colSpan={activeColumns.length} style={{ textAlign: 'center', padding: 40, color: theme.textMuted }}>No matching files or folders.</td></tr>
                )}
                {!hasSearched && (
                  <tr><td colSpan={activeColumns.length} style={{ textAlign: 'center', padding: 40, color: theme.textMuted }}>Build a query above and press Run search.</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <DetailPanel
        selected={selected}
        onClose={() => setSelectedPath(null)}
        secondaryAction={{ label: 'Open in Browser', onClick: openInBrowser }}
      />
    </div>
  );
};

export default SearchPage;

import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { theme } from '../theme';
import { fsApi } from '../api';
import { Entry, SearchField, SearchRule, SearchConnector } from '../types';

interface SearchPageProps {
  formatSize: (bytes: number) => string;
}

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

const TEXT_OPERATORS = [
  { value: 'contains', label: 'contains' },
  { value: 'equals', label: 'equals' },
  { value: 'starts_with', label: 'starts with' },
  { value: 'regex', label: 'regex' },
  { value: 'regex_i', label: 'regex (ignore case)' },
];

const NUMBER_OPERATORS = [
  { value: 'equals', label: '=' },
  { value: 'not_equals', label: '≠' },
  { value: 'gt', label: '>' },
  { value: 'lt', label: '<' },
];

const TIMESTAMP_OPERATORS = [
  { value: 'gt', label: 'after' },
  { value: 'lt', label: 'before' },
  { value: 'equals', label: 'equals' },
  { value: 'not_equals', label: 'not equals' },
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

const fieldKind = (field: SearchField): FieldKind =>
  FIELDS.find(f => f.value === field)?.kind ?? 'text';

const operatorsFor = (field: SearchField) => {
  const kind = fieldKind(field);
  if (kind === 'text') return TEXT_OPERATORS;
  if (kind === 'timestamp') return TIMESTAMP_OPERATORS;
  return NUMBER_OPERATORS;
};

const isNumericKind = (kind: FieldKind) =>
  kind === 'number' || kind === 'size' || kind === 'timestamp';

const defaultOperator = (field: SearchField) => operatorsFor(field)[0].value;

const newRule = (connector: SearchConnector = 'AND'): SearchRule => ({
  field: 'path',
  operator: 'contains',
  value: '',
  connector,
});

const inputStyle: React.CSSProperties = {
  padding: '8px 10px',
  borderRadius: '6px',
  border: `1px solid ${theme.border}`,
  background: 'rgba(0,0,0,0.3)',
  color: theme.textMain,
  fontSize: '13px',
  outline: 'none',
};

const SearchPage: React.FC<SearchPageProps> = ({ formatSize }) => {
  const navigate = useNavigate();
  const [rules, setRules] = useState<SearchRule[]>([newRule()]);
  const [sortBy, setSortBy] = useState('path');
  const [order, setOrder] = useState<'ASC' | 'DESC'>('ASC');
  const [limit, setLimit] = useState(200);

  const [results, setResults] = useState<Entry[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasSearched, setHasSearched] = useState(false);

  const updateRule = (index: number, patch: Partial<SearchRule>) => {
    setRules(prev => prev.map((r, i) => (i === index ? { ...r, ...patch } : r)));
  };

  const changeField = (index: number, field: SearchField) => {
    // Reset operator/value to sensible defaults when the field type changes.
    const operator = defaultOperator(field);
    const value = fieldKind(field) === 'fileType' ? '2' : '';
    updateRule(index, { field, operator, value });
  };

  const addRule = () => setRules(prev => [...prev, newRule('AND')]);
  const removeRule = (index: number) =>
    setRules(prev => (prev.length === 1 ? prev : prev.filter((_, i) => i !== index)));

  const runSearch = async (
    nextSortBy: string = sortBy,
    nextOrder: 'ASC' | 'DESC' = order,
  ) => {
    const activeRules = rules
      .filter(r => {
        const kind = fieldKind(r.field);
        return kind === 'fileType' || r.value.trim() !== '';
      })
      .map(r => {
        if (fieldKind(r.field) === 'timestamp' && r.value) {
          // datetime-local gives local-time ISO string; convert to unix epoch seconds.
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
    try {
      const data = await fsApi.search({
        rules: activeRules,
        sort_by: nextSortBy,
        order: nextOrder,
        limit,
        offset: 0,
      });
      setResults(data);
    } catch (err: any) {
      setError(err.message);
      setResults([]);
    } finally {
      setIsLoading(false);
    }
  };

  // Clicking a results column header re-sorts via a fresh server query.
  const sortByColumn = (key: string) => {
    const nextOrder: 'ASC' | 'DESC' =
      sortBy === key && order === 'ASC' ? 'DESC' : 'ASC';
    setSortBy(key);
    setOrder(nextOrder);
    runSearch(key, nextOrder);
  };

  const typeLabel = (t: number) =>
    t === 2 ? 'Directory' : t === 1 ? 'File' : t === 3 ? 'Symlink' : 'Other';
  const typeIcon = (t: number) => (t === 2 ? '📁' : t === 1 ? '📄' : t === 3 ? '🔗' : '⚙️');

  const openInBrowser = (e: Entry) => {
    // Directories: open the directory itself. Files/symlinks: open parent dir.
    const targetDir = e.file_type === 2
      ? e.path
      : e.path.substring(0, e.path.lastIndexOf('/')) || '/';
    navigate(`/browser?path=${encodeURIComponent(targetDir)}`);
  };

  const exportCsv = () => {
    const fmtDate = (epoch: number) => new Date(epoch * 1000).toISOString();
    const escape = (v: string | number) => {
      const s = String(v);
      return s.includes(',') || s.includes('"') || s.includes('\n')
        ? `"${s.replace(/"/g, '""')}"`
        : s;
    };

    const headers = [
      'Path', 'Type', 'File Size (bytes)', 'Dir Total Size (bytes)', 'Dir File Count',
      'Owner', 'UID', 'Group', 'GID', 'Permissions',
      'Modified', 'Accessed', 'Changed',
    ];

    const rows = results.map(e => [
      e.path,
      typeLabel(e.file_type),
      e.size_bytes,
      e.aggregates?.total_size_bytes ?? '',
      e.aggregates?.file_count ?? '',
      e.user,
      e.uid,
      e.group,
      e.gid,
      e.permissions,
      fmtDate(e.mtime),
      fmtDate(e.atime),
      fmtDate(e.ctime),
    ].map(escape).join(','));

    const csv = [headers.join(','), ...rows].join('\n');
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `cuttlefish-search-${new Date().toISOString().slice(0, 19).replace(/:/g, '-')}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div style={{
      flex: 1,
      display: 'flex',
      flexDirection: 'column',
      background: theme.panelBg,
      backdropFilter: 'blur(10px)',
      color: theme.textMain,
      overflow: 'hidden',
    }}>
      <div style={{
        padding: '24px',
        borderBottom: `1px solid ${theme.border}`,
        background: 'rgba(0,0,0,0.1)',
      }}>
        <h2 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>🔎 Advanced Search</h2>
        <div style={{ marginTop: '4px', fontSize: '13px', color: theme.textMuted }}>
          Build AND / OR conditions across paths, UIDs, GIDs, type and size — with regex support.
        </div>
      </div>

      <div style={{ flex: 1, overflowY: 'auto', padding: '24px', display: 'flex', flexDirection: 'column', gap: '20px' }}>
        {/* Query builder */}
        <div style={{
          background: 'rgba(0,0,0,0.2)',
          border: `1px solid ${theme.border}`,
          borderRadius: '10px',
          padding: '16px',
          display: 'flex',
          flexDirection: 'column',
          gap: '10px',
        }}>
          {rules.map((rule, i) => {
            const kind = fieldKind(rule.field);
            return (
              <div key={i} style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap' }}>
                {/* Connector (only between rules) */}
                <div style={{ width: '70px' }}>
                  {i === 0 ? (
                    <span style={{ color: theme.textMuted, fontSize: '13px', paddingLeft: '4px' }}>Where</span>
                  ) : (
                    <select
                      value={rule.connector}
                      onChange={e => updateRule(i, { connector: e.target.value as SearchConnector })}
                      style={{ ...inputStyle, width: '70px', fontWeight: 600 }}
                    >
                      <option value="AND">AND</option>
                      <option value="OR">OR</option>
                    </select>
                  )}
                </div>

                {/* Field */}
                <select
                  value={rule.field}
                  onChange={e => changeField(i, e.target.value as SearchField)}
                  style={{ ...inputStyle, width: '130px' }}
                >
                  {FIELDS.map(f => (
                    <option key={f.value} value={f.value}>{f.label}</option>
                  ))}
                </select>

                {/* Operator */}
                <select
                  value={rule.operator}
                  onChange={e => updateRule(i, { operator: e.target.value })}
                  style={{ ...inputStyle, width: '150px' }}
                  disabled={kind === 'fileType'}
                >
                  {kind === 'fileType'
                    ? <option value="equals">is</option>
                    : operatorsFor(rule.field).map(op => (
                        <option key={op.value} value={op.value}>{op.label}</option>
                      ))}
                </select>

                {/* Value */}
                {kind === 'fileType' ? (
                  <select
                    value={rule.value || '2'}
                    onChange={e => updateRule(i, { value: e.target.value })}
                    style={{ ...inputStyle, flex: 1, minWidth: '160px' }}
                  >
                    {FILE_TYPES.map(t => (
                      <option key={t.value} value={t.value}>{t.label}</option>
                    ))}
                  </select>
                ) : kind === 'timestamp' ? (
                  <input
                    type="datetime-local"
                    value={rule.value}
                    onChange={e => updateRule(i, { value: e.target.value })}
                    onKeyDown={e => { if (e.key === 'Enter') runSearch(); }}
                    style={{ ...inputStyle, flex: 1, minWidth: '200px', colorScheme: 'dark' }}
                  />
                ) : (
                  <input
                    type={kind === 'number' ? 'number' : 'text'}
                    value={rule.value}
                    placeholder={
                      kind === 'size'   ? 'e.g. 1G, 500M, 2T' :
                      kind === 'number' ? 'number' :
                      'value or regex'
                    }
                    onChange={e => updateRule(i, { value: e.target.value })}
                    onKeyDown={e => { if (e.key === 'Enter') runSearch(); }}
                    style={{ ...inputStyle, flex: 1, minWidth: '160px' }}
                  />
                )}

                {/* Remove */}
                <button
                  onClick={() => removeRule(i)}
                  disabled={rules.length === 1}
                  title="Remove condition"
                  style={{
                    width: '32px', height: '32px', borderRadius: '6px',
                    border: `1px solid ${theme.border}`,
                    background: 'transparent',
                    color: rules.length === 1 ? theme.textMuted : '#ff6b6b',
                    cursor: rules.length === 1 ? 'not-allowed' : 'pointer',
                    opacity: rules.length === 1 ? 0.4 : 1,
                    fontSize: '16px',
                  }}
                >
                  ×
                </button>
              </div>
            );
          })}

          <div>
            <button
              onClick={addRule}
              style={{
                padding: '6px 12px', borderRadius: '6px',
                border: `1px dashed ${theme.border}`,
                background: 'transparent', color: theme.textMuted,
                cursor: 'pointer', fontSize: '13px',
              }}
            >
              + Add condition
            </button>
          </div>
        </div>

        {/* Controls: sort, order, limit, search */}
        <div style={{ display: 'flex', gap: '12px', alignItems: 'center', flexWrap: 'wrap' }}>
          <label style={{ fontSize: '13px', color: theme.textMuted }}>Sort by</label>
          <select value={sortBy} onChange={e => setSortBy(e.target.value)} style={{ ...inputStyle, width: '140px' }}>
            {SORT_OPTIONS.map(o => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
          <select value={order} onChange={e => setOrder(e.target.value as 'ASC' | 'DESC')} style={{ ...inputStyle, width: '130px' }}>
            <option value="ASC">Ascending</option>
            <option value="DESC">Descending</option>
          </select>

          <label style={{ fontSize: '13px', color: theme.textMuted }}>Limit</label>
          <input
            type="number"
            value={limit}
            min={1}
            max={1000}
            onChange={e => setLimit(Math.max(1, Math.min(1000, Number(e.target.value) || 1)))}
            style={{ ...inputStyle, width: '90px' }}
          />

          <div style={{ marginLeft: 'auto', display: 'flex', gap: '8px' }}>
            <button
              onClick={exportCsv}
              disabled={results.length === 0}
              title="Export current results to CSV"
              style={{
                padding: '9px 18px', borderRadius: '8px',
                border: `1px solid ${theme.border}`,
                background: 'transparent', color: theme.textMuted,
                fontWeight: 600, fontSize: '14px',
                cursor: results.length === 0 ? 'default' : 'pointer',
                opacity: results.length === 0 ? 0.4 : 1,
              }}
            >
              Export CSV
            </button>
            <button
              onClick={() => runSearch()}
              disabled={isLoading}
              style={{
                padding: '9px 22px', borderRadius: '8px', border: 'none',
                background: theme.accentPurple, color: 'white',
                fontWeight: 600, fontSize: '14px',
                cursor: isLoading ? 'default' : 'pointer',
                opacity: isLoading ? 0.6 : 1,
              }}
            >
              {isLoading ? 'Searching…' : 'Search'}
            </button>
          </div>
        </div>

        {error && (
          <div style={{ color: '#ff6b6b', fontSize: '13px' }}>Error: {error}</div>
        )}

        {/* Results */}
        <div style={{
          flex: 1,
          border: `1px solid ${theme.border}`,
          borderRadius: '10px',
          overflowX: 'auto',
        }}>
          <table style={{ width: '100%', minWidth: '1100px', borderCollapse: 'collapse', fontSize: '13px' }}>
            <thead>
              <tr style={{ color: theme.textMuted, background: 'rgba(0,0,0,0.25)' }}>
                {[
                  { key: 'path',       label: 'Path',        align: 'left'  },
                  { key: 'file_type',  label: 'Type',        align: 'left'  },
                  { key: 'size_bytes', label: 'File Size',   align: 'right' },
                  { key: 'dir_total_size', label: 'Dir Total', align: 'right' },
                  { key: 'uid',        label: 'Owner',       align: 'left'  },
                  { key: 'gid',        label: 'Group',       align: 'left'  },
                  { key: 'mtime',      label: 'Modified',    align: 'right' },
                  { key: 'atime',      label: 'Accessed',    align: 'right' },
                  { key: 'ctime',      label: 'Changed',     align: 'right' },
                ].map((col, ci) => (
                  <th
                    key={ci}
                    onClick={col.key ? () => sortByColumn(col.key!) : undefined}
                    style={{
                      padding: '10px 12px', fontWeight: 600, fontSize: '11px',
                      textTransform: 'uppercase', letterSpacing: '0.05em',
                      textAlign: col.align as any,
                      cursor: col.key ? 'pointer' : 'default',
                      userSelect: 'none',
                      borderBottom: `2px solid ${theme.border}`,
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {col.label}{col.key && sortBy === col.key ? (order === 'ASC' ? ' 🔼' : ' 🔽') : ''}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {results.map((e, i) => (
                <tr
                  key={i}
                  onClick={() => openInBrowser(e)}
                  title="Open in File Browser"
                  style={{ borderBottom: `1px solid ${theme.border}`, transition: 'background 0.2s', cursor: 'pointer' }}
                  onMouseEnter={ev => (ev.currentTarget.style.background = theme.hover)}
                  onMouseLeave={ev => (ev.currentTarget.style.background = 'transparent')}
                >
                  <td style={{ padding: '9px 12px', fontFamily: 'monospace', wordBreak: 'break-all', maxWidth: '340px' }}>
                    {typeIcon(e.file_type)} {e.path}
                  </td>
                  <td style={{ padding: '9px 12px', color: theme.textMuted, whiteSpace: 'nowrap' }}>
                    {typeLabel(e.file_type)}
                  </td>
                  <td style={{ padding: '9px 12px', textAlign: 'right', fontFamily: 'monospace', color: theme.textMuted, whiteSpace: 'nowrap' }}>
                    {formatSize(e.size_bytes)}
                  </td>
                  <td style={{ padding: '9px 12px', textAlign: 'right', fontFamily: 'monospace', whiteSpace: 'nowrap' }}>
                    {e.aggregates
                      ? <span>{formatSize(e.aggregates.total_size_bytes)}<span style={{ color: theme.textMuted, fontSize: '11px' }}> ({e.aggregates.file_count.toLocaleString()} files)</span></span>
                      : <span style={{ color: theme.textMuted }}>—</span>}
                  </td>
                  <td style={{ padding: '9px 12px', fontFamily: 'monospace', whiteSpace: 'nowrap' }}>
                    <span style={{ color: theme.textMain }}>{e.user}</span>
                    <span style={{ color: theme.textMuted, fontSize: '11px' }}> ({e.uid})</span>
                  </td>
                  <td style={{ padding: '9px 12px', fontFamily: 'monospace', whiteSpace: 'nowrap' }}>
                    <span style={{ color: theme.textMain }}>{e.group}</span>
                    <span style={{ color: theme.textMuted, fontSize: '11px' }}> ({e.gid})</span>
                  </td>
                  <td style={{ padding: '9px 12px', textAlign: 'right', color: theme.textMuted, whiteSpace: 'nowrap', fontSize: '12px' }}>
                    {new Date(e.mtime * 1000).toLocaleString()}
                  </td>
                  <td style={{ padding: '9px 12px', textAlign: 'right', color: theme.textMuted, whiteSpace: 'nowrap', fontSize: '12px' }}>
                    {new Date(e.atime * 1000).toLocaleString()}
                  </td>
                  <td style={{ padding: '9px 12px', textAlign: 'right', color: theme.textMuted, whiteSpace: 'nowrap', fontSize: '12px' }}>
                    {new Date(e.ctime * 1000).toLocaleString()}
                  </td>
                </tr>
              ))}
              {!isLoading && hasSearched && results.length === 0 && !error && (
                <tr>
                  <td colSpan={9} style={{ textAlign: 'center', padding: '40px', color: theme.textMuted }}>
                    No matching files or folders.
                  </td>
                </tr>
              )}
              {!hasSearched && (
                <tr>
                  <td colSpan={9} style={{ textAlign: 'center', padding: '40px', color: theme.textMuted }}>
                    Build a query above and press Search.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {hasSearched && results.length > 0 && (
          <div style={{ fontSize: '12px', color: theme.textMuted }}>
            {results.length} result{results.length === 1 ? '' : 's'}
            {results.length >= limit ? ` (limited to ${limit})` : ''}
          </div>
        )}
      </div>
    </div>
  );
};

export default SearchPage;

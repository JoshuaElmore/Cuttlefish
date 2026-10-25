import React, { useEffect, useState } from 'react';
import { theme } from '../theme';
import { fsApi } from '../api';
import { Grant } from '../types';
import { BlueprintFrame, inputStyle, btnPrimaryStyle, btnSecondaryStyle, btnReset } from '../components/Blueprint';

const th: React.CSSProperties = {
  textAlign: 'left', fontSize: 11, letterSpacing: '0.08em', textTransform: 'uppercase',
  color: theme.textMuted, padding: 8, borderBottom: `1px solid ${theme.border}`,
};
const td: React.CSSProperties = { padding: '9px 8px', borderBottom: `1px solid ${theme.borderSoft}` };
const heading: React.CSSProperties = { fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 16, marginBottom: 12 };
const smallBtn: React.CSSProperties = { ...btnSecondaryStyle, padding: '4px 12px', fontSize: 13 };

const when = (iso: string) => new Date(iso).toLocaleString();
const message = (e: unknown) => (e instanceof Error ? e.message : String(e));

const AdminPage: React.FC = () => {
  const [grants, setGrants] = useState<Grant[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [group, setGroup] = useState('');
  const [prefix, setPrefix] = useState('');

  const load = () => {
    setError(null);
    fsApi.listGrants()
      .then(setGrants)
      .catch(e => setError(message(e)));
  };
  useEffect(load, []);

  const addGrant = () => {
    if (!group.trim() || !prefix.trim()) { setFormError('Group name and path prefix are both required.'); return; }
    setFormError(null);
    fsApi.addGrant(group.trim(), prefix.trim())
      .then(() => { setGroup(''); setPrefix(''); load(); })
      .catch(e => setFormError(message(e)));
  };

  const deleteGrant = (g: Grant) => {
    if (!window.confirm(`Remove ${g.group_name}'s access to ${g.path_prefix}?`)) return;
    fsApi.deleteGrant(g.id).then(load).catch(e => setError(message(e)));
  };

  return (
    <div style={{ display: 'flex', flex: 1, overflow: 'hidden', background: theme.bg, color: theme.text }}>
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, overflow: 'hidden' }}>
        <div style={{ flex: 'none', padding: '16px 28px', borderBottom: `1px solid ${theme.border}`, fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 19 }}>
          Admin
        </div>

        <div style={{ flex: 1, overflow: 'auto', padding: '24px 28px', display: 'flex', flexDirection: 'column', gap: 32 }}>
          {error && (
            <div style={{ padding: '10px 14px', fontSize: 13, color: theme.danger, border: `1px solid ${theme.danger}` }}>{error}</div>
          )}

          <section>
            <div style={heading}>Access grants</div>
            <BlueprintFrame style={{ padding: 16, marginBottom: 16 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                <input value={group} onChange={e => setGroup(e.target.value)} placeholder="Group name" style={{ ...inputStyle, width: 200 }} />
                <input
                  value={prefix}
                  onChange={e => setPrefix(e.target.value)}
                  onKeyDown={e => { if (e.key === 'Enter') addGrant(); }}
                  placeholder="Path prefix, e.g. /srv/data"
                  style={{ ...inputStyle, width: 280 }}
                />
                <button type="button" onClick={addGrant} style={{ ...btnReset, ...btnPrimaryStyle, padding: '7px 14px' }}>Add</button>
              </div>
              {formError && <div style={{ marginTop: 10, fontSize: 13, color: theme.danger }}>{formError}</div>}
            </BlueprintFrame>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
              <thead>
                <tr>
                  <th style={th}>Group</th>
                  <th style={th}>Prefix</th>
                  <th style={th}>Created by</th>
                  <th style={th}>Created at</th>
                  <th style={th} />
                </tr>
              </thead>
              <tbody>
                {grants.map(g => (
                  <tr key={g.id}>
                    <td style={td}>{g.group_name}</td>
                    <td style={{ ...td, wordBreak: 'break-all' }}>{g.path_prefix}</td>
                    <td style={{ ...td, color: theme.textMuted2 }}>{g.created_by}</td>
                    <td style={{ ...td, color: theme.textMuted2 }}>{when(g.created_at)}</td>
                    <td style={{ ...td, textAlign: 'right' }}><button type="button" onClick={() => deleteGrant(g)} style={{ ...btnReset, ...smallBtn }}>Delete</button></td>
                  </tr>
                ))}
                {grants.length === 0 && (
                  <tr><td colSpan={5} style={{ textAlign: 'center', padding: 40, color: theme.textMuted }}>No grants yet.</td></tr>
                )}
              </tbody>
            </table>
          </section>
        </div>
      </div>
    </div>
  );
};

export default AdminPage;

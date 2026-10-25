import React, { useEffect, useState } from 'react';
import { theme } from '../theme';
import { fsApi } from '../api';
import { Grant, GroupType, IdentityPreview, IdentityRule } from '../types';
import { BlueprintFrame, inputStyle, btnPrimaryStyle, btnSecondaryStyle, btnReset } from '../components/Blueprint';

const th: React.CSSProperties = {
  textAlign: 'left', fontSize: 11, letterSpacing: '0.08em', textTransform: 'uppercase',
  color: theme.textMuted, padding: 8, borderBottom: `1px solid ${theme.border}`,
};
const td: React.CSSProperties = { padding: '9px 8px', borderBottom: `1px solid ${theme.borderSoft}` };
const heading: React.CSSProperties = { fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 16, marginBottom: 12 };
const smallBtn: React.CSSProperties = { ...btnSecondaryStyle, padding: '4px 12px', fontSize: 13 };

const when = (iso: string) => new Date(iso).toLocaleString();
const groupTypeLabel: Record<GroupType, string> = { idp: 'IdP group', unix: 'Unix group' };
const message = (e: unknown) => (e instanceof Error ? e.message : String(e));

const AdminPage: React.FC = () => {
  const [grants, setGrants] = useState<Grant[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [groupType, setGroupType] = useState<GroupType>('idp');
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
    fsApi.addGrant(groupType, group.trim(), prefix.trim())
      .then(() => { setGroup(''); setPrefix(''); load(); })
      .catch(e => setFormError(message(e)));
  };

  const deleteGrant = (g: Grant) => {
    if (!window.confirm(`Remove ${groupTypeLabel[g.group_type]} ${g.group_name}'s access to ${g.path_prefix}?`)) return;
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
                <select
                  value={groupType}
                  onChange={e => setGroupType(e.target.value as GroupType)}
                  aria-label="Group type"
                  style={{ ...inputStyle, width: 130 }}
                >
                  <option value="idp">{groupTypeLabel.idp}</option>
                  <option value="unix">{groupTypeLabel.unix}</option>
                </select>
                <input value={group} onChange={e => setGroup(e.target.value)} placeholder={groupType === 'unix' ? 'Unix group, e.g. research' : 'IdP group name'} style={{ ...inputStyle, width: 200 }} />
                <input
                  value={prefix}
                  onChange={e => setPrefix(e.target.value)}
                  onKeyDown={e => { if (e.key === 'Enter') addGrant(); }}
                  placeholder="Path prefix, e.g. /srv/data"
                  style={{ ...inputStyle, width: 280 }}
                />
                <button type="button" onClick={addGrant} style={{ ...btnReset, ...btnPrimaryStyle, padding: '7px 14px' }}>Add</button>
              </div>
              <div style={{ marginTop: 10, fontSize: 13, color: theme.textMuted }}>
                An <b>IdP group</b> matches the groups your identity provider sends; IdP group <code>*</code> applies to every signed-in user.
                A <b>Unix group</b> matches the groups the indexed host lists for the user's Unix name from the identity mapping below
                (only for users who own indexed files; changes land on the next index run). <code>{'{user}'}</code> in the path becomes that
                Unix name, so <code>*</code> → <code>/home/{'{user}'}</code> gives joshua@corp.com <code>/home/joshua</code>.
              </div>
              {formError && <div style={{ marginTop: 10, fontSize: 13, color: theme.danger }}>{formError}</div>}
            </BlueprintFrame>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
              <thead>
                <tr>
                  <th style={th}>Type</th>
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
                    <td style={{ ...td, color: theme.textMuted2 }}>{groupTypeLabel[g.group_type]}</td>
                    <td style={td}>{g.group_name}</td>
                    <td style={{ ...td, wordBreak: 'break-all' }}>{g.path_prefix}</td>
                    <td style={{ ...td, color: theme.textMuted2 }}>{g.created_by}</td>
                    <td style={{ ...td, color: theme.textMuted2 }}>{when(g.created_at)}</td>
                    <td style={{ ...td, textAlign: 'right' }}><button type="button" onClick={() => deleteGrant(g)} style={{ ...btnReset, ...smallBtn }}>Delete</button></td>
                  </tr>
                ))}
                {grants.length === 0 && (
                  <tr><td colSpan={6} style={{ textAlign: 'center', padding: 40, color: theme.textMuted }}>No grants yet.</td></tr>
                )}
              </tbody>
            </table>
          </section>

          <IdentityMapping />
        </div>
      </div>
    </div>
  );
};

// Maps IdP identities to Unix usernames in the scan: drives {user} in grants and
// which user/group rows a restricted user sees in User & Group Usage.
const IdentityMapping: React.FC = () => {
  const [rules, setRules] = useState<IdentityRule[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [match, setMatch] = useState('');
  const [replace, setReplace] = useState('');
  const [probe, setProbe] = useState('');
  const [probeGroups, setProbeGroups] = useState('');
  const [preview, setPreview] = useState<IdentityPreview | null>(null);

  const load = () => { fsApi.listIdentityRules().then(setRules).catch(e => setError(message(e))); };
  useEffect(load, []);

  const add = () => {
    setError(null);
    fsApi.addIdentityRule(match.trim(), replace.trim())
      .then(() => { setMatch(''); setReplace(''); setPreview(null); load(); })
      .catch(e => setError(message(e)));
  };
  const remove = (r: IdentityRule) => {
    if (!window.confirm(`Remove the rule for "${r.match_suffix || '(any)'}"?`)) return;
    fsApi.deleteIdentityRule(r.id).then(() => { setPreview(null); load(); }).catch(e => setError(message(e)));
  };
  const test = () => {
    if (!probe.trim()) return;
    fsApi.previewIdentity(probe.trim(), probeGroups).then(setPreview).catch(e => setError(message(e)));
  };

  return (
    <section>
      <div style={heading}>Identity mapping</div>
      <div style={{ fontSize: 13, color: theme.textMuted, marginBottom: 12 }}>
        Rewrites an IdP identity's ending into the Unix username in the scan. The longest matching suffix wins; with no
        match the <code>@domain</code> is dropped. Leave "replace with" empty to strip a domain, or leave "suffix" empty to
        append to names nothing else matched.
      </div>
      <BlueprintFrame style={{ padding: 16, marginBottom: 16 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
          <input value={match} onChange={e => setMatch(e.target.value)} placeholder="Suffix, e.g. @corp.com" style={{ ...inputStyle, width: 220 }} />
          <span style={{ color: theme.textMuted }}>→</span>
          <input
            value={replace}
            onChange={e => setReplace(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') add(); }}
            placeholder="Replace with, e.g. @ad.corp.com"
            style={{ ...inputStyle, width: 240 }}
          />
          <button type="button" onClick={add} style={{ ...btnReset, ...btnPrimaryStyle, padding: '7px 14px' }}>Add</button>
        </div>
        {error && <div style={{ marginTop: 10, fontSize: 13, color: theme.danger }}>{error}</div>}
      </BlueprintFrame>
      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14, marginBottom: 16 }}>
        <thead>
          <tr>
            <th style={th}>Suffix</th>
            <th style={th}>Replace with</th>
            <th style={th}>Created by</th>
            <th style={th} />
          </tr>
        </thead>
        <tbody>
          {rules.map(r => (
            <tr key={r.id}>
              <td style={td}>{r.match_suffix || <span style={{ color: theme.textMuted }}>(any)</span>}</td>
              <td style={td}>{r.replace_with || <span style={{ color: theme.textMuted }}>(remove)</span>}</td>
              <td style={{ ...td, color: theme.textMuted2 }}>{r.created_by}</td>
              <td style={{ ...td, textAlign: 'right' }}><button type="button" onClick={() => remove(r)} style={{ ...btnReset, ...smallBtn }}>Delete</button></td>
            </tr>
          ))}
          {rules.length === 0 && (
            <tr><td colSpan={4} style={{ textAlign: 'center', padding: 24, color: theme.textMuted }}>No rules: identities map to the part before @.</td></tr>
          )}
        </tbody>
      </table>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
        <input
          value={probe}
          onChange={e => setProbe(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') test(); }}
          placeholder="Test an identity, e.g. joshua@corp.com"
          style={{ ...inputStyle, width: 300 }}
        />
        <input
          value={probeGroups}
          onChange={e => setProbeGroups(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') test(); }}
          placeholder="IdP groups, comma-separated (optional)"
          style={{ ...inputStyle, width: 280 }}
        />
        <button type="button" onClick={test} style={{ ...btnReset, ...smallBtn }}>Test</button>
      </div>
      {preview && (
        <div style={{ marginTop: 10, fontSize: 13 }}>
          <code>{preview.identity}</code> → <code>{preview.unix_user}</code>{' '}
          {preview.uid === null
            ? <span style={{ color: theme.danger }}>— no such user in the scan</span>
            : <span style={{ color: theme.textMuted2 }}>— uid {preview.uid}; Unix groups: {preview.groups.join(', ') || 'none recorded'}</span>}
          <PreviewAccess preview={preview} />
        </div>
      )}
    </section>
  );
};

const idpGroupsSource: Record<IdentityPreview['idp_groups_source'], string> = {
  given: 'as entered',
  session: 'from their current session',
  none: "none: not signed in and none entered, so only * and Unix-group grants can match",
};

// The access the previewed identity would get: its role, and for a restricted
// user each grant that matches, with {user} filled in.
const PreviewAccess: React.FC<{ preview: IdentityPreview }> = ({ preview }) => (
  <div style={{ marginTop: 10 }}>
    <div style={{ color: theme.textMuted2 }}>
      IdP groups ({idpGroupsSource[preview.idp_groups_source]}): {preview.idp_groups.join(', ') || '—'}
    </div>
    <div style={{ marginTop: 6 }}>
      Role: <b>{preview.role}</b>
      {preview.role === 'admin' && <span style={{ color: theme.textMuted2 }}> — sees everything, plus this Admin page</span>}
      {preview.role === 'viewer' && <span style={{ color: theme.textMuted2 }}> — sees everything</span>}
    </div>
    {preview.role === 'restricted' && (
      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14, marginTop: 8 }}>
        <thead>
          <tr>
            <th style={th}>Can read</th>
            <th style={th}>Via</th>
            <th style={th}>Grant prefix</th>
          </tr>
        </thead>
        <tbody>
          {preview.grants.map(g => (
            <tr key={`${g.group_type}/${g.group_name}/${g.path_prefix}`}>
              <td style={{ ...td, wordBreak: 'break-all' }}>
                {g.path || <span style={{ color: theme.danger }}>skipped: "{preview.unix_user}" can't be used for {'{user}'}</span>}
              </td>
              <td style={td}>{groupTypeLabel[g.group_type]} {g.group_name}</td>
              <td style={{ ...td, color: theme.textMuted2, wordBreak: 'break-all' }}>{g.path_prefix}</td>
            </tr>
          ))}
          {preview.grants.length === 0 && (
            <tr><td colSpan={3} style={{ textAlign: 'center', padding: 24, color: theme.textMuted }}>No grants match: this user would see nothing.</td></tr>
          )}
        </tbody>
      </table>
    )}
  </div>
);

export default AdminPage;

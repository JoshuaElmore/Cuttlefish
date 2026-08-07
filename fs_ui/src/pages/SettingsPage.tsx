import React, { useState } from 'react';
import { theme } from '../theme';
import { BlueprintFrame, SegmentedToggle, inputStyle, btnPrimaryStyle, btnSecondaryStyle } from '../components/Blueprint';
import { RefreshCwIcon } from '../icons';

// Preview-only: fs_config.yml has no write-back API today (config.go loads it
// once at process start), and this form touches secrets (DB password, session
// secret, OIDC client secret). Wiring a real save/run-now endpoint is a
// separate, security-sensitive change — this screen is visual scaffolding for
// that future work, not a live control panel.
const labelStyle: React.CSSProperties = { display: 'block', fontSize: 12, marginBottom: 5, color: theme.textMuted2 };
const fullInput: React.CSSProperties = { ...inputStyle, width: '100%', minHeight: 36, padding: '6px 10px', fontSize: 14 };

const randomHex = (bytes = 16) => {
  const arr = new Uint8Array(bytes);
  (window.crypto || (window as any).msCrypto).getRandomValues(arr);
  return Array.from(arr, b => b.toString(16).padStart(2, '0')).join('');
};

const SettingsPage: React.FC = () => {
  const [authMode, setAuthMode] = useState<'local' | 'oidc'>('local');
  const [sessionSecret, setSessionSecret] = useState('a3f9c1e6b2d84f7091ac5e3b8d2f10c4');
  const [status, setStatus] = useState<string | null>(null);

  const notify = (msg: string) => {
    setStatus(msg);
    window.setTimeout(() => setStatus(null), 3000);
  };

  return (
    <div style={{ flex: 1, overflow: 'auto', padding: '24px 28px', background: theme.bg, color: theme.text }}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 24, maxWidth: 640 }}>
        <div>
          <div style={{ fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 16, marginBottom: 12 }}>Database</div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <div><label style={labelStyle}>Host</label><input defaultValue="localhost" style={fullInput} /></div>
            <div><label style={labelStyle}>Database name</label><input defaultValue="fs_index" style={fullInput} /></div>
            <div><label style={labelStyle}>User</label><input defaultValue="postgres" style={fullInput} /></div>
            <div><label style={labelStyle}>Password</label><input type="password" defaultValue="••••••••••" style={fullInput} /></div>
            <div>
              <label style={labelStyle}>SSL mode</label>
              <select defaultValue="require" style={fullInput}>
                <option>require</option><option>disable</option><option>verify-full</option>
              </select>
            </div>
          </div>
        </div>

        <div>
          <div style={{ fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 16, marginBottom: 12 }}>Authentication</div>
          <div style={{ marginBottom: 14 }}>
            <SegmentedToggle
              options={[{ value: 'local', label: 'Local' }, { value: 'oidc', label: 'OIDC / SSO' }]}
              value={authMode}
              onChange={setAuthMode}
            />
          </div>
          {authMode === 'local' ? (
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <div><label style={labelStyle}>Admin username</label><input defaultValue="admin" style={fullInput} /></div>
              <div><label style={labelStyle}>Admin password</label><input type="password" defaultValue="••••••••••" style={fullInput} /></div>
            </div>
          ) : (
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <div><label style={labelStyle}>Issuer</label><input placeholder="https://accounts.google.com" style={fullInput} /></div>
              <div><label style={labelStyle}>Redirect URL</label><input placeholder="http://host/auth/callback" style={fullInput} /></div>
              <div><label style={labelStyle}>Client ID</label><input style={fullInput} /></div>
              <div><label style={labelStyle}>Client secret</label><input type="password" style={fullInput} /></div>
            </div>
          )}
          <div style={{ marginTop: 12 }}>
            <label style={labelStyle}>Session secret</label>
            <div style={{ display: 'flex', gap: 8 }}>
              <input type="password" value={sessionSecret} readOnly style={{ ...fullInput, flex: 1, fontFamily: 'ui-monospace,monospace' }} />
              <div style={{ ...btnSecondaryStyle, padding: '0 14px' }} onClick={() => { setSessionSecret(randomHex()); notify('Generated locally — not yet saved to fs_config.yml'); }}>
                Regenerate
              </div>
            </div>
          </div>
        </div>

        <div>
          <div style={{ fontFamily: theme.fontHeading, fontWeight: 600, fontSize: 16, marginBottom: 12 }}>Indexing Schedule</div>
          <BlueprintFrame style={{ padding: '14px 16px', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <div>
              <div style={{ fontSize: 14 }}>Nightly at 02:00 — chains to aggregation on success</div>
              <div style={{ fontSize: 12, color: theme.textMuted, marginTop: 3 }}>Configured via cron / systemd timer, not this UI yet</div>
            </div>
            <div style={{ ...btnSecondaryStyle, borderColor: theme.accent, color: theme.accent700 }} onClick={() => notify('Manual scan trigger is not wired up yet')}>
              <RefreshCwIcon />
              Run now
            </div>
          </BlueprintFrame>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 12 }}>
          {status && <div style={{ fontSize: 12, color: theme.textMuted }}>{status}</div>}
          <div style={btnPrimaryStyle} onClick={() => notify('Preview only — Settings does not write fs_config.yml yet')}>
            Save changes
          </div>
        </div>
      </div>
    </div>
  );
};

export default SettingsPage;

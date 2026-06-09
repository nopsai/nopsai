import { Copy, Trash2 } from 'lucide-react';
import type { ServiceAccountToken } from './model';
import { formatAccessTimestamp } from './presentation';

type ServiceAccountTokenRevealProps = {
  token: ServiceAccountToken | null;
  copyLabel: string;
  onCopy: () => void | Promise<void>;
};

export function ServiceAccountTokenReveal({ token, copyLabel, onCopy }: ServiceAccountTokenRevealProps) {
  if (!token?.token) return null;
  return (
    <div className="access-token-reveal">
      <div className="min-w-0">
        <p className="access-card__label">One-time token</p>
        <code>{token.token}</code>
        <p className="text-[11px] text-[var(--text-secondary)] mt-2">Store this value now. It will not be shown again.</p>
      </div>
      <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={() => void onCopy()}>
        <Copy className="h-4 w-4" />
        <span>{copyLabel}</span>
      </button>
    </div>
  );
}

type ServiceAccountTokenPanelProps = {
  tokens: ServiceAccountToken[];
  loading: boolean;
  error: string | null;
  tokenName: string;
  onTokenNameChange: (name: string) => void;
  onCreate: () => void | Promise<void>;
  onRevoke: (tokenID: string) => void | Promise<void>;
};

export function ServiceAccountTokenPanel({
  tokens,
  loading,
  error,
  tokenName,
  onTokenNameChange,
  onCreate,
  onRevoke,
}: ServiceAccountTokenPanelProps) {
  return (
    <section className="access-editor-section access-editor-section--plain">
      <div className="access-minimal-section__header">
        <p className="text-sm font-medium text-[var(--text-primary)]">Tokens</p>
        <span className="text-[11px] text-[var(--text-secondary)]">{tokens.length} active</span>
      </div>
      <div className="access-editor-inline-add">
        <input className="pipelines-input flex-1" value={tokenName} onChange={event => onTokenNameChange(event.target.value)} placeholder="rotation" />
        <button type="button" className="glass-button-subtle" onClick={() => void onCreate()} disabled={loading || !tokenName.trim()}>
          Create token
        </button>
      </div>
      {error && <div className="access-error-banner">{error}</div>}
      <div className="space-y-2">
        {loading ? (
          <p className="text-[12px] text-[var(--text-secondary)]">Loading tokens…</p>
        ) : !tokens.length ? (
          <p className="text-[12px] text-[var(--text-secondary)]">No active tokens.</p>
        ) : (
          tokens.map(token => (
            <div key={token.id} className="access-minimal-row access-minimal-row--stack">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <span className="access-chip access-chip--muted">{token.name}</span>
                  <span className="access-chip access-chip--muted">••••{token.token_suffix}</span>
                </div>
                <p className="text-[11px] text-[var(--text-secondary)] mt-2">
                  Created {formatAccessTimestamp(token.created_at)}
                  {token.expires_at ? ` · Expires ${formatAccessTimestamp(token.expires_at)}` : ' · Never expires'}
                  {token.last_used_at ? ` · Last used ${formatAccessTimestamp(token.last_used_at)}` : ''}
                </p>
              </div>
              <button type="button" className="access-inline-btn access-inline-btn--danger" onClick={() => void onRevoke(token.id)} disabled={loading}>
                <Trash2 className="h-4 w-4" strokeWidth={1.9} aria-hidden="true" />
                <span>Revoke</span>
              </button>
            </div>
          ))
        )}
      </div>
    </section>
  );
}

type ScopeUsageItemMeta = {
  createdAt?: string;
  updatedAt?: string;
};

export type ScopeUsagePipeline = {
  name: string;
  description: string;
  path: string;
  version: string;
  source: string;
};

export type ScopeUsageTrigger = {
  slug: string;
  scope: string;
  pipelines: string[];
  event: string;
  branches: string[];
  tags: string[];
};

export type ScopeUsageSelection = {
  type: 'variable' | 'secret';
  name: string;
  meta?: ScopeUsageItemMeta;
  pipelines: string[];
};

type ScopeUsagePanelProps = {
  selection: ScopeUsageSelection | null;
  pipelineMetadata: Map<string, ScopeUsagePipeline>;
  triggers: ScopeUsageTrigger[];
  loading: boolean;
  error: string | null;
};

function formatTimestamp(value?: string): string {
  const raw = (value || '').trim();
  if (!raw) return '—';
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleString();
}

function PipelineCard({ identifier, metadata }: { identifier: string; metadata?: ScopeUsagePipeline }) {
  const title = metadata?.name || identifier;
  const pathDisplay = metadata?.path ? `/${metadata.path}` : '/';
  const href = `#/pipelines/${identifier.split('/').map(encodeURIComponent).join('/')}`;
  return (
    <a className="scope-related-card" href={href}>
      <div>
        <h5 className="scope-related-card__title">{title}</h5>
        <p className="scope-related-card__path">{pathDisplay}</p>
        {metadata?.description ? <p className="scope-related-card__description">{metadata.description}</p> : null}
      </div>
      <div className="scope-related-card__meta">
        <div className="scope-related-card__meta-row">
          <span className="scope-related-card__meta-label">version:</span>
          <span className="scope-related-card__meta-value">{metadata?.version || 'latest'}</span>
        </div>
        <div className="scope-related-card__meta-row">
          <span className="scope-related-card__meta-label">source:</span>
          <span className="scope-related-card__meta-value">{metadata?.source || 'Config Repository'}</span>
        </div>
      </div>
    </a>
  );
}

function TriggerCard({ trigger }: { trigger: ScopeUsageTrigger }) {
  const href = `#/triggers/${trigger.slug.split('/').map(encodeURIComponent).join('/')}`;
  const pipelineCount = trigger.pipelines.length;
  const pipelineSummary = pipelineCount ? `${pipelineCount} pipeline${pipelineCount === 1 ? '' : 's'}` : 'No pipelines linked';
  return (
    <a className="scope-related-card" href={href}>
      <div>
        <h5 className="scope-related-card__title">{trigger.slug}</h5>
        <p className="scope-related-card__path">{trigger.scope ? `/${trigger.scope}` : '/'}</p>
      </div>
      <div className="scope-related-card__meta">
        <div className="scope-related-card__meta-row">
          <span className="scope-related-card__meta-label">event:</span>
          <span className="scope-related-card__meta-value">{trigger.event}</span>
        </div>
        <div className="scope-related-card__meta-row">
          <span className="scope-related-card__meta-label">pipelines:</span>
          <span className="scope-related-card__meta-value">{pipelineSummary}</span>
        </div>
        <div className="scope-related-card__meta-row">
          <span className="scope-related-card__meta-label">branches:</span>
          <span className="scope-related-card__meta-value">{trigger.branches.length ? trigger.branches.join(', ') : 'All branches'}</span>
        </div>
        <div className="scope-related-card__meta-row">
          <span className="scope-related-card__meta-label">tags:</span>
          <span className="scope-related-card__meta-value">{trigger.tags.length ? trigger.tags.join(', ') : 'No tags'}</span>
        </div>
      </div>
    </a>
  );
}

export function ScopeUsagePanel({ selection, pipelineMetadata, triggers, loading, error }: ScopeUsagePanelProps) {
  const pipelineEmpty = loading
    ? 'Loading impact analysis…'
    : error || `No pipelines declare this ${selection?.type || 'item'}.`;
  const triggersEmpty = loading ? 'Loading impact analysis…' : error || 'No triggers reference this scope.';

  return (
    <article className="glass-card p-5 rounded-2xl border border-[var(--border-primary)]">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-xl font-semibold text-[var(--text-primary)]">
            {selection ? selection.name : 'Select a variable or secret'}
          </p>
        </div>
      </div>
      {selection ? (
        <dl className="grid grid-cols-2 gap-2 text-sm mt-3">
          <div>
            <dt className="text-[var(--text-secondary)]">Updated</dt>
            <dd className="font-medium">{formatTimestamp(selection.meta?.updatedAt)}</dd>
          </div>
          <div>
            <dt className="text-[var(--text-secondary)]">Created</dt>
            <dd className="font-medium">{formatTimestamp(selection.meta?.createdAt)}</dd>
          </div>
        </dl>
      ) : (
        <p className="text-sm text-[var(--text-secondary)] mt-3">Pick a variable or secret to see details and usage.</p>
      )}
      <div className="scope-main-content mt-4">
        <section>
          <h4>Related Pipelines</h4>
          <div className="scope-related-list" data-empty={selection ? pipelineEmpty : 'Select an item'}>
            {!loading && !error && selection
              ? selection.pipelines.map(identifier => (
                  <PipelineCard key={`pipe-${identifier}`} identifier={identifier} metadata={pipelineMetadata.get(identifier)} />
                ))
              : null}
          </div>
        </section>
        <section>
          <h4>Related Triggers</h4>
          <div className="scope-related-list" data-empty={triggersEmpty}>
            {!loading && !error
              ? triggers.map(trigger => (
                  <TriggerCard key={`tr-${trigger.slug}-${trigger.scope}-${trigger.event}`} trigger={trigger} />
                ))
              : null}
          </div>
        </section>
      </div>
    </article>
  );
}

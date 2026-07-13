import { CompactResourceCard } from '../../components/CompactResourceCard';
import { ObjectIcon } from '../../components/ObjectIcon';
import {
  gitWebhookSourceTeamLabel,
  sourceStatusLabel,
  type GitWebhookSource,
} from './model';

export function GitWebhookSourceCards({
  sources,
  selectedID,
  onSelect,
}: {
  sources: GitWebhookSource[];
  selectedID?: string;
  onSelect: (id: string) => void;
}) {
  return (
    <div className="compact-resource-grid" data-testid="git-webhook-source-card-list">
      {sources.map(source => {
        const name = source.name || source.id;
        return (
          <CompactResourceCard
            key={source.id}
            className="compact-resource-card--bordered git-webhook-source-card"
            icon={<ObjectIcon type="git-webhook-source" />}
            tone="blue"
            title={name}
            subtitle={<span className="font-mono">{source.id}</span>}
            description={source.description}
            selected={selectedID === source.id}
            selectionLabel={`Select Git webhook source ${name}`}
            onSelect={() => onSelect(source.id)}
            badges={(
              <>
                <span className={`runner-pill ${source.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>
                  {sourceStatusLabel(source)}
                </span>
                {source.managed_by_config_repo ? (
                  <span className="runner-pill runner-pill--link">
                    <ObjectIcon type="gitops" className="h-3.5 w-3.5" />
                    GitOps
                  </span>
                ) : null}
              </>
            )}
            facts={[
              { label: 'Provider', value: source.provider, mono: true },
              { label: 'Team', value: gitWebhookSourceTeamLabel(source) },
              { label: 'Auth', value: source.auth_mode },
              { label: 'Repositories', value: source.repository_allowlist.length },
            ]}
          />
        );
      })}
    </div>
  );
}

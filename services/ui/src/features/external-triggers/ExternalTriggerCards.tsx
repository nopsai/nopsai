import { GitBranch, Zap } from 'lucide-react';
import { CompactResourceCard } from '../../components/CompactResourceCard';
import {
  externalTriggerGroupLabel,
  externalTriggerScopeLabel,
  type ExternalTrigger,
} from './model';

export function ExternalTriggerCards({
  triggers,
  selectedID,
  onSelect,
}: {
  triggers: ExternalTrigger[];
  selectedID: string;
  onSelect: (id: string) => void;
}) {
  return (
    <div className="compact-resource-grid" data-testid="external-trigger-card-list">
      {triggers.map(trigger => {
        const callerTypes = Array.from(new Set((trigger.allowed_callers || []).map(caller => caller.type))).join(', ') || 'none';
        const managed = Boolean(trigger.managed_by_config_repo);
        const name = trigger.name || trigger.id;
        return (
          <CompactResourceCard
            key={trigger.id}
            className="compact-resource-card--bordered external-trigger-card"
            icon={<Zap />}
            tone="cyan"
            title={name}
            subtitle={<span className="font-mono">{trigger.id}</span>}
            description={trigger.description}
            selected={trigger.id === selectedID}
            selectionLabel={`Select external trigger ${name}`}
            onSelect={() => onSelect(trigger.id)}
            badges={(
              <>
                <span className={`runner-pill ${trigger.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>
                  {trigger.enabled ? 'Enabled' : 'Disabled'}
                </span>
                {managed ? (
                  <span className="runner-pill runner-pill--link">
                    <GitBranch className="h-3.5 w-3.5" />
                    GitOps
                  </span>
                ) : null}
              </>
            )}
            facts={[
              { label: 'Pipeline', value: trigger.pipeline, mono: true, title: trigger.pipeline },
              { label: 'Run group', value: externalTriggerGroupLabel(trigger.run_group_path) },
              {
                label: 'Access',
                value: `${externalTriggerScopeLabel(trigger.scope)} · ${callerTypes}`,
                title: `${externalTriggerScopeLabel(trigger.scope)} · ${callerTypes}`,
              },
            ]}
          />
        );
      })}
    </div>
  );
}

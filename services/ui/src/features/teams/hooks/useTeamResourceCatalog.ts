import { useEffect, useMemo, useState } from 'react';
import { fetchExternalTriggers } from '../../external-triggers/api';
import { fetchGitWebhookSources } from '../../git-webhook-sources/api';
import { fetchKnowledgeContexts } from '../../knowledge-context/api';
import { fetchPipelineList } from '../../pipelines/api';
import { fetchSchedules } from '../../schedules/api';
import { fetchScopeCatalogs } from '../../scopes/api';
import { fetchStepList } from '../../steps/api';
import { fetchCredentials } from '../../system/credentials/api';
import { fetchTriggers } from '../../triggers/api';
import {
  buildCredentialTeamResources,
  buildExternalTriggerTeamResources,
  buildGitWebhookSourceTeamResources,
  buildKnowledgeContextTeamResources,
  buildPipelineTeamResources,
  buildScheduleTeamResources,
  buildScopeTeamResources,
  buildStepTeamResources,
  buildTriggerTeamResources,
  filterTeamLinkedResources,
  normalizeTeamResourcePath,
  type TeamLinkedResource,
  type TeamResourceCatalogState,
} from '../resourceCatalogModel';

function createEmptyCatalog(teamPath = '', loading = false): TeamResourceCatalogState {
  return {
    teamPath,
    loading,
    error: null,
    resources: [],
  };
}

function resultResources<T>(
  result: PromiseSettledResult<T>,
  build: (value: T) => TeamLinkedResource[]
): TeamLinkedResource[] {
  return result.status === 'fulfilled' ? build(result.value) : [];
}

function catalogError(results: Array<{ label: string; result: PromiseSettledResult<unknown> }>): string | null {
  const failed = results.filter(item => item.result.status === 'rejected').map(item => item.label);
  if (!failed.length) return null;
  if (failed.length === results.length) return 'Unable to load linked resources.';
  return `Unable to load ${failed.join(', ')}.`;
}

export function useTeamResourceCatalog({ teamPath }: { teamPath: string }): TeamResourceCatalogState {
  const normalizedTeamPath = useMemo(() => normalizeTeamResourcePath(teamPath), [teamPath]);
  const [catalog, setCatalog] = useState<TeamResourceCatalogState>(() => createEmptyCatalog(normalizedTeamPath, true));

  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      const [
        pipelineResult,
        stepResult,
        triggerResult,
        externalTriggerResult,
        gitWebhookSourceResult,
        scheduleResult,
        knowledgeContextResult,
        scopeResult,
        credentialResult,
      ] = await Promise.allSettled([
        fetchPipelineList(),
        fetchStepList(),
        fetchTriggers(),
        fetchExternalTriggers(),
        fetchGitWebhookSources(),
        fetchSchedules(),
        fetchKnowledgeContexts(),
        fetchScopeCatalogs(),
        fetchCredentials(),
      ] as const);

      if (cancelled) return;
      const resources = filterTeamLinkedResources([
        ...resultResources(pipelineResult, buildPipelineTeamResources),
        ...resultResources(stepResult, buildStepTeamResources),
        ...resultResources(triggerResult, buildTriggerTeamResources),
        ...resultResources(externalTriggerResult, buildExternalTriggerTeamResources),
        ...resultResources(gitWebhookSourceResult, buildGitWebhookSourceTeamResources),
        ...resultResources(scheduleResult, buildScheduleTeamResources),
        ...resultResources(knowledgeContextResult, buildKnowledgeContextTeamResources),
        ...resultResources(scopeResult, buildScopeTeamResources),
        ...resultResources(credentialResult, buildCredentialTeamResources),
      ], normalizedTeamPath);

      setCatalog({
        teamPath: normalizedTeamPath,
        loading: false,
        error: catalogError([
          { label: 'pipelines', result: pipelineResult },
          { label: 'steps', result: stepResult },
          { label: 'triggers', result: triggerResult },
          { label: 'external triggers', result: externalTriggerResult },
          { label: 'Git webhook sources', result: gitWebhookSourceResult },
          { label: 'schedules', result: scheduleResult },
          { label: 'knowledge context', result: knowledgeContextResult },
          { label: 'scopes', result: scopeResult },
          { label: 'credentials', result: credentialResult },
        ]),
        resources,
      });
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, [normalizedTeamPath]);

  if (catalog.teamPath === normalizedTeamPath) return catalog;
  return createEmptyCatalog(normalizedTeamPath, true);
}

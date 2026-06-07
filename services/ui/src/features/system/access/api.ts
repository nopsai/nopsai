import { fetchSystemJson } from '../api';
import {
  buildAccessResourceCatalog,
  type AccessResourceCatalog,
  type AccessResourceCatalogSources,
} from './resourceCatalog';

type CatalogSourceKey = keyof AccessResourceCatalogSources;

const CATALOG_REQUESTS: Array<{ key: CatalogSourceKey; path: string }> = [
  { key: 'groups', path: '/v1/groups' },
  { key: 'pipelines', path: '/v1/pipelines' },
  { key: 'triggers', path: '/v1/overrides' },
  { key: 'externalTriggers', path: '/v1/external-triggers' },
  { key: 'secretScopes', path: '/v1/secrets/scopes' },
  { key: 'variableScopes', path: '/v1/variables/scopes' },
];

export async function fetchAccessResourceCatalog(): Promise<AccessResourceCatalog> {
  const results = await Promise.allSettled(CATALOG_REQUESTS.map(request => fetchSystemJson(request.path)));
  const sources: AccessResourceCatalogSources = {
    groups: [],
    pipelines: [],
    triggers: [],
    externalTriggers: [],
    secretScopes: [],
    variableScopes: [],
  };

  results.forEach((result, index) => {
    const request = CATALOG_REQUESTS[index];
    if (result.status === 'rejected') {
      console.error(`Failed to load Access resource catalog source ${request.path}`, result.reason);
      return;
    }
    sources[request.key] = Array.isArray(result.value) ? result.value : [];
  });

  return buildAccessResourceCatalog(sources);
}

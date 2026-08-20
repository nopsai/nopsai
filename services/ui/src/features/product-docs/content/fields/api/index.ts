import { platform_probesRoutes } from './platform-probes.js';
import { first_install_setupRoutes } from './first-install-setup.js';
import { authenticationRoutes } from './authentication.js';
import { identity_administrationRoutes } from './identity-administration.js';
import { access_controlRoutes } from './access-control.js';
import { pipelines_and_stepsRoutes } from './pipelines-and-steps.js';
import { runsRoutes } from './runs.js';
import { schedulesRoutes } from './schedules.js';
import { external_triggersRoutes } from './external-triggers.js';
import { git_integrationRoutes } from './git-integration.js';
import { teamsRoutes } from './teams.js';
import { gitops_and_config_syncRoutes } from './gitops-and-config-sync.js';
import { variables_and_secretsRoutes } from './variables-and-secrets.js';
import { ai_configurationRoutes } from './ai-configuration.js';
import { knowledge_contextRoutes } from './knowledge-context.js';
import { dashboardsRoutes } from './dashboards.js';
import { monitoringRoutes } from './monitoring.js';
import { system_operationsRoutes } from './system-operations.js';
import { assistant_and_ai_surfacesRoutes } from './assistant-and-ai-surfaces.js';
import { internal_service_routesRoutes } from './internal-service-routes.js';
import type { WikiApiRoute } from '../../types.js';

/**
 * Every REST route registered in services/nopsai/routes.go, one module per
 * functional area.
 *
 * Routes are documented one area at a time: a route
 * with `depth` carries parameters, samples, errors, and covering tests, while
 * the rest still carry the index row of method, path, access, and purpose.
 * Every route passes the same chain — request ID, CORS, body limit, logging,
 * recovery, audit, authentication, then authorization — and `access` describes
 * the gate before resource-level AAA checks run.
 */
export const apiRoutes: WikiApiRoute[] = [
  ...platform_probesRoutes,
  ...first_install_setupRoutes,
  ...authenticationRoutes,
  ...identity_administrationRoutes,
  ...access_controlRoutes,
  ...pipelines_and_stepsRoutes,
  ...runsRoutes,
  ...schedulesRoutes,
  ...external_triggersRoutes,
  ...git_integrationRoutes,
  ...teamsRoutes,
  ...gitops_and_config_syncRoutes,
  ...variables_and_secretsRoutes,
  ...ai_configurationRoutes,
  ...knowledge_contextRoutes,
  ...dashboardsRoutes,
  ...monitoringRoutes,
  ...system_operationsRoutes,
  ...assistant_and_ai_surfacesRoutes,
  ...internal_service_routesRoutes,
];

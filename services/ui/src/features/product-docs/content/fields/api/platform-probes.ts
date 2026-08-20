import type { WikiApiRoute } from '../../types.js';

/**
 * Platform probes.
 *
 * These five routes answer before authentication and before setup, which is what
 * makes them useful when nothing else works. Documented at probe depth: the
 * response and what an operator should assert on is the whole contract.
 */
export const platform_probesRoutes: WikiApiRoute[] = [
  {
    method: 'GET',
    path: '/healthz',
    area: 'Platform probes',
    access: 'public',
    purpose: 'Readiness. Stays unready while setup preflight is retrying an unreachable database.',
    depth: 'probe',
    responses: [
      { status: 200, description: 'The API is ready to serve requests.', contentType: 'application/json', sample: '{"status":"ok"}' },
      {
        status: 503,
        description:
          'Preflight mode only: the platform is up but not ready. The body is the full preflight response, so the blocking check is visible without a second call.',
        contentType: 'application/json',
        sample: '{"ready":false,"can_login":false,"mode":"setup","checks":[{"id":"database","label":"Database","status":"error","message":"database is not reachable yet","required":true}]}',
      },
    ],
    sideEffects: ['None. The probe reads state and never writes.'],
    coveringTests: ['services/nopsai/auth_middleware_test.go'],
    evidence: ['services/nopsai/health_handler.go', 'services/nopsai/setup_preflight.go'],
    notes: 'Use this for readiness gates. A cold start answers 503 until PostgreSQL is reachable, then flips to 200 without a restart.',
  },
  {
    method: 'GET',
    path: '/livez',
    area: 'Platform probes',
    access: 'public',
    purpose: 'Liveness. Answers as soon as the process is up, independent of database state.',
    depth: 'probe',
    responses: [
      { status: 200, description: 'The process is alive.', contentType: 'application/json', sample: '{"status":"alive"}' },
    ],
    sideEffects: ['None.'],
    coveringTests: ['services/nopsai/auth_middleware_test.go'],
    evidence: ['services/nopsai/health_handler.go'],
    notes: 'Never gate a restart on this returning 200 with a broken database — that is what /healthz is for.',
  },
  {
    method: 'GET',
    path: '/version',
    area: 'Platform probes',
    access: 'public',
    purpose: 'Build identity: product and API versions, supported CLI and runner ranges, capabilities, and the release manifest digest.',
    depth: 'probe',
    responses: [
      {
        status: 200,
        description: 'Public build information. Deliberately carries no deployment configuration and no credentials.',
        contentType: 'application/json',
        sample:
          '{\n  "productVersion": "0.22",\n  "commit": "e387a81d",\n  "buildDate": "2026-08-19T10:04:11Z",\n  "apiVersion": "v1",\n  "cliCompatibility": ">=0.20",\n  "runnerCompatibility": ">=0.20",\n  "runnerProtocolVersion": 1,\n  "capabilities": [],\n  "releaseManifestDigest": ""\n}',
      },
    ],
    sideEffects: ['None.'],
    coveringTests: ['services/nopsai/version_handler_test.go'],
    evidence: ['services/nopsai/version_handler.go', 'pkg/buildinfo/buildinfo.go'],
    notes: 'Released CLIs read this to reject incompatible mutating requests before sending them.',
  },
  {
    method: 'GET',
    path: '/metrics',
    area: 'Platform probes',
    access: 'public',
    purpose: 'Prometheus metrics, including identity-provider capability and authorization grant ownership series.',
    depth: 'probe',
    parameters: [],
    responses: [
      {
        status: 200,
        description: 'Prometheus text exposition format.',
        contentType: 'text/plain; version=0.0.4',
        sample: '# HELP nopsai_build_info Build identity of the running platform.\n# TYPE nopsai_build_info gauge\nnopsai_build_info{version="0.22"} 1',
      },
      { status: 401, description: 'Only when METRICS_REQUIRE_AUTH=true and the request carries no valid bearer token.' },
    ],
    sideEffects: ['None.'],
    evidence: ['services/nopsai/routes.go'],
    notes: 'Set METRICS_REQUIRE_AUTH=true to require a bearer token. Leave it public only where the scrape path is already private.',
  },
  {
    method: 'GET',
    path: '/favicon.ico',
    area: 'Platform probes',
    access: 'public',
    purpose: 'Empty cacheable browser probe so missing-favicon requests do not create bearer token errors in audit logs.',
    depth: 'probe',
    responses: [{ status: 200, description: 'Empty body. It exists to keep browser probes out of the authentication path.' }],
    sideEffects: ['None. It deliberately produces no audit record.'],
    coveringTests: ['services/nopsai/auth_middleware_test.go'],
    evidence: ['services/nopsai/routes.go'],
  },
];

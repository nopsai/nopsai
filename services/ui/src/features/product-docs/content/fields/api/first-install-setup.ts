import type { WikiApiRoute } from '../../types.js';

/**
 * First-install setup.
 *
 * Only `/v1/setup/preflight` is public, so a stuck install can be diagnosed
 * without a token. The rest of the surface stays reachable while the workspace
 * is locked but still requires authentication: the bootstrap administrator signs
 * in with the seeded credentials and then runs the wizard. Normal routes stay
 * locked until the bootstrap succeeds once.
 */
export const first_install_setupRoutes: WikiApiRoute[] = [
  {
    method: 'GET',
    path: '/v1/setup/preflight',
    area: 'First-install setup',
    access: 'public',
    purpose: 'Reports what still blocks setup, including a database that is still starting.',
    depth: 'full',
    parameters: [],
    requestSample: {
      title: 'Read preflight',
      language: 'bash',
      code: 'curl -s "$NOPSAI_URL/v1/setup/preflight" | jq',
      expectedOutput: 'A check list with a `ready` flag. Every required check with status `error` is a blocker.',
      placeholders: ['`$NOPSAI_URL` is the API address, `http://localhost:8080` on a local install.'],
    },
    responses: [
      {
        status: 200,
        description: 'Preflight ran. Read `ready` rather than the status code when the platform is already serving.',
        contentType: 'application/json',
        sample:
          '{\n  "ready": true,\n  "can_login": true,\n  "mode": "ready",\n  "config_path": "/app/config.yml",\n  "env_file_path": "/app/.env",\n  "checks": [\n    {\n      "id": "database",\n      "label": "Database",\n      "status": "ok",\n      "message": "database is reachable",\n      "required": true\n    }\n  ]\n}',
      },
      {
        status: 503,
        description: 'Preflight-only mode, and the platform is not ready yet. The body is the same document.',
        contentType: 'application/json',
      },
    ],
    errors: [
      {
        status: 503,
        cause: 'A required check is failing, most often a database that has not finished starting.',
        action: 'Read `checks[].message`. A check may carry `suggested_env`, which names the variables that would resolve it.',
      },
    ],
    sideEffects: ['None. Preflight inspects configuration and connectivity; it changes nothing.'],
    coveringTests: ['services/nopsai/setup_wizard_test.go', 'services/nopsai/enterprise_gates_test.go'],
    evidence: ['services/nopsai/setup_preflight.go'],
    notes: 'The one genuinely public setup route: a stuck install has no working login, so diagnosis cannot require one.',
  },
  {
    method: 'GET',
    path: '/v1/setup/status',
    area: 'First-install setup',
    access: 'authenticated',
    purpose: 'Whether first-install setup has already completed.',
    depth: 'full',
    parameters: [],
    requestSample: {
      title: 'Check whether setup is done',
      language: 'bash',
      code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/setup/status" | jq \'{completed, profile, counts}\'',
      expectedOutput: '`completed: true` once the one-time bootstrap has run.',
    },
    responses: [
      {
        status: 200,
        description: 'Setup state, what was seeded, and the checks the wizard evaluates.',
        contentType: 'application/json',
        sample:
          '{\n  "completed": true,\n  "completed_at": "2026-08-19T10:12:44Z",\n  "profile": "local",\n  "runtime_env": "local",\n  "counts": {},\n  "checks": [],\n  "starter_profiles": []\n}',
      },
    ],
    errors: [
      { status: 500, cause: 'Setup status could not be built, which usually means the database is unreachable.', action: 'Check `/v1/setup/preflight` first — it names the failing dependency.' },
    ],
    sideEffects: ['None.'],
    coveringTests: ['services/nopsai/setup_wizard_test.go'],
    evidence: ['services/nopsai/setup_wizard_handlers.go', 'services/nopsai/setup_wizard.go'],
  },
  {
    method: 'GET',
    path: '/v1/setup/templates',
    area: 'First-install setup',
    access: 'authenticated',
    purpose: 'GitOps seed templates offered by the wizard.',
    depth: 'full',
    parameters: [
      {
        name: 'profile',
        in: 'query',
        type: 'string',
        required: false,
        description: 'Which starter profile to render templates for. An unknown value is normalised rather than rejected.',
        example: 'profile=local',
      },
      {
        name: 'repositories',
        in: 'query',
        type: 'string',
        required: false,
        repeatable: true,
        description: 'Repositories to seed configuration for, shaping the generated team and trigger documents.',
        example: 'repositories=acme/payments',
      },
    ],
    requestSample: {
      title: 'Render the seed templates',
      language: 'bash',
      code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/setup/templates?profile=local" | jq \'.files[].path\'',
      expectedOutput: 'The paths of every file the wizard would seed into a configuration repository.',
    },
    responses: [
      {
        status: 200,
        description: 'The profile that was resolved and the files it renders.',
        contentType: 'application/json',
        sample: '{\n  "profile": "local",\n  "files": [\n    { "path": "setting/system/auth.yaml", "content": "..." }\n  ]\n}',
      },
    ],
    errors: [],
    notes: 'There is no route-specific failure: an unrecognised `profile` is normalised to a known one rather than rejected, so check the `profile` in the response rather than expecting a 400.',
    sideEffects: ['None. Rendering a template writes nothing.'],
    coveringTests: ['services/nopsai/setup_wizard_test.go'],
    evidence: ['services/nopsai/setup_wizard_handlers.go', 'services/nopsai/setup_wizard_templates.go'],
  },
  {
    method: 'GET',
    path: '/v1/setup/templates.zip',
    area: 'First-install setup',
    access: 'authenticated',
    purpose: 'Same templates as a downloadable archive.',
    depth: 'full',
    parameters: [
      { name: 'profile', in: 'query', type: 'string', required: false, description: 'Starter profile to render, as for the JSON route.', example: 'profile=local' },
      { name: 'repositories', in: 'query', type: 'string', required: false, repeatable: true, description: 'Repositories to seed configuration for.', example: 'repositories=acme/payments' },
    ],
    requestSample: {
      title: 'Download the seed archive',
      language: 'bash',
      code: 'curl -s -o nopsai-templates.zip -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/setup/templates.zip?profile=local"\nunzip -l nopsai-templates.zip',
      expectedOutput: 'A zip containing the same files the JSON route lists, ready to commit into a configuration repository.',
    },
    responses: [{ status: 200, description: 'Zip archive of the rendered templates.', contentType: 'application/zip' }],
    errors: [],
    notes: 'Same normalisation as the JSON route: no route-specific failure, and an unknown `profile` yields the default rather than an error.',
    sideEffects: ['None.'],
    coveringTests: ['services/nopsai/setup_wizard_test.go'],
    evidence: ['services/nopsai/setup_wizard_handlers.go'],
  },
  {
    method: 'POST',
    path: '/v1/setup/bootstrap',
    area: 'First-install setup',
    access: 'authenticated',
    purpose: 'Runs the one-time bootstrap. Normal authenticated routes stay locked until this succeeds once.',
    depth: 'full',
    parameters: [],
    requestSample: {
      title: 'Run the bootstrap',
      language: 'bash',
      code:
        'curl -sX POST "$NOPSAI_URL/v1/setup/bootstrap" \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  -H "Content-Type: application/json" \\\n  --data @bootstrap.json | jq',
      expectedOutput: 'The resulting setup status. Re-reading `/v1/setup/status` afterwards reports `completed: true`.',
      placeholders: [
        '`bootstrap.json` carries the wizard answers: the chosen profile, the administrator, the starter profiles to seed, and the GitOps repositories.',
      ],
    },
    responses: [
      {
        status: 200,
        description: 'Bootstrap applied. The response is the same document `/v1/setup/status` returns.',
        contentType: 'application/json',
        sample:
          '{\n  "completed": true,\n  "completed_at": "2026-08-19T10:12:44Z",\n  "profile": "local",\n  "runtime_env": "local",\n  "env_file_path": "/app/.env",\n  "counts": {},\n  "checks": [],\n  "starter_profiles": []\n}',
      },
    ],
    errors: [
      { status: 400, cause: 'The payload is malformed, or an answer is not valid for the chosen profile.', action: 'The message names the field. Apply errors deliberately include the actionable write or configuration reason.' },
      { status: 500, cause: 'Local secret generation or a configuration write failed.', action: 'Read the message: it names the path or setting that could not be written, which is usually a permissions problem in the mounted config volume.' },
    ],
    sideEffects: [
      'Creates the first administrator and marks setup complete.',
      'May generate local secrets and write them to the env file the response names.',
      'May seed the GitOps layout and repository teams for the chosen profile.',
    ],
    coveringTests: ['services/nopsai/setup_wizard_test.go'],
    evidence: ['services/nopsai/setup_wizard_handlers.go', 'services/nopsai/bootstrap_schema.go'],
    notes: 'It works exactly once. It needs a token — the bootstrap administrator’s — but no AAA resource check, because there is nothing configured yet to check against.',
  },
];

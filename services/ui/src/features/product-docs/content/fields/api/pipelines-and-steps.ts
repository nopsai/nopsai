import type { WikiApiRoute } from '../../types.js';

const VALIDATION_OK = '{\n  "valid": true,\n  "errors": [],\n  "warnings": []\n}';

const VALIDATION_FAILED =
  '{\n  "valid": false,\n  "errors": [\n    {\n      "message": "step \\"package\\" consumes an output of \\"build\\" without a dependency path",\n      "path": "steps[3].variables.BUILD_TAG",\n      "line": 34,\n      "code": "missing_dependency_path"\n    }\n  ],\n  "warnings": []\n}';

/**
 * Pipelines and steps.
 *
 * Both write routes take the YAML document as the request body rather than a
 * JSON wrapper, and both validation routes always answer 200 — a rejected
 * document is a `valid: false` body, not an HTTP error.
 */
export const pipelines_and_stepsRoutes: WikiApiRoute[] = [
  {
    method: 'GET',
    path: '/v1/pipelines',
    area: 'Pipelines and steps',
    access: 'authorized',
    purpose: 'Lists pipelines the caller can see.',
    depth: 'full',
    parameters: [
      {
        name: 'include_source',
        in: 'query',
        type: 'boolean',
        required: false,
        defaultValue: 'false',
        description: 'Include where each pipeline came from, which distinguishes a GitOps-managed definition from a database one.',
        example: 'include_source=true',
      },
    ],
    requestSample: {
      title: 'List pipelines',
      language: 'bash',
      code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/pipelines?include_source=true" | jq',
      expectedOutput: 'One entry per pipeline the caller may see, already filtered by AAA.',
    },
    responses: [
      {
        status: 200,
        description: 'Pipelines ordered by path then name. The list is filtered per caller, so two callers can legitimately see different rows.',
        contentType: 'application/json',
        sample: '[\n  {\n    "id": "platform/release-service",\n    "source": "config_repo",\n    "version": "latest",\n    "updated_at": "2026-08-19T09:58:12Z"\n  }\n]',
      },
    ],
    errors: [
      { status: 500, cause: 'The pipeline query failed.', action: 'Platform fault; check database reachability.' },
      { status: 503, cause: 'Authorization is unavailable, so the list cannot be filtered safely.', action: 'Check AAA. The platform refuses to answer rather than return an unfiltered list.' },
    ],
    sideEffects: ['None.'],
    coveringTests: ['services/nopsai/pipeline_handlers_test.go'],
    evidence: ['services/nopsai/pipeline_handlers.go'],
    notes: 'An empty list can mean "no pipelines" or "none you may see". Compare with `/v1/auth/me` before assuming the former.',
  },
  {
    method: 'GET',
    path: '/v1/pipelines/{pipelineName...}',
    area: 'Pipelines and steps',
    access: 'authorized',
    purpose: 'Reads one pipeline definition.',
    depth: 'full',
    parameters: [
      {
        name: 'pipelineName',
        in: 'path',
        type: 'string',
        required: true,
        description: 'Pipeline identifier. It is a catch-all segment, so a team-prefixed name with slashes is passed as-is.',
        example: 'platform/release-service',
      },
    ],
    requestSample: {
      title: 'Read a pipeline definition',
      language: 'bash',
      code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/pipelines/platform/release-service" | jq -r .definition',
      expectedOutput: 'The stored YAML definition, plus the metadata describing where it came from.',
    },
    responses: [
      {
        status: 200,
        description: 'The definition and its provenance.',
        contentType: 'application/json',
        sample: '{\n  "id": "platform/release-service",\n  "source": "config_repo",\n  "version": "latest",\n  "definition": "name: release-service\\ncontainer_image: alpine:3.20\\n..."\n}',
      },
    ],
    errors: [
      { status: 400, cause: 'The name is empty or malformed.', action: 'Use the identifier from the list route.' },
      { status: 403, cause: 'The caller may not read this pipeline.', action: 'A 403 rather than a 404 means the pipeline exists and access is the problem.' },
      { status: 404, cause: 'No pipeline with that identifier.', action: 'Check the team prefix: `release-service` and `platform/release-service` are different pipelines.' },
      { status: 502, cause: 'A GitOps-backed definition could not be fetched from its repository.', action: 'Check the configuration repository connection and sync status.' },
      { status: 503, cause: 'Authorization is unavailable.', action: 'Check AAA.' },
      { status: 500, cause: 'The definition could not be read.', action: 'Platform fault.' },
    ],
    sideEffects: ['None.'],
    coveringTests: ['services/nopsai/pipeline_handlers_test.go'],
    evidence: ['services/nopsai/pipeline_handlers.go'],
  },
  {
    method: 'PUT',
    path: '/v1/pipelines/{pipelineName...}',
    area: 'Pipelines and steps',
    access: 'authorized',
    purpose: 'Creates or updates a pipeline.',
    depth: 'full',
    parameters: [
      { name: 'pipelineName', in: 'path', type: 'string', required: true, description: 'Identifier to store the definition under.', example: 'platform/release-service' },
    ],
    requestSample: {
      title: 'Save a pipeline',
      language: 'bash',
      code:
        'curl -sX PUT "$NOPSAI_URL/v1/pipelines/platform/release-service" \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  -H "Content-Type: application/yaml" \\\n  --data-binary @release-service.yaml',
      expectedOutput: '201 with the stored record. The body is the YAML document itself — there is no JSON wrapper.',
      placeholders: ['`release-service.yaml` is the pipeline manifest.'],
    },
    responses: [
      { status: 201, description: 'Stored. The same status is returned for a create and an update.', contentType: 'application/json', sample: '{\n  "id": "platform/release-service",\n  "source": "database",\n  "version": "latest",\n  "updated_at": "2026-08-19T10:22:31Z"\n}' },
    ],
    errors: [
      { status: 400, cause: 'The YAML is malformed, or the pipeline fails validation.', action: 'Run `POST /v1/pipelines/validate` first: it returns every issue with a path and a line instead of the first failure.' },
      { status: 500, cause: 'The definition could not be persisted.', action: 'Retry; nothing was stored.' },
    ],
    sideEffects: [
      'Editing a GitOps-managed pipeline creates a database override, which then shows as drift against Git until it is pushed or discarded.',
      'Writes an audit record.',
    ],
    coveringTests: ['services/nopsai/pipeline_handlers_test.go'],
    evidence: ['services/nopsai/pipeline_handlers.go'],
    notes: 'The request body is the YAML document, not `{"definition": "..."}`. Sending JSON gets a parse failure that looks like a schema error.',
  },
  {
    method: 'DELETE',
    path: '/v1/pipelines/{pipelineName...}',
    area: 'Pipelines and steps',
    access: 'authorized',
    purpose: 'Deletes a pipeline.',
    depth: 'full',
    parameters: [
      { name: 'pipelineName', in: 'path', type: 'string', required: true, description: 'Identifier of the pipeline to delete.', example: 'platform/release-service' },
    ],
    requestSample: {
      title: 'Delete a pipeline',
      language: 'bash',
      code: 'curl -sX DELETE -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/pipelines/platform/release-service" -w "%{http_code}\\n"',
      expectedOutput: '204. Existing run records are not deleted with it.',
    },
    responses: [{ status: 204, description: 'Deleted.', sample: '' }],
    errors: [
      { status: 400, cause: 'The name is empty or malformed.', action: 'Use the identifier from the list route.' },
      { status: 500, cause: 'The delete could not be persisted.', action: 'Retry.' },
    ],
    sideEffects: [
      'Removes the definition. Runs it already produced stay, so history survives the pipeline.',
      'A GitOps-managed pipeline reappears on the next sync unless it is removed from the repository too.',
      'Writes an audit record.',
    ],
    coveringTests: ['services/nopsai/pipeline_handlers_test.go'],
    evidence: ['services/nopsai/pipeline_handlers.go'],
  },
  {
    method: 'POST',
    path: '/v1/pipelines/validate',
    area: 'Pipelines and steps',
    access: 'authenticated',
    purpose: 'Validates pipeline YAML without saving it.',
    depth: 'full',
    parameters: [],
    requestSample: {
      title: 'Validate a definition',
      language: 'bash',
      code:
        'curl -sX POST "$NOPSAI_URL/v1/pipelines/validate" \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  -H "Content-Type: application/yaml" \\\n  --data-binary @release-service.yaml | jq',
      expectedOutput: '`valid: true` with empty error and warning lists, or the issues with their paths and line numbers.',
      placeholders: ['A JSON body works too, with the document under `yaml` or `content`.'],
    },
    responses: [
      { status: 200, description: 'The document is valid.', contentType: 'application/json', sample: VALIDATION_OK },
      {
        status: 200,
        description: 'The document is not valid. Validation failures are a body, not an HTTP error — the request itself succeeded.',
        contentType: 'application/json',
        sample: VALIDATION_FAILED,
      },
    ],
    errors: [
      { status: 400, cause: 'The request payload could not be read at all — invalid JSON when a JSON content type was declared.', action: 'Send YAML with a YAML content type, or JSON with the document under `yaml` or `content`.' },
    ],
    sideEffects: ['None. Validation never stores anything.'],
    coveringTests: ['services/nopsai/pipeline_handlers_test.go'],
    evidence: ['services/nopsai/validation_handlers.go', 'services/nopsai/validation_contract.go'],
    notes: 'Always check `valid` rather than the status code. A rejected pipeline returns 200 with `valid: false`.',
  },
  {
    method: 'GET',
    path: '/v1/steps',
    area: 'Pipelines and steps',
    access: 'authorized',
    purpose: 'Lists reusable step definitions.',
    depth: 'full',
    parameters: [
      { name: 'include_source', in: 'query', type: 'boolean', required: false, defaultValue: 'false', description: 'Include where each step definition came from.', example: 'include_source=true' },
    ],
    requestSample: {
      title: 'List reusable steps',
      language: 'bash',
      code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/steps" | jq',
      expectedOutput: 'The step definitions a pipeline can pull in with `include: step:<identifier>`.',
    },
    responses: [
      { status: 200, description: 'Reusable steps the caller may see.', contentType: 'application/json', sample: '[\n  {\n    "id": "platform/shared/checkout",\n    "source": "config_repo",\n    "updated_at": "2026-08-18T16:40:02Z"\n  }\n]' },
    ],
    errors: [
      { status: 500, cause: 'The step query failed.', action: 'Platform fault.' },
      { status: 503, cause: 'Authorization is unavailable.', action: 'Check AAA; the list is not returned unfiltered.' },
    ],
    sideEffects: ['None.'],
    coveringTests: ['services/nopsai/pipeline_handlers_test.go'],
    evidence: ['services/nopsai/pipeline_handlers.go'],
  },
  {
    method: 'GET',
    path: '/v1/steps/{stepPath...}',
    area: 'Pipelines and steps',
    access: 'authorized',
    purpose: 'Reads one reusable step.',
    depth: 'full',
    parameters: [
      { name: 'stepPath', in: 'path', type: 'string', required: true, description: 'Step identifier, including its team prefix.', example: 'platform/shared/checkout' },
    ],
    requestSample: {
      title: 'Read a reusable step',
      language: 'bash',
      code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/steps/platform/shared/checkout" | jq -r .definition',
      expectedOutput: 'The stored step definition, which is what `include: step:platform/shared/checkout` expands to.',
    },
    responses: [
      { status: 200, description: 'The step definition and its provenance.', contentType: 'application/json', sample: '{\n  "id": "platform/shared/checkout",\n  "source": "config_repo",\n  "definition": "name: checkout\\nscript: |\\n  git clone ...\\n"\n}' },
    ],
    errors: [
      { status: 404, cause: 'No reusable step with that identifier.', action: 'List the steps: an `include:` that names a missing step fails pipeline validation with the same identifier.' },
      { status: 403, cause: 'The caller may not read this step.', action: 'Check ownership of the team that holds it.' },
    ],
    sideEffects: ['None.'],
    coveringTests: ['services/nopsai/pipeline_handlers_test.go'],
    evidence: ['services/nopsai/pipeline_handlers.go'],
  },
  {
    method: 'PUT',
    path: '/v1/steps/{stepName...}',
    area: 'Pipelines and steps',
    access: 'authorized',
    purpose: 'Creates or updates a reusable step.',
    depth: 'full',
    parameters: [
      { name: 'stepName', in: 'path', type: 'string', required: true, description: 'Identifier to store the step under.', example: 'platform/shared/checkout' },
    ],
    requestSample: {
      title: 'Save a reusable step',
      language: 'bash',
      code:
        'curl -sX PUT "$NOPSAI_URL/v1/steps/platform/shared/checkout" \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  -H "Content-Type: application/yaml" \\\n  --data-binary @checkout.yaml',
      expectedOutput: '201 with the stored record.',
      placeholders: ['`checkout.yaml` is a step definition, not a whole pipeline.'],
    },
    responses: [
      { status: 201, description: 'Stored, for both create and update.', contentType: 'application/json', sample: '{\n  "id": "platform/shared/checkout",\n  "source": "database",\n  "updated_at": "2026-08-19T10:31:44Z"\n}' },
    ],
    errors: [
      { status: 400, cause: 'The YAML is malformed or the step fails validation.', action: 'Validate first: a reusable step has its own rules and rejects pipeline-only directives.' },
      { status: 500, cause: 'The definition could not be persisted.', action: 'Retry.' },
    ],
    sideEffects: ['Every pipeline that includes this step picks up the change on its next run.', 'Writes an audit record.'],
    coveringTests: ['services/nopsai/pipeline_handlers_test.go'],
    evidence: ['services/nopsai/pipeline_handlers.go'],
    notes: 'A shared step is shared blast radius: it takes effect for every pipeline that includes it, without those pipelines changing.',
  },
  {
    method: 'DELETE',
    path: '/v1/steps/{stepName...}',
    area: 'Pipelines and steps',
    access: 'authorized',
    purpose: 'Deletes a reusable step.',
    depth: 'full',
    parameters: [
      { name: 'stepName', in: 'path', type: 'string', required: true, description: 'Identifier of the step to delete.', example: 'platform/shared/checkout' },
    ],
    requestSample: {
      title: 'Delete a reusable step',
      language: 'bash',
      code: 'curl -sX DELETE -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/steps/platform/shared/checkout" -w "%{http_code}\\n"',
      expectedOutput: '204.',
    },
    responses: [{ status: 204, description: 'Deleted.', sample: '' }],
    errors: [
      { status: 400, cause: 'The name is empty or malformed.', action: 'Use the identifier from the list route.' },
      { status: 500, cause: 'The delete could not be persisted.', action: 'Retry.' },
    ],
    sideEffects: ['Pipelines that still include the step fail validation on their next save or run.', 'Writes an audit record.'],
    coveringTests: ['services/nopsai/pipeline_handlers_test.go'],
    evidence: ['services/nopsai/pipeline_handlers.go'],
    notes: 'Nothing blocks deleting a step other pipelines include. Check usage before removing a shared definition.',
  },
  {
    method: 'POST',
    path: '/v1/steps/validate',
    area: 'Pipelines and steps',
    access: 'authenticated',
    purpose: 'Validates a reusable step definition.',
    depth: 'full',
    parameters: [],
    requestSample: {
      title: 'Validate a step definition',
      language: 'bash',
      code:
        'curl -sX POST "$NOPSAI_URL/v1/steps/validate" \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  -H "Content-Type: application/yaml" \\\n  --data-binary @checkout.yaml | jq',
      expectedOutput: 'The same `valid`, `errors`, `warnings` shape the pipeline validator returns.',
    },
    responses: [
      { status: 200, description: 'Validation ran. Read `valid` rather than the status code.', contentType: 'application/json', sample: VALIDATION_OK },
    ],
    errors: [
      { status: 400, cause: 'The payload could not be read.', action: 'Send YAML with a YAML content type, or JSON with the document under `yaml` or `content`.' },
    ],
    sideEffects: ['None.'],
    coveringTests: ['services/nopsai/pipeline_handlers_test.go'],
    evidence: ['services/nopsai/validation_handlers.go'],
    notes: 'A reusable step is validated against step rules, so a document that passes here can still be rejected as a pipeline, and the reverse.',
  },
];

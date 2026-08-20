import type { WikiApiRoute } from '../../types.js';

const TRIGGER_RECORD =
  '{\n  "id": "3d2b1f88-77aa-4c19-9f3d-52b0c4e7a901",\n  "name": "start-first-pipeline",\n  "enabled": true,\n  "pipeline": "platform/release-service",\n  "scope": "platform/production",\n  "run_team_path": "platform/payments",\n  "allowed_callers": [{ "service_account": "release-bot" }],\n  "variable_mapping": { "RELEASE_CHANNEL": "payload.channel" },\n  "rate_limit": { "per_minute": 10 }\n}';

/**
 * External API triggers.
 *
 * A named entry point that lets another system start one specific pipeline.
 * `allowed_callers` narrows an already-authorized set: AAA still authorizes the
 * caller against the trigger resource, so an empty list widens nothing.
 */
export const external_triggersRoutes: WikiApiRoute[] = [
  {
    method: 'GET',
    path: '/v1/external-triggers',
    area: 'External triggers',
    access: 'authorized',
    purpose: 'Lists external triggers the caller can see.',
    depth: 'full',
    parameters: [],
    requestSample: {
      title: 'List external triggers',
      language: 'bash',
      code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/external-triggers" | jq \'.[] | {id, name, pipeline, enabled}\'',
      expectedOutput: 'One entry per trigger, filtered to what the caller may see.',
    },
    responses: [{ status: 200, description: 'Triggers visible to the caller.', contentType: 'application/json', sample: '[' + TRIGGER_RECORD + ']' }],
    errors: [
      { status: 405, cause: 'A method other than GET.', action: 'Use GET.' },
      { status: 503, cause: 'Authorization is unavailable, so the list cannot be filtered.', action: 'Check AAA.' },
      { status: 500, cause: 'The query failed.', action: 'Platform fault.' },
    ],
    sideEffects: ['None.'],
    coveringTests: ['services/nopsai/external_triggers_test.go'],
    evidence: ['services/nopsai/external_triggers.go'],
  },
  {
    method: 'POST',
    path: '/v1/external-triggers',
    area: 'External triggers',
    access: 'authorized',
    purpose: 'Creates an external trigger.',
    depth: 'full',
    parameters: [],
    requestSample: {
      title: 'Create a trigger',
      language: 'bash',
      code:
        'curl -sX POST "$NOPSAI_URL/v1/external-triggers" \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  -H "Content-Type: application/json" \\\n  --data @trigger.json | jq -r .id',
      expectedOutput: 'The created trigger, including the id used to invoke it.',
      placeholders: [
        '`trigger.json` mirrors the GitOps document: `name`, `pipeline`, `scope`, `run_team_path`, `allowed_callers`, `variable_mapping`, `payload_schema`, and `rate_limit`.',
      ],
    },
    responses: [{ status: 201, description: 'Trigger created.', contentType: 'application/json', sample: TRIGGER_RECORD }],
    errors: [
      { status: 400, cause: 'The document is invalid: a missing pipeline, an unusable caller entry, or a malformed mapping.', action: 'The message names the field that failed normalisation.' },
      { status: 401, cause: 'The caller identity could not be resolved for the created-by record.', action: 'Use a user or service account token, not an anonymous request.' },
      { status: 405, cause: 'A method other than POST.', action: 'Use POST.' },
      { status: 500, cause: 'The trigger could not be stored.', action: 'Retry.' },
    ],
    sideEffects: ['Creates an entry point that can start runs.', 'Records who created it.', 'Writes an audit record.'],
    coveringTests: ['services/nopsai/external_triggers_test.go', 'services/nopsai/external_triggers_schema_test.go'],
    evidence: ['services/nopsai/external_triggers.go', 'services/nopsai/external_triggers_gitops.go'],
    notes: 'The JSON body and the GitOps YAML document carry the same fields, so a trigger created here can be exported into a configuration repository unchanged.',
  },
  {
    method: 'GET',
    path: '/v1/external-triggers/{id}',
    area: 'External triggers',
    access: 'authorized',
    purpose: 'Reads one external trigger.',
    depth: 'full',
    parameters: [{ name: 'id', in: 'path', type: 'uuid', required: true, description: 'Trigger identifier.', example: '3d2b1f88-77aa-4c19-9f3d-52b0c4e7a901' }],
    requestSample: {
      title: 'Read a trigger',
      language: 'bash',
      code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/external-triggers/$TRIGGER_ID" | jq',
      expectedOutput: 'The full trigger document, including its caller list and payload contract.',
    },
    responses: [{ status: 200, description: 'The trigger.', contentType: 'application/json', sample: TRIGGER_RECORD }],
    errors: [
      { status: 404, cause: 'No trigger with that id.', action: 'Confirm the id from the list route.' },
      { status: 405, cause: 'A method other than GET.', action: 'Use GET.' },
      { status: 500, cause: 'The trigger could not be loaded.', action: 'Platform fault.' },
    ],
    sideEffects: ['None.'],
    coveringTests: ['services/nopsai/external_triggers_test.go'],
    evidence: ['services/nopsai/external_triggers.go'],
  },
  {
    method: 'PUT',
    path: '/v1/external-triggers/{id}',
    area: 'External triggers',
    access: 'authorized',
    purpose: 'Replaces an external trigger definition.',
    depth: 'full',
    parameters: [{ name: 'id', in: 'path', type: 'uuid', required: true, description: 'Trigger to replace.', example: '3d2b1f88-77aa-4c19-9f3d-52b0c4e7a901' }],
    requestSample: {
      title: 'Replace a trigger',
      language: 'bash',
      code:
        'curl -sX PUT "$NOPSAI_URL/v1/external-triggers/$TRIGGER_ID" \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  -H "Content-Type: application/json" \\\n  --data @trigger.json | jq',
      expectedOutput: 'The stored trigger after the replacement.',
      placeholders: ['A PUT replaces the whole document. Omitted fields are cleared, not kept.'],
    },
    responses: [{ status: 200, description: 'Trigger replaced.', contentType: 'application/json', sample: TRIGGER_RECORD }],
    errors: [
      { status: 400, cause: 'The document is invalid.', action: 'The message names the field.' },
      { status: 404, cause: 'No trigger with that id.', action: 'Create it instead.' },
      { status: 405, cause: 'A method other than PUT or PATCH.', action: 'Use PUT to replace or PATCH to merge.' },
      { status: 500, cause: 'The update could not be persisted.', action: 'Retry.' },
    ],
    sideEffects: ['Changes what callers may invoke and what the trigger starts.', 'Writes an audit record.'],
    coveringTests: ['services/nopsai/external_triggers_test.go'],
    evidence: ['services/nopsai/external_triggers.go'],
    notes: 'Disabling with `enabled: false` is safer than deleting while you investigate a misbehaving caller: the invocation history stays attached.',
  },
  {
    method: 'PATCH',
    path: '/v1/external-triggers/{id}',
    area: 'External triggers',
    access: 'authorized',
    purpose: 'Partially updates an external trigger.',
    depth: 'full',
    parameters: [{ name: 'id', in: 'path', type: 'uuid', required: true, description: 'Trigger to update.', example: '3d2b1f88-77aa-4c19-9f3d-52b0c4e7a901' }],
    requestSample: {
      title: 'Disable a trigger without changing anything else',
      language: 'bash',
      code:
        'curl -sX PATCH "$NOPSAI_URL/v1/external-triggers/$TRIGGER_ID" \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  -H "Content-Type: application/json" \\\n  -d \'{"enabled":false}\' | jq \'{id, enabled}\'',
      expectedOutput: 'The trigger with `enabled: false`; every other field keeps its stored value.',
    },
    responses: [{ status: 200, description: 'Trigger updated.', contentType: 'application/json', sample: TRIGGER_RECORD }],
    errors: [
      { status: 400, cause: 'A supplied field is invalid.', action: 'The message names the field.' },
      { status: 404, cause: 'No trigger with that id.', action: 'Confirm the id.' },
      { status: 500, cause: 'The update could not be persisted.', action: 'Retry.' },
    ],
    sideEffects: ['Writes an audit record.'],
    coveringTests: ['services/nopsai/external_triggers_test.go'],
    evidence: ['services/nopsai/external_triggers.go'],
  },
  {
    method: 'DELETE',
    path: '/v1/external-triggers/{id}',
    area: 'External triggers',
    access: 'authorized',
    purpose: 'Deletes an external trigger.',
    depth: 'full',
    parameters: [{ name: 'id', in: 'path', type: 'uuid', required: true, description: 'Trigger to delete.', example: '3d2b1f88-77aa-4c19-9f3d-52b0c4e7a901' }],
    requestSample: {
      title: 'Delete a trigger',
      language: 'bash',
      code: 'curl -sX DELETE -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/external-triggers/$TRIGGER_ID" -w "%{http_code}\\n"',
      expectedOutput: '204. Callers holding the id start receiving 404.',
    },
    responses: [{ status: 204, description: 'Deleted.', sample: '' }],
    errors: [
      { status: 404, cause: 'No trigger with that id.', action: 'It may already be deleted.' },
      { status: 405, cause: 'A method other than DELETE.', action: 'Use DELETE.' },
      { status: 500, cause: 'The delete could not be persisted.', action: 'Retry.' },
    ],
    sideEffects: ['Removes the entry point. Runs it already started are unaffected.', 'Writes an audit record.'],
    coveringTests: ['services/nopsai/external_triggers_test.go'],
    evidence: ['services/nopsai/external_triggers.go'],
  },
  {
    method: 'POST',
    path: '/v1/external-triggers/{id}/invoke',
    area: 'External triggers',
    access: 'authorized',
    purpose: 'Starts a run through the trigger.',
    depth: 'full',
    parameters: [{ name: 'id', in: 'path', type: 'uuid', required: true, description: 'Trigger to invoke.', example: '3d2b1f88-77aa-4c19-9f3d-52b0c4e7a901' }],
    requestSample: {
      title: 'Invoke with an idempotency key',
      language: 'bash',
      code:
        'curl -sX POST "$NOPSAI_URL/v1/external-triggers/$TRIGGER_ID/invoke" \\\n  -H "Authorization: Bearer $CALLER_TOKEN" \\\n  -H "Content-Type: application/json" \\\n  -d \'{"idempotency_key":"change-4821","payload":{"channel":"stable"}}\' | jq',
      expectedOutput: '202 with the run id on the first call; 200 with the same run id when the key is replayed.',
      placeholders: ['`$CALLER_TOKEN` must belong to an identity named in `allowed_callers`.'],
    },
    responses: [
      { status: 202, description: 'Accepted: a run was queued from this invocation.', contentType: 'application/json', sample: '{\n  "run_id": "9c1f7a5e-2b44-4d2f-8f2a-2c9f0b6d4e11",\n  "trigger_event_id": "b0f1...",\n  "status": "queued"\n}' },
      { status: 200, description: 'The idempotency key was already used and its run is returned instead of starting a second one.', contentType: 'application/json', sample: '{\n  "run_id": "9c1f7a5e-2b44-4d2f-8f2a-2c9f0b6d4e11",\n  "trigger_event_id": "b0f1...",\n  "status": "queued"\n}' },
    ],
    errors: [
      { status: 400, cause: 'The payload fails `payload_schema`, or the body is malformed.', action: 'The schema is checked before a run is created, so a rejected payload costs nothing.' },
      { status: 401, cause: 'No usable caller identity on the request.', action: 'Send a token for an identity in `allowed_callers`.' },
      { status: 403, cause: 'The caller is authenticated but not permitted to invoke this trigger.', action: '`allowed_callers` narrows an already-authorized set; AAA must also allow the caller on the trigger resource.' },
      { status: 404, cause: 'No trigger with that id, or it is disabled.', action: 'Check `enabled` on the trigger.' },
      { status: 409, cause: 'The same idempotency key is currently being processed.', action: 'Wait and retry: the first call is still in flight.' },
      { status: 429, cause: '`rate_limit.per_minute` was exceeded over the previous minute.', action: 'Back off. The limit is per trigger, not per caller.' },
      { status: 503, cause: 'Authorization is unavailable.', action: 'Check AAA; the invocation is refused rather than run unauthorized.' },
      { status: 500, cause: 'The run could not be created.', action: 'Retry with the same idempotency key.' },
    ],
    sideEffects: [
      'Records an invocation, accepted or rejected, with the caller and the reason.',
      'Creates a run and maps payload values into run variables through `variable_mapping`.',
      'Updates `last_used_at` on the trigger.',
    ],
    coveringTests: ['services/nopsai/external_triggers_test.go'],
    evidence: ['services/nopsai/external_triggers.go'],
    notes: 'The idempotency key is scoped by trigger and caller. Two systems using the same key against the same trigger do not collide.',
  },
  {
    method: 'GET',
    path: '/v1/external-triggers/{id}/invocations',
    area: 'External triggers',
    access: 'authorized',
    purpose: 'Lists invocation history for a trigger.',
    depth: 'full',
    parameters: [
      { name: 'id', in: 'path', type: 'uuid', required: true, description: 'Trigger identifier.', example: '3d2b1f88-77aa-4c19-9f3d-52b0c4e7a901' },
      { name: 'limit', in: 'query', type: 'integer', required: false, description: 'Page size for the history.', example: 'limit=50' },
    ],
    requestSample: {
      title: 'Read invocation history',
      language: 'bash',
      code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/external-triggers/$TRIGGER_ID/invocations" | jq',
      expectedOutput: 'Accepted and rejected calls alike, each with its caller and outcome.',
    },
    responses: [
      {
        status: 200,
        description: 'Invocation records, newest first.',
        contentType: 'application/json',
        sample: '[\n  {\n    "id": "6a3c...",\n    "trigger_id": "3d2b1f88-77aa-4c19-9f3d-52b0c4e7a901",\n    "caller_type": "service_account",\n    "caller_id": "release-bot",\n    "status": "queued",\n    "run_id": "9c1f7a5e-2b44-4d2f-8f2a-2c9f0b6d4e11"\n  }\n]',
      },
    ],
    errors: [
      { status: 405, cause: 'A method other than GET.', action: 'Use GET.' },
      { status: 500, cause: 'The history query failed.', action: 'Platform fault.' },
    ],
    sideEffects: ['None.'],
    coveringTests: ['services/nopsai/external_triggers_test.go'],
    evidence: ['services/nopsai/external_triggers.go'],
    notes: 'This is the fastest answer to "my system says it is triggering runs and nothing happens": a rejected call is recorded here with its reason.',
  },
];

import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { assistantMessageRequestBody, assistantReadableErrorText } from './api.js';

describe('assistant api helpers', () => {
  it('summarizes HTML gateway errors without leaking markup', () => {
    const message = assistantReadableErrorText(
      '<html><head><title>502 Bad Gateway</title></head><body><center><h1>502 Bad Gateway</h1></center><hr><center>nginx/1.27.5</center></body></html><!-- padding -->',
      'Failed to send message (502)',
    );

    assert.equal(message.includes('<html>'), false);
    assert.equal(message.includes('<!--'), false);
    assert.match(message, /^Failed to send message \(502\):/);
    assert.match(message, /502 Bad Gateway/);
    assert.match(message, /nginx\/1\.27\.5/);
  });

  it('keeps plain API errors concise', () => {
    assert.equal(assistantReadableErrorText('Authorization unavailable', 'Failed to load'), 'Authorization unavailable');
  });

  it('adds sanitized page context only when the message has route context', () => {
    assert.deepEqual(assistantMessageRequestBody({
      content: 'explain this',
      selected_llm_profile: 'assistant',
    }), {
      content: 'explain this',
      selected_llm_profile: 'assistant',
    });

    assert.deepEqual(assistantMessageRequestBody({
      content: 'explain this',
      selected_llm_profile: 'assistant',
      page_context: {
        title: 'Pipeline runs',
        path: '/pipelineruns/recent/run-1',
        route: '/pipelineruns/:tab/:run_id',
        area: 'Pipeline Runs',
        run_id: 'run-1',
        query: { token: 'secret', status: 'failure' },
      },
    }), {
      content: 'explain this',
      selected_llm_profile: 'assistant',
      page_context: {
        title: 'Pipeline runs',
        path: '/pipelineruns/recent/run-1',
        route: '/pipelineruns/:tab/:run_id',
        area: 'pipeline_runs',
        tab: '',
        team_path: '',
        resource_type: '',
        resource_id: '',
        resource_name: '',
        scope: '',
        pipeline_id: '',
        run_id: 'run-1',
        repository: '',
        query: { status: 'failure' },
        params: {},
      },
    });
  });
});

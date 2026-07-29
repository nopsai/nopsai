import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { canonicalHashRouterURL } from './hashRouterUrl.js';

describe('hash router URL canonicalization', () => {
  it('drops stale pre-hash path and search values from hash-routed pages', () => {
    assert.equal(
      canonicalHashRouterURL({
        pathname: '/system/config',
        search: '?tab=ai-usage',
        hash: '#/dashboards/b6d7f0b9-c5fe-437b-9385-3b9eb4dddc82?tab=release-readiness',
      }),
      '/#/dashboards/b6d7f0b9-c5fe-437b-9385-3b9eb4dddc82?tab=release-readiness'
    );
  });

  it('leaves already canonical hash routes and non-route hashes alone', () => {
    assert.equal(
      canonicalHashRouterURL({ pathname: '/', search: '', hash: '#/dashboards?tab=release-readiness' }),
      null
    );
    assert.equal(
      canonicalHashRouterURL({ pathname: '/docs', search: '', hash: '#section-one' }),
      null
    );
  });
});

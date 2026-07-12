import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  agentProfileFormFromRecord,
  agentProfilePayloadFromForm,
  agentProfileSection,
  agentProfileSourceLabel,
  duplicateAgentProfileForm,
  normalizeAgentProfilesPayload,
  type AgentProfileRecord,
} from './model.js';

test('normalizes agent profile payloads with explicit hidden defaults and stable ordering', () => {
  const payload = normalizeAgentProfilesPayload({
    default_profile: '',
    profiles: [
      {
        id: 'sre',
        display_name: 'SRE',
        instructions: 'Protect reliability.',
        enabled: false,
        source: 'gitops',
        references: ['pipeline deploy'],
        usage_count: 1,
      },
      {
        id: 'devops-engineer',
        display_name: 'DevOps Engineer',
        role: 'Senior DevOps Engineer',
        instructions: 'Automate safely.',
        built_in: true,
        source: 'built-in',
      },
      { id: '   ' },
    ],
  });

  assert.equal(payload.default_profile, '');
  assert.deepEqual(
    payload.profiles.map(profile => profile.id),
    ['devops-engineer', 'sre']
  );
  assert.equal(payload.profiles[0]?.enabled, true);
  assert.equal(payload.profiles[1]?.enabled, false);
  assert.equal(payload.profiles[1]?.role, '');
  assert.deepEqual(payload.profiles[1]?.references, ['pipeline deploy']);
});

test('normalizes agent profile payloads with built-in fallback default when omitted', () => {
  const payload = normalizeAgentProfilesPayload({
    profiles: [
      {
        id: 'sre',
        display_name: 'SRE',
        instructions: 'Protect reliability.',
      },
    ],
  });

  assert.equal(payload.default_profile, 'devops-engineer');
});

test('builds agent profile form state and API payloads', () => {
  const profile: AgentProfileRecord = {
    id: 'release-manager',
    display_name: 'Release Manager',
    role: 'Senior Release Manager',
    description: 'Coordinates releases.',
    instructions: 'Check rollout evidence.',
    enabled: true,
    source: 'ui',
    usage_count: 0,
    references: [],
  };

  assert.deepEqual(agentProfileFormFromRecord(profile), {
    id: 'release-manager',
    display_name: 'Release Manager',
    role: 'Senior Release Manager',
    description: 'Coordinates releases.',
    instructions: 'Check rollout evidence.',
    enabled: true,
  });
  assert.deepEqual(duplicateAgentProfileForm(profile), {
    id: 'release-manager-custom',
    display_name: 'Release Manager Custom',
    role: 'Senior Release Manager',
    description: 'Coordinates releases.',
    instructions: 'Check rollout evidence.',
    enabled: true,
  });
  assert.deepEqual(
    agentProfilePayloadFromForm({
      id: ' release-manager ',
      display_name: ' Release Manager ',
      role: '   ',
      description: ' Coordinates releases. ',
      instructions: ' Check rollout evidence. ',
      enabled: false,
    }),
    {
      id: 'release-manager',
      display_name: 'Release Manager',
      role: '',
      description: 'Coordinates releases.',
      instructions: 'Check rollout evidence.',
      enabled: false,
    }
  );
});

test('classifies agent profile sections and source labels', () => {
  assert.equal(agentProfileSection({ source: 'built-in', built_in: true } as AgentProfileRecord), 'built-in');
  assert.equal(agentProfileSection({ source: 'gitops' } as AgentProfileRecord), 'gitops');
  assert.equal(agentProfileSection({ source: 'ui' } as AgentProfileRecord), 'custom');
  assert.equal(agentProfileSourceLabel('built-in'), 'Built-in');
  assert.equal(agentProfileSourceLabel('gitops'), 'GitOps');
  assert.equal(agentProfileSourceLabel('ui'), 'Custom');
});

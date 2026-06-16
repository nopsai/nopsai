import { describe, expect, it } from 'vitest';
import { getObjectIconComponent } from '../../components/objectIconRegistry';
import { kindIcon, kindIconType } from './presentation';

describe('Knowledge Context presentation', () => {
  it('maps knowledge kinds to shared object icon types', () => {
    expect(kindIconType('guardrail')).toBe('knowledge-guardrail');
    expect(kindIconType('policy')).toBe('knowledge-policy');
    expect(kindIconType('runbook')).toBe('knowledge-runbook');
    expect(kindIconType('reference')).toBe('knowledge-reference');
    expect(kindIconType('example')).toBe('knowledge-example');
    expect(kindIconType('architecture')).toBe('knowledge-default');
    expect(kindIcon('guardrail')).toBe(getObjectIconComponent('knowledge-guardrail'));
  });
});

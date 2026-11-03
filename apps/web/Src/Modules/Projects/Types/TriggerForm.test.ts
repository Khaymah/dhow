import { describe, expect, it } from 'vitest';

import type { TManualInput } from '@/Modules/Projects/Api/RunsApi';
import { initialValues, requestInputs, triggerSchema } from '@/Modules/Projects/Types/TriggerForm';

const INPUTS: TManualInput[] = [
  { Name: 'dry', Type: 'boolean', Required: false, Default: 'true', Options: [] },
  { Name: 'target', Type: 'choice', Required: true, Options: ['staging', 'production'], Default: 'staging' },
  { Name: 'note', Type: 'string', Required: true, Options: [] },
  { Name: 'region', Type: 'choice', Required: false, Options: ['eu', 'us'] },
];

describe('the manual run form', () => {
  it('starts from the defaults of every type', () => {
    expect(initialValues(INPUTS)).toEqual({ dry: true, target: 'staging', note: '', region: '' });
  });

  it('accepts typed values and leaves out empty optional ones', () => {
    const values = { dry: false, target: 'production', note: 'ship it', region: '' };
    const parsed = triggerSchema(INPUTS).safeParse({ Ref: 'main', Sha: '', Inputs: values });
    expect(parsed.success).toBe(true);
    expect(requestInputs(INPUTS, values)).toEqual({ dry: false, target: 'production', note: 'ship it' });
  });

  it('names every wrong field with its message key', () => {
    const parsed = triggerSchema(INPUTS).safeParse({ Ref: ' ', Sha: 'abc', Inputs: { dry: 'yes', target: 'moon', note: ' ', region: 'mars' } });
    expect(parsed.success).toBe(false);
    const issues = Object.fromEntries((parsed.error?.issues ?? []).map((issue) => [issue.path.join('.'), issue.message]));
    expect(issues).toMatchObject({
      Ref: 'runs.trigger.error.ref',
      Sha: 'runs.trigger.error.sha',
      'Inputs.target': 'runs.trigger.error.choice',
      'Inputs.note': 'runs.trigger.error.required',
    });
    expect(issues['Inputs.dry']).toBeDefined();
    expect(issues['Inputs.region']).toBeDefined();
  });

  it('accepts a full commit hash of either length', () => {
    for (const sha of ['a'.repeat(40), 'b'.repeat(64)]) {
      expect(triggerSchema([]).safeParse({ Ref: 'refs/tags/v1', Sha: sha, Inputs: {} }).success).toBe(true);
    }
  });
});

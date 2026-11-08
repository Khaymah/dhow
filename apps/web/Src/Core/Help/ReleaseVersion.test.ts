import { describe, expect, it } from 'vitest';

import { isNewerRelease } from '@/Core/Help/ReleaseVersion';

describe('isNewerRelease', () => {
  it.each([
    ['0.1.0', null, true],
    ['0.1.0', undefined, true],
    ['0.2.0', '0.1.0', true],
    ['0.10.0', '0.9.0', true],
    ['1.0.0', '1.0.0-rc.2', true],
    ['1.0.0-rc.2', '1.0.0-rc.1', true],
    ['0.1.0', '0.1.0', false],
    ['0.1.0', '0.2.0', false],
    ['1.0.0-rc.1', '1.0.0', false],
    ['1.0.0', 'garbage', true],
  ])('%s against %s is %s', (latest, seen, expected) => {
    expect(isNewerRelease(latest, seen)).toBe(expected);
  });
});
